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

// renderPDF builds an A4 PDF from the anonymized Document, formatted like the
// real doctor templates: left body (никакого justify), centred bold title,
// right-aligned ИБ№, centred date, bold section labels.
func renderPDF(doc Document) ([]byte, error) {
	pdf := newCyrillicPDF()
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	usableW := pageW - 40 // page width minus left+right margins

	for _, l := range buildDocLines(doc) {
		pdfRenderLine(pdf, l, usableW)
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
	case kindLabelValue:
		pdfLabelValue(pdf, l.label, l.value, usableW)
	default:
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.MultiCell(usableW, pdfLineH, l.text, "", "L", false)
		pdf.Ln(1)
	}
}

// pdfLabelValue writes a «Метка: значение» line: bold label then regular value,
// wrapping naturally to the left margin. Uses pdf.Write so the bold→regular
// switch happens mid-line; the final Ln closes the paragraph. Left-aligned by
// construction (Write flows from the left margin), never justified.
func pdfLabelValue(pdf *fpdf.Fpdf, label, value string, _ float64) {
	pdf.SetFont(pdfFontFamily, "B", pdfBodyPt)
	pdf.Write(pdfLineH, label+":")
	if value != "" {
		pdf.SetFont(pdfFontFamily, "", pdfBodyPt)
		pdf.Write(pdfLineH, " "+value)
	}
	pdf.Ln(pdfLineH)
	pdf.Ln(1.5)
}
