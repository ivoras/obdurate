package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrInvalidInput  = errors.New("invalid input")
	ErrConflict      = errors.New("conflict")
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// now returns the current UTC time as a fixed-width RFC3339 string with
// second granularity (e.g. "2026-07-18T09:08:43Z", always 20 chars), so
// lexicographic order equals chronological order.
func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// parseTime accepts both the current second-granularity format and the
// fractional-second RFC3339Nano strings written by older versions.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}

func nullString(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	s := ns.String
	return &s
}

func int64Ptr(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	v := ni.Int64
	return &v
}

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$`)

const maxSlugLen = 64

// normalizeSlug trims, lowercases, and validates a project/board name.
// Valid slugs are lowercase ASCII letters, digits, '-' or '_', starting and
// ending with a letter or digit, at most maxSlugLen characters.
func normalizeSlug(name, what string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("%w: %s name is required", ErrInvalidInput, what)
	}
	if len(name) > maxSlugLen {
		return "", fmt.Errorf("%w: %s name is longer than %d characters", ErrInvalidInput, what, maxSlugLen)
	}
	if !slugRe.MatchString(name) {
		return "", fmt.Errorf("%w: %s name %q must be a slug: lowercase letters, digits, '-' or '_', starting and ending with a letter or digit", ErrInvalidInput, what, name)
	}
	return name, nil
}

// normalizeMetadataKey trims, lowercases, and validates a task metadata key
// using the same slug rules as project/board names.
func normalizeMetadataKey(key string) (string, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return "", fmt.Errorf("%w: metadata key is required", ErrInvalidInput)
	}
	if len(key) > maxSlugLen {
		return "", fmt.Errorf("%w: metadata key is longer than %d characters", ErrInvalidInput, maxSlugLen)
	}
	if !slugRe.MatchString(key) {
		return "", fmt.Errorf("%w: metadata key %q must be a slug: lowercase letters, digits, '-' or '_', starting and ending with a letter or digit", ErrInvalidInput, key)
	}
	return key, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

func wrapUnique(err error, what string) error {
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: %s already exists", ErrAlreadyExists, what)
	}
	return err
}
