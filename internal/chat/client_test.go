package chat

import "testing"

func TestCleanThinkingTags(t *testing.T) {
	cases := map[string]string{
		"plain answer":                           "plain answer",
		"a<think>reasoning</think>b":             "ab",
		"<think>only reasoning</think>answer":    "answer",
		"answer<think>trailing reasoning":        "answer",
		"</think>\nfinal answer":                 "final answer",
		"<think>one</think><think>two</think>ok": "ok",
	}
	for input, want := range cases {
		if got := cleanThinkingTags(input); got != want {
			t.Fatalf("cleanThinkingTags(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCleanThinkingTagsTerminatesOnDanglingClose(t *testing.T) {
	if got := cleanThinkingTags("</think>"); got != "" {
		t.Fatalf("expected empty output, got %q", got)
	}
}
