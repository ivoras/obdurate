package store

import (
	"errors"
	"strconv"
	"testing"

	"obdurate/internal/model"
)

func TestDeveloperCreateResolve(t *testing.T) {
	s := newTestStore(t)
	slack := "U123ABC"
	d, err := s.CreateDeveloper("Alice Smith", "Alice@Example.com", "alice", &slack, model.RoleLead)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, ref := range []string{strconv.FormatInt(d.ID, 10), "alice@example.com", "ALICE", "u123abc"} {
		got, err := s.ResolveDeveloper(ref)
		if err != nil {
			t.Errorf("resolve %q: %v", ref, err)
			continue
		}
		if got.ID != d.ID {
			t.Errorf("resolve %q: id = %d, want %d", ref, got.ID, d.ID)
		}
	}
	if _, err := s.ResolveDeveloper("ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("resolve missing: err = %v, want ErrNotFound", err)
	}
}

func TestDeveloperValidationAndUniqueness(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateDeveloper("", "a@x.com", "a", nil, model.RoleDeveloper); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty name: err = %v, want ErrInvalidInput", err)
	}
	if _, err := s.CreateDeveloper("A", "a@x.com", "a", nil, model.Role("boss")); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad role: err = %v, want ErrInvalidInput", err)
	}
	if _, err := s.CreateDeveloper("A", "a@x.com", "a", nil, model.RoleDeveloper); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.CreateDeveloper("B", "A@X.COM", "b", nil, model.RoleDeveloper); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate email: err = %v, want ErrAlreadyExists", err)
	}
	if _, err := s.CreateDeveloper("B", "b@x.com", "A", nil, model.RoleDeveloper); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate username: err = %v, want ErrAlreadyExists", err)
	}
}

func TestDeveloperUpdate(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateDeveloper("A", "a@x.com", "a", nil, model.RoleDeveloper); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Empty required fields are rejected on update.
	for _, u := range []DeveloperUpdate{
		{Name: strP("  ")},
		{Email: strP("")},
		{Username: strP("")},
	} {
		if _, err := s.UpdateDeveloper("a", u); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("update %+v: err = %v, want ErrInvalidInput", u, err)
		}
	}
	if _, err := s.UpdateDeveloper("a", DeveloperUpdate{Role: (*model.Role)(strP("boss"))}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad role: err = %v, want ErrInvalidInput", err)
	}
	// Set and clear slack id.
	d, err := s.UpdateDeveloper("a", DeveloperUpdate{SlackID: strP("U9")})
	if err != nil {
		t.Fatalf("set slack: %v", err)
	}
	if d.SlackID == nil || *d.SlackID != "U9" {
		t.Errorf("slack id = %v, want U9", d.SlackID)
	}
	d, err = s.UpdateDeveloper("a", DeveloperUpdate{SlackID: strP("")})
	if err != nil {
		t.Fatalf("clear slack: %v", err)
	}
	if d.SlackID != nil {
		t.Errorf("slack id = %v, want nil after clear", d.SlackID)
	}
	// Trimming applies.
	d, err = s.UpdateDeveloper("a", DeveloperUpdate{Email: strP("  new@x.com  ")})
	if err != nil {
		t.Fatalf("update email: %v", err)
	}
	if d.Email != "new@x.com" {
		t.Errorf("email = %q, want trimmed", d.Email)
	}
}

func TestDeveloperDeleteUnassignsTasks(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "t", AssigneeRef: "alice", WatcherRefs: []string{"alice"}})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := f.s.DeleteDeveloper("alice"); err != nil {
		t.Fatalf("delete developer: %v", err)
	}
	got, err := f.s.GetTask(task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.AssigneeID != nil || got.AssigneeRef != "" {
		t.Errorf("task still assigned after developer delete: %+v", got)
	}
	if len(got.WatcherRefs) != 0 {
		t.Errorf("watchers = %v, want none", got.WatcherRefs)
	}
}

func TestListDevelopersEmptyNonNil(t *testing.T) {
	s := newTestStore(t)
	list, err := s.ListDevelopers()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list == nil {
		t.Error("ListDevelopers returned nil slice; must be empty non-nil for JSON []")
	}
}
