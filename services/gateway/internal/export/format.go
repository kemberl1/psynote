// Structural parser for the diary body, used by the DOCX renderer.
//
// Бланки МИС с фото отделения:
//   - ежедневный: жирный только «Осмотр лечащим врачом», без подчёркиваний и пустых строк;
//   - за 10 дней: ИБ в шапке справа и внизу слева, «ОСМОТР» по центру;
//   - Arial 11pt, выравнивание влево.
package export

import (
	"regexp"
	"strings"
)

// consultNoteRE matches specialist consultation lines from the corpus сборник.
var consultNoteRE = regexp.MustCompile(` от \d{2}\.\d{2}\.\d{2,4}:`)

// datePrefixedRE matches diary narrative lines starting with a date
// (including multi-day ranges like 23-25.12.2025).
var datePrefixedRE = regexp.MustCompile(`^\d{2}(?:-\d{2})?\.\d{2}\.\d{4}`)

// dateNarrativeRE captures date + rest of daily narrative.
var dateNarrativeRE = regexp.MustCompile(`^(\d{2}(?:-\d{2})?\.\d{2}\.\d{4})\s+(.*)$`)

// lineKind classifies one logical line of the diary for formatting.
type lineKind int

const (
	kindPlain            lineKind = iota // обычный абзац, слева
	kindDailyNarrative                   // DD.MM.YYYY + narrative (дата жирным)
	kindCaseNo                           // «ИБ №…» → по правому краю
	kindTitle                            // legacy title
	kindDailyTitle                       // «Осмотр лечащим врачом» → слева, жирным
	kindDailyDate                        // «Дата: ДД.ММ.ГГГГ ЧЧ:ММ» → слева, обычным
	kindDailySignature                   // должность и ФИО рядом, подчёркнуты вместе
	kindExamTitle                        // «ОСМОТР» → по центру, жирным; ИБ в right
	kindExamSubtitle                     // подзаголовок осмотра → по центру
	kindDateTime                         // официальная дата 10-дневного осмотра → слева
	kindSignatureCaption                 // «Фамилия, имя, отчество …»
	kindSignatureValue                   // ФИО под подписью
	kindLabelValue                       // «Метка: значение» → жирная метка
	kindConsultNote                      // консультации → слева
	kindDoctorSignature                  // подпись врача (legacy)
	kindBlank                            // пустая строка бланка
)

// textSpan is one formatted run inside a paragraph.
type textSpan struct {
	Text      string
	Bold      bool
	Underline bool
}

// docLine is one formatted line of the document.
//
//   - kindLabelValue uses label (без двоеточия) + value;
//   - spans, when non-nil, drive mixed bold/underline rendering;
//   - иначе используется text.
type docLine struct {
	kind  lineKind
	label string
	value string
	text  string
	right string // ФИО в ежедневной подписи
	spans []textSpan
	quiet bool // ежедневный осмотр: без жирного и без лишних отступов
}

// knownSectionLabels — метки секций реальных шаблонов.
var knownSectionLabels = map[string]bool{
	"жалобы": true,
	"анамнез заболевания (дополнения к анамнезу)":                true,
	"анамнез жизни (дополнения к анамнезу)":                      true,
	"физикальное исследование":                                   true,
	"физикальное исследование, локальный статус (его изменение)": true,
	"психический статус":                                         true,
	"психический статус (его изменение)":                         true,
	"соматический статус":                                        true,
	"неврологический статус":                                     true,
	"неврологический статус (его изменение)":                     true,
	"диагноз": true,
	"основное заболевание":             true,
	"осложнение основного заболевания": true,
	"сопутствующие заболевания":        true,
	"синдром": true,
	"дополнительные сведения":                                    true,
	"дополнительные сведения о заболевании":                      true,
	"обоснование диагноза (при наличии дополнительных сведений)": true,
	"назначения": true,
	"выполнены медицинские вмешательства":    true,
	"план обследования (дополнения к плану)": true,
	"план лечения (дополнения к плану)":      true,
	"этапный эпикриз":                        true,
	"лечащий врач":                           true,
	"заведующий отделением":                  true,
}

