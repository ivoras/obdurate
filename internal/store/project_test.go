package store

import (
	"errors"
	"strconv"
	"testing"
)

func TestProjectSlugEnforcement(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject("Widget", "desc", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Name != "widget" {
		t.Errorf("name = %q, want lowercased %q", p.Name, "widget")
	}
	if _, err := s.CreateProject("My Project", "", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("space name: err = %v, want ErrInvalidInput", err)
	}
	if _, err := s.UpdateProject("widget", ProjectUpdate{Name: strP("Bad Name")}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("rename invalid: err = %v, want ErrInvalidInput", err)
	}
	up, err := s.UpdateProject("widget", ProjectUpdate{Name: strP("GADGET")})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if up.Name != "gadget" {
		t.Errorf("renamed = %q, want %q", up.Name, "gadget")
	}
}

func TestProjectDuplicateAndResolve(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject("p1", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.CreateProject("P1", "", ""); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate (case-insensitive): err = %v, want ErrAlreadyExists", err)
	}
	for _, ref := range []string{"p1", "P1", strconv.FormatInt(p.ID, 10)} {
		got, err := s.ResolveProject(ref)
		if err != nil {
			t.Errorf("resolve %q: %v", ref, err)
			continue
		}
		if got.ID != p.ID {
			t.Errorf("resolve %q: id = %d, want %d", ref, got.ID, p.ID)
		}
	}
	if _, err := s.ResolveProject("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("resolve missing: err = %v, want ErrNotFound", err)
	}
}

func TestEnsureDefaults(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	b, err := s.ResolveBoard("default/main")
	if err != nil {
		t.Fatalf("default/main not seeded: %v", err)
	}
	cols, err := s.ListColumns(b.ID)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	if len(cols) != 3 || cols[0].Name != "Todo" || cols[1].Name != "Doing" || cols[2].Name != "Done" {
		t.Errorf("seeded columns = %+v, want Todo/Doing/Done", cols)
	}
	// Idempotent.
	if err := s.EnsureDefaults(); err != nil {
		t.Fatalf("second EnsureDefaults: %v", err)
	}
	list, _ := s.ListProjects()
	if len(list) != 1 {
		t.Errorf("projects after re-run = %d, want 1", len(list))
	}
	// A deleted default is not recreated while other projects exist.
	if _, err := s.CreateProject("real", "", ""); err != nil {
		t.Fatalf("create real: %v", err)
	}
	if err := s.DeleteProject("default", ""); err != nil {
		t.Fatalf("delete default: %v", err)
	}
	if err := s.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults with existing project: %v", err)
	}
	if _, err := s.ResolveProject("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("default recreated despite existing projects: err = %v", err)
	}
	// With zero projects it is reseeded.
	if err := s.DeleteProject("real", ""); err != nil {
		t.Fatalf("delete real: %v", err)
	}
	if err := s.EnsureDefaults(); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if _, err := s.ResolveProject("default"); err != nil {
		t.Errorf("default not reseeded on empty db: %v", err)
	}
}

func TestProjectDeleteCascades(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "doomed"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := f.s.DeleteProject("p1", ""); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := f.s.ResolveBoard("p1/b1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("board survived project delete: err = %v", err)
	}
	if _, err := f.s.GetTask(task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("task survived project delete: err = %v", err)
	}
}

func TestListProjectsEmptyNonNil(t *testing.T) {
	s := newTestStore(t)
	list, err := s.ListProjects()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list == nil {
		t.Error("ListProjects returned nil slice; must be empty non-nil for JSON []")
	}
}
