// Package export builds final Word/PDF documents from generated diary text.
//
// See docs/02_system_architecture.md §4 (роль Go: экспорт документов) and
// docs/10_roadmap_stepbystep.md Этап 6.
//
// Этап 1 (каркас): только интерфейс и заглушка. Потоковая генерация .docx/.pdf
// и подстановка плейсхолдеров на клиенте — будущие этапы.
package export

import "context"

// Format enumerates supported export formats.
type Format string

const (
	FormatDOCX Format = "docx"
	FormatPDF  Format = "pdf"
)

// Exporter renders diary text into a downloadable document.
type Exporter interface {
	// Export returns the rendered file bytes for the given format.
	Export(ctx context.Context, format Format, content string) ([]byte, error)
}

// Stub is a no-op placeholder.
type Stub struct{}

// Export is a placeholder. TODO(этап 6): реализовать генерацию .docx/.pdf.
func (Stub) Export(_ context.Context, _ Format, _ string) ([]byte, error) {
	return nil, nil
}
