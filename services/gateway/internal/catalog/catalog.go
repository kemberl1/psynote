// Package catalog serves the static reference data the frontend needs to start
// building UI before persistence-backed configuration exists:
//
//   - document types (docs/07 §3, GET /api/v1/document-types);
//   - questionnaire JSON schema per document type (docs/06, docs/07 §3,
//     GET /api/v1/questionnaire?document_type=...).
//
// РЕШЕНИЕ ПО docs (где живёт схема опросника):
// docs/05 §2.2 предусматривает таблицу questionnaire_schema (версионируемый
// JSONB), а docs/06 §3 фиксирует САМ ФОРМАТ схемы. RAG-сервис
// (services/rag/app/questionnaire.py) НЕ публикует схему для фронта — он лишь
// маппит ответы в промпт. Поэтому на Этапе 5 gateway отдаёт КАНОНИЧЕСКУЮ
// статическую схему из этого пакета (источник правды до наполнения таблицы
// questionnaire_schema на следующих этапах). Это даёт фронту (Этап 6/7)
// достаточно, чтобы строить динамический опросник, и согласуется с docs/06 §4–5.
// Данные здесь — чистый конфиг, без ПДн (docs/05 §1).
package catalog

// DocumentType is one entry of GET /api/v1/document-types (docs/07 §3).
type DocumentType struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	IsActive bool   `json:"is_active"`
}

// Option is a select/multiselect option (docs/06 §3). prompt is the clinical
// formulation used downstream by RAG; the frontend renders label.
type Option struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Prompt string `json:"prompt,omitempty"`
}

// Conditional describes which questions to reveal for a given value (docs/06 §3).
type Conditional struct {
	IfValue string   `json:"if_value"`
	Show    []string `json:"show"`
}

// Question is one questionnaire item (docs/06 §3).
type Question struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Type        string        `json:"type"` // select | multiselect | text | number | boolean
	Required    bool          `json:"required"`
	AllowCustom bool          `json:"allow_custom"`
	Default     any           `json:"default,omitempty"`
	Options     []Option      `json:"options,omitempty"`
	Conditional []Conditional `json:"conditional,omitempty"`
}

// Schema is the questionnaire payload for one document type (docs/07 §3).
type Schema struct {
	DocumentType string     `json:"document_type"`
	Version      int        `json:"version"`
	Questions    []Question `json:"questions"`
}

// schemaVersion is the canonical version of the static schema served here.
// (docs/07 §3 returns meta.version = this.)
const schemaVersion = 1

// DocumentTypes mirrors deploy/initdb/02_seed.sql (docs/05 §2.2, docs/07 §3).
func DocumentTypes() []DocumentType {
	return []DocumentType{
		{Code: "daily", Title: "Ежедневный дневник", IsActive: true},
		{Code: "exam_10d", Title: "Осмотр (раз в 10 дней)", IsActive: true},
	}
}

// IsKnownDocumentType reports whether code is a supported document type.
func IsKnownDocumentType(code string) bool {
	for _, dt := range DocumentTypes() {
		if dt.Code == code {
			return true
		}
	}
	return false
}

// Questionnaire returns the schema for a document type, or (nil,false) when the
// type is unknown.
func Questionnaire(docType string) (*Schema, bool) {
	switch docType {
	case "daily":
		return &Schema{DocumentType: "daily", Version: schemaVersion, Questions: dailyQuestions()}, true
	case "exam_10d":
		// Расширенный = базовые вопросы ежедневного + разделы осмотра (docs/06 §5).
		qs := append(dailyQuestions(), examQuestions()...)
		return &Schema{DocumentType: "exam_10d", Version: schemaVersion, Questions: qs}, true
	default:
		return nil, false
	}
}

