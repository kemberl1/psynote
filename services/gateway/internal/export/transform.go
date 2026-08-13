// Transform generated diary text before Word/PDF/TXT render.
//
// Preview in UI and the downloaded file must be the same document
// (docs/07 §7, docs/08 §5.3): template headers + named sections.
// Empty filler sections are dropped per generation rules (docs/03,
// rag/generation.py): no «Жалобы не предъявляет», «Без дополнений»,
// «Без изменений» without a real finding — and never a value without
// its section title.
package export

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// skipExactValues — whole-section bodies that mean «nothing to write».
var skipExactValues = map[string]bool{
	"данных нет":                 true,
	"без дополнений":             true,
	"не предъявляет":             true,
	"жалоб не предъявляет":       true,
	"нет":                        true,
	"-":                          true,
	"—":                          true,
	"без изменений":              true,
	"без отрицательной динамики": true,
	"без особенностей":           true,
}

// mergeSubstitutions returns client substitutions with server defaults from metadata.
func mergeSubstitutions(doc Document, client map[string]string) map[string]string {
	out := make(map[string]string)
	d := doc.GeneratedAt
	if d.IsZero() {
		d = time.Now()
	}
	out["[ДАТА]"] = d.Format("02.01.2006")
	out["[ВРЕМЯ]"] = fmt.Sprintf("%02d:%02d", d.Hour(), d.Minute())
	for k, v := range client {
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

func applySubstitutionsMap(content string, subs map[string]string) string {
	if len(subs) == 0 {
		return content
	}
	pairs := make([]string, 0, len(subs)*2)
	for k, v := range subs {
		if k != "" {
			pairs = append(pairs, k, v)
		}
	}
	if len(pairs) == 0 {
		return content
	}
	return strings.NewReplacer(pairs...).Replace(content)
}

// transformContent keeps the template layout the UI shows, fills
// placeholders, and drops empty filler sections (docs/03 clinical rules).
func transformContent(doc Document, subs map[string]string) string {
	merged := mergeSubstitutions(doc, subs)
	content := applySubstitutionsMap(doc.Content, merged)
	return dropEmptySections(content)
}

func normalizeSkipValue(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.Map(func(r rune) rune {
		switch r {
		case '.', ',', ';', ':', '!', '?', '"', '\'', '«', '»', '(', ')':
			return -1
		case '—', '–':
			return '-'
		default:
			if unicode.IsSpace(r) {
				return ' '
			}
			return r
		}
	}, v)
	return strings.Join(strings.Fields(v), " ")
}

func isSkipSectionValue(v string) bool {
	v = normalizeSkipValue(v)
	if v == "" {
		return true
	}
	return skipExactValues[v]
}

func isDiagnosisKeepLabel(label string) bool {
	l := strings.ToLower(strings.TrimSpace(label))
	return l == "диагноз" ||
		strings.Contains(l, "заболеван") ||
		l == "синдром" ||
		strings.Contains(l, "осложнен")
}

func shouldDropSection(label, value string) bool {
	l := strings.ToLower(strings.TrimSpace(label))
	v := strings.ToLower(strings.TrimSpace(value))
	norm := normalizeSkipValue(value)

	// «Диагноз:» is a structural heading; keep even if the code is on the next line.
	if l == "диагноз" && norm == "" {
		return false
	}
	if isSkipSectionValue(value) {
		if isDiagnosisKeepLabel(label) && (norm == "не выявлено" || norm == "-") {
			return false
		}
		if l == "диагноз" {
			return false
		}
		return true
	}

	if strings.Contains(l, "жалоб") &&
		(strings.Contains(v, "не предъявля") || strings.Contains(v, "жалоб нет")) {
		return true
	}
	if strings.Contains(l, "анамнез") && strings.Contains(v, "без дополнен") {
		return true
	}
	if strings.Contains(l, "план обследования") &&
		(strings.Contains(v, "без изменен") || strings.Contains(v, "без дополнен")) {
		return true
	}
	if strings.Contains(l, "физикальное") &&
		(strings.Contains(v, "без изменен") || strings.Contains(v, "без отрицательной динамики")) {
		return true
	}
	if strings.Contains(l, "соматический") &&
		(strings.Contains(v, "без изменен") || strings.Contains(v, "без особенностей")) {
		return true
	}
	if strings.Contains(l, "вмешательства") &&
		strings.Contains(v, "осмотр лечащим врачом") && len([]rune(norm)) < 40 {
		return true
	}
	return false
}

func splitKnownSection(line string) (label, value string, ok bool) {
	label, value, ok = splitLabelValue(line)
	if !ok || !isKnownSectionLabel(label) {
		return "", "", false
	}
	return label, value, true
}

func isOrphanSkipLine(line string) bool {
	if _, _, ok := splitKnownSection(line); ok {
		return false
	}
	return isSkipSectionValue(line)
}

func filterSectionBody(body []string) []string {
	var out []string
	for _, raw := range body {
		trim := strings.TrimSpace(raw)
		if trim == "" {
			if len(out) > 0 {
				out = append(out, "")
			}
			continue
		}
		if isOrphanSkipLine(trim) {
			continue
		}
		out = append(out, raw)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func combinedSectionValue(rest string, body []string) string {
	parts := make([]string, 0, 1+len(body))
	if strings.TrimSpace(rest) != "" {
		parts = append(parts, strings.TrimSpace(rest))
	}
	for _, l := range body {
		if t := strings.TrimSpace(l); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

func dropEmptySections(content string) string {
	var out []string
	var (
		hasSec   bool
		secLabel string
		secHead  string
		secRest  string
		secBody  []string
	)

	flush := func() {
		if !hasSec {
			return
		}
		body := filterSectionBody(secBody)
		value := combinedSectionValue(secRest, body)
		if shouldDropSection(secLabel, value) {
			hasSec = false
			secBody = nil
			return
		}
		out = append(out, secHead)
		out = append(out, body...)
		hasSec = false
		secBody = nil
	}

	for _, raw := range strings.Split(normalizeNewlines(content), "\n") {
		line := normalizeGeneratedLine(raw)
		if label, rest, ok := splitKnownSection(line); ok {
			flush()
			hasSec = true
			secLabel = label
			secRest = rest
			secHead = line
			secBody = nil
			continue
		}
		if hasSec {
			secBody = append(secBody, line)
			continue
		}
		if line == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if isOrphanSkipLine(line) {
			continue
		}
		out = append(out, line)
	}
	flush()

	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
