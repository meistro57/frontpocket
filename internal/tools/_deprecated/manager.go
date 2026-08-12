package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/meistro57/frontpocket/internal/chat"
	"github.com/meistro57/frontpocket/internal/tools"
)

// ServerConfig describes one MCP server to spawn: a short name used as the
// tool-name prefix, the command to run, and its arguments.
type ServerConfig struct {
	Name    string
	Command string
	Args    []string
}

// Manager owns a set of running MCP server subprocesses and exposes their
// combined tool set as tools.Tool values, so the chat loop doesn't need to
// know or care that these particular tools live behind a subprocess instead
// of a direct Go call.
type Manager struct {
	logger  *slog.Logger
	clients []*Client
}

// NewManager starts one Client per configured server. A server that fails to
// start is logged and skipped rather than failing the whole startup — Eli
// should still boot and chat with whichever MCP servers are actually
// reachable, not refuse to start because one of them (e.g. a Python venv
// that isn't set up yet) isn't ready.
func NewManager(ctx context.Context, configs []ServerConfig, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{logger: logger}
	for _, cfg := range configs {
		name := strings.TrimSpace(cfg.Name)
		if name == "" || strings.TrimSpace(cfg.Command) == "" {
			continue
		}
		client := NewClient(name, cfg.Command, cfg.Args)
		if err := client.Start(ctx); err != nil {
			logger.Warn("mcp server failed to start — skipping", "server", name, "error", err)
			continue
		}
		m.clients = append(m.clients, client)
		logger.Info("mcp server started", "server", name)
	}
	return m
}

// Close stops every managed subprocess. Call this on server shutdown.
func (m *Manager) Close() {
	for _, c := range m.clients {
		_ = c.Close()
	}
}

// Tools queries tools/list on every running server and returns one
// tools.Tool per remote tool, named "<server>__<tool>" to keep servers from
// colliding. Call this once at startup and register the results into a
// tools.Registry — re-call it later only if you expect a server's tool set
// to have changed at runtime.
func (m *Manager) Tools(ctx context.Context) []tools.Tool {
	var out []tools.Tool
	for _, client := range m.clients {
		infos, err := client.ListTools(ctx)
		if err != nil {
			m.logger.Warn("mcp tools/list failed — skipping server", "server", client.Name(), "error", err)
			continue
		}
		for _, info := range infos {
			out = append(out, &remoteTool{
				client:       client,
				remoteName:   info.Name,
				prefixedName: fmt.Sprintf("%s__%s", client.Name(), info.Name),
				description:  fmt.Sprintf("[%s] %s", client.Name(), info.Description),
				schema:       info.InputSchema,
			})
		}
	}
	return out
}

// remoteTool adapts one MCP server's tool to the tools.Tool interface.
type remoteTool struct {
	client       *Client
	remoteName   string
	prefixedName string
	description  string
	schema       json.RawMessage
}

func (t *remoteTool) Definition() chat.ToolDefinition {
	params := t.schema
	if len(params) == 0 {
		params = []byte(`{"type":"object","properties":{}}`)
	}
	return chat.ToolDefinition{
		Name:        t.prefixedName,
		Description: t.description,
		Parameters:  params,
	}
}

func (t *remoteTool) Execute(ctx context.Context, argumentsJSON string) (string, error) {
	result, err := t.client.CallTool(ctx, t.remoteName, argumentsJSON)
	if err != nil {
		// Fold into the returned string rather than the Go error — see the
		// Tool interface contract in package tools: ordinary failures should
		// go back to the model as text, not bubble up as a registry error.
		return fmt.Sprintf("error calling %s: %v", t.prefixedName, err), nil
	}
	return result, nil
}
