// DOCX rendering via raw OOXML (archive/zip + encoding/xml), Go stdlib only.
//
// Зачем без сторонней библиотеки: .docx — это ZIP-контейнер с несколькими
// XML-частями (WordprocessingML). Минимально-валидный документ требует частей
// ([Content_Types].xml, _rels/.rels, word/document.xml; добавляем word/styles.xml
// для дефолтного шрифта Times New Roman). Формирование напрямую полностью
// свободно от лицензионных рисков (unioffice требует платную лицензию для ряда
// функций) и даёт полный контроль над разметкой. Результат открывается в MS
// Word / LibreOffice / Google Docs.
//
// ЭТАП 8.1 — форматирование под реальные шаблоны врача:
//   - НЕТ justify: тело по ЛЕВОМУ краю (фикс «растянутых строк» приёмки Этапа 8);
//   - шрифт Times New Roman 11pt (24 half-points в шапке) через rFonts + styles;
//   - «ИБ №…» — по правому краю мелким кеглем; заголовок «ОСМОТР …» — по центру
//     жирным прописными; строка даты/времени — по центру;
//   - секции «Метка: значение» — метка ЖИРНЫМ run'ом, значение обычным, в одном
//     абзаце; неизвестные строки — обычный абзац слева; подпись внизу.
//
// Структуру строк даёт общий парсер (format.go).
package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"
)

// OOXML namespaces / boilerplate.
const (
	docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
		`</Types>`

	docxRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`</Relationships>`

	// docRels links document.xml → styles.xml.
	docDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`</Relationships>`

	wNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

	// docxFont is the body/heading font of the real templates.
	docxFont = "Times New Roman"

	// Font sizes in half-points (OOXML w:sz unit): 22 == 11pt, 18 == 9pt.
	szBody   = 22
	szCaseNo = 18
	szTitle  = 24

	// styles.xml sets Times New Roman 11pt as the document default so that any
	// run without explicit rFonts still renders in the right font.
	docxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="` + wNS + `">` +
		`<w:docDefaults><w:rPrDefault><w:rPr>` +
		`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>` +
		`<w:sz w:val="22"/><w:szCs w:val="22"/>` +
		`<w:lang w:val="ru-RU"/>` +
		`</w:rPr></w:rPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal">` +
		`<w:name w:val="Normal"/>` +
		`<w:pPr><w:jc w:val="left"/></w:pPr>` +
		`<w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>` +
		`<w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr>` +
		`</w:style>` +
		`</w:styles>`
)

