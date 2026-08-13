package export

import (
	"strings"
	"testing"
	"time"
)

func TestTransformDaily_KeepsTemplateLikeUI(t *testing.T) {
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
			"План обследования (дополнения к плану): Без изменений\n" +
			"Лечащий врач: [ФИО_ВРАЧА]",
		Substitutions: map[string]string{"[ФИО_ВРАЧА]": "Иванова И.И."},
	}

	out := transformContent(doc, doc.Substitutions)

	if !strings.Contains(out, "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ") {
		t.Error("daily export must keep the same header as UI")
	}
	if !strings.Contains(out, "Психический статус:") {
		t.Error("psychiatric section must keep its title")
	}
	if !strings.Contains(out, "31.08.2025") {
		t.Errorf("date placeholder must be filled: %q", out)
	}
	if !strings.Contains(out, "время: 10:00") {
		t.Errorf("exam time must be 10:00, got: %q", out)
	}
	if strings.Contains(out, "14:30") {
		t.Errorf("must not use generation clock: %q", out)
	}
	if strings.Contains(out, "[ФИО_ВРАЧА]") {
		t.Errorf("doctor placeholder must be filled: %q", out)
	}
	if !strings.Contains(out, "Иванова И.И.") {
		t.Error("missing substituted doctor name")
	}
	if strings.Contains(strings.ToLower(out), "жалобы") {
		t.Errorf("empty complaints must be dropped: %q", out)
	}
	if strings.Contains(strings.ToLower(out), "без дополнений") {
		t.Errorf("empty anamnesis must be dropped: %q", out)
	}
	if strings.Contains(strings.ToLower(out), "без изменений") {
		t.Errorf("plan «без изменений» must be dropped: %q", out)
	}
	if !strings.Contains(out, "F32.10") {
		t.Error("diagnosis must stay")
	}
	if !strings.Contains(out, "ИБ №") {
		t.Error("case-number line must stay like UI")
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
	if !strings.Contains(out, "Психический статус:") {
		t.Error("kept section must retain its title")
	}
}

func TestTransformDaily_OrphanBezIzmeneniyDropped(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"Психический статус: Сознание ясное.\n" +
			"План обследования (дополнения к плану):\n" +
			"Без изменений.\n" +
			"Без изменений\n",
	}
	out := transformContent(doc, nil)
	low := strings.ToLower(out)
	if strings.Contains(low, "без изменений") {
		t.Errorf("orphan «Без изменений» must not appear without a section: %q", out)
	}
	if strings.Contains(out, "План обследования") {
		t.Errorf("empty plan section must be dropped entirely: %q", out)
	}
	if !strings.Contains(out, "Психический статус:") {
		t.Errorf("status must remain: %q", out)
	}
}

func TestTransformExam10d_MarkdownLabels(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 6, 26, 18, 53, 0, 0, time.UTC),
		Content: "ИБ №[НОМЕР_ИБ]\n" +
			"ОСМОТР лечащим врачом совместно с заведующим отделением\n" +
			"[ДАТА] время: [ВРЕМЯ]\n\n" +
			"**Жалобы:** Активно жалоб не предъявляет.\n" +
			"**Психический статус:** Сознание не помрачено. Настроение сниженное.\n" +
			"**Диагноз:**\n" +
			"**Основное заболевание:** F50.0 Нервная анорексия.\n" +
			"**Этапный эпикриз:**\n" +
			"За прошедший период состояние с отрицательной динамикой.\n" +
			"Лечащий врач: [ФИО_ВРАЧА]",
		Substitutions: map[string]string{"[ФИО_ВРАЧА]": "Иванова И.И."},
	}
	out := transformContent(doc, doc.Substitutions)
	if strings.Contains(out, "**") {
		t.Errorf("markdown stars must be stripped: %q", out)
	}
	if strings.Contains(out, "Жалобы") {
		t.Errorf("«не предъявляет» complaints must be dropped: %q", out)
	}
	if !strings.Contains(out, "Психический статус:") {
		t.Error("missing psychiatric section from markdown export")
	}
	if !strings.Contains(out, "F50.0") {
		t.Error("missing diagnosis from markdown export")
	}
	if !strings.Contains(out, "отрицательной динамикой") {
		t.Error("missing epicrisis body from markdown export")
	}
	if !strings.Contains(out, "ОСМОТР лечащим врачом совместно с заведующим отделением") {
		t.Error("exam header must match UI, not a rebuilt corpus title")
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
	if !strings.Contains(out, "ИБ №12345") {
		t.Errorf("case number = %q", out)
	}
	if !strings.Contains(out, "ОСМОТР лечащим врачом совместно с заведующим отделением") {
		t.Errorf("title missing in %q", out)
	}
	if !strings.Contains(out, "08.09.2025") {
		t.Errorf("date placeholder not filled: %q", out)
	}
	if !strings.Contains(out, "Жалобы: на сохраняющуюся тревогу") {
		t.Errorf("real complaints must stay: %q", out)
	}
}

