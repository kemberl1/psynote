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

	if !strings.Contains(out, "Осмотр лечащим врачом") {
		t.Error("daily export must use the MIS daily header")
	}
	if !strings.Contains(out, "Психический статус:") {
		t.Error("psychiatric section must keep its title")
	}
	if !strings.Contains(out, "Дата: 31.08.2025 10:00") {
		t.Errorf("daily date must be numeric: %q", out)
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
	if !strings.Contains(strings.ToLower(out), "жалобы") {
		t.Errorf("complaints line must stay in the MIS form: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "без дополнений") {
		t.Errorf("anamnesis «без дополнений» must stay: %q", out)
	}
	if !strings.Contains(out, "F32.10") {
		t.Error("diagnosis must stay")
	}
	if strings.Contains(out, "ИБ №") {
		t.Error("daily exam must not keep the case-number line")
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

func TestTransformDaily_KeepsPlanDefaults(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"Психический статус: Сознание ясное.\n" +
			"План обследования (дополнения к плану): без дополнений\n" +
			"План лечения (дополнения к плану): без дополнений\n",
	}
	out := transformContent(doc, nil)
	if !strings.Contains(out, "План обследования") {
		t.Errorf("exam plan line must stay: %q", out)
	}
	if !strings.Contains(out, "План лечения") {
		t.Errorf("treatment plan line must stay: %q", out)
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
	if !strings.Contains(out, "Жалобы") {
		t.Errorf("complaints line must stay in the MIS form: %q", out)
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
	if !strings.Contains(out, "ОСМОТР\nлечащим врачом совместно с заведующим отделением") {
		t.Error("exam header must be split into title and subtitle")
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
	if strings.HasSuffix(strings.TrimSpace(out), "ИБ №12345") && strings.Count(out, "ИБ №12345") > 1 {
		t.Errorf("case number must not be repeated under the exam: %q", out)
	}
	if !strings.Contains(out, "ОСМОТР\nлечащим врачом совместно с заведующим отделением") {
		t.Errorf("title missing in %q", out)
	}
	if !strings.Contains(out, "«08» сентября 2025 г.") {
		t.Errorf("official date not filled: %q", out)
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
		if l.kind == kindDailyTitle && strings.Contains(l.text, "Осмотр лечащим врачом") {
			foundTitle = true
		}
		if l.kind == kindLabelValue && strings.Contains(l.label, "Психический статус") {
			foundStatus = true
		}
	}
	if !foundTitle {
		t.Error("daily export must keep Осмотр лечащим врачом")
	}
	if !foundStatus {
		t.Error("expected kindLabelValue for psychiatric status")
	}
}

func TestTransformDaily_KeepsInterventionsLine(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"Психический статус: Сознание ясное.\n" +
			"Выполнены медицинские вмешательства: осмотр врачом-психиатром детским.\n",
	}
	out := transformContent(doc, nil)
	if !strings.Contains(out, "Осмотр лечащим врачом") {
		t.Errorf("header must stay: %q", out)
	}
	if !strings.Contains(out, "вмешательства") {
		t.Errorf("interventions line must stay: %q", out)
	}
}

func TestTransformDaily_RewritesNumericDateHeader(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"13.08.2026 время: 10:45\n" +
			"Психический статус: Сознание ясное.\n",
	}
	out := transformContent(doc, nil)
	if !strings.Contains(out, "Дата: 13.08.2026 10:45") {
		t.Errorf("numeric header must become daily date: %q", out)
	}
	if strings.Contains(out, "13.08.2026 время: 10:45") {
		t.Errorf("old numeric header must not remain: %q", out)
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
	if date != "«27» июля 2026 г." {
		t.Errorf("date = %q, want «27» июля 2026 г. from title", date)
	}
	if clock != "10 час. 00 мин." {
		t.Errorf("clock = %q, want 10 час. 00 мин.", clock)
	}

	date, clock = DiaryStamp(
		"Ежедневный осмотр",
		map[string]any{"diary_date": "2026-08-01"},
		time.Date(2026, 8, 13, 9, 1, 0, 0, time.UTC),
	)
	if date != "«01» августа 2026 г." || clock != "10 час. 00 мин." {
		t.Errorf("got %s %s, want «01» августа 2026 г. 10 час. 00 мин.", date, clock)
	}

	doc := Document{
		Title:       "День 11 · 30.07.2026 · Ежедневный осмотр",
		GeneratedAt: time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC),
		Content:     "[ДАТА] время: [ВРЕМЯ]\nЛечащий врач: [ФИО_ВРАЧА]",
		Substitutions: map[string]string{
			"[ДАТА]":      "13.08.2026",
			"[ВРЕМЯ]":     "11:05",
			"[ФИО_ВРАЧА]": "Врач Т.",
		},
	}
	out := transformContent(doc, doc.Substitutions)
	if !strings.Contains(out, "«30» июля 2026 г. время: 10 час. 00 мин.") {
		t.Errorf("batch day must keep its own official date: %q", out)
	}
	if strings.Contains(out, "13.08.2026") || strings.Contains(out, "11:05") {
		t.Errorf("must ignore client generate-now stamp: %q", out)
	}
	if !strings.Contains(out, "Врач Т.") {
		t.Errorf("doctor name still applies: %q", out)
	}
}

func TestTransform_FixesObviousTypos(t *testing.T) {
	doc := Document{
		GeneratedAt: time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC),
		Content:     "Основное заболевание: F71.18 Сидрос психмоторной расторможенности",
	}
	out := transformContent(doc, nil)
	if strings.Contains(out, "Сидрос") || strings.Contains(out, "психмоторной") {
		t.Errorf("obvious typos must be fixed: %q", out)
	}
	if !strings.Contains(out, "Синдром психомоторной") {
		t.Errorf("want Синдром психомоторной, got %q", out)
	}

	complaints := transformContent(Document{
		GeneratedAt: time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC),
		Content:     "Жалобы: самостоятельно не предъявляет",
	}, nil)
	if strings.Contains(complaints, "самостоятельно") {
		t.Errorf("complaints must not keep «самостоятельно»: %q", complaints)
	}
	if !strings.Contains(complaints, "Жалобы: не предъявляет") {
		t.Errorf("want Жалобы: не предъявляет, got %q", complaints)
	}
}

func TestTransformDaily_SignatureOrderAndSpacing(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n" +
			"Жалобы: не предъявляет\n" +
			"\n" +
			"Анамнез заболевания (дополнения к анамнезу): без дополнений\n" +
			"Анамнез жизни (дополнения к анамнезу): без дополнений\n" +
			"Физикальное исследование, локальный статус (его изменение): Т 36,6\n" +
			"Неврологический статус: без острой неврологической симптоматики\n" +
			"Диагноз:\n" +
			"Лечащий врач: [ФИО_ВРАЧА]",
		Substitutions: map[string]string{
			"[ФИО_ВРАЧА]":       "Иванов Иван Иванович",
			"[ДОЛЖНОСТЬ_ВРАЧА]": "врач-психиатр детский",
		},
	}
	out := transformContent(doc, doc.Substitutions)
	if !strings.Contains(out, "врач-психиатр детский Иванов Иван Иванович") {
		t.Errorf("daily signature order: %q", out)
	}
	if strings.Contains(out, "\n\n") {
		t.Errorf("daily must not keep blank lines between sections: %q", out)
	}
}

