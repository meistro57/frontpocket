package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestSessionFallbackExpires(t *testing.T) {
	s := &Server{
		sessionFallback:   make(map[string]sessionFallbackEntry),
		defaultSessionTTL: time.Millisecond,
	}
	s.setSessionFallback(memory.SessionState{SessionID: "s1"}, time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/memory/session", nil)
	if _, found, err := s.getSessionState(req, "s1"); err != nil || found {
		t.Fatalf("expected expired session to be missing, found=%v err=%v", found, err)
	}
	if len(s.sessionFallback) != 0 {
		t.Fatalf("expected expired entry to be pruned, got %d entries", len(s.sessionFallback))
	}
}

func TestSessionFallbackIsCapped(t *testing.T) {
	s := &Server{
		sessionFallback:   make(map[string]sessionFallbackEntry),
		defaultSessionTTL: time.Hour,
	}
	for i := 0; i < maxSessionFallbackEntries+25; i++ {
		s.setSessionFallback(memory.SessionState{SessionID: fmt.Sprintf("s%d", i)}, time.Hour)
	}
	if len(s.sessionFallback) > maxSessionFallbackEntries {
		t.Fatalf("expected at most %d fallback sessions, got %d", maxSessionFallbackEntries, len(s.sessionFallback))
	}
}

func TestSetSessionStateReportsStoreStatus(t *testing.T) {
	s := &Server{
		sessionFallback:   make(map[string]sessionFallbackEntry),
		defaultSessionTTL: time.Hour,
	}
	req := httptest.NewRequest(http.MethodPost, "/memory/session", nil)
	ctx, status := memory.WithStoreStatus(req.Context())
	req = req.WithContext(ctx)

	if err := s.setSessionState(req, memory.SessionState{SessionID: "s1"}, 60); err != nil {
		t.Fatalf("set session failed: %v", err)
	}
	if status.Degraded {
		t.Fatal("expected no degradation without a Redis client")
	}
}
