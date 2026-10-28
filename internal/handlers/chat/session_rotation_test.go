package chat

import (
	"context"
	"testing"
	"time"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
)

func TestGetBestConnection_SessionAffinityPinning(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := db.NewRepo(database)
	h := NewChatHandler(repo)

	// Seed 3 active codex connections
	seedConnDB(t, database, "codex", "conn-cx-1", "tok-1", "https://mock.example.com/1")
	seedConnDB(t, database, "codex", "conn-cx-2", "tok-2", "https://mock.example.com/2")
	seedConnDB(t, database, "codex", "conn-cx-3", "tok-3", "https://mock.example.com/3")

	// Set round-robin strategy for codex
	if err := repo.SetProviderStrategy("codex", db.ProviderStrategy{
		RotateStrategy: "round-robin",
	}); err != nil {
		t.Fatalf("SetProviderStrategy: %v", err)
	}

	ctxSessA := handlerutil.WithSessionID(context.Background(), "session-repo-A")
	ctxSessB := handlerutil.WithSessionID(context.Background(), "session-repo-B")

	// Turn 1 for Session A -> picks conn-cx-1
	c1, _, err := h.GetBestConnectionWithContext(ctxSessA, "codex", "", nil, "gpt-5.6")
	if err != nil || c1 == nil || c1.ID != "conn-cx-1" {
		t.Fatalf("session A turn 1: expected conn-cx-1, got %v (err=%v)", c1, err)
	}

	// Turn 1 for Session B -> round-robin alternates to conn-cx-2
	c2, _, err := h.GetBestConnectionWithContext(ctxSessB, "codex", "", nil, "gpt-5.6")
	if err != nil || c2 == nil || c2.ID != "conn-cx-2" {
		t.Fatalf("session B turn 1: expected conn-cx-2, got %v (err=%v)", c2, err)
	}

	// Turn 2 for Session A -> must still be pinned to conn-cx-1!
	// (Under naive round-robin, this would have rotated to conn-cx-3)
	c3, _, err := h.GetBestConnectionWithContext(ctxSessA, "codex", "", nil, "gpt-5.6")
	if err != nil || c3 == nil || c3.ID != "conn-cx-1" {
		t.Fatalf("session A turn 2: expected conn-cx-1 (affinity pinned), got %v (err=%v)", c3, err)
	}

	// Turn 2 for Session B -> must still be pinned to conn-cx-2!
	// (Under naive round-robin, this would have rotated to conn-cx-1)
	c4, _, err := h.GetBestConnectionWithContext(ctxSessB, "codex", "", nil, "gpt-5.6")
	if err != nil || c4 == nil || c4.ID != "conn-cx-2" {
		t.Fatalf("session B turn 2: expected conn-cx-2 (affinity pinned), got %v (err=%v)", c4, err)
	}
}

func TestGetBestConnection_SessionAffinityCooldownFailover(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := db.NewRepo(database)
	h := NewChatHandler(repo)

	seedConnDB(t, database, "codex", "conn-fail-1", "tok-1", "https://mock.example.com/1")
	seedConnDB(t, database, "codex", "conn-fail-2", "tok-2", "https://mock.example.com/2")

	if err := repo.SetProviderStrategy("codex", db.ProviderStrategy{
		RotateStrategy: "round-robin",
	}); err != nil {
		t.Fatalf("SetProviderStrategy: %v", err)
	}

	ctxSess := handlerutil.WithSessionID(context.Background(), "session-failover")

	// Turn 1 -> pins to conn-fail-1
	c1, _, err := h.GetBestConnectionWithContext(ctxSess, "codex", "", nil, "gpt-5.6")
	if err != nil || c1 == nil || c1.ID != "conn-fail-1" {
		t.Fatalf("turn 1: expected conn-fail-1, got %v (err=%v)", c1, err)
	}

	// Mark conn-fail-1 in cooldown (blocked for 5 minutes)
	cooldownUntil := time.Now().Add(5 * time.Minute).Format(time.RFC3339)
	blockedData := `{"rateLimitedUntil":"` + cooldownUntil + `"}`
	if _, err := database.Exec(`UPDATE providerConnections SET data = ? WHERE id = ?`, blockedData, "conn-fail-1"); err != nil {
		t.Fatalf("update cooldown failed: %v", err)
	}

	// Turn 2 for session-failover -> conn-fail-1 is in cooldown, so it must evict and re-route to conn-fail-2!
	c2, _, err := h.GetBestConnectionWithContext(ctxSess, "codex", "", nil, "gpt-5.6")
	if err != nil || c2 == nil || c2.ID != "conn-fail-2" {
		t.Fatalf("turn 2 after cooldown: expected failover to conn-fail-2, got %v (err=%v)", c2, err)
	}

	// Turn 3 -> should now stay pinned to conn-fail-2
	c3, _, err := h.GetBestConnectionWithContext(ctxSess, "codex", "", nil, "gpt-5.6")
	if err != nil || c3 == nil || c3.ID != "conn-fail-2" {
		t.Fatalf("turn 3: expected to stay on conn-fail-2, got %v (err=%v)", c3, err)
	}
}

func TestGetBestConnection_NonSessionTrafficFallsThrough(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := db.NewRepo(database)
	h := NewChatHandler(repo)

	seedConnDB(t, database, "codex", "conn-ns-1", "tok-1", "https://mock.example.com/1")
	seedConnDB(t, database, "codex", "conn-ns-2", "tok-2", "https://mock.example.com/2")

	if err := repo.SetProviderStrategy("codex", db.ProviderStrategy{
		RotateStrategy: "round-robin",
	}); err != nil {
		t.Fatalf("SetProviderStrategy: %v", err)
	}

	ctxNoSession := context.Background()

	// 1st non-session request -> conn-ns-1
	c1, _, err := h.GetBestConnectionWithContext(ctxNoSession, "codex", "", nil, "gpt-5.6")
	if err != nil || c1 == nil || c1.ID != "conn-ns-1" {
		t.Fatalf("call 1: expected conn-ns-1, got %v", c1)
	}

	// 2nd non-session request -> rotates to conn-ns-2
	c2, _, err := h.GetBestConnectionWithContext(ctxNoSession, "codex", "", nil, "gpt-5.6")
	if err != nil || c2 == nil || c2.ID != "conn-ns-2" {
		t.Fatalf("call 2: expected conn-ns-2, got %v", c2)
	}

	// 3rd non-session request -> rotates to conn-ns-1
	c3, _, err := h.GetBestConnectionWithContext(ctxNoSession, "codex", "", nil, "gpt-5.6")
	if err != nil || c3 == nil || c3.ID != "conn-ns-1" {
		t.Fatalf("call 3: expected conn-ns-1, got %v", c3)
	}
}
