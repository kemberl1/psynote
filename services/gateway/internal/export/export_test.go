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

// TestExportDOCX_LeftAlign checks body paragraphs use left alignment
// matching the MIS forms on the reference photos.
func TestExportDOCX_LeftAlign(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, `w:jc w:val="left"`) {
		t.Error("docx body should use left alignment (w:jc=left) per MIS forms")
	}
}

// TestExportDOCX_Arial checks the font matches the MIS forms (Arial 11).
func TestExportDOCX_Arial(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, `w:ascii="Arial"`) {
		t.Error("docx runs do not use Arial")
	}
	styles := docxPart(t, data, "word/styles.xml")
	if !strings.Contains(styles, "Arial") {
		t.Error("docx styles.xml does not set Arial default")
	}
}

// TestExportDOCX_DailyMatchesTemplate checks Word keeps the same header/sections as UI.
func TestExportDOCX_DailyMatchesTemplate(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	documentXML := docxDocumentXML(t, data)
	if !strings.Contains(documentXML, "Осмотр лечащим врачом") {
		t.Error("daily docx must use the MIS daily header")
	}
	if !strings.Contains(documentXML, "Дата: 19.09.2025 10:00") {
		t.Error("daily docx should include numeric date from GeneratedAt")
	}
	if !strings.Contains(documentXML, "Настроение сниженное") {
		t.Error("daily docx missing psychiatric narrative")
	}
	if !strings.Contains(documentXML, "Психический статус") {
		t.Error("daily docx must keep section titles")
	}
	if !strings.Contains(strings.ToLower(documentXML), "без дополнений") {
		t.Error("anamnesis «без дополнений» must stay in the MIS form")
	}
}

func TestExportDOCX_DailyOnlyTitleIsBold(t *testing.T) {
	data, err := New().Export(context.Background(), FormatDOCX, sampleDoc())
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	xml := docxDocumentXML(t, data)
	if !runHasBold(xml, "Осмотр лечащим врачом") {
		t.Error("daily title must be bold")
	}
	if runHasBold(xml, "Жалобы") {
		t.Error("daily section labels must not be bold")
	}
	if runHasBold(xml, "Дата:") {
		t.Error("daily date must not be bold")
	}
	if strings.Contains(xml, `w:leader="underscore"`) || strings.Contains(xml, `<w:u w:val="single"/>`) {
		t.Error("daily exam must not underline")
	}
	if !strings.Contains(xml, "Физикальное исследование") {
		t.Error("docx missing physical exam section label")
	}
}

func TestExportDOCX_DailySignatureKeepsPartsTogether(t *testing.T) {
	doc := sampleDoc()
	doc.Substitutions = map[string]string{
		"[ФИО_ВРАЧА]":       "Иванов И.И.",
		"[ДОЛЖНОСТЬ_ВРАЧА]": "врач-терапевт",
	}
	data, err := New().Export(context.Background(), FormatDOCX, doc)
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	xml := docxDocumentXML(t, data)
	if strings.Contains(xml, "<w:tab/>") {
		t.Fatalf("daily signature must not use a right tab:\n%s", xml)
	}
	if !strings.Contains(xml, "врач-терапевт Иванов И.И.") {
		t.Fatalf("expected signature parts next to each other:\n%s", xml)
	}
	i := strings.Index(xml, "врач-терапевт Иванов И.И.")
	start := strings.LastIndex(xml[:i], "<w:p>")
	end := strings.Index(xml[i:], "</w:p>")
	para := xml[start : i+end]
	if !strings.Contains(para, `<w:u w:val="single"/>`) {
		t.Fatalf("filled daily signature must be underlined:\n%s", para)
	}
	if strings.Contains(para, `w:before="`) && !strings.Contains(para, `w:before="0"`) {
		t.Fatalf("daily signature must sit on the next line without extra space:\n%s", para)
	}
}