// dailyQuestions builds the daily diary questionnaire (docs/06 §4).
func dailyQuestions() []Question {
	return []Question{
		{
			ID: "dynamics", Label: "Динамика состояния", Type: "select",
			Required: true, AllowCustom: true, Default: "no_change",
			Options: []Option{
				{Value: "no_change", Label: "без существенных изменений", Prompt: "Динамика состояния: без существенных изменений."},
				{Value: "positive", Label: "с положительной динамикой", Prompt: "Состояние с положительной динамикой."},
				{Value: "slight_improvement", Label: "с незначительным улучшением", Prompt: "Состояние с незначительным улучшением."},
				{Value: "worsening", Label: "с ухудшением", Prompt: "Состояние с ухудшением."},
				{Value: "stable_positive", Label: "со стойкой положительной динамикой", Prompt: "Состояние со стойкой положительной динамикой."},
			},
		},
		{
			ID: "productive_symptoms", Label: "Психопродуктивная симптоматика", Type: "select",
			Required: true, AllowCustom: false, Default: "not_detected",
			Options: []Option{
				{Value: "not_detected", Label: "не выявлена", Prompt: "Психопродуктивная симптоматика не выявлена."},
				{Value: "detected", Label: "выявлена", Prompt: "Выявлена психопродуктивная симптоматика."},
			},
			Conditional: []Conditional{{IfValue: "detected", Show: []string{"productive_symptoms_detail"}}},
		},
		{
			ID: "productive_symptoms_detail", Label: "Характер психопродуктивной симптоматики", Type: "text",
			Required: false, AllowCustom: true,
		},
		{
			ID: "mood", Label: "Фон настроения", Type: "select",
			Required: true, AllowCustom: true, Default: "even",
			Options: []Option{
				{Value: "even", Label: "ровный, без снижения", Prompt: "Фон настроения ровный, без снижения."},
				{Value: "lowered", Label: "снижен", Prompt: "Настроение снижено."},
				{Value: "unstable", Label: "неустойчивый", Prompt: "Фон настроения неустойчивый."},
				{Value: "dysphoric", Label: "с оттенком дисфории", Prompt: "Фон настроения с оттенком дисфории."},
				{Value: "elevated", Label: "ситуационно повышен", Prompt: "Настроение ситуационно повышено."},
			},
			Conditional: []Conditional{
				{IfValue: "lowered", Show: []string{"mood_detail"}},
				{IfValue: "unstable", Show: []string{"mood_detail"}},
			},
		},
		{
			ID: "mood_detail", Label: "Детали настроения", Type: "multiselect",
			Required: false, AllowCustom: true,
			Options: []Option{
				{Value: "tearfulness", Label: "плаксивость", Prompt: "плаксивость"},
				{Value: "anxiety", Label: "тревога", Prompt: "тревога"},
				{Value: "irritability", Label: "раздражительность", Prompt: "раздражительность"},
				{Value: "melancholy", Label: "тоскливость", Prompt: "тоскливость"},
				{Value: "lability", Label: "эмоциональная лабильность", Prompt: "эмоциональная лабильность"},
			},
		},
		{
			ID: "behavior", Label: "Поведение и режим", Type: "select",
			Required: true, AllowCustom: true, Default: "ordered",
			Options: []Option{
				{Value: "ordered", Label: "упорядочен, режим соблюдает", Prompt: "Поведение упорядоченное, режим соблюдает."},
				{Value: "minor_remarks", Label: "режим на негрубых замечаниях", Prompt: "Режим соблюдает на негрубых замечаниях."},
				{Value: "violates", Label: "нарушает режим", Prompt: "Нарушает режим отделения."},
				{Value: "restless", Label: "двигательно беспокоен", Prompt: "Двигательно беспокоен."},
			},
			Conditional: []Conditional{{IfValue: "violates", Show: []string{"behavior_detail"}}},
		},
		{
			ID: "behavior_detail", Label: "Характер нарушений режима", Type: "text",
			Required: false, AllowCustom: true,
		},
		{
			ID: "contact", Label: "Общение и контакт", Type: "multiselect",
			Required: false, AllowCustom: true,
			Options: []Option{
				{Value: "productive", Label: "доступен продуктивному контакту", Prompt: "доступен продуктивному контакту"},
				{Value: "selective_children", Label: "общается с детьми избирательно", Prompt: "общается с детьми избирательно"},
				{Value: "isolated", Label: "держится обособленно", Prompt: "держится обособленно"},
				{Value: "polite_staff", Label: "с персоналом вежлив", Prompt: "с персоналом вежлив"},
				{Value: "negativistic", Label: "негативистичен", Prompt: "негативистичен"},
			},
		},
		{
			ID: "sleep", Label: "Сон", Type: "select",
			Required: true, AllowCustom: true, Default: "not_disturbed",
			Options: []Option{
				{Value: "not_disturbed", Label: "не нарушен", Prompt: "Сон не нарушен."},
				{Value: "hard_to_fall_asleep", Label: "трудности засыпания", Prompt: "Сон с трудностями засыпания."},
				{Value: "superficial", Label: "поверхностный", Prompt: "Сон поверхностный."},
				{Value: "sufficient", Label: "достаточен", Prompt: "Сон достаточный."},
			},
		},
		{
			ID: "appetite", Label: "Аппетит", Type: "select",
			Required: true, AllowCustom: true, Default: "preserved",
			Options: []Option{
				{Value: "preserved", Label: "сохранён", Prompt: "Аппетит сохранён."},
				{Value: "decreased", Label: "снижен", Prompt: "Аппетит снижен."},
				{Value: "selective", Label: "избирательный", Prompt: "Аппетит избирательный."},
				{Value: "increased", Label: "повышен", Prompt: "Аппетит повышен."},
			},
		},
		{
			ID: "tolerance", Label: "Переносимость терапии", Type: "select",
			Required: true, AllowCustom: true, Default: "good",
			Options: []Option{
				{Value: "good", Label: "переносит хорошо", Prompt: "Терапию переносит хорошо."},
				{Value: "satisfactory", Label: "удовлетворительно", Prompt: "Терапию переносит удовлетворительно."},
				{Value: "adverse", Label: "есть нежелательные явления", Prompt: "Отмечаются нежелательные явления на фоне терапии."},
				{Value: "none", Label: "терапию не получает", Prompt: "Терапию не получает."},
			},
			Conditional: []Conditional{{IfValue: "adverse", Show: []string{"adverse_detail"}}},
		},
		{
			ID: "adverse_detail", Label: "Нежелательные явления", Type: "text",
			Required: false, AllowCustom: true,
		},
		{
			ID: "complaints", Label: "Жалобы", Type: "select",
			Required: true, AllowCustom: true, Default: "none",
			Options: []Option{
				{Value: "none", Label: "не предъявляет", Prompt: "Жалоб активно не предъявляет."},
				{Value: "cannot_formulate", Label: "самостоятельно не формирует", Prompt: "Жалобы самостоятельно не формирует."},
				{Value: "present", Label: "есть жалобы", Prompt: "Предъявляет жалобы."},
			},
			Conditional: []Conditional{{IfValue: "present", Show: []string{"complaints_detail"}}},
		},
		{
			ID: "complaints_detail", Label: "Какие жалобы", Type: "text",
			Required: false, AllowCustom: true,
		},
		{
			ID: "events_detail", Label: "События дня", Type: "text",
			Required: false, AllowCustom: true,
		},
	}
}