func isKnownSectionLabel(label string) bool {
	return knownSectionLabels[strings.ToLower(strings.TrimSpace(label))]
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func normalizeGeneratedLine(line string) string {
	return strings.TrimSpace(strings.ReplaceAll(line, "**", ""))
}

func classifyLine(line string) docLine {
	upper := strings.ToUpper(line)
	switch {
	case strings.HasPrefix(line, "ИБ №") || strings.HasPrefix(upper, "ИБ N"):
		return docLine{kind: kindCaseNo, text: line}
	case upper == "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ" || strings.EqualFold(line, dailyExamTitle):
		return docLine{kind: kindDailyTitle, text: dailyExamTitle}
	case dailyDatePrefixRE.MatchString(line):
		return docLine{kind: kindDailyDate, text: line}
	case upper == "ОСМОТР":
		return docLine{kind: kindExamTitle, text: line}
	case strings.EqualFold(line, "лечащим врачом совместно с заведующим отделением"):
		return docLine{kind: kindExamSubtitle, text: line}
	case strings.HasPrefix(upper, "ОСМОТР") && strings.Contains(upper, "ЗАВЕДУЮЩ"):
		return docLine{kind: kindTitle, text: line}
	case strings.HasPrefix(upper, "ОСМОТР") && len(line) > len("ОСМОТР"):
		return docLine{kind: kindDailyTitle, text: dailyExamTitle}
	case isDateTimeLine(line):
		return docLine{kind: kindDateTime, text: line}
	case datePrefixedRE.MatchString(line):
		return docLine{kind: kindDailyNarrative, text: line}
	case strings.HasPrefix(line, "Фамилия, имя, отчество"):
		return docLine{kind: kindSignatureCaption, text: line}
	}
	if role, name, ok := splitDailySignature(line); ok {
		return docLine{kind: kindDailySignature, text: role, right: name}
	}
	if isDailyPlaceholderSig(line) {
		return docLine{kind: kindDailySignature, text: line}
	}
	switch {
	case isDoctorSignatureLine(line):
		return docLine{kind: kindDoctorSignature, text: line}
	case isConsultNoteLine(line):
		return docLine{kind: kindConsultNote, text: line}
	}
	if label, value, ok := splitLabelValue(line); ok && isKnownSectionLabel(label) {
		return docLine{kind: kindLabelValue, label: label, value: value}
	}
	return docLine{kind: kindPlain, text: line}
}

func isConsultNoteLine(line string) bool {
	return consultNoteRE.MatchString(line)
}

func isDoctorSignatureLine(line string) bool {
	if strings.HasPrefix(line, "Врач-") || strings.HasPrefix(line, "Леч.врач") {
		return true
	}
	if strings.HasPrefix(line, "Лечащий врач:") || strings.HasPrefix(line, "Заведующий отделением:") {
		return true
	}
	return false
}

var dailySigSpacesRE = regexp.MustCompile(`\s{2,}`)
var initialsRE = regexp.MustCompile(`^[А-ЯЁA-Z]\.[А-ЯЁA-Z]\.?$`)
var personWordRE = regexp.MustCompile(`^[А-ЯЁA-Z][а-яёa-z\-]+$`)

func isDailyPlaceholderSig(line string) bool {
	return strings.Contains(line, phDoctorPos) &&
		strings.Contains(line, phDoctorName) &&
		!strings.Contains(line, ",")
}

func splitDailySignature(line string) (role, name string, ok bool) {
	if i := strings.Index(line, "\t"); i >= 0 {
		role = strings.TrimSpace(line[:i])
		name = strings.TrimSpace(line[i+1:])
		if role != "" && name != "" {
			return role, name, true
		}
	}
	trimmed := strings.TrimSpace(line)
	if isDailyPlaceholderSig(trimmed) {
		if i := strings.Index(trimmed, phDoctorName); i > 0 {
			role = strings.TrimSpace(trimmed[:i])
			name = strings.TrimSpace(trimmed[i:])
			if role != "" && name != "" {
				return role, name, true
			}
		}
	}
	parts := dailySigSpacesRE.Split(trimmed, 2)
	if len(parts) == 2 && looksLikeDoctorRole(parts[0]) && looksLikePersonName(parts[1]) {
		return parts[0], parts[1], true
	}
	fields := strings.Fields(trimmed)
	for nameLen := 4; nameLen >= 2; nameLen-- {
		if nameLen >= len(fields) {
			continue
		}
		name = strings.Join(fields[len(fields)-nameLen:], " ")
		role = strings.Join(fields[:len(fields)-nameLen], " ")
		if looksLikeDoctorRole(role) && looksLikePersonName(name) {
			return role, name, true
		}
	}
	return "", "", false
}

func looksLikeDoctorRole(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "врач") || strings.HasPrefix(lower, "леч.")
}

func looksLikePersonName(s string) bool {
	fields := strings.Fields(s)
	if len(fields) < 2 || len(fields) > 4 {
		return false
	}
	if strings.Contains(strings.ToLower(s), "врач") {
		return false
	}
	for _, f := range fields {
		switch strings.ToLower(strings.Trim(f, ".,")) {
		case "детский", "психиатр", "терапевт", "невролог", "педиатр":
			return false
		}
	}
	last := fields[len(fields)-1]
	if initialsRE.MatchString(last) {
		return personWordRE.MatchString(fields[0])
	}
	for _, f := range fields {
		if !personWordRE.MatchString(f) {
			return false
		}
	}
	return true
}

func isDateTimeLine(line string) bool {
	if strings.Contains(line, "[ДАТА]") {
		return true
	}
	if strings.Contains(line, "время:") && strings.Contains(line, "час.") {
		return true
	}
	if strings.HasPrefix(line, "«") && strings.Contains(line, "г.") {
		return true
	}
	return false
}

