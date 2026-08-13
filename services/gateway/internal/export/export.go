// Package export is the gateway's document export service (docs/02 §4 «роль Go:
// экспорт документов», docs/07 §7 POST /requests/{id}/export). It renders the
// ANONYMIZED diary text into a downloadable Word (.docx) or PDF file.
//
// ПРИВАТНОСТЬ (docs/05 §1, docs/09): экспортируется ТОЛЬКО обезличенный текст
// (тот, что лежит в content_anonymized / отдан /generate). Подстановки реальных
// значений плейсхолдеров (например [ДАТА]→19.09.2025) приходят с клиента и
// применяются ЗДЕСЬ, В ПАМЯТИ (docs/07 §7) — результат отдаётся файлом и нигде
// не сохраняется. Имя файла формируется БЕЗ ПДн (тип документа + дата).
//
// ВЫБОР БИБЛИОТЕК И ЛИЦЕНЗИИ (только свободные, заказчик не платит):
//   - .docx: формируется НАПРЯМУЮ как OOXML через стандартную библиотеку Go
//     (archive/zip + encoding/xml). Сторонних зависимостей нет — это надёжнее
//     и проще лицензионно, чем подключать docx-библиотеку (unioffice требует
//     лицензию для части функций — отвергнут). Лицензия: Go stdlib (BSD-3).
//   - .pdf: github.com/go-pdf/fpdf — живой форк архивированного jung-kurt/gofpdf,
//     лицензия MIT/BSD-3 (свободная). Кириллица — встроенный TTF DejaVuSans
//     (см. pdf.go, font_embed.go), лицензия Bitstream Vera / DejaVu (свободная,
//     разрешает встраивание и коммерческое использование).
package export

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Format enumerates supported export formats (docs/07 §7: docx | pdf | txt).
type Format string

const (
	FormatDOCX Format = "docx"
	FormatPDF  Format = "pdf"
	FormatTXT  Format = "txt"
)

// ParseFormat validates a raw format string from the request (docs/07 §7).
func ParseFormat(s string) (Format, bool) {
	switch Format(strings.ToLower(strings.TrimSpace(s))) {
	case FormatDOCX:
		return FormatDOCX, true
	case FormatPDF:
		return FormatPDF, true
	case FormatTXT:
		return FormatTXT, true
	default:
		return "", false
	}
}

// ContentType returns the HTTP Content-Type for a format (docs/07 §7).
func (f Format) ContentType() string {
	switch f {
	case FormatDOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case FormatPDF:
		return "application/pdf"
	case FormatTXT:
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// Document is the privacy-safe input to a render: an ANONYMIZED title + body.
// No PII fields exist here by design (docs/05 §1).
type Document struct {
	// Title — человекочитаемый заголовок (тип дневника). Без ПДн.
	Title string
	// DocumentTypeCode — код типа (daily | exam_10d), используется в имени файла.
	DocumentTypeCode string
	// GeneratedAt — дата генерации (fallback для [ДАТА], если нет даты осмотра).
	GeneratedAt time.Time
	// Content — обезличенный текст дневника с плейсхолдерами ([ДАТА], …).
	Content string
	// Answers — обезличенные ответы (diary_date и т.п.) для штампа осмотра.
	Answers map[string]any
	// Substitutions — плейсхолдеры от клиента (ФИО врача и т.п.).
	// [ДАТА] и [ВРЕМЯ] считает сервер: дата осмотра, время всегда 10:00.
	Substitutions map[string]string
}

// Exporter renders an anonymized Document into downloadable file bytes.
type Exporter interface {
	Export(ctx context.Context, format Format, doc Document) ([]byte, error)
	ExportBatch(ctx context.Context, format Format, docs []Document) ([]byte, error)
}

// renderer is the concrete Exporter used in production. It is stateless and
// safe for concurrent use; the PDF font is loaded lazily once (см. pdf.go).
type renderer struct{}

// New returns the production Exporter.
func New() Exporter { return renderer{} }

// Export dispatches to the per-format renderer (single document).
func (r renderer) Export(ctx context.Context, format Format, doc Document) ([]byte, error) {
	return r.ExportBatch(ctx, format, []Document{doc})
}

// ExportBatch renders multiple diaries into one combined file, in order.
func (renderer) ExportBatch(_ context.Context, format Format, docs []Document) ([]byte, error) {
	if len(docs) == 0 {
		return nil, fmt.Errorf("export: no documents")
	}
	switch format {
	case FormatDOCX:
		return renderDOCXBatch(docs)
	case FormatPDF:
		return renderPDFBatch(docs)
	case FormatTXT:
		return renderTXTBatch(docs), nil
	default:
		return nil, fmt.Errorf("export: unsupported format %q", format)
	}
}

// headerLine is the document header shown above the body: «<Title> · <дата>».
// Date is rendered in DD.MM.YYYY (RU). No PII.
func headerLine(doc Document) string {
	d := doc.GeneratedAt
	if d.IsZero() {
		d = time.Now()
	}
	return fmt.Sprintf("%s · %s", strings.TrimSpace(doc.Title), d.Format("02.01.2006"))
}

// splitParagraphs breaks the body into paragraphs on blank lines, preserving
// the diary's section structure. Single newlines inside a block become soft
// line breaks within one paragraph.
func splitParagraphs(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	rawBlocks := strings.Split(normalized, "\n\n")
	out := make([]string, 0, len(rawBlocks))
	for _, b := range rawBlocks {
		b = strings.TrimRight(b, " \t\n")
		b = strings.TrimLeft(b, "\n")
		if strings.TrimSpace(b) == "" {
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// renderTXT is a trivial plain-text export (header + content), used as a
// lightweight format also offered by the contract (docs/07 §7: txt).
func renderTXT(doc Document) []byte {
	var b strings.Builder
	b.WriteString(headerLine(doc))
	b.WriteString("\n\n")
	b.WriteString(transformContent(doc, doc.Substitutions))
	b.WriteString("\n")
	return []byte(b.String())
}

func renderTXTBatch(docs []Document) []byte {
	var b strings.Builder
	for i, doc := range docs {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		b.Write(renderTXT(doc))
	}
	return []byte(b.String())
}

// Filename builds a PII-free download name: diary_<type>_<YYYY-MM-DD>.<ext>
// (docs/07 §7, docs/09 — никаких ФИО). Falls back to "document" for an unknown
// type code.
func Filename(doc Document, format Format) string {
	typeSlug := doc.DocumentTypeCode
	if typeSlug == "" {
		typeSlug = "document"
	}
	d := doc.GeneratedAt
	if d.IsZero() {
		d = time.Now()
	}
	return fmt.Sprintf("diary_%s_%s.%s", typeSlug, d.Format("2006-01-02"), format)
}

// BatchFilename builds a PII-free name for combined export: diaries_batch_<from>_to_<to>.<ext>
func BatchFilename(docs []Document, format Format) string {
	if len(docs) == 0 {
		return fmt.Sprintf("diaries_batch.%s", format)
	}
	from := docs[0].GeneratedAt
	to := docs[len(docs)-1].GeneratedAt
	if from.IsZero() {
		from = time.Now()
	}
	if to.IsZero() {
		to = from
	}
	return fmt.Sprintf("diaries_batch_%s_to_%s.%s",
		from.Format("2006-01-02"), to.Format("2006-01-02"), format)
}
