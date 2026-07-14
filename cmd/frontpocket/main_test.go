package main

import (
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	output := captureStdout(t, func() {
		err := run([]string{"--help"})
		if err != nil {
			t.Fatalf("run --help failed: %v", err)
		}
	})

	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected help output to contain Usage, got:\n%s", output)
	}
	if !strings.Contains(output, "frontpocket [command] [options]") {
		t.Fatalf("expected root usage output, got:\n%s", output)
	}
	if !strings.Contains(output, "Commands:") || !strings.Contains(output, "Import memory data from supported sources.") {
		t.Fatalf("expected commands reference in help output, got:\n%s", output)
	}
	if !strings.Contains(output, "minddrill") || !strings.Contains(output, "Serve the MindDrill memory explorer in your browser.") {
		t.Fatalf("expected minddrill command reference in help output, got:\n%s", output)
	}
	if !strings.Contains(output, "memory-loop") || !strings.Contains(output, "Run source-backed memory curation and review workflows.") {
		t.Fatalf("expected memory-loop command reference in help output, got:\n%s", output)
	}
	if !strings.Contains(output, "mcp") || !strings.Contains(output, "Expose FrontPocket search tools over MCP for external agents.") {
		t.Fatalf("expected mcp command reference in help output, got:\n%s", output)
	}
	if !strings.Contains(output, "Subcommands:") || !strings.Contains(output, "ingest chatgpt      Import from a ChatGPT export zip or folder.") || !strings.Contains(output, "ingest claude       Import from a Claude export folder.") {
		t.Fatalf("expected subcommand reference in help output, got:\n%s", output)
	}
	if !strings.Contains(output, "frontpocket ingest --help") || !strings.Contains(output, "frontpocket ingest claude --help") || !strings.Contains(output, "frontpocket minddrill --help") || !strings.Contains(output, "frontpocket memory-loop --help") || !strings.Contains(output, "frontpocket mcp --help") {
		t.Fatalf("expected nested help references in help output, got:\n%s", output)
	}
}
