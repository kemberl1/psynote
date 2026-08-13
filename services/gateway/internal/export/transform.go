// Transform generated diary text before Word/TXT render.
//
// Preview in UI and the downloaded file must be the same document
// (docs/07 §7, docs/08 §5.3): MIS exam template headers + named sections.
// Official defaults («не предъявляет», «без дополнений») stay in the form.
package export

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const diaryExamHour = 10
const diaryExamMinute = 0

var ruMonthsGenitive = [...]string{
	"", "января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

var titleDateRE = regexp.MustCompile(`(\d{2})\.(\d{2})\.(\d{4})`)

// numericDateTimeRE matches «13.08.2026 время: 10:45» (old generated header).
var numericDateTimeRE = regexp.MustCompile(
	`^(\d{2})\.(\d{2})\.(\d{4})\s+время:\s+(\d{1,2}):(\d{2})\s*$`,
)

// skipExactValues — значения, которых нет в бланке МИС (мусор генерации).
var skipExactValues = map[string]bool{
	"данных нет": true,
}

func answerString(answers map[string]any, key string) string {
	if answers == nil {
		return ""
	}
	raw, ok := answers[key]
	if !ok || raw == nil {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func officialDate(t time.Time) string {
	return fmt.Sprintf("«%02d» %s %d г.", t.Day(), ruMonthsGenitive[t.Month()], t.Year())
}

func officialTime(hour, minute int) string {
	return fmt.Sprintf("%02d час. %02d мин.", hour, minute)
}

func parseDMY(s string) (time.Time, bool) {
	m := titleDateRE.FindStringSubmatch(s)
	if len(m) < 4 {
		return time.Time{}, false
	}
	t, err := time.Parse("02.01.2006", m[1]+"."+m[2]+"."+m[3])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// DiaryStamp is the header date/time for a diary: examination day, 10 час. 00 мин.
func DiaryStamp(title string, answers map[string]any, generatedAt time.Time) (date string, clock string) {
	clock = officialTime(diaryExamHour, diaryExamMinute)
	if iso := answerString(answers, "diary_date"); iso != "" {
		if t, err := time.Parse("2006-01-02", iso); err == nil {
			return officialDate(t), clock
		}
		if t, ok := parseDMY(iso); ok {
			return officialDate(t), clock
		}
	}
	if t, ok := parseDMY(title); ok {
		return officialDate(t), clock
	}
	d := generatedAt
	if d.IsZero() {
		d = time.Now()
	}
	return officialDate(d), clock
}

// mergeSubstitutions fills [ДАТА]/[ВРЕМЯ] from the examination day (not generate-now)
// and applies other client placeholders (doctor name, case number).
func mergeSubstitutions(doc Document, client map[string]string) map[string]string {
	out := make(map[string]string)
	date, clock := DiaryStamp(doc.Title, doc.Answers, doc.GeneratedAt)
	out["[ДАТА]"] = date
	out["[ВРЕМЯ]"] = clock
	for k, v := range client {
		if k == "" || v == "" || k == "[ДАТА]" || k == "[ВРЕМЯ]" {
			continue
		}
		out[k] = v
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
// placeholders, and rewrites a numeric date header into the MIS form.
func transformContent(doc Document, subs map[string]string) string {
	content := rewriteNumericDateHeaders(doc.Content)
	content = rewriteHeadSignatureCaption(content)
	content = rewriteSignaturePlaceholders(content)
	merged := mergeSubstitutions(doc, subs)
	content = applySubstitutionsMap(content, merged)
	content = strings.ReplaceAll(content, "[ДОЛЖНОСТЬ_ВРАЧА]", "")
	content = tidySignatureCommas(content)
	content = fixObviousTypos(content)
	content = dropEmptySections(content)
	return normalizeDailySpacing(content)
}

const doctorSignatureCaption = "Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись"
const headSignatureCaption = "Фамилия, имя, отчество (при наличии) заведующего отделением, подпись"

func rewriteHeadSignatureCaption(content string) string {
	old := doctorSignatureCaption + "\n[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"
	neu := headSignatureCaption + "\n[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"
	return strings.ReplaceAll(content, old, neu)
}

var dailyDoctorFooterRE = regexp.MustCompile(`(?m)^Лечащий врач:?\s*\[ФИО_ВРАЧА\]\s*$`)

const dailyDoctorLine = "Лечащий врач, [ДОЛЖНОСТЬ_ВРАЧА], [ФИО_ВРАЧА]"

func rewriteSignaturePlaceholders(content string) string {
	content = normalizeNewlines(content)
	replacement := "[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]"
	if isDailyExam(content) {
		replacement = dailyDoctorLine
	}
	content = dailyDoctorFooterRE.ReplaceAllString(content, replacement)
	doctorWithPos := doctorSignatureCaption + "\n[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]"
	doctorOnly := doctorSignatureCaption + "\n[ФИО_ВРАЧА]"
	if !strings.Contains(content, "[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]") {
		content = strings.ReplaceAll(content, doctorOnly, doctorWithPos)
	}
	return content
}

var emptyCommaRE = regexp.MustCompile(`,\s*,`)

func tidySignatureCommas(content string) string {
	lines := strings.Split(normalizeNewlines(content), "\n")
	for i, line := range lines {
		out := line
		for emptyCommaRE.MatchString(out) {
			out = emptyCommaRE.ReplaceAllString(out, ",")
		}
		if !strings.Contains(out, ":") {
			out = strings.TrimSpace(strings.Trim(out, ","))
			out = strings.Join(strings.Fields(out), " ")
		}
		lines[i] = out
	}
	return strings.Join(lines, "\n")
}

func isDailyExam(content string) bool {
	return strings.Contains(strings.ToUpper(content), "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ")
}

func nextNonEmptyLine(lines []string, from int) string {
	for i := from; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

func isDailyBlankAfterLabel(label string) bool {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "анамнез жизни (дополнения к анамнезу)", "неврологический статус":
		return true
	default:
		return false
	}
}

// normalizeDailySpacing inserts the two MIS blank lines in the daily exam
// and drops extra empty lines between other sections. exam_10d is unchanged.
func normalizeDailySpacing(content string) string {
	if !isDailyExam(content) {
		return content
	}
	lines := strings.Split(normalizeNewlines(content), "\n")
	compacted := make([]string, 0, len(lines))
	for i, raw := range lines {
		if strings.TrimSpace(raw) != "" {
			compacted = append(compacted, raw)
			continue
		}
		prev := ""
		if n := len(compacted); n > 0 {
			prev = compacted[n-1]
		}
		next := nextNonEmptyLine(lines, i+1)
		if prev != "" && next != "" {
			if _, _, prevSec := splitKnownSection(prev); prevSec {
				if _, _, nextSec := splitKnownSection(next); nextSec {
					continue
				}
			}
		}
		if len(compacted) > 0 && compacted[len(compacted)-1] == "" {
			continue
		}
		compacted = append(compacted, "")
	}

	out := make([]string, 0, len(compacted)+2)
	for i, line := range compacted {
		out = append(out, line)
		label, _, ok := splitKnownSection(line)
		if !ok || !isDailyBlankAfterLabel(label) {
			continue
		}
		if i+1 < len(compacted) && strings.TrimSpace(compacted[i+1]) == "" {
			continue
		}
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

func rewriteNumericDateHeaders(content string) string {
	lines := strings.Split(normalizeNewlines(content), "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(strings.ReplaceAll(raw, "**", ""))
		m := numericDateTimeRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		t, err := time.Parse("02.01.2006", m[1]+"."+m[2]+"."+m[3])
		if err != nil {
			continue
		}
		hour, minute := diaryExamHour, diaryExamMinute
		fmt.Sscanf(m[4], "%d", &hour)
		fmt.Sscanf(m[5], "%d", &minute)
		lines[i] = officialDate(t) + " время: " + officialTime(hour, minute)
	}
	return strings.Join(lines, "\n")
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

func shouldDropSection(label, value string) bool {
	// Бланк МИС держит все строки, включая «не предъявляет» / «без дополнений».
	// Вырезаем только мусор генерации («данных нет»).
	if normalizeSkipValue(value) == "данных нет" {
		return true
	}
	_ = label
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
