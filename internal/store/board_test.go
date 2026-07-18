package store

import (
	"errors"
	"strconv"
	"testing"
)

func TestBoardResolveForms(t *testing.T) {
	f := newFixture(t)
	for _, ref := range []string{"p1/b1", "P1/B1", "b1", strconv.FormatInt(f.board.ID, 10)} {
		got, err := f.s.ResolveBoard(ref)
		if err != nil {
			t.Errorf("resolve %q: %v", ref, err)
			continue
		}
		if got.ID != f.board.ID {
			t.Errorf("resolve %q: id = %d, want %d", ref, got.ID, f.board.ID)
		}
	}
	if _, err := f.s.ResolveBoard("p1/nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing board: err = %v, want ErrNotFound", err)
	}
}

func TestBoardAmbiguousName(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.CreateProject("p2", "", ""); err != nil {
		t.Fatalf("create p2: %v", err)
	}
	if _, err := f.s.CreateBoard("p2", "b1", "", ""); err != nil {
		t.Fatalf("create p2/b1: %v", err)
	}
	if _, err := f.s.ResolveBoard("b1"); !errors.Is(err, ErrConflict) {
		t.Errorf("ambiguous bare name: err = %v, want ErrConflict", err)
	}
	if _, err := f.s.ResolveBoard("p2/b1"); err != nil {
		t.Errorf("qualified ref should resolve: %v", err)
	}
}

func TestBoardSlugAndDuplicate(t *testing.T) {
	f := newFixture(t)
	b, err := f.s.CreateBoard("p1", "Sprint-1", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.Name != "sprint-1" {
		t.Errorf("name = %q, want %q", b.Name, "sprint-1")
	}
	if _, err := f.s.CreateBoard("p1", "bad name", "", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("invalid name: err = %v, want ErrInvalidInput", err)
	}
	if _, err := f.s.CreateBoard("p1", "B1", "", ""); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate in project: err = %v, want ErrAlreadyExists", err)
	}
	// Same name in another project is fine.
	if _, err := f.s.CreateProject("p2", "", ""); err != nil {
		t.Fatalf("create p2: %v", err)
	}
	if _, err := f.s.CreateBoard("p2", "b1", "", ""); err != nil {
		t.Errorf("same board name in other project: %v", err)
	}
}

func TestBoardDeleteCascadesTasks(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "doomed"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := f.s.DeleteBoard("p1/b1", ""); err != nil {
		t.Fatalf("delete board: %v", err)
	}
	if _, err := f.s.GetTask(task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("task survived board delete: err = %v", err)
	}
}

func columnNames(t *testing.T, s *Store, boardID int64) []string {
	t.Helper()
	cols, err := s.ListColumns(boardID)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	names := make([]string, 0, len(cols))
	for i, c := range cols {
		if c.Position != i {
			t.Errorf("column %q position = %d, want %d (compact ordering)", c.Name, c.Position, i)
		}
		names = append(names, c.Name)
	}
	return names
}

func TestColumnAddPositionsAndClamp(t *testing.T) {
	f := newFixture(t)
	// Insert between Doing (1) and Done (2).
	if _, err := f.s.AddColumn("p1/b1", "Review", intPtr(2), ""); err != nil {
		t.Fatalf("add review: %v", err)
	}
	// Negative clamps to 0.
	if _, err := f.s.AddColumn("p1/b1", "Icebox", intPtr(-5), ""); err != nil {
		t.Fatalf("add icebox: %v", err)
	}
	// Past-the-end clamps to append.
	if _, err := f.s.AddColumn("p1/b1", "Archive", intPtr(99), ""); err != nil {
		t.Fatalf("add archive: %v", err)
	}
	got := columnNames(t, f.s, f.board.ID)
	want := []string{"Icebox", "Todo", "Doing", "Review", "Done", "Archive"}
	if len(got) != len(want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columns = %v, want %v", got, want)
		}
	}
}

func TestColumnRenameReorderDelete(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.RenameColumn("p1/b1", "Todo", "Doing", ""); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("rename to duplicate: err = %v, want ErrAlreadyExists", err)
	}
	if _, err := f.s.RenameColumn("p1/b1", "Todo", "Backlog", ""); err != nil {
		t.Fatalf("rename: %v", err)
	}
	// Reorder Done to front; out-of-range clamps.
	if _, err := f.s.ReorderColumn("p1/b1", "Done", -3, ""); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	got := columnNames(t, f.s, f.board.ID)
	want := []string{"Done", "Backlog", "Doing"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columns = %v, want %v", got, want)
		}
	}
	// Delete refuses when the column holds tasks.
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "x", ColumnRef: "Backlog"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := f.s.DeleteColumn("p1/b1", "Backlog", ""); !errors.Is(err, ErrConflict) {
		t.Errorf("delete non-empty: err = %v, want ErrConflict", err)
	}
	// Empty column deletes and positions compact.
	if err := f.s.DeleteColumn("p1/b1", "Doing", ""); err != nil {
		t.Fatalf("delete empty: %v", err)
	}
	got = columnNames(t, f.s, f.board.ID) // also asserts compact positions
	want = []string{"Done", "Backlog"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columns = %v, want %v", got, want)
		}
	}
}

func TestListBoardsEmptyNonNil(t *testing.T) {
	s := newTestStore(t)
	list, err := s.ListBoards("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list == nil {
		t.Error("ListBoards returned nil slice; must be empty non-nil for JSON []")
	}
}
