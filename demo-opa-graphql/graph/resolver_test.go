package graph

import (
	"sync"
	"testing"
)

func TestNewResolver(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil)
	if r == nil {
		t.Fatal("expected non-nil resolver")
	}
	if r.Authorizer != nil {
		t.Error("expected nil Authorizer when passed nil")
	}
	if len(r.users) != 2 {
		t.Errorf("users count = %d, want 2", len(r.users))
	}
	if r.postSeq != 1000 {
		t.Errorf("postSeq = %d, want 1000", r.postSeq)
	}
}

func TestNextPostID_Sequential(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil)
	id1 := r.nextPostID()
	id2 := r.nextPostID()
	id3 := r.nextPostID()

	if id1 != "post-1001" {
		t.Errorf("id1 = %q, want %q", id1, "post-1001")
	}
	if id2 != "post-1002" {
		t.Errorf("id2 = %q, want %q", id2, "post-1002")
	}
	if id3 != "post-1003" {
		t.Errorf("id3 = %q, want %q", id3, "post-1003")
	}
}

func TestNextPostID_Concurrent(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil)
	const n = 100
	ids := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = r.nextPostID()
		}(i)
	}
	wg.Wait()

	// Check all IDs are unique
	seen := map[string]struct{}{}
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate post ID: %s", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != n {
		t.Errorf("unique IDs = %d, want %d", len(seen), n)
	}
}

func TestLookupUser_Found(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil)
	record := r.lookupUser("user-1", []string{"extra"})
	if record.ID != "user-1" {
		t.Errorf("ID = %q, want %q", record.ID, "user-1")
	}
	if record.Name != "Alice" {
		t.Errorf("Name = %q, want %q", record.Name, "Alice")
	}
	// Should have union of base roles ("user") and extra roles ("extra")
	if len(record.Roles) != 2 {
		t.Fatalf("Roles = %v, want [user extra]", record.Roles)
	}
}

func TestLookupUser_NotFound(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil)
	record := r.lookupUser("unknown-user", []string{"viewer"})
	if record.ID != "unknown-user" {
		t.Errorf("ID = %q, want %q", record.ID, "unknown-user")
	}
	if record.Name != "unknown-user" {
		t.Errorf("Name = %q, want %q", record.Name, "unknown-user")
	}
	if len(record.Roles) != 1 || record.Roles[0] != "viewer" {
		t.Errorf("Roles = %v, want [viewer]", record.Roles)
	}
}

func TestLookupUser_FallbackNilRoles(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil)
	record := r.lookupUser("user-1", nil)
	if record.Name != "Alice" {
		t.Errorf("Name = %q, want %q", record.Name, "Alice")
	}
	if len(record.Roles) != 1 || record.Roles[0] != "user" {
		t.Errorf("Roles = %v, want [user]", record.Roles)
	}
}
