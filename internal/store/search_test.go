package store

import (
	"errors"
	"strings"
	"testing"
)

func TestSearchTasksRanksAndHighlights(t *testing.T) {
	f := newFixture(t)
	strong, err := f.s.CreateTask(TaskCreate{
		BoardRef: "p1/b1", Title: "Login page crashes",
		Description: "Users report the login page crashes on submit",
	})
	if err != nil {
		t.Fatalf("create strong: %v", err)
	}
	weak, err := f.s.CreateTask(TaskCreate{
		BoardRef: "p1/b1", Title: "Add dark mode",
		Description: "Support a dark theme; mentions login once for SSO",
	})
	if err != nil {
		t.Fatalf("create weak: %v", err)
	}
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "Unrelated", Description: "nothing to do with it"}); err != nil {
		t.Fatalf("create unrelated: %v", err)
	}

	hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "login"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0].ID != strong.ID || hits[1].ID != weak.ID {
		t.Errorf("order = [%d %d], want [%d %d] (title+description match ranks above single mention)",
			hits[0].ID, hits[1].ID, strong.ID, weak.ID)
	}
	if hits[0].Rank >= hits[1].Rank {
		t.Errorf("rank[0]=%v should be more negative (better) than rank[1]=%v", hits[0].Rank, hits[1].Rank)
	}
	if !strings.Contains(hits[0].TitleHighlight, "**login**") && !strings.Contains(strings.ToLower(hits[0].TitleHighlight), "**login**") {
		t.Errorf("title highlight = %q, want **login** wrapped (case-insensitive)", hits[0].TitleHighlight)
	}
	if !strings.Contains(hits[0].DescriptionHighlight, "**login**") {
		t.Errorf("description highlight = %q, want **login** wrapped", hits[0].DescriptionHighlight)
	}
}

func TestSearchTasksHydratesRelations(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{
		BoardRef: "p1/b1", Title: "Findable task", Description: "unique-search-term-xyz",
		Tags: []string{"bug"}, WatcherRefs: []string{"bob"}, AssigneeRef: "alice",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.s.SetTaskMetadata(task.ID, "pr", "#123", ""); err != nil {
		t.Fatalf("set metadata: %v", err)
	}

	hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "unique-search-term-xyz"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	hit := hits[0]
	if len(hit.Tags) != 1 || hit.Tags[0] != "bug" {
		t.Errorf("tags = %v, want [bug]", hit.Tags)
	}
	if len(hit.WatcherRefs) != 1 || hit.WatcherRefs[0] != "bob" {
		t.Errorf("watchers = %v, want [bob]", hit.WatcherRefs)
	}
	if hit.AssigneeRef != "alice" {
		t.Errorf("assignee = %q, want alice", hit.AssigneeRef)
	}
	if hit.Metadata["pr"] != "#123" {
		t.Errorf("metadata = %v, want pr=#123", hit.Metadata)
	}
}

func TestSearchTasksBoardAndProjectFilter(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.CreateBoard("p1", "b2", "", ""); err != nil {
		t.Fatalf("create b2: %v", err)
	}
	if _, err := f.s.CreateProject("p2", "", ""); err != nil {
		t.Fatalf("create p2: %v", err)
	}
	if _, err := f.s.CreateBoard("p2", "b1", "", ""); err != nil {
		t.Fatalf("create p2/b1: %v", err)
	}
	mk := func(board string) {
		t.Helper()
		if _, err := f.s.CreateTask(TaskCreate{BoardRef: board, Title: "findme widget", Description: ""}); err != nil {
			t.Fatalf("create on %s: %v", board, err)
		}
	}
	mk("p1/b1")
	mk("p1/b2")
	mk("p2/b1")

	cases := []struct {
		name string
		f    TaskSearchFilter
		want int
	}{
		{"no filter", TaskSearchFilter{Query: "widget"}, 3},
		{"by board", TaskSearchFilter{Query: "widget", BoardRef: "p1/b1"}, 1},
		{"by project", TaskSearchFilter{Query: "widget", ProjectRef: "p1"}, 2},
	}
	for _, c := range cases {
		hits, err := f.s.SearchTasks(c.f)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(hits) != c.want {
			t.Errorf("%s: %d hits, want %d", c.name, len(hits), c.want)
		}
	}
}

func TestSearchTasksStaysInSyncOnUpdateAndDelete(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "Original marker term", Description: ""})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "marker"}); err != nil || len(hits) != 1 {
		t.Fatalf("initial search: hits=%v err=%v", hits, err)
	}

	if _, err := f.s.UpdateTask(task.ID, TaskUpdate{Title: strP("Renamed away")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "marker"}); err != nil || len(hits) != 0 {
		t.Errorf("after rename, search for old title: hits=%v err=%v, want 0", hits, err)
	}
	if hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "renamed"}); err != nil || len(hits) != 1 {
		t.Errorf("after rename, search for new title: hits=%v err=%v, want 1", hits, err)
	}

	if err := f.s.DeleteTask(task.ID, ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "renamed"}); err != nil || len(hits) != 0 {
		t.Errorf("after delete: hits=%v err=%v, want 0", hits, err)
	}
}

func TestSearchTasksValidation(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.SearchTasks(TaskSearchFilter{Query: "  "}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty query: err = %v, want ErrInvalidInput", err)
	}
	if _, err := f.s.SearchTasks(TaskSearchFilter{Query: "x", BoardRef: "p1/nope"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad board ref: err = %v, want ErrNotFound", err)
	}
}

// TestSearchTasksSpecialCharactersDontErrorFTS5Syntax guards against a bug
// found while building this feature: raw FTS5 MATCH treats a bare hyphen as
// the NOT operator, so an entirely ordinary query like "PROJ-123" (exactly
// the shape of a value stored under the task metadata `pr` key) used to
// fail with a confusing "no such column: 123" parse error. buildMatchQuery
// quotes every term so hyphens and quotes are literal instead of syntax.
func TestSearchTasksSpecialCharactersDontErrorFTS5Syntax(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.CreateTask(TaskCreate{
		BoardRef: "p1/b1", Title: "PROJ-123 fix", Description: `ticket has "quotes" and a colon: here`,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, q := range []string{"PROJ-123", "proj-123", `has "quotes" inside`, "colon:", `AND OR NOT ( )`} {
		if _, err := f.s.SearchTasks(TaskSearchFilter{Query: q}); err != nil {
			t.Errorf("query %q returned an error (want none): %v", q, err)
		}
	}
	hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "PROJ-123"})
	if err != nil {
		t.Fatalf("search PROJ-123: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("PROJ-123 hits = %d, want 1", len(hits))
	}
}

func TestSearchTasksEmptyResultNonNil(t *testing.T) {
	f := newFixture(t)
	hits, err := f.s.SearchTasks(TaskSearchFilter{Query: "nothing-matches-this-xyz"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if hits == nil {
		t.Error("SearchTasks returned nil slice; must be empty non-nil for JSON []")
	}
}
