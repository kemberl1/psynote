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
	content = rewriteExam10dHeader(content)
	content = rewriteHeadSignatureCaption(content)
	content = rewriteSignaturePlaceholders(content)
	content = forceCanonicalSignatures(content)
	merged := mergeSubstitutions(doc, subs)
	content = applySubstitutionsMap(content, merged)
	content = tidySignatureCommas(content)
	content = fixObviousTypos(content)
	content = dropEmptySections(content)
	content = normalizeDailySpacing(content)
	content = rewriteDailyForm(content)
	return ensureExam10dCaseNo(content, caseNoFromContent(content, merged))
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
const phDoctorName = "[ФИО_ВРАЧА]"
const phDoctorPos = "[ДОЛЖНОСТЬ_ВРАЧА]"
const phHeadName = "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"
const phHeadPos = "[ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ]"
const phLU = "[ЛУ]"
const exam10dDoctorLine = phDoctorName + ", " + phDoctorPos
const exam10dHeadLine = phHeadName + ", " + phHeadPos + ", " + phLU
const dailyPlaceholderSig = phDoctorPos + " " + phDoctorName

func forceCanonicalSignatures(content string) string {
	daily := isDailyExam(content)
	lines := strings.Split(normalizeNewlines(content), "\n")
	out := make([]string, 0, len(lines)+2)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch line {
		case doctorSignatureCaption:
			i = skipFollowingSignatureValue(lines, i)
			if daily {
				out = append(out, dailyPlaceholderSig)
			} else {
				out = append(out, doctorSignatureCaption, exam10dDoctorLine)
			}
		case headSignatureCaption:
			i = skipFollowingSignatureValue(lines, i)
			out = append(out, headSignatureCaption, exam10dHeadLine)
		default:
			out = append(out, lines[i])
		}
	}
	return strings.Join(out, "\n")
}

func skipFollowingSignatureValue(lines []string, i int) int {
	j := i + 1
	for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
		j++
	}
	if j < len(lines) && isFollowingSignatureValue(lines[j]) {
		return j
	}
	return i
}

func isFollowingSignatureValue(raw string) bool {
	line := strings.TrimSpace(raw)
	if line == "" || line == doctorSignatureCaption || line == headSignatureCaption {
		return false
	}
	if _, _, ok := splitKnownSection(line); ok {
		return false
	}
	upper := strings.ToUpper(line)
	if upper == "ОСМОТР" || strings.EqualFold(line, dailyExamTitle) || strings.HasPrefix(line, "ИБ") {
		return false
	}
	if strings.HasPrefix(line, "Дата:") || isDateTimeLine(line) {
		return false
	}
	return true
}

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
		if !strings.Contains(out, ":") && !strings.Contains(out, "\t") {
			out = strings.TrimSpace(strings.Trim(out, ","))
			out = strings.Join(strings.Fields(out), " ")
		}
		lines[i] = out
	}
	return strings.Join(lines, "\n")
}

const dailyExamTitle = "Осмотр лечащим врачом"
const exam10dCombinedHeader = "ОСМОТР лечащим врачом совместно с заведующим отделением"
const exam10dSplitHeader = "ОСМОТР\nлечащим врачом совместно с заведующим отделением"

func isDailyExam(content string) bool {
	for _, raw := range strings.Split(normalizeNewlines(content), "\n") {
		line := strings.TrimSpace(raw)
		upper := strings.ToUpper(line)
		if upper == "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ" || strings.EqualFold(line, dailyExamTitle) {
			return true
		}
	}
	return false
}

func rewriteExam10dHeader(content string) string {
	if isDailyExam(content) {
		return content
	}
	return strings.ReplaceAll(content, exam10dCombinedHeader, exam10dSplitHeader)
}

var officialDateLineRE = regexp.MustCompile(
	`^«\s*(\d{1,2})\s*»\s+(\p{L}+)\s+(\d{4})\s+г\.(?:\s+время:)?\s+(\d{1,2})\s+час\.\s+(\d{1,2})\s*мин\.?$`,
)

var dailyDatePrefixRE = regexp.MustCompile(`(?i)^дата:\s*`)

func monthFromGenitive(name string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	for i, m := range ruMonthsGenitive {
		if i > 0 && m == n {
			return i
		}
	}
	return 0
}

func formatDailyDateLine(day, month, year, hour, minute int) string {
	return fmt.Sprintf("Дата: %02d.%02d.%04d %02d:%02d", day, month, year, hour, minute)
}

func rewriteDailyDateLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	body := dailyDatePrefixRE.ReplaceAllString(trimmed, "")
	if m := officialDateLineRE.FindStringSubmatch(body); m != nil {
		month := monthFromGenitive(m[2])
		if month == 0 {
			return "", false
		}
		var day, year, hour, minute int
		fmt.Sscanf(m[1], "%d", &day)
		fmt.Sscanf(m[3], "%d", &year)
		fmt.Sscanf(m[4], "%d", &hour)
		fmt.Sscanf(m[5], "%d", &minute)
		return formatDailyDateLine(day, month, year, hour, minute), true
	}
	if m := numericDateTimeRE.FindStringSubmatch(body); m != nil {
		var day, month, year, hour, minute int
		fmt.Sscanf(m[1], "%d", &day)
		fmt.Sscanf(m[2], "%d", &month)
		fmt.Sscanf(m[3], "%d", &year)
		fmt.Sscanf(m[4], "%d", &hour)
		fmt.Sscanf(m[5], "%d", &minute)
		return formatDailyDateLine(day, month, year, hour, minute), true
	}
	if dailyDatePrefixRE.MatchString(trimmed) {
		return trimmed, true
	}
	return "", false
}

func splitPositionAndName(rest string) (pos, name string) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", ""
	}
	parts := strings.Split(rest, ", ")
	if len(parts) >= 2 {
		last := strings.TrimSpace(parts[len(parts)-1])
		lower := strings.ToLower(last)
		if !strings.Contains(lower, "врач") && len(strings.Fields(last)) >= 2 {
			return strings.TrimSpace(strings.Join(parts[:len(parts)-1], ", ")), last
		}
	}
	if strings.Contains(strings.ToLower(rest), "врач") {
		return rest, ""
	}
	return "", rest
}

func rewriteDailyDoctorLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "Лечащий врач") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "Лечащий врач"))
	rest = strings.TrimLeft(rest, ":,—–-")
	rest = strings.TrimSpace(rest)
	real := strings.ReplaceAll(rest, phDoctorPos, "")
	real = strings.ReplaceAll(real, phDoctorName, "")
	for strings.Contains(real, ",,") {
		real = strings.ReplaceAll(real, ",,", ",")
	}
	real = strings.Trim(real, ",; \t")
	pos, name := splitPositionAndName(real)
	if pos == "" {
		pos = phDoctorPos
	}
	if name == "" {
		name = phDoctorName
	}
	return pos + " " + name, true
}

// rewriteDailyForm приводит ежедневный осмотр к бланку МИС с фото:
// «Осмотр лечащим врачом», «Дата: ДД.ММ.ГГГГ ЧЧ:ММ», подпись должность + ФИО.
func rewriteDailyForm(content string) string {
	if !isDailyExam(content) {
		return content
	}
	var out []string
	for _, raw := range strings.Split(normalizeNewlines(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(line, "ИБ №") || strings.HasPrefix(upper, "ИБ N") {
			continue
		}
		if upper == "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ" || strings.EqualFold(line, dailyExamTitle) {
			out = append(out, dailyExamTitle)
			continue
		}
		if line == doctorSignatureCaption {
			continue
		}
		if rewritten, ok := rewriteDailyDateLine(line); ok {
			out = append(out, rewritten)
			continue
		}
		if rewritten, ok := rewriteDailyDoctorLine(line); ok {
			out = append(out, rewritten)
			continue
		}
		out = append(out, line)
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

func formatCaseNo(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "ИБ №")
	s = strings.TrimPrefix(s, "ИБ N")
	s = strings.TrimSpace(s)
	if s == "" || hasUnresolvedPlaceholder(s) {
		return "ИБ №"
	}
	return "ИБ №" + s
}

func isCaseNoLine(line string) bool {
	s := strings.TrimSpace(line)
	upper := strings.ToUpper(s)
	return strings.HasPrefix(s, "ИБ №") || strings.HasPrefix(upper, "ИБ N")
}

func caseNoFromContent(content string, subs map[string]string) string {
	if subs != nil {
		if v := formatCaseNo(subs["[НОМЕР_ИБ]"]); v != "ИБ №" {
			return v
		}
	}
	for _, raw := range strings.Split(normalizeNewlines(content), "\n") {
		if isCaseNoLine(raw) && !hasUnresolvedPlaceholder(raw) {
			return formatCaseNo(raw)
		}
	}
	return "ИБ №"
}

// ensureExam10dCaseNo держит ИБ только в шапке текста, без [НОМЕР_ИБ] и без хвоста под осмотром.
func ensureExam10dCaseNo(content, ib string) string {
	if isDailyExam(content) {
		return content
	}
	if !strings.Contains(content, "заведующим") && !strings.Contains(strings.ToUpper(content), "ОСМОТР") {
		return content
	}
	lines := strings.Split(normalizeNewlines(content), "\n")
	out := make([]string, 0, len(lines)+1)
	saw := false
	for _, raw := range lines {
		if isCaseNoLine(raw) {
			if !saw {
				out = append(out, ib)
				saw = true
			}
			continue
		}
		out = append(out, raw)
	}
	if !saw {
		out = append([]string{ib}, out...)
	}
	return strings.Join(out, "\n")
}

// normalizeDailySpacing убирает пустые строки между разделами ежедневника.
func normalizeDailySpacing(content string) string {
	if !isDailyExam(content) {
		return content
	}
	var out []string
	for _, raw := range strings.Split(normalizeNewlines(content), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		out = append(out, raw)
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