func TestExportDOCX_ValuesAfterColonAreNotBold(t *testing.T) {
	doc := sampleDoc()
	doc.Content = "Анамнез заболевания (дополнения к анамнезу): без дополнений\n" +
		"Основное заболевание: F71.18 Сидрос психмоторной расторможенности\n" +
		"Сопутствующие заболевания: не выявлено\n" +
		"Дополнительные сведения о заболевании: нет\n"
	data, err := New().Export(context.Background(), FormatDOCX, doc)
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	xml := docxDocumentXML(t, data)
	if !strings.Contains(xml, "Синдром психомоторной") {
		t.Error("docx should fix obvious typo Сидрос/психмоторной")
	}
	for _, value := range []string{"без дополнений", "не выявлено"} {
		if runHasBold(xml, value) {
			t.Errorf("value %q must not be bold after the colon", value)
		}
	}
}

func runHasBold(documentXML, needle string) bool {
	idx := strings.Index(documentXML, needle)
	if idx < 0 {
		return false
	}
	start := strings.LastIndex(documentXML[:idx], "<w:r>")
	if start < 0 {
		return false
	}
	run := documentXML[start:idx]
	return strings.Contains(run, "<w:b/>")
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
		"docx": true, "PDF": false, " txt ": true, "xlsx": false, "": false,
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
	if got := Filename(d, FormatDOCX); got != "diary_document_2025-09-19.docx" {
		t.Errorf("fallback filename=%q", got)
	}
}

func TestContentType(t *testing.T) {
	if FormatDOCX.ContentType() != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Error("docx content-type wrong")
	}
	if FormatTXT.ContentType() != "text/plain; charset=utf-8" {
		t.Error("txt content-type wrong")
	}
}

func TestExportDOCX_Exam10dLayout(t *testing.T) {
	doc := Document{
		Title:            "Осмотр за 10 дней",
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
		Content: "ИБ №[НОМЕР_ИБ]\n" +
			"ОСМОТР\n" +
			"лечащим врачом совместно с заведующим отделением\n" +
			"[ДАТА] время: [ВРЕМЯ]\n" +
			"Жалобы: не предъявляет\n" +
			"Этапный эпикриз: Состояние с улучшением.\n" +
			"Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись\n" +
			"[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]\n" +
			"Фамилия, имя, отчество (при наличии) заведующего отделением, подпись\n" +
			"[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]",
		Substitutions: map[string]string{
			"[НОМЕР_ИБ]":           "20261207",
			"[ФИО_ВРАЧА]":          "Весова Ксения Юрьевна",
			"[ДОЛЖНОСТЬ_ВРАЧА]":    "Врач-психиатр детский",
			"[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]": "Демахин Геннадий Сергеевич, Заведующий (ОПО№2) отделение",
		},
	}
	data, err := New().Export(context.Background(), FormatDOCX, doc)
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	xml := docxDocumentXML(t, data)
	if !strings.Contains(xml, "ОСМОТР") {
		t.Error("10-day title missing")
	}
	if !strings.Contains(xml, "ИБ №20261207") {
		t.Error("case number missing from 10-day body")
	}
	if i := strings.Index(xml, "ИБ №20261207"); i >= 0 {
		start := strings.LastIndex(xml[:i], "<w:p>")
		end := strings.Index(xml[i:], "</w:p>")
		if start >= 0 && end >= 0 && !strings.Contains(xml[start:i+end], `w:jc w:val="right"`) {
			t.Error("ИБ № must be right-aligned at the top of the 10-day exam")
		}
	}
	if !strings.Contains(xml, "августа") || !strings.Contains(xml, "время:") {
		t.Error("official date missing")
	}
	if strings.Contains(xml, "Дата: 10.08.2026") {
		t.Error("10-day must keep official date, not daily numeric")
	}
	if !strings.Contains(xml, `w:val="center"`) {
		t.Error("10-day title/subtitle/date must be centered")
	}
	if i := strings.Index(xml, ">ОСМОТР<"); i >= 0 {
		start := strings.LastIndex(xml[:i], "<w:p>")
		end := strings.Index(xml[i:], "</w:p>")
		if start >= 0 && end >= 0 && strings.Contains(xml[start:i+end], "<w:tab/>") {
			t.Error("ИБ must not sit on the ОСМОТР line")
		}
	}
	if !strings.Contains(xml, "Весова Ксения Юрьевна") {
		t.Error("doctor signature missing")
	}
	if !strings.Contains(xml, `<w:u w:val="single"/>`) {
		t.Error("10-day date and signature must be underlined")
	}
	if !runHasBold(xml, "Жалобы") {
		t.Error("10-day section labels must stay bold")
	}
}

