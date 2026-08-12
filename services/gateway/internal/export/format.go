// Structural parser for the diary body, shared by the DOCX and PDF renderers.
//
// Форматирование под корпус (`Документы/02_корпус/…`):
//   - Times New Roman 11pt, justify;
//   - ежедневный: жирная ДАТА, обычный текст; короткие метки «Физикальное…»;
//   - осмотр 10д: центр «ОСМОТР», дата/время с подчёркнутыми полями;
//   - метки секций жирные; подписи врача — центр / подчёркнутое ФИО.
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

// examDateTimeRE parses «08» декабря 2025 г.  время: 12 час. 14 мин.
var examDateTimeRE = regexp.MustCompile(
	`^«\s*(\d{1,2})\s*»\s+(\p{L}+)\s+(\d{4})\s+г\.\s+время:\s+(\d{1,2})\s+час\.\s+(\d{1,2})\s*мин\.?$`,
)

// lineKind classifies one logical line of the diary for formatting.
type lineKind int

const (
	kindPlain            lineKind = iota // обычный абзац, justify
	kindDailyNarrative                   // DD.MM.YYYY + narrative (дата жирным)
	kindCaseNo                           // «ИБ №…» → по правому краю, мелко
	kindTitle                            // legacy centred title
	kindExamTitle                        // «ОСМОТР» → по центру, жирным
	kindExamSubtitle                     // подзаголовок осмотра → по центру
	kindDateTime                         // строка даты/времени → по центру + underline
	kindSignatureCaption                 // «Фамилия, имя, отчество …»
	kindSignatureValue                   // ФИО под подписью — подчёркнуто
	kindLabelValue                       // «Метка: значение» → жирная метка
	kindConsultNote                      // консультации → слева
	kindDoctorSignature                  // подпись врача → по центру
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
	spans []textSpan
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
	"основное заболевание":                   true,
	"осложнение основного заболевания":       true,
	"сопутствующие заболевания":              true,
	"синдром":                                true,
	"дополнительные сведения":                true,
	"назначения":                             true,
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
	case upper == "ОСМОТР":
		return docLine{kind: kindExamTitle, text: line}
	case strings.EqualFold(line, "лечащим врачом совместно с заведующим отделением"):
		return docLine{kind: kindExamSubtitle, text: line}
	case strings.HasPrefix(upper, "ОСМОТР") && len(line) > len("ОСМОТР"):
		return docLine{kind: kindTitle, text: line}
	case isDateTimeLine(line):
		return docLine{kind: kindDateTime, text: line}
	case datePrefixedRE.MatchString(line):
		return docLine{kind: kindDailyNarrative, text: line}
	case strings.HasPrefix(line, "Фамилия, имя, отчество"):
		return docLine{kind: kindSignatureCaption, text: line}
	case isDoctorSignatureLine(line):
		return docLine{kind: kindDoctorSignature, text: line}
	case isConsultNoteLine(line):
		return docLine{kind: kindConsultNote, text: line}
	}
	if label, value, ok := splitLabelValue(line); ok {
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

// isDiagnosisLikeLabel — в корпусе значения диагноза/синдрома часто жирные.
func isDiagnosisLikeLabel(label string) bool {
	l := strings.ToLower(strings.TrimSpace(label))
	return l == "диагноз" ||
		strings.Contains(l, "заболеван") ||
		l == "синдром" ||
		strings.Contains(l, "осложнен")
}

// enrichSpans adds mixed bold/underline runs for kinds that need them.
func enrichSpans(l docLine) docLine {
	switch l.kind {
	case kindDailyNarrative:
		l.spans = dailyNarrativeSpans(l.text)
	case kindDateTime:
		l.spans = examDateTimeSpans(l.text)
	case kindSignatureValue:
		l.spans = []textSpan{{Text: l.text, Underline: true}}
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

func examDateTimeSpans(line string) []textSpan {
	m := examDateTimeRE.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return []textSpan{{Text: line}}
	}
	day, month, yearStr, hour, minute := m[1], m[2], m[3], m[4], m[5]
	// Год: «20» + подчёркнутые две последние цифры — как в сборнике.
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

func doctorSignatureSpans(text string) []textSpan {
	// «Врач-психиатр … ФИО» / «Леч.врач … ФИО» — роль обычная, ФИО без underline
	// (в ежедневных заготовках ФИО не подчёркнуто; подчёркивание — только у осмотра).
	return []textSpan{{Text: text}}
}

// parseDocLines splits the body into classified lines.
func parseDocLines(content string) []docLine {
	norm := normalizeNewlines(content)
	var out []docLine
	prevCaption := false
	for _, raw := range strings.Split(norm, "\n") {
		line := normalizeGeneratedLine(raw)
		if line == "" {
			continue
		}
		l := classifyLine(line)
		// Строка сразу после подписи-подсказки — подчёркнутое ФИО.
		if prevCaption && l.kind == kindPlain {
			l.kind = kindSignatureValue
			l.text = line
		}
		prevCaption = l.kind == kindSignatureCaption
		out = append(out, enrichSpans(l))
	}
	return out
}

// buildDocLines transforms template output into corpus layout, then classifies.
func buildDocLines(doc Document) []docLine {
	content := transformContent(doc, doc.Substitutions)
	return parseDocLines(content)
}
