package util

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Value-taking companions to the ToNullable* family in conversions.go.
//
// Those take a POINTER, because a nil pointer is the natural spelling of
// "absent" for an optional API field. A great deal of code instead holds the
// zero value — an empty string, uuid.Nil — and wants the same NULL. Without
// these it writes the three-line conversion inline, and four sibling libraries
// had grown twenty-four copies of exactly that between them.

// TextOrNull maps "" to SQL NULL. The pointer-taking form is ToNullableText.
func TextOrNull(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// UUIDOrNull maps uuid.Nil to SQL NULL. The pointer-taking form is ToNullableUUID.
func UUIDOrNull(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// JSONBOrDefault encodes v for a jsonb column, falling back to the supplied
// literal when v is nil. The default is raw JSON text, not a Go value, because
// the columns this feeds are NOT NULL with defaults like "{}" and "[]".
func JSONBOrDefault(v any, defaultJSON string) ([]byte, error) {
	if v == nil {
		return []byte(defaultJSON), nil
	}
	return json.Marshal(v)
}

// RemapJSON round-trips a decoded map into a typed struct. Useful where a
// payload arrives as map[string]any — a jsonb column, an MCP argument bag — and
// the caller wants the struct its tags describe.
func RemapJSON(m map[string]any, out any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// Truncate cuts s to at most max BYTES and marks the cut. Use it for machine
// fields — an error string bound for a column with a length limit.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// TruncateRunes cuts s to at most maxRunes CHARACTERS and marks the cut. Use it
// for anything a person reads: cutting by bytes splits a multi-byte rune and
// renders as a replacement character.
func TruncateRunes(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// OrDefault returns v unless it is empty.
func OrDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
