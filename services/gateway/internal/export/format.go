// Structural parser for the diary body, shared by the DOCX and PDF renderers.
//
// ЗАЧЕМ: приёмка Этапа 8 показала, что «плоский» вывод (весь дневник одним
// потоком одинаковых justify-абзацев) не похож на реальные шаблоны врача
// («шаблон каждый день.odt», «шаблон раз в 10 дней.odt»). Реальные шаблоны
// (Times New Roman 11pt) устроены так:
//   - строка «ИБ №…» — по ПРАВОМУ краю, мелким кеглем;
//   - заголовок «ОСМОТР …» — по ЦЕНТРУ, ЖИРНЫМ;
//   - строка даты/времени «[ДАТА] время: [ВРЕМЯ]» — по ЦЕНТРУ;
//   - тело: «Метка: значение» — метка ЖИРНАЯ, значение обычным, в одном абзаце,
//     выравнивание по ЛЕВОМУ краю (НЕ justify!);
//   - внизу — подпись врача.
//
// Этот парсер раскладывает сгенерированный (обезличенный) текст в типизированные
// строки docLine, которые docx.go и pdf.go превращают в форматирование. Парсер
// устойчив к порядку: главный принцип — «строка с двоеточием → жирная метка +
// обычное значение», всё неизвестное → обычный абзац слева.
//
// ПРИВАТНОСТЬ: парсер работает только с обезличенным текстом и плейсхолдерами
// ([ДАТА], [ВРЕМЯ], [НОМЕР_ИБ], [ФИО_ВРАЧА], …) — никаких ПДн не вводит.
package export

import "strings"

// lineKind classifies one logical line of the diary for formatting.
type lineKind int

const (
	kindPlain            lineKind = iota // обычный абзац, по левому краю
	kindCaseNo                           // «ИБ №…» → по правому краю, мелко
	kindTitle                            // «ОСМОТР …» → по центру, жирным
	kindDateTime                         // строка даты/времени → по центру
	kindSignatureCaption                 // «Фамилия, имя, отчество …» подпись-подпись
	kindLabelValue                       // «Метка: значение» → жирная метка + значение
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
	"соматический статус":                                        true,
	"неврологический статус":                                     true,
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

// classifyLine maps one trimmed, non-empty line to a docLine.
func classifyLine(line string) docLine {
	upper := strings.ToUpper(line)
	switch {
	case strings.HasPrefix(line, "ИБ №") || strings.HasPrefix(upper, "ИБ N"):
		return docLine{kind: kindCaseNo, text: line}
	case strings.HasPrefix(upper, "ОСМОТР"):
		return docLine{kind: kindTitle, text: line}
	case isDateTimeLine(line):
		return docLine{kind: kindDateTime, text: line}
	case strings.HasPrefix(line, "Фамилия, имя, отчество"):
		return docLine{kind: kindSignatureCaption, text: line}
	}
	if label, value, ok := splitLabelValue(line); ok {
		return docLine{kind: kindLabelValue, label: label, value: value}
	}
	return docLine{kind: kindPlain, text: line}
}

// isDateTimeLine detects the centred date/time header line. It matches the
// placeholder form ([ДАТА] … время: [ВРЕМЯ]) and the real template form
// («10» апреля 2026 г. время: 16 час. 33 мин.).
func isDateTimeLine(line string) bool {
	if strings.Contains(line, "[ДАТА]") {
		return true
	}
	if strings.Contains(line, "время:") {
		return true
	}
	if strings.HasPrefix(line, "«") {
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
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		out = append(out, classifyLine(line))
	}
	return out
}

// buildDocLines parses the document body and, if the body does not already
// carry a centred title line («ОСМОТР …»), synthesizes a header (ИБ №, title,
// date/time) from the document type/metadata. The header uses only neutral
// placeholders — the doctor fills real values at export time (приватность).
func buildDocLines(doc Document) []docLine {
	lines := parseDocLines(doc.Content)
	for _, l := range lines {
		if l.kind == kindTitle {
			return lines // body already includes the template header
		}
	}
	return append(synthHeader(doc), lines...)
}

// synthHeader derives the top header lines from the document type. Real values
// are never inserted — only placeholders ([НОМЕР_ИБ], [ДАТА], [ВРЕМЯ]).
func synthHeader(doc Document) []docLine {
	var title string
	switch doc.DocumentTypeCode {
	case "exam_10d":
		title = "ОСМОТР лечащим врачом совместно с заведующим отделением"
	case "daily":
		title = "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ"
	default:
		if t := strings.TrimSpace(doc.Title); t != "" {
			title = t
		} else {
			title = "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ"
		}
	}
	return []docLine{
		{kind: kindCaseNo, text: "ИБ №[НОМЕР_ИБ]"},
		{kind: kindTitle, text: title},
		{kind: kindDateTime, text: "[ДАТА] время: [ВРЕМЯ]"},
	}
}
