// Transform LLM template output into corpus сборник layout before rendering.
//
// Daily diaries: compact narrative (DD.MM.YYYY + prose), not the full ОСМОТР
// template. Ten-day exams: centred ОСМОТР header + structured sections.
package export

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	placeholderRE = regexp.MustCompile(`\[[А-ЯA-Z_]+\]`)
	headerLineRE  = regexp.MustCompile(`(?i)^(иб\s*№|осмотр)`)
)

// section is one «Метка: значение» block from the generated template.
type section struct {
	label string
	value string
}

// skipSectionValues — empty/trivial section bodies dropped in compact export.
var skipSectionValues = map[string]bool{
	"данных нет":      true,
	"без дополнений":  true,
	"не предъявляет":  true,
	"не предъявляет.": true,
	"нет":             true,
	"-":               true,
	"—":               true,
}

// dailyNarrativeKeys — sections merged into the opening date-prefixed paragraph.
var dailyNarrativeKeys = []string{
	"Психический статус",
	"Жалобы",
	"Анамнез заболевания (дополнения к анамнезу)",
	"Анамнез жизни (дополнения к анамнезу)",
	"Соматический статус",
}

// dailyInlineKeys — physical/neuro sections kept as «Метка: значение» lines.
var dailyInlineKeys = []struct {
	src  string
	dest string
}{
	{"Физикальное исследование, локальный статус (его изменение)",
		"Физикальное исследование, локальный статус (его изменение)"},
	{"Неврологический статус", "Неврологический статус (его изменение)"},
}

// examSectionOrder — body section order for 10-day exams (corpus reference).
var examSectionOrder = []string{
	"Жалобы",
	"Анамнез заболевания (дополнения к анамнезу)",
	"Анамнез жизни (дополнения к анамнезу)",
	"Физикальное исследование, локальный статус (его изменение)",
	"Неврологический статус",
	"Психический статус",
	"Диагноз",
	"Основное заболевание",
	"Сопутствующие заболевания",
	"Назначения",
	"Выполнены медицинские вмешательства",
	"План обследования (дополнения к плану)",
	"План лечения (дополнения к плану)",
	"Этапный эпикриз",
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
		if k == "" {
			continue
		}
		pairs = append(pairs, k, v)
	}
	if len(pairs) == 0 {
		return content
	}
	return strings.NewReplacer(pairs...).Replace(content)
}

// transformContent reshapes generated template text into corpus сборник layout.
func transformContent(doc Document, subs map[string]string) string {
	merged := mergeSubstitutions(doc, subs)
	content := applySubstitutionsMap(doc.Content, merged)

	switch doc.DocumentTypeCode {
	case "daily":
		return transformDaily(content, doc.GeneratedAt, merged)
	case "exam_10d":
		return transformExam10d(content, doc.GeneratedAt, merged)
	default:
		return content
	}
}

func isSkipSectionValue(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return true
	}
	return skipSectionValues[v]
}

func isTemplateBoilerplate(line string) bool {
	trim := strings.TrimSpace(line)
	if trim == "" {
		return true
	}
	upper := strings.ToUpper(trim)
	if headerLineRE.MatchString(trim) {
		return true
	}
	if strings.HasPrefix(upper, "ОСМОТР") {
		return true
	}
	if isDateTimeLine(trim) {
		return true
	}
	if strings.HasPrefix(trim, "Фамилия, имя, отчество") {
		return true
	}
	if strings.HasPrefix(trim, "Лечащий врач:") || strings.HasPrefix(trim, "Заведующий отделением:") {
		return true
	}
	return false
}

