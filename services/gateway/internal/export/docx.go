// DOCX rendering via raw OOXML (archive/zip + encoding/xml), Go stdlib only.
//
// Форматирование как в UI-предпросмотре (шаблон daily / exam_10d):
//   - Times New Roman 11pt; поля ~2,5 см; justify;
//   - шапка ИБ / ОСМОТР / дата; метки секций жирные.
package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"
)

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

	docDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`</Relationships>`

	wNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

	docxFont = "Times New Roman"

	szBody      = 22 // 11pt
	szCaseNo    = 18 // 9pt
	szTitle     = 22
	szSignature = 22

	docxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="` + wNS + `">` +
		`<w:docDefaults><w:rPrDefault><w:rPr>` +
		`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>` +
		`<w:sz w:val="22"/><w:szCs w:val="22"/>` +
		`<w:lang w:val="ru-RU"/>` +
		`</w:rPr></w:rPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal">` +
		`<w:name w:val="Normal"/>` +
		`<w:pPr><w:jc w:val="both"/><w:spacing w:after="0" w:line="256" w:lineRule="auto"/></w:pPr>` +
		`<w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>` +
		`<w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr>` +
		`</w:style>` +
		`</w:styles>`
)

func renderDOCX(doc Document) ([]byte, error) {
	return renderDOCXBatch([]Document{doc})
}

func renderDOCXBatch(docs []Document) ([]byte, error) {
	var body bytes.Buffer

	for i, doc := range docs {
		if i > 0 {
			body.WriteString(paragraphXML("", paraOpts{spaceAfter: 200}))
		}
		for _, l := range buildDocLines(doc) {
			body.WriteString(docxParagraph(l))
		}
	}

	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="` + wNS + `">` +
		`<w:body>` +
		body.String() +
		`<w:sectPr>` +
		`<w:pgSz w:w="11906" w:h="16838"/>` +
		`<w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800" w:header="720" w:footer="720" w:gutter="0"/>` +
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

func docxParagraph(l docLine) string {
	switch l.kind {
	case kindCaseNo:
		return paragraphXML(l.text, paraOpts{align: "right", sizeHalfPt: szCaseNo, spaceAfter: 40})
	case kindExamTitle:
		return paragraphXML(strings.ToUpper(l.text), paraOpts{align: "center", bold: true, sizeHalfPt: szTitle, spaceAfter: 0})
	case kindExamSubtitle:
		return paragraphXML(l.text, paraOpts{align: "center", sizeHalfPt: szBody, spaceAfter: 40})
	case kindTitle:
		return paragraphXML(strings.ToUpper(l.text), paraOpts{align: "center", bold: true, sizeHalfPt: szTitle, spaceAfter: 40})
	case kindDateTime:
		return paragraphSpans(l.spans, paraOpts{align: "center", sizeHalfPt: szBody, spaceAfter: 160}, l.text)
	case kindSignatureCaption:
		return paragraphXML(l.text, paraOpts{align: "both", sizeHalfPt: szCaseNo, spaceBefore: 160, spaceAfter: 0})
	case kindSignatureValue:
		return paragraphSpans(l.spans, paraOpts{align: "both", sizeHalfPt: szBody, spaceAfter: 80}, l.text)
	case kindConsultNote:
		return paragraphXML(l.text, paraOpts{align: "left", sizeHalfPt: szBody, spaceAfter: 40})
	case kindDoctorSignature:
		return paragraphXML(l.text, paraOpts{align: "center", sizeHalfPt: szSignature, spaceBefore: 80, spaceAfter: 120})
	case kindDailyNarrative:
		return paragraphSpans(l.spans, paraOpts{align: "both", sizeHalfPt: szBody, spaceAfter: 40}, l.text)
	case kindLabelValue:
		return paragraphLabelValue(l.label, l.value, paraOpts{align: "both", sizeHalfPt: szBody, spaceAfter: 40})
	default:
		return paragraphXML(l.text, paraOpts{align: "both", sizeHalfPt: szBody, spaceAfter: 40})
	}
}

type paraOpts struct {
	bold        bool
	align       string
	sizeHalfPt  int
	spaceBefore int
	spaceAfter  int
}

type runOpts struct {
	bold       bool
	underline  bool
	sizeHalfPt int
}

func paragraphPr(o paraOpts) string {
	var b strings.Builder
	b.WriteString("<w:pPr>")
	b.WriteString(`<w:spacing`)
	if o.spaceBefore > 0 {
		b.WriteString(` w:before="`)
		b.WriteString(itoa(o.spaceBefore))
		b.WriteString(`"`)
	}
	b.WriteString(` w:after="`)
	b.WriteString(itoa(o.spaceAfter))
	b.WriteString(`" w:line="256" w:lineRule="auto"/>`)
	align := o.align
	if align == "" {
		align = "both"
	}
	b.WriteString(`<w:jc w:val="`)
	b.WriteString(align)
	b.WriteString(`"/>`)
	b.WriteString("</w:pPr>")
	return b.String()
}

func runXML(text string, o runOpts) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("<w:r><w:rPr>")
	b.WriteString(`<w:rFonts w:ascii="` + docxFont + `" w:hAnsi="` + docxFont + `" w:cs="` + docxFont + `"/>`)
	if o.bold {
		b.WriteString("<w:b/><w:bCs/>")
	}
	if o.underline {
		b.WriteString(`<w:u w:val="single"/>`)
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

func paragraphXML(text string, o paraOpts) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	b.WriteString(paragraphPr(o))
	b.WriteString(runXML(text, runOpts{bold: o.bold, sizeHalfPt: o.sizeHalfPt}))
	b.WriteString("</w:p>")
	return b.String()
}

func paragraphSpans(spans []textSpan, o paraOpts, fallback string) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	b.WriteString(paragraphPr(o))
	if len(spans) == 0 {
		b.WriteString(runXML(fallback, runOpts{bold: o.bold, sizeHalfPt: o.sizeHalfPt}))
	} else {
		for _, s := range spans {
			b.WriteString(runXML(s.Text, runOpts{
				bold:       s.Bold || o.bold,
				underline:  s.Underline,
				sizeHalfPt: o.sizeHalfPt,
			}))
		}
	}
	b.WriteString("</w:p>")
	return b.String()
}

func paragraphLabelValue(label, value string, o paraOpts) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	b.WriteString(paragraphPr(o))
	labelText := label + ":"
	valueBold := isDiagnosisLikeLabel(label)
	if value == "" {
		b.WriteString(runXML(labelText, runOpts{bold: true, sizeHalfPt: o.sizeHalfPt}))
	} else {
		b.WriteString(runXML(labelText, runOpts{bold: true, sizeHalfPt: o.sizeHalfPt}))
		b.WriteString(runXML(" "+value, runOpts{bold: valueBold, sizeHalfPt: o.sizeHalfPt}))
	}
	b.WriteString("</w:p>")
	return b.String()
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

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
