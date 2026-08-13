package export

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// sampleContent mirrors a real diary body: header lines + «Метка: значение»
// sections + signature. Placeholders only — no PII.
const sampleContent = "ИБ №[НОМЕР_ИБ]\n" +
	"ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
	"[ДАТА] время: [ВРЕМЯ]\n\n" +
	"Жалобы: не предъявляет\n" +
	"Анамнез заболевания (дополнения к анамнезу): без дополнений\n" +
	"Психический статус: Настроение сниженное, отмечается тревожность. Сон нарушен.\n" +
	"Физикальное исследование, локальный статус (его изменение): Т – 36,6 С.\n" +
	"Диагноз:\n" +
	"Назначения: продолжить терапию [ДАТА].\n" +
	"Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись\n" +
	"Лечащий врач: [ФИО_ВРАЧА]"

func sampleDoc() Document {
	return Document{
		Title:            "Ежедневный дневник · сниженное настроение",
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2025, 9, 19, 10, 0, 0, 0, time.UTC),
		Content:          sampleContent,
	}
}

func docxDocumentXML(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("docx is not a valid zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open document.xml: %v", err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatal("word/document.xml not found in docx")
	return ""
}

func docxPart(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("docx is not a valid zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	return ""
}

func TestExportDOCX_ValidZipContainsText(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("docx is empty")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("docx is not a valid zip: %v", err)
	}
	want := map[string]bool{
		"[Content_Types].xml": false,
		"_rels/.rels":         false,
		"word/document.xml":   false,
		"word/styles.xml":     false,
	}
	for _, f := range zr.File {
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("docx missing required part %q", name)
		}
	}

	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, "Настроение сниженное") {
		t.Error("docx document.xml does not contain expected Cyrillic body text")
	}
}

// TestExportDOCX_CorpusJustify checks body paragraphs use justify (both),
// matching the corpus сборник formatting.
func TestExportDOCX_CorpusJustify(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, `w:jc w:val="both"`) {
		t.Error("docx body should use justify alignment (w:jc=both) per corpus сборник")
	}
}

// TestExportDOCX_TimesNewRoman checks the font matches the corpus сборник default.
func TestExportDOCX_TimesNewRoman(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, `w:ascii="Times New Roman"`) {
		t.Error("docx runs do not use Times New Roman")
	}
	styles := docxPart(t, data, "word/styles.xml")
	if !strings.Contains(styles, "Times New Roman") {
		t.Error("docx styles.xml does not set Times New Roman default")
	}
}

// TestExportDOCX_DailyMatchesTemplate checks Word keeps the same header/sections as UI.
func TestExportDOCX_DailyMatchesTemplate(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ") {
		t.Error("daily docx must keep ОСМОТР header like UI")
	}
	if !strings.Contains(documentXML, "19.09.2025") {
		t.Error("daily docx should include date from GeneratedAt")
	}
	if !strings.Contains(documentXML, "Настроение сниженное") {
		t.Error("daily docx missing psychiatric narrative")
	}
	if !strings.Contains(documentXML, "Психический статус") {
		t.Error("daily docx must keep section titles")
	}
	if strings.Contains(strings.ToLower(documentXML), "без дополнений") {
		t.Error("empty anamnesis must not appear in docx")
	}
}

// TestExportDOCX_BoldSectionLabels checks label:value uses bold label runs.
func TestExportDOCX_BoldSectionLabels(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, "<w:b/>") {
		t.Error("docx should contain bold runs for narrative/labels")
	}
	if !strings.Contains(documentXML, "Физикальное исследование") {
		t.Error("docx missing physical exam section label")
	}
	if !strings.Contains(documentXML, `w:jc w:val="both"`) {
		t.Error("docx body should use justify alignment (w:jc=both) per corpus сборник")
	}
}

func TestExportPDF_ValidAndCyrillic(t *testing.T) {
	data, err := New().Export(context.Background(), FormatPDF, sampleDoc())
	if err != nil {
		t.Fatalf("export pdf: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("pdf is empty")
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Errorf("pdf does not start with %%PDF marker, got %q", data[:min(8, len(data))]) //nolint:gocritic
	}
	if !bytes.Contains(data, []byte("%%EOF")) {
		t.Error("pdf does not contain EOF trailer")
	}
	if !bytes.Contains(data, []byte("FontFile2")) {
		t.Error("pdf does not embed a TrueType font (FontFile2 missing) — Cyrillic would not render")
	}
}

// TestExportPDF_CyrillicDoesNotPanic feeds heavy Cyrillic content to ensure the
// UTF-8 font path never panics or errors.
func TestExportPDF_CyrillicDoesNotPanic(t *testing.T) {
	doc := sampleDoc()
	doc.Content = strings.Repeat("Психический статус: Пациент спокоен, ориентирован. Жалоб не предъявляет.\n", 200)
	data, err := New().Export(context.Background(), FormatPDF, doc)
	if err != nil {
		t.Fatalf("export pdf (heavy cyrillic): %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Error("heavy-cyrillic pdf is not valid")
	}
}

func TestExportTXT(t *testing.T) {
	data, err := New().Export(context.Background(), FormatTXT, sampleDoc())
	if err != nil {
		t.Fatalf("export txt: %v", err)
	}
	if !strings.Contains(string(data), "Настроение сниженное") {
		t.Error("txt missing body text")
	}
	if !strings.Contains(string(data), "19.09.2025") {
		t.Error("txt missing header date")
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]bool{
		"docx": true, "PDF": true, " txt ": true, "xlsx": false, "": false,
	}
	for in, ok := range cases {
		if _, got := ParseFormat(in); got != ok {
			t.Errorf("ParseFormat(%q)=%v, want %v", in, got, ok)
		}
	}
}

func TestFilenameNoPII(t *testing.T) {
	got := Filename(sampleDoc(), FormatDOCX)
	want := "diary_daily_2025-09-19.docx"
	if got != want {
		t.Errorf("Filename=%q, want %q", got, want)
	}
	d := sampleDoc()
	d.DocumentTypeCode = ""
	if got := Filename(d, FormatPDF); got != "diary_document_2025-09-19.pdf" {
		t.Errorf("fallback filename=%q", got)
	}
}

func TestContentType(t *testing.T) {
	if FormatDOCX.ContentType() != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Error("docx content-type wrong")
	}
	if FormatPDF.ContentType() != "application/pdf" {
		t.Error("pdf content-type wrong")
	}
}

func TestExportBatchDOCX(t *testing.T) {
	docs := []Document{sampleDoc(), sampleDoc()}
	data, err := New().ExportBatch(context.Background(), FormatDOCX, docs)
	if err != nil {
		t.Fatalf("export batch docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	// Two diaries → duplicate body text appears twice.
	if strings.Count(documentXML, "Настроение сниженное") < 2 {
		t.Error("batch docx should contain both diary bodies")
	}
}

func TestBatchFilename(t *testing.T) {
	docs := []Document{
		sampleDoc(),
		{DocumentTypeCode: "daily", GeneratedAt: time.Date(2025, 9, 25, 0, 0, 0, 0, time.UTC)},
	}
	got := BatchFilename(docs, FormatDOCX)
	if got != "diaries_batch_2025-09-19_to_2025-09-25.docx" {
		t.Errorf("BatchFilename=%q", got)
	}
}
