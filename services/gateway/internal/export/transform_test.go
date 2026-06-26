package export

import (
	"strings"
	"testing"
	"time"
)

func TestTransformDaily_CompactNarrative(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2025, 8, 31, 14, 30, 0, 0, time.UTC),
		Content: "ИБ №[НОМЕР_ИБ]\n" +
			"ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"[ДАТА] время: [ВРЕМЯ]\n\n" +
			"Жалобы: не предъявляет\n" +
			"Анамнез заболевания (дополнения к анамнезу): без дополнений\n" +
			"Психический статус: Сознание не помрачено. Настроение сниженное.\n" +
			"Физикальное исследование, локальный статус (его изменение): Т – 36,6 С.\n" +
			"Неврологический статус: Лицо симметричное.\n" +
			"Диагноз:\n" +
			"Основное заболевание: F32.10\n" +
			"Лечащий врач: [ФИО_ВРАЧА]",
		Substitutions: map[string]string{"[ФИО_ВРАЧА]": "Иванова И.И."},
	}

	out := transformContent(doc, doc.Substitutions)

	if strings.Contains(out, "ОСМОТР") {
		t.Error("daily export must not contain ОСМОТР header")
	}
	if strings.Contains(out, "[ДАТА]") || strings.Contains(out, "[ФИО_ВРАЧА]") {
		t.Errorf("placeholders must be filled or removed: %q", out)
	}
	if !strings.HasPrefix(out, "31.08.2025 Сознание не помрачено") {
		t.Errorf("expected date-prefixed narrative, got: %q", out)
	}
	if !strings.Contains(out, "Физикальное исследование, локальный статус (его изменение):") {
		t.Error("missing physical exam section")
	}
	if !strings.Contains(out, "Неврологический статус (его изменение):") {
		t.Error("missing neuro section with (его изменение)")
	}
	if !strings.Contains(out, "Врач-психиатр") {
		t.Error("missing doctor signature line")
	}
	if strings.Contains(out, "F32.10") {
		t.Error("daily compact export should skip diagnosis block")
	}
}

func TestTransformDaily_SkipsEmptySections(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2025, 9, 1, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"Жалобы: Данных нет\n" +
			"Психический статус: Контакт сохранён.\n",
	}
	out := transformContent(doc, nil)
	if strings.Contains(strings.ToLower(out), "данных нет") {
		t.Errorf("empty sections must be skipped: %q", out)
	}
	if !strings.Contains(out, "Контакт сохранён") {
		t.Errorf("expected psychiatric content: %q", out)
	}
}

func TestTransformExam10d_StructuredHeader(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2025, 9, 8, 10, 5, 0, 0, time.UTC),
		Content: "ИБ №[НОМЕР_ИБ]\n" +
			"ОСМОТР лечащим врачом совместно с заведующим отделением\n" +
			"[ДАТА] время: [ВРЕМЯ]\n" +
			"Жалобы: на сохраняющуюся тревогу\n" +
			"Диагноз:\n" +
			"Основное заболевание: F32.10\n" +
			"Лечащий врач: Иванова И.И.",
		Substitutions: map[string]string{"[НОМЕР_ИБ]": "12345"},
	}
	out := transformContent(doc, doc.Substitutions)
	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected structured exam, got %q", out)
	}
	if lines[0] != "ИБ №12345" {
		t.Errorf("case number line = %q", lines[0])
	}
	if lines[1] != "ОСМОТР" {
		t.Errorf("title = %q, want ОСМОТР", lines[1])
	}
	if lines[2] != "лечащим врачом совместно с заведующим отделением" {
		t.Errorf("subtitle = %q", lines[2])
	}
	if !strings.Contains(lines[3], "сентября 2025") {
		t.Errorf("date line = %q", lines[3])
	}
}

func TestBuildDocLines_DailyNoExamHeader(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2025, 8, 31, 0, 0, 0, 0, time.UTC),
		Content:          sampleContent,
	}
	lines := buildDocLines(doc)
	for _, l := range lines {
		if l.kind == kindTitle || l.kind == kindExamTitle {
			t.Errorf("daily export must not have exam title, got kind %d text %q", l.kind, l.text)
		}
	}
	foundNarrative := false
	for _, l := range lines {
		if l.kind == kindDailyNarrative {
			foundNarrative = true
		}
	}
	if !foundNarrative {
		t.Error("expected kindDailyNarrative in daily export")
	}
}

func TestClassifyLine_ExamTitle(t *testing.T) {
	l := classifyLine("ОСМОТР")
	if l.kind != kindExamTitle {
		t.Errorf("kind = %d, want kindExamTitle", l.kind)
	}
	l = classifyLine("лечащим врачом совместно с заведующим отделением")
	if l.kind != kindExamSubtitle {
		t.Errorf("kind = %d, want kindExamSubtitle", l.kind)
	}
}
