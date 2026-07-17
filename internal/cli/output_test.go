package cli

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"obdurate/internal/store"
)

func TestSplitCSVFlag(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"a", []string{"a"}},
		{" a, b ,,c ", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		if got := splitCSVFlag(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCSVFlag(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSetFlagsMutuallyExclusive(t *testing.T) {
	p := NewPrinter()
	if err := p.SetFlags(true, true, false); err == nil {
		t.Error("json+csv accepted, want error")
	}
	if err := p.SetFlags(false, true, true); err == nil {
		t.Error("csv+toon accepted, want error")
	}
	if err := p.SetFlags(true, false, false); err != nil || p.Mode != OutputJSON {
		t.Errorf("json flag: err=%v mode=%v", err, p.Mode)
	}
	if err := p.SetFlags(false, false, false); err != nil || p.Mode != OutputTable {
		t.Errorf("no flags: err=%v mode=%v", err, p.Mode)
	}
}

func TestExitCodeMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{store.ErrNotFound, 2},
		{fmt.Errorf("wrapped: %w", store.ErrNotFound), 2},
		{store.ErrAlreadyExists, 3},
		{store.ErrInvalidInput, 3},
		{store.ErrConflict, 3},
		{errors.New("boom"), 1},
	}
	for _, c := range cases {
		if got := exitCode(c.err); got != c.want {
			t.Errorf("exitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}
