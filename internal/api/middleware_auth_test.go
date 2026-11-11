package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAPIKeyMiddlewareAcceptsMatchingKey(t *testing.T) {
	handler := APIKeyMiddleware("secret-key", "", okHandler())

	req := httptest.NewRequest(http.MethodGet, "/memory/search", nil)
	req.Header.Set("X-FrontPocket-Key", "secret-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for matching key, got %d", rr.Code)
	}
}

func TestAPIKeyMiddlewareRejectsMissingAndWrongKeys(t *testing.T) {
	handler := APIKeyMiddleware("secret-key", "", okHandler())

	for name, key := range map[string]string{
		"missing": "",
		"wrong":   "other-key",
	} {
		req := httptest.NewRequest(http.MethodGet, "/memory/search", nil)
		if key != "" {
			req.Header.Set("X-FrontPocket-Key", key)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for %s key, got %d", name, rr.Code)
		}
	}
}

func TestAPIKeyMiddlewareRejectsUnconfiguredPlaceholderKey(t *testing.T) {
	handler := APIKeyMiddleware("change-me", "", okHandler())

	req := httptest.NewRequest(http.MethodGet, "/memory/search", nil)
	req.Header.Set("X-FrontPocket-Key", "change-me")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when the shipped placeholder key is used, got %d", rr.Code)
	}
}

func TestAPIKeyMiddlewareRejectsEmptyConfiguredKey(t *testing.T) {
	handler := APIKeyMiddleware("", "", okHandler())

	req := httptest.NewRequest(http.MethodGet, "/memory/search", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when no key is configured, got %d", rr.Code)
	}
}

func TestAPIKeyMiddlewareLeavesHealthOpen(t *testing.T) {
	handler := APIKeyMiddleware("secret-key", "", okHandler())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected health check to bypass auth, got %d", rr.Code)
	}
}

func TestCORSCoversAuthRejections(t *testing.T) {
	handler := CORSMiddleware([]string{"http://localhost:5173"}, APIKeyMiddleware("secret-key", "", okHandler()))

	req := httptest.NewRequest(http.MethodPost, "/memory/search", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected CORS header on auth rejection, got %q", got)
	}
}

func TestCORSAnswersPreflightBeforeAuth(t *testing.T) {
	handler := CORSMiddleware([]string{"http://localhost:5173"}, APIKeyMiddleware("secret-key", "", okHandler()))

	req := httptest.NewRequest(http.MethodOptions, "/memory/search", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 preflight, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected CORS header on preflight, got %q", got)
	}
}