// examQuestions builds the extra sections of the 10-day exam (docs/06 §5).
func examQuestions() []Question {
	return []Question{
		{
			ID: "anamnesis_disease", Label: "Анамнез заболевания (дополнения)", Type: "select",
			Required: false, AllowCustom: true, Default: "no_additions",
			Options: []Option{
				{Value: "no_additions", Label: "без дополнений", Prompt: "Анамнез заболевания: без дополнений."},
				{Value: "present", Label: "есть дополнения", Prompt: "Имеются дополнения к анамнезу заболевания."},
			},
			Conditional: []Conditional{{IfValue: "present", Show: []string{"anamnesis_detail"}}},
		},
		{ID: "anamnesis_detail", Label: "Дополнения к анамнезу", Type: "text", Required: false, AllowCustom: true},
		{
			ID: "physical_status", Label: "Физикальный статус", Type: "select",
			Required: false, AllowCustom: true, Default: "unremarkable",
			Options: []Option{
				{Value: "unremarkable", Label: "без особенностей", Prompt: "Физикальное исследование: состояние удовлетворительное, без особенностей."},
				{Value: "changes", Label: "есть изменения", Prompt: "В физикальном статусе отмечаются изменения."},
			},
			Conditional: []Conditional{{IfValue: "changes", Show: []string{"physical_detail"}}},
		},
		{ID: "physical_detail", Label: "Физикальный статус (изменения)", Type: "text", Required: false, AllowCustom: true},
		{
			ID: "neuro_status", Label: "Неврологический статус", Type: "select",
			Required: false, AllowCustom: true, Default: "no_acute",
			Options: []Option{
				{Value: "no_acute", Label: "без острой неврологической симптоматики", Prompt: "Неврологический статус: без острой неврологической симптоматики."},
				{Value: "detailed_normal", Label: "развёрнутый нормальный блок", Prompt: "Неврологический статус: очаговой и менингеальной симптоматики не выявлено."},
				{Value: "changes", Label: "есть изменения", Prompt: "В неврологическом статусе отмечаются изменения."},
			},
			Conditional: []Conditional{{IfValue: "changes", Show: []string{"neuro_detail"}}},
		},
		{ID: "neuro_detail", Label: "Неврологический статус (изменения)", Type: "text", Required: false, AllowCustom: true},
		{
			ID: "criticism", Label: "Критика к состоянию", Type: "select",
			Required: false, AllowCustom: true, Default: "formal",
			Options: []Option{
				{Value: "absent", Label: "отсутствует", Prompt: "Критика к своему состоянию отсутствует."},
				{Value: "formal", Label: "формальная", Prompt: "Критика к состоянию формальная."},
				{Value: "forming", Label: "формируется", Prompt: "Критика к состоянию формируется."},
				{Value: "preserved", Label: "сохранна", Prompt: "Критика к состоянию сохранна."},
			},
		},
		{
			ID: "suicidal", Label: "Суицидальные тенденции", Type: "select",
			Required: false, AllowCustom: false, Default: "not_detected",
			Options: []Option{
				{Value: "not_detected", Label: "не выявлены", Prompt: "Суицидальных тенденций не выявлено."},
				{Value: "detected", Label: "выявлены", Prompt: "Выявлены суицидальные тенденции."},
			},
		},
		{ID: "diagnosis", Label: "Основное заболевание (МКБ-10)", Type: "text", Required: false, AllowCustom: true},
		{
			ID: "syndrome", Label: "Синдром", Type: "select",
			Required: false, AllowCustom: true,
			Options: []Option{
				{Value: "anxiety_depressive", Label: "тревожно-депрессивный", Prompt: "тревожно-депрессивный синдром"},
				{Value: "psychopathic", Label: "психопатоподобный", Prompt: "психопатоподобный синдром"},
				{Value: "emotional_volitional", Label: "эмоционально-волевой неустойчивости", Prompt: "синдром эмоционально-волевой неустойчивости"},
				{Value: "anxious", Label: "тревожный", Prompt: "тревожный синдром"},
				{Value: "asthenic", Label: "астенический", Prompt: "астенический синдром"},
			},
		},
		{ID: "comorbidities", Label: "Сопутствующие заболевания", Type: "text", Required: false, AllowCustom: true},
		{ID: "prescriptions", Label: "Назначения", Type: "text", Required: false, AllowCustom: true},
		{ID: "interventions_detail", Label: "Выполненные вмешательства", Type: "text", Required: false, AllowCustom: true},
		{
			ID: "period_dynamics", Label: "Динамика за период (эпикриз)", Type: "select",
			Required: false, AllowCustom: true, Default: "improvement",
			Options: []Option{
				{Value: "improvement", Label: "с улучшением в условиях отделения", Prompt: "Психическое состояние с улучшением в условиях отделения."},
				{Value: "slight_improvement", Label: "с незначительным улучшением", Prompt: "Психическое состояние с незначительным улучшением."},
				{Value: "no_improvement", Label: "без заметного улучшения", Prompt: "Психическое состояние без заметного улучшения."},
				{Value: "no_change", Label: "без существенных изменений", Prompt: "Психическое состояние без существенных изменений."},
			},
		},
	}
}
