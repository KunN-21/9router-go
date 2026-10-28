package chat

import (
	"testing"
	"time"
)

func TestSessionAffinityStore_BindGetEvict(t *testing.T) {
	store := NewSessionAffinityStore(100, 1*time.Hour)

	// Miss on non-existent
	if conn, ok := store.Get("codex", "gpt-5.6", "sess-1"); ok || conn != "" {
		t.Fatalf("expected miss, got %q", conn)
	}

	// Bind and Get
	store.Bind("codex", "gpt-5.6", "sess-1", "conn-a")
	if conn, ok := store.Get("codex", "gpt-5.6", "sess-1"); !ok || conn != "conn-a" {
		t.Fatalf("expected conn-a, got %q (ok=%v)", conn, ok)
	}

	// Multi-model separation within same session (e.g., fast model vs main model)
	store.Bind("codex", "gpt-5-mini", "sess-1", "conn-b")
	if conn, ok := store.Get("codex", "gpt-5-mini", "sess-1"); !ok || conn != "conn-b" {
		t.Fatalf("expected conn-b for mini, got %q", conn)
	}
	if conn, ok := store.Get("codex", "gpt-5.6", "sess-1"); !ok || conn != "conn-a" {
		t.Fatalf("expected conn-a for 5.6, got %q", conn)
	}

	// Evict
	store.Evict("codex", "gpt-5.6", "sess-1")
	if conn, ok := store.Get("codex", "gpt-5.6", "sess-1"); ok || conn != "" {
		t.Fatalf("expected empty after evict, got %q", conn)
	}
}

func TestSessionAffinityStore_EvictConnection(t *testing.T) {
	store := NewSessionAffinityStore(100, 1*time.Hour)
	store.Bind("codex", "gpt-5.6", "sess-1", "conn-dead")
	store.Bind("codex", "gpt-5.6", "sess-2", "conn-dead")
	store.Bind("codex", "gpt-5.6", "sess-3", "conn-alive")

	store.EvictConnection("conn-dead")
	if _, ok := store.Get("codex", "gpt-5.6", "sess-1"); ok {
		t.Error("sess-1 should be evicted")
	}
	if _, ok := store.Get("codex", "gpt-5.6", "sess-2"); ok {
		t.Error("sess-2 should be evicted")
	}
	if conn, ok := store.Get("codex", "gpt-5.6", "sess-3"); !ok || conn != "conn-alive" {
		t.Errorf("sess-3 should survive, got %q", conn)
	}
}

func TestSessionAffinityStore_TTLExpirationAndCapacity(t *testing.T) {
	store := NewSessionAffinityStore(2, 50*time.Millisecond)
	store.Bind("codex", "m1", "s1", "c1")
	// slight tick to differentiate timestamps
	time.Sleep(5 * time.Millisecond)
	store.Bind("codex", "m1", "s2", "c2")

	// Capacity eviction: adding 3rd entry evicts oldest (s1)
	time.Sleep(5 * time.Millisecond)
	store.Bind("codex", "m1", "s3", "c3")
	if _, ok := store.Get("codex", "m1", "s1"); ok {
		t.Error("s1 should have been evicted when exceeding capacity 2")
	}
	if conn, ok := store.Get("codex", "m1", "s2"); !ok || conn != "c2" {
		t.Errorf("s2 should still exist, got %q (ok=%v)", conn, ok)
	}

	// TTL expiration
	time.Sleep(60 * time.Millisecond)
	if _, ok := store.Get("codex", "m1", "s3"); ok {
		t.Error("s3 should expire after TTL")
	}
}