func TestTransformExam10d_UnresolvedCaseNoStaysBlankPrefix(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ИБ №[НОМЕР_ИБ]\n" +
			"ОСМОТР\n" +
			"лечащим врачом совместно с заведующим отделением\n" +
			"Жалобы: не предъявляет\n",
	}
	out := transformContent(doc, nil)
	if strings.Contains(out, "[НОМЕР_ИБ]") {
		t.Errorf("placeholder must not remain: %q", out)
	}
	if !strings.Contains(out, "ИБ №") {
		t.Errorf("ИБ № must stay: %q", out)
	}
	if strings.HasSuffix(strings.TrimSpace(out), "ИБ №") && !strings.HasPrefix(strings.TrimSpace(out), "ИБ №") {
		t.Errorf("ИБ № must not hang under the exam: %q", out)
	}
}

func TestTransformExam10d_MissingSignaturesKeepPlaceholders(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР\n" +
			"лечащим врачом совместно с заведующим отделением\n" +
			"Жалобы: не предъявляет\n" +
			"Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись\n" +
			"[ФИО_ВРАЧА]\n" +
			"Фамилия, имя, отчество (при наличии) заведующего отделением, подпись\n" +
			"[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]",
	}
	out := transformContent(doc, nil)
	if !strings.Contains(out, "[ФИО_ВРАЧА]") || !strings.Contains(out, "[ДОЛЖНОСТЬ_ВРАЧА]") {
		t.Errorf("doctor placeholders must stay: %q", out)
	}
	if !strings.Contains(out, "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ], [ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ], [ЛУ]") {
		t.Errorf("head placeholders must stay: %q", out)
	}
	if strings.Contains(out, "____") {
		t.Errorf("unresolved signatures must not become underscores: %q", out)
	}
}

func TestTransformExam10d_DoesNotInsertDailyBlanks(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		GeneratedAt:      time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Content: "ОСМОТР\n" +
			"лечащим врачом совместно с заведующим отделением\n" +
			"Анамнез жизни (дополнения к анамнезу): без дополнений\n" +
			"Неврологический статус (его изменение): без острой неврологической симптоматики\n" +
			"Диагноз:\n",
	}
	out := transformContent(doc, nil)
	if strings.Contains(out, "без дополнений\n\nНеврологический") {
		t.Errorf("10-day must not get daily anamnesis blank: %q", out)
	}
}
