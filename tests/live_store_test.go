package tests

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	defaultTestQdrantURL = "http://127.0.0.1:6333"
	defaultTestRedisURL  = "redis://127.0.0.1:6379/0"
)

// requireLiveStores returns Qdrant and Redis URLs for tests that need a real
// backing store, skipping the test when that store is not reachable. The
// defaults match docker-compose.yml; override with FRONTPOCKET_TEST_QDRANT_URL
// and FRONTPOCKET_TEST_REDIS_URL.
func requireLiveStores(t *testing.T) (string, string) {
	t.Helper()

	qdrantURL := firstNonEmptyEnv("FRONTPOCKET_TEST_QDRANT_URL", defaultTestQdrantURL)
	redisURL := firstNonEmptyEnv("FRONTPOCKET_TEST_REDIS_URL", defaultTestRedisURL)

	if os.Getenv("FRONTPOCKET_SKIP_INTEGRATION") != "" {
		t.Skip("skipping: FRONTPOCKET_SKIP_INTEGRATION is set")
	}
	if !hostReachable(t, qdrantURL, "6333", "http") {
		t.Skipf("skipping: no Qdrant reachable at %s (start docker compose or set FRONTPOCKET_TEST_QDRANT_URL)", qdrantURL)
	}
	if !hostReachable(t, redisURL, "6379", "redis") {
		t.Skipf("skipping: no Redis reachable at %s (start docker compose or set FRONTPOCKET_TEST_REDIS_URL)", redisURL)
	}
	return qdrantURL, redisURL
}

func firstNonEmptyEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// testCollectionName returns a run-unique collection name and drops it when the
// test finishes, so integration tests never inherit vectors from an earlier run
// (a stale collection with different dimensions fails dimension checks).
func testCollectionName(t *testing.T, qdrantURL, base string) string {
	t.Helper()

	name := fmt.Sprintf("%s_%d", base, time.Now().UnixNano())
	t.Cleanup(func() {
		endpoint := strings.TrimRight(qdrantURL, "/") + "/collections/" + name
		req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
		if err != nil {
			t.Logf("could not build cleanup request for %s: %v", name, err)
			return
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("could not drop test collection %s: %v", name, err)
			return
		}
		_ = resp.Body.Close()
	})
	return name
}

func hostReachable(t *testing.T, rawURL, defaultPort, defaultScheme string) bool {
	t.Helper()

	if !strings.Contains(rawURL, "://") {
		rawURL = defaultScheme + "://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid store URL %q: %v", rawURL, err)
	}

	port := parsed.Port()
	if port == "" {
		port = defaultPort
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(parsed.Hostname(), port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
