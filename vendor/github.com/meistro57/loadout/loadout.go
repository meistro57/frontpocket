// Package loadout is a small, dependency-free toolkit for giving an LLM
// chat client callable tools: a provider-agnostic ToolDefinition/ToolCall
// vocabulary, a Tool interface, a Registry to hold a set of them, an MCP
// (Model Context Protocol) stdio client for pulling in tools from external
// servers (see package mcp), and a Tavily-backed web search tool.
//
// It has no opinion about which chat provider is on the other end — OpenAI,
// OpenRouter, DeepSeek, and Ollama's /v1 API all speak the same
// tools/tool_calls JSON shape ToolDefinition and ToolCall are modeled on, so
// the same Loadout works unchanged across any of them. Domain-specific
// tools (e.g. one that searches a particular app's own database) stay in
// that app and just implement the Tool interface — Loadout only owns the
// generic vocabulary and the tools that are genuinely portable (web search,
// MCP).
package loadout

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolDefinition describes one callable tool in OpenAI-style function-calling
// shape. Parameters is a raw JSON Schema object, e.g.:
//
//	{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is a single tool invocation requested by the model. Arguments is
// the raw JSON arguments object exactly as the model produced it — callers
// are responsible for unmarshalling and validating it themselves.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is one callable capability the model can invoke. Execute receives the
// raw JSON arguments object the model produced and returns the result as a
// string that gets fed back to the model as a tool-role message.
//
// Implementations should not return a Go error for ordinary failures (bad
// args, an upstream request failing, no results) — return that as a
// human-readable "error: ..." string instead, so the model sees what went
// wrong and can adjust. Reserve the Go error for genuinely unexpected
// failures the caller should log and treat as a broken tool.
type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, argumentsJSON string) (string, error)
}

// Registry holds a set of tools. Safe for concurrent use; tools are
// typically registered once at startup, then only read from during
// requests.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool, keyed by its own declared name. Registering a second
// tool under a name that's already taken replaces the first — callers
// wiring up MCP-backed tools are responsible for prefixing names (see
// package mcp's Manager) to avoid accidental collisions between servers.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Definition().Name] = t
}

// Len reports how many tools are currently registered.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Definitions returns every registered tool's definition, ready to hand to
// your chat client's tool-calling request.
func (r *Registry) Definitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Definition())
	}
	return out
}

// Call dispatches a single tool invocation by name. The returned string is
// always meant to go straight back to the model as a tool-role message
// (Tool implementations already fold their own failures into it); the Go
// error return is reserved for "no such tool registered".
func (r *Registry) Call(ctx context.Context, name, argumentsJSON string) (string, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, argumentsJSON)
}