func TestExportDOCX_MissingSignaturesKeepPlaceholders(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР\n" +
			"лечащим врачом совместно с заведующим отделением\n" +
			"Жалобы: не предъявляет\n" +
			"Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись\n" +
			"[ФИО_ВРАЧА]\n" +
			"Фамилия, имя, отчество (при наличии) заведующего отделением, подпись\n" +
			"[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]",
	}
	data, err := New().Export(context.Background(), FormatDOCX, doc)
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	xml := docxDocumentXML(t, data)
	for _, ph := range []string{"[ФИО_ВРАЧА]", "[ДОЛЖНОСТЬ_ВРАЧА]", "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]", "[ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ]", "[ЛУ]"} {
		if !strings.Contains(xml, ph) {
			t.Errorf("missing placeholder %s", ph)
		}
	}
	if !strings.Contains(xml, "[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]") {
		t.Error("10-day doctor line must be [ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]")
	}
	if !strings.Contains(xml, "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ], [ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ], [ЛУ]") {
		t.Error("head line must be [ФИО_ЗАВ_ОТДЕЛЕНИЕМ], [ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ], [ЛУ]")
	}
	if strings.Contains(xml, "____") {
		t.Error("unresolved signatures must not become underscores")
	}
}

func TestExportDOCX_KeepsCaseNoWithoutPlaceholder(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
		Content: "ИБ №[НОМЕР_ИБ]\n" +
			"ОСМОТР\n" +
			"лечащим врачом совместно с заведующим отделением\n" +
			"Жалобы: не предъявляет\n",
	}
	data, err := New().Export(context.Background(), FormatDOCX, doc)
	if err != nil {
		t.Fatalf("export docx: %v", err)
	}
	xml := docxDocumentXML(t, data)
	if strings.Contains(xml, "[НОМЕР_ИБ]") {
		t.Error("unresolved case-number placeholder must not appear in Word")
	}
	if !strings.Contains(xml, "ИБ №") {
		t.Error("ИБ № must stay at the top of the 10-day exam even without a number")
	}
	if !strings.Contains(xml, "ОСМОТР") {
		t.Error("title must stay")
	}
}

func TestExportBatchDOCX_BlankLineBetweenExams(t *testing.T) {
	ten := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР\nлечащим врачом совместно с заведующим отделением\n" +
			"Жалобы: не предъявляет\n",
	}
	data, err := New().ExportBatch(context.Background(), FormatDOCX, []Document{ten, sampleDoc()})
	if err != nil {
		t.Fatalf("export batch: %v", err)
	}
	xml := docxDocumentXML(t, data)
	if strings.Contains(xml, `w:br w:type="page"`) {
		t.Error("exams must not be separated by a page break")
	}
	if !strings.Contains(xml, "Осмотр лечащим врачом") {
		t.Error("daily exam missing from batch")
	}
	tenAt := strings.Index(xml, "не предъявляет")
	dailyAt := strings.Index(xml, "Осмотр лечащим врачом")
	if tenAt < 0 || dailyAt < 0 || dailyAt < tenAt {
		t.Fatal("expected 10-day exam before the daily exam")
	}
	if !strings.Contains(xml[tenAt:dailyAt], "</w:p><w:p>") {
		t.Error("expected one blank paragraph between exams")
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
