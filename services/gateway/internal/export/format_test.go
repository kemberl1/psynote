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
		{"ОСМОТР ЛЕЧАЩИМ ВРАЧОМ", kindTitle, "", ""},
		{"[ДАТА] время: [ВРЕМЯ]", kindDateTime, "", ""},
		{"«10» апреля 2026 г. время: 16 час.", kindDateTime, "", ""},
		{"Жалобы: не предъявляет", kindLabelValue, "Жалобы", "не предъявляет"},
		{"Диагноз:", kindLabelValue, "Диагноз", ""},
		{"Неизвестная произвольная строка без двоеточия", kindPlain, "", ""},
		{"Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись", kindSignatureCaption, "", ""},
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

// TestBuildDocLines_UsesBodyHeader keeps the body header when «ОСМОТР …» present
// and does not duplicate it.
func TestBuildDocLines_UsesBodyHeader(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "daily",
		Content:          "ИБ №[НОМЕР_ИБ]\nОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n[ДАТА] время: [ВРЕМЯ]\nЖалобы: нет",
	}
	lines := buildDocLines(doc)
	titleCount := 0
	for _, l := range lines {
		if l.kind == kindTitle {
			titleCount++
		}
	}
	if titleCount != 1 {
		t.Errorf("title count = %d, want 1 (header not duplicated)", titleCount)
	}
}

// TestBuildDocLines_SynthesizesHeader adds a header (placeholders only) when the
// body lacks a title line.
func TestBuildDocLines_SynthesizesHeader(t *testing.T) {
	doc := Document{
		DocumentTypeCode: "exam_10d",
		Content:          "Жалобы: не предъявляет\nДиагноз: F84.1",
	}
	lines := buildDocLines(doc)
	if len(lines) < 4 {
		t.Fatalf("expected synthesized header + body, got %d lines", len(lines))
	}
	if lines[0].kind != kindCaseNo {
		t.Errorf("first synth line kind = %d, want kindCaseNo", lines[0].kind)
	}
	if lines[1].kind != kindTitle {
		t.Errorf("second synth line kind = %d, want kindTitle", lines[1].kind)
	}
	// Privacy: synthesized header carries only placeholders, no real values.
	if lines[0].text != "ИБ №[НОМЕР_ИБ]" {
		t.Errorf("synth case no = %q, want placeholder", lines[0].text)
	}
}