// renderDOCX builds a valid .docx from the anonymized Document, formatted to
// resemble the real doctor templates (см. format.go).
func renderDOCX(doc Document) ([]byte, error) {
	var body bytes.Buffer

	for _, l := range buildDocLines(doc) {
		body.WriteString(docxParagraph(l))
	}

	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="` + wNS + `">` +
		`<w:body>` +
		body.String() +
		// Section properties (A4 portrait, ~1-inch margins).
		`<w:sectPr>` +
		`<w:pgSz w:w="11906" w:h="16838"/>` +
		`<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="708" w:footer="708" w:gutter="0"/>` +
		`</w:sectPr>` +
		`</w:body></w:document>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := []struct {
		name, data string
	}{
		{"[Content_Types].xml", docxContentTypes},
		{"_rels/.rels", docxRels},
		{"word/_rels/document.xml.rels", docDocumentRels},
		{"word/styles.xml", docxStyles},
		{"word/document.xml", document},
	}
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write([]byte(p.data)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// docxParagraph renders one classified line as a <w:p>. Alignment, weight and
// size follow the real templates; body is LEFT-aligned (never justified).
func docxParagraph(l docLine) string {
	switch l.kind {
	case kindCaseNo:
		// «ИБ №…» — справа, мелким кеглем.
		return paragraphXML(l.text, paraOpts{align: "right", sizeHalfPt: szCaseNo, spaceAfter: 60})
	case kindTitle:
		// Заголовок — по центру, жирным, прописными.
		return paragraphXML(strings.ToUpper(l.text), paraOpts{align: "center", bold: true, sizeHalfPt: szTitle, spaceAfter: 60})
	case kindDateTime:
		// Дата/время — по центру.
		return paragraphXML(l.text, paraOpts{align: "center", sizeHalfPt: szBody, spaceAfter: 200})
	case kindSignatureCaption:
		return paragraphXML(l.text, paraOpts{align: "left", sizeHalfPt: szCaseNo, spaceBefore: 200, spaceAfter: 40})
	case kindLabelValue:
		// Метка ЖИРНАЯ + значение обычным, в одном абзаце, по левому краю.
		return labelValueParagraph(l.label, l.value)
	default:
		return paragraphXML(l.text, paraOpts{align: "left", sizeHalfPt: szBody, spaceAfter: 80})
	}
}

// paraOpts controls one paragraph's run/paragraph properties.
type paraOpts struct {
	bold        bool
	align       string // "left" | "center" | "right" | "" (defaults to left)
	sizeHalfPt  int    // font size in half-points (OOXML w:sz unit); 22 == 11pt
	spaceBefore int    // w:spacing w:before in twips
	spaceAfter  int    // w:spacing w:after in twips
}

// runOpts controls a single run's formatting inside a paragraph.
type runOpts struct {
	bold       bool
	sizeHalfPt int
}

// paragraphPr renders the <w:pPr> block for the given paragraph options.
func paragraphPr(o paraOpts) string {
	var b strings.Builder
	b.WriteString("<w:pPr>")
	if o.spaceBefore > 0 || o.spaceAfter > 0 {
		b.WriteString(`<w:spacing`)
		if o.spaceBefore > 0 {
			b.WriteString(` w:before="`)
			b.WriteString(itoa(o.spaceBefore))
			b.WriteString(`"`)
		}
		if o.spaceAfter > 0 {
			b.WriteString(` w:after="`)
			b.WriteString(itoa(o.spaceAfter))
			b.WriteString(`"`)
		}
		b.WriteString(` w:line="264" w:lineRule="auto"/>`)
	}
	// Alignment: explicit left when empty; never "both" (no justify).
	align := o.align
	if align == "" {
		align = "left"
	}
	b.WriteString(`<w:jc w:val="`)
	b.WriteString(align)
	b.WriteString(`"/>`)
	b.WriteString("</w:pPr>")
	return b.String()
}

// runXML renders one <w:r> with the given text and run formatting. Soft line
// breaks (\n) become <w:br/>. Text is XML-escaped; Times New Roman is forced.
func runXML(text string, o runOpts) string {
	var b strings.Builder
	b.WriteString("<w:r><w:rPr>")
	b.WriteString(`<w:rFonts w:ascii="` + docxFont + `" w:hAnsi="` + docxFont + `" w:cs="` + docxFont + `"/>`)
	if o.bold {
		b.WriteString("<w:b/>")
	}
	size := o.sizeHalfPt
	if size == 0 {
		size = szBody
	}
	b.WriteString(`<w:sz w:val="`)
	b.WriteString(itoa(size))
	b.WriteString(`"/><w:szCs w:val="`)
	b.WriteString(itoa(size))
	b.WriteString(`"/>`)
	b.WriteString("</w:rPr>")

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("<w:br/>")
		}
		b.WriteString(`<w:t xml:space="preserve">`)
		b.WriteString(xmlEscape(line))
		b.WriteString("</w:t>")
	}
	b.WriteString("</w:r>")
	return b.String()
}

// paragraphXML renders one <w:p> with a single run.
func paragraphXML(text string, o paraOpts) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	b.WriteString(paragraphPr(o))
	b.WriteString(runXML(text, runOpts{bold: o.bold, sizeHalfPt: o.sizeHalfPt}))
	b.WriteString("</w:p>")
	return b.String()
}

// labelValueParagraph renders «Метка: значение» as ONE left-aligned paragraph
// with a bold run for the label (incl. the colon) and a regular run for the
// value — exactly like the real templates (bold T12 span + regular T4 span).
func labelValueParagraph(label, value string) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	b.WriteString(paragraphPr(paraOpts{align: "left", spaceAfter: 80}))
	// Bold label including the colon.
	b.WriteString(runXML(label+":", runOpts{bold: true, sizeHalfPt: szBody}))
	if value != "" {
		// Leading space separates label and value within the line.
		b.WriteString(runXML(" "+value, runOpts{bold: false, sizeHalfPt: szBody}))
	}
	b.WriteString("</w:p>")
	return b.String()
}

// xmlEscape escapes a string for XML text content.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// itoa is a tiny integer formatter (avoids importing strconv just here).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