func splitLabelValue(line string) (label, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	label = strings.TrimSpace(line[:idx])
	if label == "" {
		return "", "", false
	}
	value = strings.TrimSpace(line[idx+1:])
	return label, value, true
}

// enrichSpans adds mixed bold/underline runs for kinds that need them.
func enrichSpans(l docLine) docLine {
	if l.quiet {
		switch l.kind {
		case kindDailyNarrative, kindDoctorSignature:
			l.spans = []textSpan{{Text: l.text}}
		}
		return l
	}
	switch l.kind {
	case kindDailyNarrative:
		l.spans = dailyNarrativeSpans(l.text)
	case kindDateTime:
		l.spans = examDateTimeSpans(l.text)
	case kindSignatureValue:
		if hasUnresolvedPlaceholder(l.text) {
			l.spans = []textSpan{{Text: l.text}}
		} else {
			l.spans = []textSpan{{Text: l.text, Underline: true}}
		}
	case kindDoctorSignature:
		l.spans = doctorSignatureSpans(l.text)
	}
	return l
}

func dailyNarrativeSpans(text string) []textSpan {
	m := dateNarrativeRE.FindStringSubmatch(text)
	if m == nil {
		// Дата без продолжения или нестандарт — жирным целиком (как в заготовках).
		return []textSpan{{Text: text, Bold: true}}
	}
	date, rest := m[1], m[2]
	out := []textSpan{{Text: date + " ", Bold: true}}
	if rest != "" {
		out = append(out, textSpan{Text: rest})
	}
	return out
}

var examDateTimeRE = regexp.MustCompile(
	`^«\s*(\d{1,2})\s*»\s+(\p{L}+)\s+(\d{4})\s+г\.\s+время:\s+(\d{1,2})\s+час\.\s+(\d{1,2})\s*мин\.?$`,
)

func examDateTimeSpans(line string) []textSpan {
	m := examDateTimeRE.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return []textSpan{{Text: line}}
	}
	day, month, yearStr, hour, minute := m[1], m[2], m[3], m[4], m[5]
	yearPrefix, yearSuffix := yearStr, ""
	if len(yearStr) == 4 {
		yearPrefix = yearStr[:2]
		yearSuffix = yearStr[2:]
	}
	return []textSpan{
		{Text: "«"},
		{Text: pad2(day), Underline: true},
		{Text: "» "},
		{Text: month, Underline: true},
		{Text: " " + yearPrefix},
		{Text: yearSuffix, Underline: true},
		{Text: " г.  время: "},
		{Text: pad2(hour), Underline: true},
		{Text: " час. "},
		{Text: pad2(minute), Underline: true},
		{Text: " мин."},
	}
}

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func hasUnresolvedPlaceholder(s string) bool {
	return strings.Contains(s, "[") && strings.Contains(s, "]")
}

func doctorSignatureSpans(text string) []textSpan {
	// «Врач-психиатр … ФИО» / «Леч.врач … ФИО» — роль обычная, ФИО без underline
	// (в ежедневных заготовках ФИО не подчёркнуто; подчёркивание — только у осмотра).
	return []textSpan{{Text: text}}
}

// parseDocLines splits the body into classified lines.
func displayCaseNo(line string) string {
	s := strings.TrimSpace(line)
	if hasUnresolvedPlaceholder(s) {
		return "ИБ №"
	}
	return s
}

func parseDocLines(content string) []docLine {
	norm := normalizeNewlines(content)
	daily := isDailyExam(norm)
	var out []docLine
	prevCaption := false
	for _, raw := range strings.Split(norm, "\n") {
		line := normalizeGeneratedLine(raw)
		if line == "" {
			prevCaption = false
			continue
		}
		l := classifyLine(line)
		if l.kind == kindCaseNo {
			l.text = displayCaseNo(line)
		}
		if daily && l.kind != kindDailyTitle {
			l.quiet = true
		}
		if daily && l.kind == kindDoctorSignature {
			if role, name, ok := splitDailySignature(line); ok {
				l.kind = kindDailySignature
				l.text = role
				l.right = name
			} else {
				l.kind = kindDailySignature
				l.text = line
				l.right = ""
			}
		}
		if prevCaption && (l.kind == kindPlain || l.kind == kindDoctorSignature) {
			l.kind = kindSignatureValue
			l.text = line
			l.right = ""
		}
		prevCaption = l.kind == kindSignatureCaption
		out = append(out, enrichSpans(l))
	}
	for len(out) > 0 && out[0].kind == kindBlank {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1].kind == kindBlank {
		out = out[:len(out)-1]
	}
	return out
}

// buildDocLines fills placeholders, drops empty sections, then classifies.
func buildDocLines(doc Document) []docLine {
	content := transformContent(doc, doc.Substitutions)
	return parseDocLines(content)
}
