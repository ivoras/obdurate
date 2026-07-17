package store

import (
	"path/filepath"
	"strings"
	"testing"

	"obdurate/internal/db"
	"obdurate/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return New(sqlDB)
}

// fixture is a store pre-populated with two developers, a project, and a board.
type fixture struct {
	s     *Store
	alice *model.Developer
	bob   *model.Developer
	proj  *model.Project
	board *model.Board
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s := newTestStore(t)
	alice, err := s.CreateDeveloper("Alice Smith", "alice@example.com", "alice", nil, model.RoleLead)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := s.CreateDeveloper("Bob Jones", "bob@example.com", "bob", nil, model.RoleDeveloper)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	p, err := s.CreateProject("p1", "test project")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	b, err := s.CreateBoard("p1", "b1", "test board")
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	return &fixture{s: s, alice: alice, bob: bob, proj: p, board: b}
}

func intPtr(i int) *int                      { return &i }
func strP(s string) *string                  { return &s }
func prioP(p model.Priority) *model.Priority { return &p }

func TestNormalizeSlug(t *testing.T) {
	valid := map[string]string{
		"widget":       "widget",
		"Widget":       "widget",
		"  SPRINT-1  ": "sprint-1",
		"a":            "a",
		"a_b-c9":       "a_b-c9",
	}
	for in, want := range valid {
		got, err := normalizeSlug(in, "test")
		if err != nil {
			t.Errorf("normalizeSlug(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeSlug(%q) = %q, want %q", in, got, want)
		}
	}
	invalid := []string{
		"", "   ", "my project", "a/b", "-lead", "trail-", "_x", "x_",
		"naïve", "a.b", strings.Repeat("x", maxSlugLen+1),
	}
	for _, in := range invalid {
		if _, err := normalizeSlug(in, "test"); err == nil {
			t.Errorf("normalizeSlug(%q): expected error, got none", in)
		}
	}
}

func TestSameTagSet(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{[]string{"a"}, []string{"A"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{[]string{"a"}, []string{"a", "b"}, false},
		{[]string{"a"}, []string{"b"}, false},
	}
	for _, c := range cases {
		if got := sameTagSet(c.a, c.b); got != c.want {
			t.Errorf("sameTagSet(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
