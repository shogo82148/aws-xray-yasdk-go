//go:build go1.19
// +build go1.19

package xray

import (
	"reflect"
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

func FuzzParseTraceHeader(f *testing.F) {
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5")
	f.Add("ROOT=1-5E645F3E-1DFAD076A177C5CCC5DE12F5")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=1")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=0")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=?")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;foo=bar;hoge=fuga")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;foo")
	f.Add("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=invalid")
	f.Add("foo=bar;hoge=fuga;Sampled=1;Parent=03babb4ba280be51;Root=1-5e645f3e-1dfad076a177c5ccc5de12f5")

	f.Fuzz(func(t *testing.T, s string) {
		h0 := ParseTraceHeader(s)
		h1 := ParseTraceHeader(h0.String())
		if !reflect.DeepEqual(h0, h1) {
			t.Errorf("parsing result not match: %q and %q: input %q", h0, h1, s)
		}
	})
}
