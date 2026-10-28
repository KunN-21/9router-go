package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
)

func TestFallback_RebindsAffinityOn429(t *testing.T) {
	// srv1 returns 429 Too Many Requests
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"requests","code":"rate_limit_exceeded"}}`))
	}))
	defer srv1.Close()

	// srv2 returns 200 OK
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	}))
	defer srv2.Close()

	database, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := db.NewRepo(database)
	h := NewChatHandler(repo)

	seedConnDB(t, database, "deepseek", "conn-fo-1", "sk-1", srv1.URL)
	seedConnDB(t, database, "deepseek", "conn-fo-2", "sk-2", srv2.URL)

	if err := repo.SetProviderStrategy("deepseek", db.ProviderStrategy{
		RotateStrategy: "round-robin",
	}); err != nil {
		t.Fatalf("SetProviderStrategy: %v", err)
	}

	sessionID := "sess-failover-429"
	model := "deepseek-chat"

	// Pre-bind to conn-fo-1
	h.getSessionAffinity().Bind("deepseek", model, sessionID, "conn-fo-1")
	if bound, ok := h.getSessionAffinity().Get("deepseek", model, sessionID); !ok || bound != "conn-fo-1" {
		t.Fatalf("initial binding expected conn-fo-1, got %q", bound)
	}

	ctx := handlerutil.WithSessionID(context.Background(), sessionID)
	rec := httptest.NewRecorder()
	reqBody := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`)

	err := h.handleAccountFallback(ctx, rec, "deepseek", model, "", reqBody, false, false, "/v1/chat/completions")
	if err != nil {
		t.Fatalf("handleAccountFallback failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from fallback, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify affinity was re-bound to conn-fo-2!
	boundAfter, ok := h.getSessionAffinity().Get("deepseek", model, sessionID)
	if !ok || boundAfter != "conn-fo-2" {
		t.Errorf("affinity after 429 failover = %q, want conn-fo-2", boundAfter)
	}
}
