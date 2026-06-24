package core

import "testing"

func TestFormatParseKeyRoundTrip(t *testing.T) {
	cases := []struct {
		prefix string
		seq    int64
	}{
		{"JEE", 1},
		{"JEE", 12},
		{"ABC", 9999},
		{"X1", 7},
	}
	for _, c := range cases {
		key := FormatKey(c.prefix, c.seq)
		prefix, seq, err := ParseKey(key)
		if err != nil {
			t.Fatalf("ParseKey(%q) error: %v", key, err)
		}
		if prefix != c.prefix || seq != c.seq {
			t.Errorf("round-trip %q -> (%q,%d), want (%q,%d)", key, prefix, seq, c.prefix, c.seq)
		}
	}
}

func TestParseKey(t *testing.T) {
	t.Run("lowercase prefix normalized", func(t *testing.T) {
		prefix, seq, err := ParseKey("  jee-42 ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prefix != "JEE" || seq != 42 {
			t.Errorf("got (%q,%d), want (JEE,42)", prefix, seq)
		}
	})
	bad := []string{"", "JEE", "JEE-", "-12", "JEE-0", "JEE-x", "JEE-1-2-", "12"}
	for _, k := range bad {
		if _, _, err := ParseKey(k); err == nil {
			t.Errorf("ParseKey(%q) = nil error, want error", k)
		}
	}
	// A hyphenated prefix keeps everything before the final hyphen.
	prefix, seq, err := ParseKey("WEB-API-3")
	if err != nil || prefix != "WEB-API" || seq != 3 {
		t.Errorf("ParseKey(WEB-API-3) = (%q,%d,%v), want (WEB-API,3,nil)", prefix, seq, err)
	}
}

func TestValidPrefix(t *testing.T) {
	good := []string{"JE", "JEE", "ABC123", "web", "X1234567Z"}
	for _, p := range good {
		if !ValidPrefix(p) {
			t.Errorf("ValidPrefix(%q) = false, want true", p)
		}
	}
	bad := []string{"", "J", "1AB", "AB_C", "AB C", "TOOLONGPREFIX", "AB-C"}
	for _, p := range bad {
		if ValidPrefix(p) {
			t.Errorf("ValidPrefix(%q) = true, want false", p)
		}
	}
}

func TestNormalizePrefix(t *testing.T) {
	if got := NormalizePrefix("  jee "); got != "JEE" {
		t.Errorf("NormalizePrefix = %q, want JEE", got)
	}
}
