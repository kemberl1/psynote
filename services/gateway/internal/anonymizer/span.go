package anonymizer

import "sort"

// span is a half-open byte range [Start, End) in the original text that a
// detector has flagged. Byte offsets (not rune indices) are used so that
// regexp.FindAllStringIndex results can be stored directly.
type span struct {
	Start int
	End   int
	Type  EntityType
	// protected marks whitelist regions (МКБ, препараты) that must NOT be
	// replaced. They participate in overlap resolution to shield clinical text.
	protected bool
}

func (s span) len() int { return s.End - s.Start }

// overlaps reports whether two spans share at least one byte.
func (a span) overlaps(b span) bool {
	return a.Start < b.End && b.Start < a.End
}

// spanSet accumulates detector hits and resolves them into a clean,
// non-overlapping replacement plan.
type spanSet struct {
	spans []span
}

func (ss *spanSet) add(start, end int, t EntityType) {
	if end > start {
		ss.spans = append(ss.spans, span{Start: start, End: end, Type: t})
	}
}

// addProtected registers a whitelist region (level 6 docs/04). Protected spans
// suppress any PII span that would otherwise overlap them.
func (ss *spanSet) addProtected(start, end int) {
	if end > start {
		ss.spans = append(ss.spans, span{Start: start, End: end, protected: true})
	}
}

// resolve produces a sorted, non-overlapping list of spans to replace.
// Conflict rules (fail-closed bias, docs/04):
//   - a protected (whitelist) span always wins over an overlapping PII span;
//   - between two PII spans, the longer span wins; ties break on higher
//     EntityType.priority(); remaining ties keep the earlier start.
//
// The returned slice excludes protected spans (they are kept verbatim).
func (ss *spanSet) resolve() []span {
	if len(ss.spans) == 0 {
		return nil
	}

	sorted := make([]span, len(ss.spans))
	copy(sorted, ss.spans)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.len() != b.len() {
			return a.len() > b.len() // longer first
		}
		return a.Type.priority() > b.Type.priority()
	})

	var kept []span
	for _, cand := range sorted {
		conflict := false
		for i := range kept {
			if !cand.overlaps(kept[i]) {
				continue
			}
			// Protected always shields; if either side is protected the PII
			// candidate is dropped and the protected region is preserved.
			if kept[i].protected || cand.protected {
				if cand.protected && !kept[i].protected {
					// Replace a previously kept PII span with the protection.
					kept[i] = cand
				}
				conflict = true
				break
			}
			conflict = true // first (longer/higher-priority) span already won
			break
		}
		if !conflict {
			kept = append(kept, cand)
		}
	}

	// Drop protected spans from the replacement plan and re-sort by position.
	out := kept[:0]
	for _, s := range kept {
		if !s.protected {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// apply rewrites text by substituting each resolved span with its typed
// placeholder. It returns the new text and a per-category count of replacements
// (значения НЕ возвращаются и НЕ логируются — docs/04 §7).
func apply(text string, spans []span) (string, map[EntityType]int) {
	counts := make(map[EntityType]int)
	if len(spans) == 0 {
		return text, counts
	}

	var b []byte
	prev := 0
	for _, s := range spans {
		if s.Start < prev { // safety: skip any residual overlap
			continue
		}
		b = append(b, text[prev:s.Start]...)
		b = append(b, s.Type.Placeholder()...)
		counts[s.Type]++
		prev = s.End
	}
	b = append(b, text[prev:]...)
	return string(b), counts
}
