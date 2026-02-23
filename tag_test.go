package imap

import (
	"strings"
	"testing"

	"github.com/rs/xid"
)

func TestTagFormat(t *testing.T) {
	// Generate several tags to check properties
	for range 100 {
		tag := strings.ToUpper(xid.New().String())

		if len(tag) != 20 {
			t.Fatalf("expected tag length 20, got %d: %q", len(tag), tag)
		}

		for _, c := range tag {
			if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'V')) {
				t.Fatalf("tag contains invalid character %q in %q", string(c), tag)
			}
		}
	}
}

func TestTagUniqueness(t *testing.T) {
	seen := make(map[string]struct{})
	for range 1000 {
		tag := strings.ToUpper(xid.New().String())
		if _, ok := seen[tag]; ok {
			t.Fatalf("duplicate tag generated: %q", tag)
		}
		seen[tag] = struct{}{}
	}
}
