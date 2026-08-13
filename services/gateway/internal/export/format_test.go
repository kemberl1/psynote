package export

import (
	"strings"
	"testing"
)

// TestClassifyLine covers the «строка с двоеточием → метка+значение» principle
// and the header/title/signature special cases.
func TestClassifyLine(t *testing.T) {
	cases := []struct {
		in        string
		wantKind  lineKind
		wantLabel string
		wantValue string
	}{
		{"ИБ №[НОМЕР_ИБ]", kindCaseNo, "", ""},
		{"ОСМОТР", kindExamTitle, "", ""},
		{"ОСМОТР ЛЕЧАЩИМ ВРАЧОМ", kindTitle, "", ""},
		{"[ДАТА] время: [ВРЕМЯ]", kindDateTime, "", ""},
		{"«13» августа 2026 г. время: 10 час. 00 мин.", kindDateTime, "", ""},
		{"31.08.2025 Сознание не помрачено.", kindDailyNarrative, "", ""},
		{"23-25.12.2025 Сознание не помрачено.", kindDailyNarrative, "", ""},
		{"«08» декабря 2025 г.  время: 12 час. 14 мин.", kindDateTime, "", ""},
		{"Жалобы: не предъявляет", kindLabelValue, "Жалобы", "не предъявляет"},
		{"Диагноз:", kindLabelValue, "Диагноз", ""},
		{"Неизвестная произвольная строка без двоеточия", kindPlain, "", ""},
		{"Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись", kindSignatureCaption, "", ""},
		{"Педиатр от 01.09.25: режим отделения", kindConsultNote, "", ""},
		{"Врач-психиатр                                                                     Иванова И.И.", kindDoctorSignature, "", ""},
		{"Лечащий врач: [ФИО_ВРАЧА]", kindDoctorSignature, "", ""},
	}
	for _, c := range cases {
		got := classifyLine(c.in)
		if got.kind != c.wantKind {
			t.Errorf("classifyLine(%q).kind = %d, want %d", c.in, got.kind, c.wantKind)
			continue
		}
		if c.wantKind == kindLabelValue {
			if got.label != c.wantLabel || got.value != c.wantValue {
				t.Errorf("classifyLine(%q) = label %q / value %q, want %q / %q",
					c.in, got.label, got.value, c.wantLabel, c.wantValue)
			}
		}
	}
}

// TestLabelValueColonInValue ensures only the FIRST colon splits label/value
// (e.g. diagnosis codes / times in the value must survive).
func TestLabelValueColonInValue(t *testing.T) {
	l := classifyLine("Назначения: контроль в 10:00 и 18:00")
	if l.kind != kindLabelValue {
		t.Fatalf("kind = %d, want kindLabelValue", l.kind)
	}
	if l.label != "Назначения" {
		t.Errorf("label = %q, want %q", l.label, "Назначения")
	}
	if l.value != "контроль в 10:00 и 18:00" {
		t.Errorf("value = %q", l.value)
	}
}

// TestKnownSectionLabels verifies the real-template labels are recognised.
func TestKnownSectionLabels(t *testing.T) {
	for _, lbl := range []string{
		"Жалобы", "Психический статус", "Соматический статус",
		"Диагноз", "Назначения", "Этапный эпикриз",
		"Дополнительные сведения",
		"Дополнительные сведения о заболевании",
		"Обоснование диагноза (при наличии дополнительных сведений)",
		"План обследования (дополнения к плану)",
		"План лечения (дополнения к плану)",
	} {
		if !isKnownSectionLabel(lbl) {
			t.Errorf("isKnownSectionLabel(%q) = false, want true", lbl)
		}
	}
	if isKnownSectionLabel("Случайная метка") {
		t.Error("isKnownSectionLabel must be false for unknown label")
	}
}

func TestDailyNarrativeSpans_DateOnlyBold(t *testing.T) {
	spans := dailyNarrativeSpans("31.08.2025 Сознание не помрачено. Настроение ровное.")
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2: %+v", len(spans), spans)
	}
	if spans[0].Text != "31.08.2025 " || !spans[0].Bold {
		t.Errorf("date span = %+v", spans[0])
	}
	if spans[1].Bold || !strings.HasPrefix(spans[1].Text, "Сознание") {
		t.Errorf("body span = %+v", spans[1])
	}
}

func TestExamDateTimeSpans_Underlines(t *testing.T) {
	spans := examDateTimeSpans("«08» декабря 2025 г.  время: 12 час. 14 мин.")
	if len(spans) < 5 {
		t.Fatalf("expected underlined fill-ins, got %+v", spans)
	}
	var underlined []string
	for _, s := range spans {
		if s.Underline {
			underlined = append(underlined, s.Text)
		}
	}
	joined := strings.Join(underlined, "|")
	for _, want := range []string{"08", "декабря", "25", "12", "14"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing underline for %q in %q", want, joined)
		}
	}
}

func TestParseDocLines_SignatureValue(t *testing.T) {
	lines := parseDocLines(
		"Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись\n" +
			"Иванова Ирина Ивановна, Врач-психиатр\n",
	)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if lines[0].kind != kindSignatureCaption {
		t.Errorf("first kind = %d", lines[0].kind)
	}
	if lines[1].kind != kindSignatureValue {
		t.Errorf("second kind = %d, want signature value", lines[1].kind)
	}
	if len(lines[1].spans) == 0 || !lines[1].spans[0].Underline {
		t.Errorf("signature value must be underlined: %+v", lines[1].spans)
	}
}
