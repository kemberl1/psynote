// Tests for the canonical questionnaire schema (docs/06, docs/07 §3).
// Этап 7: проверяем полноту дерева вопросов, корректность conditional/каскада,
// allow_custom для multiselect и сквозную ссылочную целостность show→id.
package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

// collectIDs returns the set of question ids in a schema.
func collectIDs(s *Schema) map[string]Question {
	m := make(map[string]Question, len(s.Questions))
	for _, q := range s.Questions {
		m[q.ID] = q
	}
	return m
}

func TestQuestionnaireKnownTypes(t *testing.T) {
	for _, dt := range []string{"daily", "exam_10d"} {
		s, ok := Questionnaire(dt)
		if !ok {
			t.Fatalf("Questionnaire(%q) = ok false, want true", dt)
		}
		if s.DocumentType != dt {
			t.Errorf("document_type = %q, want %q", s.DocumentType, dt)
		}
		if s.Version != schemaVersion {
			t.Errorf("version = %d, want %d", s.Version, schemaVersion)
		}
		if len(s.Questions) == 0 {
			t.Errorf("%s: questions empty", dt)
		}
	}
}

func TestQuestionnaireUnknownType(t *testing.T) {
	if _, ok := Questionnaire("nope"); ok {
		t.Fatal("Questionnaire(unknown) = ok true, want false")
	}
}

// Каждый id уникален, conditional.show ссылается на существующий id.
func TestSchemaReferentialIntegrity(t *testing.T) {
	for _, dt := range []string{"daily", "exam_10d"} {
		s, _ := Questionnaire(dt)
		seen := map[string]bool{}
		for _, q := range s.Questions {
			if seen[q.ID] {
				t.Errorf("%s: duplicate question id %q", dt, q.ID)
			}
			seen[q.ID] = true
		}
		ids := collectIDs(s)
		for _, q := range s.Questions {
			for _, c := range q.Conditional {
				if c.IfValue == "" {
					t.Errorf("%s/%s: conditional with empty if_value", dt, q.ID)
				}
				for _, show := range c.Show {
					if _, ok := ids[show]; !ok {
						t.Errorf("%s/%s: conditional shows unknown id %q", dt, q.ID, show)
					}
				}
			}
		}
	}
}

// select/multiselect должны иметь options; if_value должен быть валидной опцией
// для select/multiselect (для boolean допустимы true/false).
func TestOptionsAndConditionalValues(t *testing.T) {
	for _, dt := range []string{"daily", "exam_10d"} {
		s, _ := Questionnaire(dt)
		for _, q := range s.Questions {
			switch q.Type {
			case "select", "multiselect":
				if len(q.Options) == 0 {
					t.Errorf("%s/%s: %s without options", dt, q.ID, q.Type)
				}
				valid := map[string]bool{}
				for _, o := range q.Options {
					valid[o.Value] = true
				}
				for _, c := range q.Conditional {
					if !valid[c.IfValue] {
						t.Errorf("%s/%s: if_value %q is not an option", dt, q.ID, c.IfValue)
					}
				}
			case "boolean":
				for _, c := range q.Conditional {
					if c.IfValue != "true" && c.IfValue != "false" {
						t.Errorf("%s/%s: boolean if_value %q, want true/false", dt, q.ID, c.IfValue)
					}
				}
			}
		}
	}
}

// Дерево daily из docs/06 §4: ключевые условные ветвления присутствуют.
func TestDailyConditionalTree(t *testing.T) {
	s, _ := Questionnaire("daily")
	ids := collectIDs(s)

	type want struct {
		parent, value, child string
	}
	wants := []want{
		{"dynamics", "worsening", "dynamics_detail"},
		{"productive_symptoms", "detected", "productive_symptoms_detail"},
		{"mood", "lowered", "mood_detail"},
		{"mood", "unstable", "mood_detail"},
		{"behavior", "violates", "behavior_detail"},
		{"sleep", "hard_to_fall_asleep", "sleep_detail"},
		{"tolerance", "adverse", "adverse_detail"},
		{"complaints", "present", "complaints_detail"},
		{"events", "therapy_correction", "events_detail"},
	}
	for _, w := range wants {
		q, ok := ids[w.parent]
		if !ok {
			t.Errorf("daily: missing parent %q", w.parent)
			continue
		}
		if !hasConditional(q, w.value, w.child) {
			t.Errorf("daily: %s=%s should show %s", w.parent, w.value, w.child)
		}
	}

	// allow_custom для multiselect деталей (docs/06 §4.2 «+ свой»).
	for _, id := range []string{"mood_detail", "behavior_detail", "sleep_detail", "events"} {
		if !ids[id].AllowCustom {
			t.Errorf("daily/%s: want allow_custom=true", id)
		}
	}
}

// Дерево exam_10d из docs/06 §5: базовый daily + разделы осмотра + каскады.
func TestExamConditionalTree(t *testing.T) {
	s, _ := Questionnaire("exam_10d")
	ids := collectIDs(s)

	// Базовые daily-вопросы унаследованы.
	for _, id := range []string{"dynamics", "mood", "sleep", "complaints"} {
		if _, ok := ids[id]; !ok {
			t.Errorf("exam_10d: missing inherited daily question %q", id)
		}
	}
	// Разделы осмотра присутствуют.
	for _, id := range []string{
		"anamnesis_disease", "physical_status", "neuro_status", "criticism",
		"thinking", "attention_memory", "intellect", "suicidal", "diagnosis",
		"syndrome", "comorbidities", "prescriptions", "interventions",
		"period_dynamics", "discharge",
	} {
		if _, ok := ids[id]; !ok {
			t.Errorf("exam_10d: missing exam question %q", id)
		}
	}
	// Каскады осмотра.
	if !hasConditional(ids["suicidal"], "detected", "suicidal_detail") {
		t.Error("exam_10d: suicidal=detected should show suicidal_detail")
	}
	if !hasConditional(ids["discharge"], "true", "discharge_detail") {
		t.Error("exam_10d: discharge=true should show discharge_detail")
	}
	if !hasConditional(ids["interventions"], "ecg", "interventions_detail") {
		t.Error("exam_10d: interventions=ecg should show interventions_detail")
	}
}

// Схема сериализуется в JSON и обратно без потери ключевых полей (docs/07 §3).
func TestSchemaJSONRoundTrip(t *testing.T) {
	s, _ := Questionnaire("exam_10d")
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Schema
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Questions) != len(s.Questions) {
		t.Errorf("round-trip lost questions: %d != %d", len(back.Questions), len(s.Questions))
	}
	// Проверяем, что group/help/allow_custom доходят до JSON.
	if !strings.Contains(string(raw), `"group"`) {
		t.Error("json missing group field")
	}
	if !strings.Contains(string(raw), `"allow_custom"`) {
		t.Error("json missing allow_custom field")
	}
}

func hasConditional(q Question, value, child string) bool {
	for _, c := range q.Conditional {
		if c.IfValue != value {
			continue
		}
		for _, s := range c.Show {
			if s == child {
				return true
			}
		}
	}
	return false
}
