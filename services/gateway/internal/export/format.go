// Structural parser for the diary body, shared by the DOCX and PDF renderers.
//
// Форматирование под корпусный сборник дневников (`Документы/02_корпус/сборник_дневников_ИБ/`, локально):
// Times New Roman 11pt, justify для тела, жирные метки секций «Метка: значение»,
// консультации слева, подпись врача мелким кеглем.
//
// ПРИВАТНОСТЬ: парсер работает только с обезличенным текстом и плейсхолдерами
// ([ДАТА], [ВРЕМЯ], [НОМЕР_ИБ], [ФИО_ВРАЧА], …) — никаких ПДн не вводит.
package export

import (
	"regexp"
	"strings"
)

// consultNoteRE matches specialist consultation lines from the corpus сборник.
var consultNoteRE = regexp.MustCompile(` от \d{2}\.\d{2}\.\d{2,4}:`)

// datePrefixedRE matches diary narrative lines starting with a date.
var datePrefixedRE = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}`)

// lineKind classifies one logical line of the diary for formatting.
type lineKind int

const (
	kindPlain            lineKind = iota // обычный абзац, justify
	kindDailyNarrative                   // DD.MM.YYYY + narrative, жирным
	kindCaseNo                           // «ИБ №…» → по правому краю, мелко
	kindTitle                            // legacy centred title
	kindExamTitle                        // «ОСМОТР» → по центру, жирным
	kindExamSubtitle                     // подзаголовок осмотра → по центру
	kindDateTime                         // строка даты/времени → по центру
	kindSignatureCaption                 // «Фамилия, имя, отчество …» подпись-подпись
	kindLabelValue                       // «Метка: значение» → жирная метка + обычное значение
	kindConsultNote                      // консультации («Педиатр от …») → слева
	kindDoctorSignature                  // подпись врача → justify, мелкий кегль
)

// docLine is one formatted line of the document.
//
//   - kindLabelValue uses label (без двоеточия) + value (может быть пустым);
//   - все остальные виды используют text.
type docLine struct {
	kind  lineKind
	label string
	value string
	text  string
}

// knownSectionLabels — метки секций реальных шаблонов (нормализованные:
// нижний регистр, без двоеточия). Используются как явный сигнал «это секция»
// (жирная метка), но НЕ являются обязательными: любая строка с двоеточием тоже
// форматируется как метка+значение. Список из «шаблон каждый день.odt» и
// «шаблон раз в 10 дней.odt».
var knownSectionLabels = map[string]bool{
	"жалобы": true,
	"анамнез заболевания (дополнения к анамнезу)":                true,
	"анамнез жизни (дополнения к анамнезу)":                      true,
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
	"назначения":                             true,
	"выполнены медицинские вмешательства":    true,
	"план обследования (дополнения к плану)": true,
	"план лечения (дополнения к плану)":      true,
	"этапный эпикриз":                        true,
	"лечащий врач":                           true,
	"заведующий отделением":                  true,
}

// isKnownSectionLabel reports whether label (без двоеточия) — известная метка
// секции из реальных шаблонов.
func isKnownSectionLabel(label string) bool {
	return knownSectionLabels[strings.ToLower(strings.TrimSpace(label))]
}

// normalizeNewlines collapses CRLF/CR to LF.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// normalizeGeneratedLine strips lightweight markdown the LLM may wrap around
// section labels (**Жалобы:** → Жалобы:). Without this, section lookup in
// transformExam10d/transformDaily fails and Word export ends up nearly empty.
func normalizeGeneratedLine(line string) string {
	return strings.TrimSpace(strings.ReplaceAll(line, "**", ""))
}

// classifyLine maps one trimmed, non-empty line to a docLine.
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
	if strings.HasPrefix(line, "Врач-") {
		return true
	}
	if strings.HasPrefix(line, "Лечащий врач:") || strings.HasPrefix(line, "Заведующий отделением:") {
		return true
	}
	return false
}

// isDateTimeLine detects the centred date/time header line. It matches the
// placeholder form ([ДАТА] … время: [ВРЕМЯ]) and the real template form
// («10» апреля 2026 г. время: 16 час. 33 мин.).
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

// splitLabelValue splits «Метка: значение» at the FIRST colon. label is
// returned without the trailing colon; value may be empty (e.g. «Диагноз:»).
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

// parseDocLines splits the body into classified lines. Blank lines are dropped;
// inter-section spacing is added by the renderers.
func parseDocLines(content string) []docLine {
	norm := normalizeNewlines(content)
	var out []docLine
	for _, raw := range strings.Split(norm, "\n") {
		line := normalizeGeneratedLine(raw)
		if line == "" {
			continue
		}
		out = append(out, classifyLine(line))
	}
	return out
}

// buildDocLines transforms template output into corpus layout, then classifies.
func buildDocLines(doc Document) []docLine {
	content := transformContent(doc, doc.Substitutions)
	return parseDocLines(content)
}
