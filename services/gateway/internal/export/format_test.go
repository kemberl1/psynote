package export

import "testing"

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
		{"«10» апреля 2026 г. время: 16 час.", kindDateTime, "", ""},
		{"31.08.2025 Сознание не помрачено.", kindDailyNarrative, "", ""},
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
		"План обследования (дополнения к плану)",
	} {
		if !isKnownSectionLabel(lbl) {
			t.Errorf("isKnownSectionLabel(%q) = false, want true", lbl)
		}
	}
	if isKnownSectionLabel("Случайная метка") {
		t.Error("isKnownSectionLabel must be false for unknown label")
	}
}
