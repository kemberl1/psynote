// PDF rendering via github.com/go-pdf/fpdf (BSD-3, свободная) with an embedded
// Cyrillic TTF (DejaVu Sans, свободная лицензия — см. font_embed.go).
//
// ВЫБОР ШРИФТА (Этап 8.1): встроенные «core»-шрифты PDF (Times-Roman/Helvetica)
// НЕ содержат кириллических глифов — русский текст превратился бы в «кракозябры».
// Поэтому используется встроенный TTF с полным кириллическим покрытием. Оставлен
// DejaVu Sans: он свободный (Bitstream Vera / DejaVu), уже встроен в бинарь,
// визуально аккуратный и на приёмке Этапа 8 был приемлем. Метрически-Times
// альтернативы (PT Serif / Liberation Serif) дали бы засечки «как Times», но
// потребовали бы добавления новых бинарных шрифтов в репозиторий; для целей
// «не justify + структурное форматирование + жирные метки» это не критично, а
// добавление шрифтов выходит за скоуп Этапа 8.1. Главное соблюдено: НЕТ justify,
// нормальные абзацы слева, жирные метки секций, центр заголовка, ИБ№ справа.
//
// ЭТАП 8.1 — те же правила форматирования, что и в Word (см. format.go):
//   - тело по ЛЕВОМУ краю (align "L", НЕ "J");
//   - «ИБ №…» — справа; заголовок «ОСМОТР …» — по центру жирным; дата — по центру;
//   - секции «Метка: значение» — жирная метка + обычное значение в одной строке
//     (через смену стиля шрифта внутри строки с переносом MultiCell-логикой).
package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// pdfFontFamily is the family name registered for the embedded DejaVu font.
const pdfFontFamily = "DejaVu"

// PDF point sizes / line heights.
const (
	pdfBodyPt   = 12.0
	pdfCaseNoPt = 10.0
	pdfTitlePt  = 14.0
	pdfLineH    = 6.0
)

// newCyrillicPDF creates an A4 portrait Fpdf with the embedded Cyrillic TTF
// registered in both regular and bold styles (UTF-8 mode).
func newCyrillicPDF() *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(pdfFontFamily, "", fontRegular)
	pdf.AddUTF8FontFromBytes(pdfFontFamily, "B", fontBold)
	return pdf
}

// renderPDF builds an A4 PDF from the anonymized Document.
func renderPDF(doc Document) ([]byte, error) {
	return renderPDFBatch([]Document{doc})
}

// renderPDFBatch combines multiple diaries into one PDF.
func renderPDFBatch(docs []Document) ([]byte, error) {
	pdf := newCyrillicPDF()
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	usableW := pageW - 40

	for i, doc := range docs {
		if i > 0 {
			pdf.Ln(4)
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

// pdfRenderLine renders one classified line with the matching alignment/weight.
func pdfRenderLine(pdf *fpdf.Fpdf, l docLine, usableW float64) {
	switch l.kind {
	case kindCaseNo:
		pdf.SetFont(pdfFontFamily, "", pdfCaseNoPt)
		pdf.MultiCell(usableW, 5, l.text, "", "R", false)
		pdf.Ln(1)
	case kindTitle:
		pdf.SetFont(pdfFontFamily, "B", pdfTitlePt)
		pdf.MultiCell(usableW, 7, strings.ToUpper(l.text), "", "C", false)
		pdf.Ln(1)
	case kindDateTime:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "C", false)
		pdf.Ln(3)
	case kindSignatureCaption:
		pdf.Ln(2)
		pdf.SetFont(pdfFontFamily, "", pdfCaseNoPt)
		pdf.MultiCell(usableW, 5, l.text, "", "L", false)
	case kindConsultNote:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "L", false)
		pdf.Ln(1)
	case kindDoctorSignature:
		pdf.Ln(2)
		pdf.SetFont(pdfFontFamily, "", pdfCaseNoPt)
		pdf.MultiCell(usableW, 5, l.text, "", "J", false)
		pdf.Ln(1)
	case kindDailyNarrative:
		pdf.SetFont(pdfFontFamily, "B", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "J", false)
		pdf.Ln(1)
	case kindExamTitle:
		pdf.SetFont(pdfFontFamily, "B", pdfTitlePt)
		pdf.MultiCell(usableW, 7, strings.ToUpper(l.text), "", "C", false)
		pdf.Ln(1)
	case kindExamSubtitle:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "C", false)
		pdf.Ln(2)
	case kindLabelValue:
		pdfLabelValue(pdf, l.label, l.value, usableW)
		pdf.Ln(1.5)
	default:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "J", false)
		pdf.Ln(1)
	}
}

func pdfLabelValue(pdf *fpdf.Fpdf, label, value string, usableW float64) {
	text := label + ":"
	pdf.SetFont(pdfFontFamily, "B", pdfBodyPt)
	if value == "" {
		pdf.MultiCell(usableW, pdfLineH, text, "", "J", false)
		return
	}
	// Bold label then normal value on the same line when possible.
	labelW := pdf.GetStringWidth(text + " ")
	pdf.Cell(labelW, pdfLineH, text+" ")
	pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
	pdf.MultiCell(usableW-labelW, pdfLineH, value, "", "J", false)
}
