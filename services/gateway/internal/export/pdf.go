// PDF rendering via github.com/go-pdf/fpdf with embedded Cyrillic TTF (DejaVu).
//
// Те же правила, что и в Word (format.go / docx.go):
//   - ежедневный: жирная дата + обычный текст;
//   - осмотр: центр заголовка, подчёркнутые поля даты/времени и ФИО;
//   - жирные метки секций; подпись врача по центру.
package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

const pdfFontFamily = "DejaVu"

const (
	pdfBodyPt   = 11.0
	pdfCaseNoPt = 9.0
	pdfTitlePt  = 11.0
	pdfLineH    = 5.5
)

func newCyrillicPDF() *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(pdfFontFamily, "", fontRegular)
	pdf.AddUTF8FontFromBytes(pdfFontFamily, "B", fontBold)
	return pdf
}

func renderPDF(doc Document) ([]byte, error) {
	return renderPDFBatch([]Document{doc})
}

func renderPDFBatch(docs []Document) ([]byte, error) {
	pdf := newCyrillicPDF()
	// Поля близко к Word (~2.5 / 3.17 см).
	pdf.SetMargins(25, 25, 25)
	pdf.SetAutoPageBreak(true, 25)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	usableW := pageW - left - right

	for i, doc := range docs {
		if i > 0 {
			pdf.Ln(6)
		}
		for _, l := range buildDocLines(doc) {
			pdfRenderLine(pdf, l, usableW)
		}
	}

	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("export: pdf render: %w", err)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("export: pdf output: %w", err)
	}
	return buf.Bytes(), nil
}

func pdfRenderLine(pdf *fpdf.Fpdf, l docLine, usableW float64) {
	switch l.kind {
	case kindCaseNo:
		pdf.SetFont(pdfFontFamily, "", pdfCaseNoPt)
		pdf.MultiCell(usableW, 5, l.text, "", "R", false)
		pdf.Ln(0.5)
	case kindTitle:
		pdf.SetFont(pdfFontFamily, "B", pdfTitlePt)
		pdf.MultiCell(usableW, 6, strings.ToUpper(l.text), "", "C", false)
	case kindExamTitle:
		pdf.SetFont(pdfFontFamily, "B", pdfTitlePt)
		pdf.MultiCell(usableW, 6, strings.ToUpper(l.text), "", "C", false)
	case kindExamSubtitle:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "C", false)
		pdf.Ln(1)
	case kindDateTime:
		pdfWriteSpans(pdf, l.spans, l.text, usableW, "C", pdfBodyPt)
		pdf.Ln(3)
	case kindSignatureCaption:
		pdf.Ln(2)
		pdf.SetFont(pdfFontFamily, "", pdfCaseNoPt)
		pdf.MultiCell(usableW, 5, l.text, "", "L", false)
	case kindSignatureValue:
		pdfWriteSpans(pdf, l.spans, l.text, usableW, "L", pdfBodyPt)
		pdf.Ln(1)
	case kindConsultNote:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "L", false)
	case kindDoctorSignature:
		pdf.Ln(1)
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "C", false)
		pdf.Ln(2)
	case kindDailyNarrative:
		pdfWriteSpans(pdf, l.spans, l.text, usableW, "J", pdfBodyPt)
		pdf.Ln(0.5)
	case kindLabelValue:
		pdfLabelValue(pdf, l.label, l.value, usableW)
		pdf.Ln(0.5)
	default:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "J", false)
	}
}

func pdfStyle(bold, underline bool) string {
	style := ""
	if bold {
		style += "B"
	}
	if underline {
		style += "U"
	}
	return style
}

// pdfWriteSpans writes mixed bold/underline runs. Falls back to MultiCell when
// the line wraps — first run on the same line, rest via MultiCell for overflow.
func pdfWriteSpans(pdf *fpdf.Fpdf, spans []textSpan, fallback string, usableW float64, align string, pt float64) {
	if len(spans) == 0 {
		pdf.SetFont(pdfFontFamily, "", pt)
		pdf.MultiCell(usableW, pdfLineH, fallback, "", align, false)
		return
	}

	// Measure total width; if fits on one line, Cell-by-Cell with alignment.
	totalW := 0.0
	for _, s := range spans {
		pdf.SetFont(pdfFontFamily, pdfStyle(s.Bold, s.Underline), pt)
		totalW += pdf.GetStringWidth(s.Text)
	}

	startX, y := pdf.GetXY()
	if align == "C" && totalW < usableW {
		pdf.SetX(startX + (usableW-totalW)/2)
	} else if align == "R" && totalW < usableW {
		pdf.SetX(startX + usableW - totalW)
	}

	remaining := usableW
	if align == "C" && totalW < usableW {
		remaining = totalW + 1
	}

	for i, s := range spans {
		style := pdfStyle(s.Bold, s.Underline)
		pdf.SetFont(pdfFontFamily, style, pt)
		w := pdf.GetStringWidth(s.Text)
		if i == len(spans)-1 || w > remaining-1 {
			// Last span or overflow — MultiCell to wrap.
			pdf.MultiCell(remaining, pdfLineH, s.Text, "", "L", false)
			if i < len(spans)-1 {
				// Continue remaining spans on next line.
				rest := ""
				for _, rs := range spans[i+1:] {
					rest += rs.Text
				}
				if rest != "" {
					pdf.SetFont(pdfFontFamily, "", pt)
					pdf.MultiCell(usableW, pdfLineH, rest, "", "J", false)
				}
			}
			_ = y
			return
		}
		pdf.Cell(w, pdfLineH, s.Text)
		remaining -= w
	}
	pdf.Ln(pdfLineH)
}

func pdfLabelValue(pdf *fpdf.Fpdf, label, value string, usableW float64) {
	text := label + ":"
	pdf.SetFont(pdfFontFamily, "B", pdfBodyPt)
	if value == "" {
		pdf.MultiCell(usableW, pdfLineH, text, "", "J", false)
		return
	}
	labelW := pdf.GetStringWidth(text + " ")
	if labelW > usableW*0.7 {
		pdf.MultiCell(usableW, pdfLineH, text+" "+value, "", "J", false)
		return
	}
	pdf.Cell(labelW, pdfLineH, text+" ")
	style := ""
	if isDiagnosisLikeLabel(label) {
		style = "B"
	}
	pdf.SetFont(pdfFontFamily, style, pdfBodyPt)
	pdf.MultiCell(usableW-labelW, pdfLineH, value, "", "J", false)
}
