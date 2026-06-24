package core

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatKey renders an issue key from a project prefix and a per-project
// sequence number, e.g. FormatKey("JEE", 12) == "JEE-12".
func FormatKey(prefix string, seq int64) string {
	return fmt.Sprintf("%s-%d", prefix, seq)
}

// ParseKey splits an issue key into its prefix and sequence number. It is the
// inverse of FormatKey and is lenient about surrounding whitespace and case in
// the prefix (which it upper-cases to match NormalizePrefix).
func ParseKey(key string) (prefix string, seq int64, err error) {
	key = strings.TrimSpace(key)
	i := strings.LastIndex(key, "-")
	if i <= 0 || i == len(key)-1 {
		return "", 0, fmt.Errorf("core: invalid issue key %q", key)
	}
	prefix = strings.ToUpper(key[:i])
	seq, err = strconv.ParseInt(key[i+1:], 10, 64)
	if err != nil || seq <= 0 {
		return "", 0, fmt.Errorf("core: invalid issue key %q", key)
	}
	return prefix, seq, nil
}

// NormalizePrefix upper-cases and trims a project key prefix. It does not
// validate; use ValidPrefix for that.
func NormalizePrefix(prefix string) string {
	return strings.ToUpper(strings.TrimSpace(prefix))
}

// ValidPrefix reports whether prefix is a usable project key prefix: 2–10
// characters, all ASCII letters or digits, starting with a letter. The check is
// applied to the normalized form.
func ValidPrefix(prefix string) bool {
	p := NormalizePrefix(prefix)
	if len(p) < 2 || len(p) > 10 {
		return false
	}
	for i, r := range p {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
