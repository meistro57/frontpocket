// Package mcp implements a minimal MCP (Model Context Protocol) client over
// stdio — enough to talk to Mark's existing Go and Python MCP servers
// (kae-mcp, frontpocket_mcp.py) without pulling in an external SDK. It
// speaks the standard MCP stdio framing those servers already implement:
// one JSON-RPC object per line on stdin/stdout, methods "initialize",
// "tools/list", "tools/call".
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// ToolInfo mirrors the MCP tools/list result shape closely enough to
// round-trip through JSON without needing the server's own type
// definitions.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client manages one MCP server subprocess over stdio for the life of the
// process. Not safe to use before Start or after Close.
//
// Calls are serialized internally (one in flight at a time). That matches
// how the reference stdio servers in this codebase actually behave — see
// the concurrency note on qdrantURL in kae/mcp/main.go — and lets this
// client stay a plain synchronous write-then-read-one-line round trip
// instead of a request-ID dispatch table.
type Client struct {
	name    string
	command string
	args    []string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int64
}

// NewClient constructs a client for one MCP server, identified by name for
// logging and tool-name prefixing. It does not start the process — call
// Start first.
func NewClient(name, command string, args []string) *Client {
	return &Client{name: name, command: command, args: args}
}

// Name returns the short identifier this client was constructed with.
func (c *Client) Name() string { return c.name }

// Start launches the subprocess and performs the MCP initialize handshake.
func (c *Client) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp %s: stdin pipe: %w", c.name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp %s: stdout pipe: %w", c.name, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcp %s: start: %w", c.name, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewReaderSize(stdout, 1<<20)

	initParams := map[string]any{
		"protocolVersion": "2025-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "eli-chat", "version": "1.0.0"},
	}
	if _, err := c.call("initialize", initParams); err != nil {
		_ = c.Close()
		return fmt.Errorf("mcp %s: initialize: %w", c.name, err)
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		_ = c.Close()
		return fmt.Errorf("mcp %s: initialized notification: %w", c.name, err)
	}
	return nil
}

// Close terminates the subprocess. Safe to call multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

// ListTools calls tools/list and returns the server's declared tools.
func (c *Client) ListTools(_ context.Context) ([]ToolInfo, error) {
	result, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		return nil, fmt.Errorf("mcp %s: decode tools/list: %w", c.name, err)
	}
	return decoded.Tools, nil
}

// CallTool invokes one tool by name with a raw JSON arguments object
// (may be empty) and returns the concatenated text content of the result.
func (c *Client) CallTool(_ context.Context, name string, argumentsJSON string) (string, error) {
	var args map[string]any
	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			return "", fmt.Errorf("mcp %s: invalid arguments for %s: %w", c.name, name, err)
		}
	}
	result, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		return "", fmt.Errorf("mcp %s: decode tools/call result: %w", c.name, err)
	}
	var text string
	for _, block := range decoded.Content {
		text += block.Text
	}
	if decoded.IsError {
		return text, fmt.Errorf("mcp %s: tool %s reported an error", c.name, name)
	}
	return text, nil
}

// call sends a JSON-RPC request and blocks for the next line of output,
// which the servers this talks to always send as the matching response
// (see the type comment on Client for why request-ID matching isn't
// needed here).
func (c *Client) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := atomic.AddInt64(&c.nextID, 1)
	req := jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("mcp %s: write %s: %w", c.name, method, err)
	}

	for {
		raw, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("mcp %s: read response to %s: %w", c.name, method, err)
		}
		if len(trimSpaceBytes(raw)) == 0 {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("mcp %s: decode response to %s: %w", c.name, method, err)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp %s: %s returned error %d: %s", c.name, method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	req := jsonRPCRequest{JSONRPC: "2.0", Method: method, Params: params}
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(line, '\n'))
	return err
}

func trimSpaceBytes(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && isSpaceByte(b[start]) {
		start++
	}
	for end > start && isSpaceByte(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
