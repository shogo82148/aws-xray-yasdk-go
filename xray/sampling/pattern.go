package sampling

import (
	"strings"
	"unicode/utf8"
)

// WildcardMatchCaseInsensitive returns whether the text matchers the pattern.
func WildcardMatchCaseInsensitive(pattern, text string) bool {
	return WildcardMatch(pattern, text, true)
}

// WildcardMatch returns whether the text matchers the pattern.
func WildcardMatch(pattern, text string, caseInsensitive bool) bool {
	// fast path
	if pattern == "" {
		return text == ""
	}
	if pattern == "*" {
		return true
	}

	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		text = strings.ToLower(text)
	}

Pattern:
	for len(pattern) > 0 {
		var star bool
		var chunk string
		star, chunk, pattern = scanChunk(pattern)
		if star && chunk == "" {
			// Trailing * matches rest of string
			return true
		}

		// Look for match at current position.
		t, ok := matchChunk(chunk, text)

		if ok && (len(t) == 0 || len(pattern) > 0) {
			text = t
			continue
		}
		if star {
			// Look for match skipping i+1 bytes.
			for i := 0; i < len(text); i++ {
				t, ok := matchChunk(chunk, text[i+1:])
				if ok {
					// if we're the last chunk, make sure we exhausted the text
					if len(pattern) == 0 && len(t) > 0 {
						continue
					}
					text = t
					continue Pattern
				}
			}
		}
		return false
	}
	return len(text) == 0
}

func scanChunk(pattern string) (star bool, chunk string, rest string) {
	for len(pattern) > 0 && pattern[0] == '*' {
		pattern = pattern[1:]
		star = true
	}
	i := strings.IndexByte(pattern, '*')
	if i < 0 {
		i = len(pattern)
	}
	return star, pattern[0:i], pattern[i:]
}

func matchChunk(chunk, s string) (rest string, ok bool) {
	for len(chunk) > 0 {
		if len(s) == 0 {
			return
		}
		switch chunk[0] {
		case '?':
			_, n := utf8.DecodeRuneInString(s)
			s = s[n:]
			chunk = chunk[1:]
		default:
			if chunk[0] != s[0] {
				return
			}
			s = s[1:]
			chunk = chunk[1:]
		}
	}
	return s, true
}
