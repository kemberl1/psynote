package anonymizer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// normalizeText performs level-1 preprocessing (docs/04 §3, уровень 1):
// unifies line breaks, collapses non-breaking spaces and zero-width junk, and
// trims trailing whitespace on each line — WITHOUT changing byte offsets in a
// way that would invalidate detection (length-preserving where possible).
//
// We deliberately keep it conservative: only characters that are safe to map
// to a regular space are replaced 1:1, so detector offsets stay aligned.
func normalizeText(s string) string {
	replacer := strings.NewReplacer(
		"\u00A0", " ", // NBSP
		"\u202F", " ", // narrow NBSP
		"\u2007", " ", // figure space
		"\uFEFF", "", // BOM / zero-width no-break space
		"\u200B", "", // zero-width space
		"\r\n", "\n",
		"\r", "\n",
	)
	return replacer.Replace(s)
}

// normalizeWord lower-cases and replaces ё→е for case-insensitive dictionary
// comparison.
func normalizeWord(w string) string {
	w = strings.ToLower(strings.TrimSpace(w))
	w = strings.ReplaceAll(w, "ё", "е")
	return w
}

// isUpperCyrillic reports whether r is an upper-case Cyrillic letter.
func isUpperCyrillic(r rune) bool {
	return unicode.IsUpper(r) && unicode.Is(unicode.Cyrillic, r)
}

// isCyrillicLetter reports whether r is any Cyrillic letter.
func isCyrillicLetter(r rune) bool {
	return unicode.IsLetter(r) && unicode.Is(unicode.Cyrillic, r)
}

// isWordRune reports whether r is a "word" character (letter or digit). Unlike
// Go RE2 `\b`, this works correctly for Cyrillic because unicode.IsLetter
// recognises non-ASCII letters (см. БАГ №1, docs/04 §3 уровень 3).
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// hasWordBoundaries reports whether the half-open byte range [start, end) in
// text is delimited on BOTH sides by a non-word rune (или краем строки). This
// is a manual, Cyrillic-aware replacement for RE2 `\b`, which matches only
// ASCII word boundaries and therefore would treat «нии» inside «улучшении» as a
// standalone token. Offsets are byte offsets (как в spanSet).
func hasWordBoundaries(text string, start, end int) bool {
	if start < 0 || end > len(text) || start > end {
		return false
	}
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(text[:start]); isWordRune(r) {
			return false
		}
	}
	if end < len(text) {
		if r, _ := utf8.DecodeRuneInString(text[end:]); isWordRune(r) {
			return false
		}
	}
	return true
}

// startsUpperCyrillic reports whether a word begins with an upper-case Cyrillic
// letter (a "Capitalised" word — a candidate for a name part).
func startsUpperCyrillic(word string) bool {
	for _, r := range word {
		return isUpperCyrillic(r)
	}
	return false
}

// stemSet matches Russian word forms across cases by shared-prefix heuristics.
// It stores nominative dictionary forms; a candidate matches when it shares a
// long common prefix with some stored form, differing only in case endings.
// This is a lightweight MVP substitute for full pymorphy-style morphology
// (docs/04 §5): сильная подсказка, страхуемая валидатором-гейтом.
type stemSet struct {
	words []string // normalized nominative forms
}

func newStemSet() *stemSet { return &stemSet{} }

func (s *stemSet) addStem(word string) {
	n := normalizeWord(word)
	if n != "" {
		s.words = append(s.words, n)
	}
}

// match reports whether candidate looks like a declension of any stored form.
func (s *stemSet) match(candidate string) bool {
	c := []rune(normalizeWord(candidate))
	if len(c) < 4 {
		return false
	}
	for _, w := range s.words {
		d := []rune(w)
		cp := commonPrefixLen(d, c)
		// Both endings may differ; require a substantial shared prefix so that
		// unrelated words (Иванов vs Ивановский) do not match.
		if cp >= 4 && cp >= len(d)-2 && cp >= len(c)-3 {
			return true
		}
	}
	return false
}

func commonPrefixLen(a, b []rune) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}
