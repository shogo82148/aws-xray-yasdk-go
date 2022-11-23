//go:build go1.19
// +build go1.19

package xray

import (
	"regexp"
	"strings"
	"testing"
)

func FuzzSanitizeSegmentName(f *testing.F) {
	f.Add("abc 123")
	f.Add(`_.:/%&#=+\-@`)
	f.Add("abc!?[](){}123")
	f.Add("こんにちは世界")
	f.Add(strings.Repeat("あ", 201))
	re := regexp.MustCompile(`\A[\p{L}\p{N}\p{Z}_.:\/%&#=+\\\-@]{1,200}\z`)
	f.Fuzz(func(t *testing.T, s string) {
		v := sanitizeSegmentName(s)
		if !re.MatchString(v) {
			t.Errorf("sanitize failed: %q -> %q", s, v)
		}
	})
}
