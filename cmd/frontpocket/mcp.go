package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/version"
)

type mcpServer struct {
	apiBaseURL   string
	apiKey       string
	apiKeyHeader string
	client       *http.Client
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type mcpToolResult struct {
	Content           []mcpTextContent `json:"content"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func runMCPCommand(args []string) error {
	_ = config.LoadDotEnv(".env")
	cfg, _ := config.Load()
	defaultBaseURL := "http://localhost:8088"
	defaultHeader := "X-FrontPocket-Key"
	defaultKey := ""
	if cfg.App.Port > 0 {
		defaultBaseURL = fmt.Sprintf("http://localhost:%d", cfg.App.Port)
	}
	if strings.TrimSpace(cfg.App.PublicURL) != "" {
		defaultBaseURL = strings.TrimRight(strings.TrimSpace(cfg.App.PublicURL), "/")
	}
	if strings.TrimSpace(cfg.Security.APIKeyHeader) != "" {
		defaultHeader = strings.TrimSpace(cfg.Security.APIKeyHeader)
	}
	if cfg.Security.RequireAPIKey {
		defaultKey = strings.TrimSpace(cfg.Security.APIKey)
	}

	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	apiURL := flags.String("api", defaultBaseURL, "FrontPocket API base URL")
	apiKeyHeader := flags.String("api-key-header", defaultHeader, "API key header name")
	apiKey := flags.String("api-key", defaultKey, "API key value for protected FrontPocket endpoints")
	timeout := flags.Duration("timeout", 20*time.Second, "HTTP timeout for FrontPocket API calls")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	server := &mcpServer{
		apiBaseURL:   strings.TrimRight(strings.TrimSpace(*apiURL), "/"),
		apiKeyHeader: strings.TrimSpace(*apiKeyHeader),
		apiKey:       strings.TrimSpace(*apiKey),
		client:       &http.Client{Timeout: *timeout},
	}
	if server.apiBaseURL == "" {
		return errors.New("--api cannot be empty")
	}
	return server.serve(os.Stdin, os.Stdout)
}

func (s *mcpServer) serve(input io.Reader, output io.Writer) error {
	reader := bufio.NewReader(input)
	for {
		payload, err := readRPCPayload(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var req rpcRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			resp := rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}
			if writeErr := writeRPCPayload(output, resp); writeErr != nil {
				return writeErr
			}
			continue
		}
		if req.JSONRPC == "" {
			req.JSONRPC = "2.0"
		}

		resp := s.handleRequest(req)
		if resp == nil {
			continue
		}
		if err := writeRPCPayload(output, *resp); err != nil {
			return err
		}
	}
}

func (s *mcpServer) handleRequest(req rpcRequest) *rpcResponse {
	if req.Method == "notifications/initialized" {
		return nil
	}
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "frontpocket-mcp",
				"version": version.Current,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		result, err := s.handleToolCall(req.Params)
		if err != nil {
			resp.Error = &rpcError{Code: -32602, Message: err.Error()}
			return resp
		}
		resp.Result = result
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return resp
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "frontpocket_health",
			"description": "Get FrontPocket API health status.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "frontpocket_search",
			"description": "Search FrontPocket memory with optional metadata filters.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":            map[string]any{"type": "string"},
					"limit":            map[string]any{"type": "integer", "minimum": 1},
					"filters":          map[string]any{"type": "object"},
					"include_proposed": map[string]any{"type": "boolean"},
					"include_rejected": map[string]any{"type": "boolean"},
					"canonical_first":  map[string]any{"type": "boolean"},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "frontpocket_context_pack",
			"description": "Build a FrontPocket source-backed context pack.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":            map[string]any{"type": "string"},
					"limit":            map[string]any{"type": "integer", "minimum": 1},
					"filters":          map[string]any{"type": "object"},
					"include_proposed": map[string]any{"type": "boolean"},
					"include_rejected": map[string]any{"type": "boolean"},
					"canonical_first":  map[string]any{"type": "boolean"},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (s *mcpServer) handleToolCall(raw json.RawMessage) (mcpToolResult, error) {
	var params toolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return mcpToolResult{}, errors.New("invalid tools/call params")
	}
	if params.Name == "" {
		return mcpToolResult{}, errors.New("tool name is required")
	}

	switch params.Name {
	case "frontpocket_health":
		return s.callAPI(http.MethodGet, "/health", nil)
	case "frontpocket_search":
		if strings.TrimSpace(getStringArg(params.Arguments, "query")) == "" {
			return mcpToolResult{}, errors.New("query is required")
		}
		return s.callAPI(http.MethodPost, "/memory/search", params.Arguments)
	case "frontpocket_context_pack":
		if strings.TrimSpace(getStringArg(params.Arguments, "query")) == "" {
			return mcpToolResult{}, errors.New("query is required")
		}
		return s.callAPI(http.MethodPost, "/memory/context-pack", params.Arguments)
	default:
		return mcpToolResult{}, fmt.Errorf("unknown tool: %s", params.Name)
	}
}

func getStringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok {
		return ""
	}
	str, _ := value.(string)
	return str
}

func (s *mcpServer) callAPI(method, path string, payload map[string]any) (mcpToolResult, error) {
	endpoint := s.apiBaseURL + path
	var body io.Reader
	if method == http.MethodPost {
		if payload == nil {
			payload = map[string]any{}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return mcpToolResult{}, err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return mcpToolResult{}, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.apiKey != "" {
		req.Header.Set(s.apiKeyHeader, s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return mcpToolResult{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcpToolResult{}, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		trimmed = "{}"
	}

	result := mcpToolResult{
		Content: []mcpTextContent{{Type: "text", Text: trimmed}},
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err == nil {
		result.StructuredContent = decoded
	}
	if resp.StatusCode >= 400 {
		result.IsError = true
		result.Content = []mcpTextContent{{Type: "text", Text: fmt.Sprintf("FrontPocket API returned %d: %s", resp.StatusCode, trimmed)}}
	}
	return result, nil
}

func readRPCPayload(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "content-length:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) != 2 {
				return nil, errors.New("invalid content-length header")
			}
			value := strings.TrimSpace(parts[1])
			length, parseErr := strconv.Atoi(value)
			if parseErr != nil || length < 0 {
				return nil, errors.New("invalid content-length value")
			}
			contentLength = length
		}
	}
	if contentLength < 0 {
		return nil, errors.New("missing content-length header")
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeRPCPayload(output io.Writer, response rpcResponse) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Content-Length: %d\r\n\r\n", len(encoded)); err != nil {
		return err
	}
	_, err = output.Write(encoded)
	return err
}
