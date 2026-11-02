<p align="center">
  <img src="logo.png" alt="Loadout logo" width="320">
</p>

# Loadout

A small, dependency-free Go toolkit for giving an LLM chat client callable
tools. No external packages — stdlib only.

## What's in it

- **`loadout.go`** — the core vocabulary: `ToolDefinition`, `ToolCall`, the
  `Tool` interface, and a `Registry` to hold a set of tools and dispatch
  calls by name.
- **`websearch_tavily.go`** — a `Tool` backed by [Tavily](https://tavily.com)'s
  search API (returns cleaned, extracted content, not just links).
- **`mcp/`** — a minimal MCP (Model Context Protocol) client over stdio.
  `mcp.Manager` spawns a set of configured MCP servers, aggregates their
  tools with a `<server>__<tool>` name prefix, and hands back `loadout.Tool`
  values ready to register.

Loadout doesn't know or care which chat provider is on the other end.
OpenAI, OpenRouter, DeepSeek, and Ollama's `/v1` API all speak the same
`tools` / `tool_calls` JSON shape `ToolDefinition` and `ToolCall` are modeled
on, so the same Loadout works unchanged across any of them — the provider's
own chat client is responsible for converting `[]ToolDefinition` into its
request format and parsing `tool_calls` back out.

## What's deliberately *not* in it

Domain-specific tools stay in whatever app owns that domain. A tool that
searches your own app's database, calls your own internal API, or touches
data only that app understands should live in that app and just implement
`loadout.Tool` — it doesn't belong in a generic toolkit. (FrontPocket's
memory-search tool is the reference example: it lives in FrontPocket, wraps
FrontPocket's own `MemoryStore`, and imports Loadout rather than the other
way around.)

## Usage sketch

```go
registry := loadout.NewRegistry()
registry.Register(loadout.NewTavilyWebSearchTool(os.Getenv("TAVILY_API_KEY")))

mgr := mcp.NewManager(ctx, []mcp.ServerConfig{
    {Name: "kae", Command: "/home/mark/kae/mcp/kae-mcp"},
    {Name: "frontpocket", Command: "python3", Args: []string{"/home/mark/mcp-servers/frontpocket_mcp.py"}},
}, logger)
defer mgr.Close()
for _, t := range mgr.Tools(ctx) {
    registry.Register(t)
}

// Hand registry.Definitions() to your chat client's tool-calling request.
// When it comes back with tool_calls, dispatch each one through
// registry.Call(ctx, call.Name, call.Arguments) and feed the result back
// as a tool-role message.
```

## Status

Built for and first consumed by [FrontPocket](../frontpocket)'s chat
endpoint (codename **Eli**). Not yet published/tagged — consuming projects
should point at it with a local `replace` directive in `go.mod` until it's
stable enough to tag a `v0.1.0`.
