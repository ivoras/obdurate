package store

import (
	"database/sql"
	"errors"
	"fmt"
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

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

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
