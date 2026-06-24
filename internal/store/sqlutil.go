package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrNotFound is returned by Get* methods when no row matches.
var ErrNotFound = errors.New("store: not found")

// likeEscaper escapes the SQLite LIKE metacharacters using '\' as the escape
// character, so caller-supplied search text is matched literally. It must be
// paired with an `ESCAPE '\'` clause in the query.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string { return likeEscaper.Replace(s) }

// timeFormat is the canonical text encoding for every timestamp column.
const timeFormat = time.RFC3339Nano

func fmtTime(t time.Time) string { return t.UTC().Format(timeFormat) }

func parseTime(s string) (time.Time, error) { return time.Parse(timeFormat, s) }

// --- nullable column conversions ---------------------------------------------

func ptrTimeToNull(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: fmtTime(*t), Valid: true}
}

func nullToPtrTime(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid || ns.String == "" {
		return nil, nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func ptrInt64ToNull(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func nullToPtrInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func ptrIntToNull(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func nullToPtrInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// A nullable bool is stored as an INTEGER column: NULL means "inherit", 0/1 an
// explicit override.
func ptrBoolToNull(p *bool) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	var v int64
	if *p {
		v = 1
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

func nullToPtrBool(n sql.NullInt64) *bool {
	if !n.Valid {
		return nil
	}
	v := n.Int64 != 0
	return &v
}