func TestBuildDocLines_DailyKeepsExamHeader(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2025, 8, 31, 0, 0, 0, 0, time.UTC),
		Content:          sampleContent,
	}
	lines := buildDocLines(doc)
	foundTitle := false
	foundStatus := false
	for _, l := range lines {
		if l.kind == kindTitle && strings.Contains(strings.ToUpper(l.text), "ОСМОТР") {
			foundTitle = true
		}
		if l.kind == kindLabelValue && strings.Contains(l.label, "Психический статус") {
			foundStatus = true
		}
	}
	if !foundTitle {
		t.Error("daily export must keep ОСМОТР ЛЕЧАЩИМ ВРАЧОМ like UI")
	}
	if !foundStatus {
		t.Error("expected kindLabelValue for psychiatric status")
	}
}

func TestTransformDaily_DropsBoilerplateInterventions(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"Психический статус: Сознание ясное.\n" +
			"Выполнены медицинские вмешательства: Осмотр лечащим врачом.\n",
	}
	out := transformContent(doc, nil)
	if !strings.Contains(out, "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ") {
		t.Errorf("header must stay: %q", out)
	}
	if strings.Contains(out, "вмешательства") {
		t.Errorf("boilerplate interventions must be dropped: %q", out)
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

func TestClassifyLine_UnknownColonStaysPlain(t *testing.T) {
	l := classifyLine("31.08.2025 время: 14:30")
	if l.kind == kindLabelValue {
		t.Errorf("must not treat date line as a section label %q", l.label)
	}
}

func TestDiaryStamp_PrefersTitleDateAndFixedTime(t *testing.T) {
	date, clock := DiaryStamp(
		"День 8 · 27.07.2026 · Ежедневный осмотр",
		nil,
		time.Date(2026, 8, 13, 14, 30, 0, 0, time.UTC),
	)
	if date != "27.07.2026" {
		t.Errorf("date = %q, want 27.07.2026 from title", date)
	}
	if clock != "10:00" {
		t.Errorf("clock = %q, want 10:00", clock)
	}

	date, clock = DiaryStamp(
		"Ежедневный осмотр",
		map[string]any{"diary_date": "2026-08-01"},
		time.Date(2026, 8, 13, 9, 1, 0, 0, time.UTC),
	)
	if date != "01.08.2026" || clock != "10:00" {
		t.Errorf("got %s %s, want 01.08.2026 10:00", date, clock)
	}

	doc := Document{
		Title:       "День 11 · 30.07.2026 · Ежедневный осмотр",
		GeneratedAt: time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC),
		Content:     "[ДАТА] время: [ВРЕМЯ]\nЛечащий врач: [ФИО_ВРАЧА]",
		Substitutions: map[string]string{
			"[ДАТА]":     "13.08.2026",
			"[ВРЕМЯ]":    "11:05",
			"[ФИО_ВРАЧА]": "Врач Т.",
		},
	}
	out := transformContent(doc, doc.Substitutions)
	if !strings.Contains(out, "30.07.2026 время: 10:00") {
		t.Errorf("batch day must keep its own date at 10:00: %q", out)
	}
	if strings.Contains(out, "13.08.2026") || strings.Contains(out, "11:05") {
		t.Errorf("must ignore client generate-now stamp: %q", out)
	}
	if !strings.Contains(out, "Врач Т.") {
		t.Errorf("doctor name still applies: %q", out)
	}
}
