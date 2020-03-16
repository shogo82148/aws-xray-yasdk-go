package sampling

import "testing"

func TestWildcardMatch(t *testing.T) {
	tc := []struct {
		pattern         string
		text            string
		caseInsensitive bool
		want            bool
	}{
		// empty pattern
		{
			pattern: "",
			text:    "",
			want:    true,
		},
		{
			pattern: "",
			text:    "a",
			want:    false,
		},

		// wildcard
		{
			pattern: "*",
			text:    "",
			want:    true,
		},
		{
			pattern: "*",
			text:    "foobar",
			want:    true,
		},
		{
			pattern: "*c",
			text:    "abc",
			want:    true,
		},
		{
			pattern: "*x",
			text:    "xxx",
			want:    true,
		},
		{
			pattern: "a*",
			text:    "a",
			want:    true,
		},
		{
			pattern: "a*b*c*d*e*",
			text:    "axbxcxdxexxx",
			want:    true,
		},
		{
			pattern: "a*b*c********d*e*",
			text:    "axbxcxdxexxx",
			want:    true,
		},

		// case sensitivity
		{
			pattern: "abc",
			text:    "abc",
			want:    true,
		},
		{
			pattern: "abc",
			text:    "ABC",
			want:    false,
		},
		{
			pattern:         "abc",
			text:            "ABC",
			caseInsensitive: true,
			want:            true,
		},

		// multibye charactor
		{
			pattern: "a?b",
			text:    "a☺b",
			want:    true,
		},
		{
			pattern: "a???b",
			text:    "a☺b",
			want:    false,
		},
	}

	for _, tt := range tc {
		got := WildcardMatch(tt.pattern, tt.text, tt.caseInsensitive)
		if got != tt.want {
			t.Errorf("WildcardMatch(%q, %q, %t): want %t, got %t",
				tt.pattern, tt.text, tt.caseInsensitive, tt.want, got)
		}
	}
}

func BenchmarkWildcardMatch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WildcardMatch(
			"*??????????",
			"☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺☺",
			true,
		)
	}
}