func isAlreadyCompactDaily(content string) bool {
	for _, raw := range strings.Split(normalizeNewlines(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if datePrefixedRE.MatchString(line) {
			return true
		}
		if strings.HasPrefix(strings.ToUpper(line), "ОСМОТР") {
			return false
		}
	}
	return false
}

func parseSections(content string) []section {
	var out []section
	for _, raw := range strings.Split(normalizeNewlines(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isTemplateBoilerplate(line) {
			continue
		}
		if label, value, ok := splitLabelValue(line); ok {
			out = append(out, section{label: label, value: value})
			continue
		}
		if len(out) > 0 {
			last := &out[len(out)-1]
			if last.value != "" {
				last.value += " "
			}
			last.value += line
		}
	}
	return out
}

func sectionsMap(sections []section) map[string]string {
	m := make(map[string]string, len(sections))
	for _, s := range sections {
		key := strings.ToLower(strings.TrimSpace(s.label))
		if existing, ok := m[key]; ok && existing != "" {
			m[key] = existing + " " + s.value
		} else {
			m[key] = s.value
		}
	}
	return m
}

func sectionValue(m map[string]string, label string) (string, bool) {
	v, ok := m[strings.ToLower(label)]
	return v, ok
}

func transformDaily(content string, date time.Time, subs map[string]string) string {
	if isAlreadyCompactDaily(content) {
		return stripRemainingPlaceholders(content, subs)
	}

	secs := sectionsMap(parseSections(content))
	d := date
	if d.IsZero() {
		d = time.Now()
	}
	dateStr := d.Format("02.01.2006")
	if v := subs["[ДАТА]"]; v != "" && !strings.Contains(v, "[") {
		dateStr = v
	}

	var narrative []string
	for _, key := range dailyNarrativeKeys {
		if v, ok := sectionValue(secs, key); ok && !isSkipSectionValue(v) {
			narrative = append(narrative, v)
		}
	}
	if len(narrative) == 0 {
		// Fallback: first non-empty section with clinical content.
		for _, s := range parseSections(content) {
			if !isSkipSectionValue(s.value) && !isDiagnosisSection(s.label) {
				narrative = append(narrative, s.value)
				break
			}
		}
	}

	var lines []string
	if len(narrative) > 0 {
		lines = append(lines, dateStr+" "+strings.Join(narrative, " "))
	}

	for _, pair := range dailyInlineKeys {
		if v, ok := sectionValue(secs, pair.src); ok && !isSkipSectionValue(v) {
			lines = append(lines, pair.dest+": "+v)
		}
	}

	for _, key := range []string{
		"Назначения",
		"План лечения (дополнения к плану)",
		"План обследования (дополнения к плану)",
	} {
		if v, ok := sectionValue(secs, key); ok && !isSkipSectionValue(v) {
			lines = append(lines, v)
		}
	}

	if v, ok := sectionValue(secs, "Выполнены медицинские вмешательства"); ok && !isSkipSectionValue(v) {
		lines = append(lines, splitConsultationLines(v)...)
	}

	if sig := doctorSignatureLine(subs, secs); sig != "" {
		lines = append(lines, sig)
	}

	if len(lines) == 0 {
		return stripRemainingPlaceholders(content, subs)
	}
	return strings.Join(lines, "\n")
}

func transformExam10d(content string, date time.Time, subs map[string]string) string {
	secs := sectionsMap(parseSections(content))
	d := date
	if d.IsZero() {
		d = time.Now()
	}

	var lines []string
	if caseNo := subs["[НОМЕР_ИБ]"]; caseNo != "" && !strings.Contains(caseNo, "[") {
		lines = append(lines, "ИБ №"+caseNo)
	}
	lines = append(lines, "ОСМОТР")
	lines = append(lines, "лечащим врачом совместно с заведующим отделением")
	lines = append(lines, formatExamDateTime(d, subs))

	for _, label := range examSectionOrder {
		v, ok := sectionValue(secs, label)
		if !ok {
			if label == "Диагноз" {
				lines = append(lines, "Диагноз:")
			}
			continue
		}
		if isSkipSectionValue(v) {
			if label == "Диагноз" {
				lines = append(lines, "Диагноз:")
			}
			continue
		}
		if isConsultBlock(label, v) {
			lines = append(lines, splitConsultationLines(v)...)
			continue
		}
		lines = append(lines, label+": "+v)
	}

	if sig := doctorSignatureLine(subs, secs); sig != "" {
		lines = append(lines, sig)
	}
	head := subs["[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"]
	if head != "" && !strings.Contains(head, "[") {
		lines = append(lines, "Заведующий отделением: "+head)
	}

	return strings.Join(lines, "\n")
}

func isDiagnosisSection(label string) bool {
	l := strings.ToLower(label)
	return strings.Contains(l, "диагноз") ||
		strings.Contains(l, "заболеван") ||
		strings.Contains(l, "осложнен")
}

func isConsultBlock(label, value string) bool {
	if label == "Выполнены медицинские вмешательства" {
		return consultNoteRE.MatchString(value)
	}
	return false
}

func splitConsultationLines(v string) []string {
	re := regexp.MustCompile(`([А-ЯA-Za-z][А-ЯA-Za-z\-]+ от \d{2}\.\d{2}\.\d{2,4}:)`)
	locs := re.FindAllStringIndex(v, -1)
	if len(locs) == 0 {
		return []string{strings.TrimSpace(v)}
	}
	var out []string
	for i, loc := range locs {
		end := len(v)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		chunk := strings.TrimSpace(v[loc[0]:end])
		if chunk != "" {
			out = append(out, chunk)
		}
	}
	return out
}

func doctorSignatureLine(subs map[string]string, secs map[string]string) string {
	doctor := subs["[ФИО_ВРАЧА]"]
	if doctor == "" || strings.Contains(doctor, "[") {
		if v, ok := sectionValue(secs, "Лечащий врач"); ok {
			doctor = strings.TrimSpace(strings.TrimPrefix(v, ":"))
		}
	}
	if doctor == "" || strings.Contains(doctor, "[") {
		return ""
	}
	return formatDoctorSignature(doctor)
}

func formatDoctorSignature(name string) string {
	const pad = 69
	role := "Врач-психиатр"
	spaces := pad - len([]rune(role))
	if spaces < 1 {
		spaces = 1
	}
	return role + strings.Repeat(" ", spaces) + name
}

var russianMonths = [...]string{
	"", "января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

func formatExamDateTime(d time.Time, subs map[string]string) string {
	if d.IsZero() {
		d = time.Now()
	}
	day := d.Day()
	month := russianMonths[d.Month()]
	year := d.Year()
	h, m := d.Hour(), d.Minute()
	if v := subs["[ДАТА]"]; v != "" && !strings.Contains(v, "[") {
		// If client passed a full Russian date line, use time from metadata.
		if strings.Contains(v, "сентября") || strings.Contains(v, "«") {
			return v
		}
	}
	return fmt.Sprintf("«%02d » %s %d г.  время: %d час. %02d мин.", day, month, year, h, m)
}

func stripRemainingPlaceholders(content string, subs map[string]string) string {
	out := content
	for _, ph := range []string{"[ДАТА]", "[ВРЕМЯ]", "[НОМЕР_ИБ]", "[ФИО_ВРАЧА]", "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]", "[ОСНОВНОЙ_ДИАГНОЗ]"} {
		if v := subs[ph]; v != "" {
			out = strings.ReplaceAll(out, ph, v)
		}
	}
	// Drop lines that still contain unfilled placeholders.
	var kept []string
	for _, raw := range strings.Split(normalizeNewlines(out), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if placeholderRE.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
