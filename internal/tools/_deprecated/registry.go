// Package tools implements Eli's Loadout — a uniform interface that lets
// the chat loop in internal/api offer memory search, web search, and
// MCP-backed tools to the model without caring which backend each one hits.
package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/meistro57/frontpocket/internal/chat"
)

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
	Definition() chat.ToolDefinition
	Execute(ctx context.Context, argumentsJSON string) (string, error)
}

// Registry holds the set of tools currently available to the chat loop. Safe
// for concurrent use; tools are typically registered once at startup, then
// only read from during requests.
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
// internal/tools/mcp.Manager) to avoid accidental collisions between
// servers.
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
// chat.Client.CompleteWithTools.
func (r *Registry) Definitions() []chat.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]chat.ToolDefinition, 0, len(r.tools))
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
