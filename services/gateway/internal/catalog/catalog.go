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
// маппит ответы в промпт. Поэтому gateway отдаёт КАНОНИЧЕСКУЮ статическую схему
// из этого пакета (источник правды до наполнения таблицы questionnaire_schema
// на следующих этапах). Это даёт фронту достаточно, чтобы строить полностью
// динамический опросник, и согласуется с docs/06 §4–5.
//
// Этап 7: дерево вопросов доведено до docs/06 — добавлены каскадные условные
// вопросы (dynamics→ухудшение, sleep→характер, events→детали), multiselect с
// «своим вариантом» (allow_custom), расширенный психический статус осмотра,
// сопутствующие/вмешательства/выписка. Контракт расширен обратносовместимо
// полями group (логическая группировка, docs/08 §5.1) и help (подсказка).
//
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
// Каскад поддержан: условный вопрос сам может иметь conditional. Множественные
// триггеры одного дочернего вопроса задаются несколькими записями Conditional
// (по одной на каждое if_value) — это обратносовместимо с форматом docs/06 §3.
type Conditional struct {
	IfValue string   `json:"if_value"`
	Show    []string `json:"show"`
}

// Question is one questionnaire item (docs/06 §3).
//
// Group — логическая секция для группировки в UI (docs/08 §5.1); условные
// дочерние вопросы наследуют группу родителя в рендере. Help — короткая
// подсказка под вопросом. Оба поля опциональны и обратносовместимы.
type Question struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Type        string        `json:"type"` // select | multiselect | text | number | boolean
	Required    bool          `json:"required"`
	AllowCustom bool          `json:"allow_custom"`
	Default     any           `json:"default,omitempty"`
	Group       string        `json:"group,omitempty"`
	Help        string        `json:"help,omitempty"`
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
// (docs/07 §3 returns meta.version = this.) Bumped to 2 on Stage 7 (полное
// дерево условных вопросов + «свой вариант» для multiselect + группы).
const schemaVersion = 2

// Группы опросника (docs/08 §5.1 — «Состояние · Поведение · Сон/Аппетит · …»).
const (
	grpState     = "Состояние"
	grpBehavior  = "Поведение и контакт"
	grpSleep     = "Сон и аппетит"
	grpTherapy   = "Терапия и жалобы"
	grpEvents    = "События дня"
	grpAnamnesis = "Жалобы и анамнез"
	grpSomatic   = "Соматический и неврологический статус"
	grpPsych     = "Психический статус"
	grpDiagnosis = "Диагноз"
	grpOrders    = "Назначения и вмешательства"
	grpEpicrisis = "Динамика и эпикриз"
)

// DocumentTypes mirrors deploy/initdb/02_seed.sql (docs/05 §2.2, docs/07 §3).
func DocumentTypes() []DocumentType {
	return []DocumentType{
		{Code: "daily", Title: "Ежедневный осмотр", IsActive: true},
		{Code: "exam_10d", Title: "Осмотр за 10 дней", IsActive: true},
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
		// ─── Группа «Состояние» ──────────────────────────────────────────────
		{
			ID: "dynamics", Label: "Динамика состояния", Type: "select",
			Required: true, AllowCustom: true, Default: "no_change", Group: grpState,
			Options: []Option{
				{Value: "no_change", Label: "без существенных изменений", Prompt: "Динамика состояния: без существенных изменений."},
				{Value: "positive", Label: "с положительной динамикой", Prompt: "Состояние с положительной динамикой."},
				{Value: "slight_improvement", Label: "с незначительным улучшением", Prompt: "Состояние с незначительным улучшением."},
				{Value: "worsening", Label: "с ухудшением", Prompt: "Состояние с ухудшением."},
				{Value: "stable_positive", Label: "со стойкой положительной динамикой", Prompt: "Состояние со стойкой положительной динамикой."},
			},
			Conditional: []Conditional{{IfValue: "worsening", Show: []string{"dynamics_detail"}}},
		},
		{
			ID: "dynamics_detail", Label: "В чём проявляется ухудшение", Type: "text",
			Required: false, AllowCustom: true, Group: grpState,
			Help: "Кратко опишите признаки ухудшения (без персональных данных).",
		},
		{
			ID: "productive_symptoms", Label: "Психопродуктивная симптоматика", Type: "select",
			Required: true, AllowCustom: false, Default: "not_detected", Group: grpState,
			Options: []Option{
				{Value: "not_detected", Label: "не выявлена", Prompt: "Психопродуктивная симптоматика не выявлена."},
				{Value: "detected", Label: "выявлена", Prompt: "Выявлена психопродуктивная симптоматика."},
			},
			Conditional: []Conditional{{IfValue: "detected", Show: []string{"productive_symptoms_detail"}}},
		},
		{
			ID: "productive_symptoms_detail", Label: "Характер психопродуктивной симптоматики", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpState,
			Help: "Выберите характер или укажите свой вариант (без персональных данных).",
			Options: []Option{
				{Value: "hallucinatory", Label: "галлюцинаторная", Prompt: "галлюцинаторная симптоматика"},
				{Value: "delusional", Label: "бредовая", Prompt: "бредовая симптоматика"},
				{Value: "illusory", Label: "иллюзорная", Prompt: "иллюзорные расстройства"},
				{Value: "obsessive", Label: "навязчивости", Prompt: "навязчивые расстройства"},
			},
		},
		{
			ID: "mood", Label: "Фон настроения", Type: "select",
			Required: true, AllowCustom: true, Default: "even", Group: grpState,
			Options: []Option{
				{Value: "even", Label: "ровный, без резких колебаний", Prompt: "Фон настроения ровный, без резких колебаний."},
				{Value: "lowered", Label: "снижен", Prompt: "Настроение снижено."},
				{Value: "unstable", Label: "неустойчивый", Prompt: "Фон настроения неустойчивый."},
				{Value: "dysphoric", Label: "с оттенком дисфории", Prompt: "Фон настроения с оттенком дисфории."},
				{Value: "elevated", Label: "ситуационно повышен", Prompt: "Настроение ситуационно повышено."},
			},
			Conditional: []Conditional{
				{IfValue: "lowered", Show: []string{"mood_detail"}},
				{IfValue: "unstable", Show: []string{"mood_detail"}},
				{IfValue: "dysphoric", Show: []string{"mood_detail"}},
			},
		},
		{
			ID: "mood_detail", Label: "Детали настроения", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpState,
			Options: []Option{
				{Value: "tearfulness", Label: "плаксивость", Prompt: "плаксивость"},
				{Value: "anxiety", Label: "тревога", Prompt: "тревога"},
				{Value: "irritability", Label: "раздражительность", Prompt: "раздражительность"},
				{Value: "melancholy", Label: "тоскливость", Prompt: "тоскливость"},
				{Value: "lability", Label: "эмоциональная лабильность", Prompt: "эмоциональная лабильность"},
			},
		},

		// ─── Группа «Поведение и контакт» ────────────────────────────────────
		{
			ID: "behavior", Label: "Поведение и режим", Type: "select",
			Required: true, AllowCustom: true, Default: "ordered", Group: grpBehavior,
			Options: []Option{
				{Value: "ordered", Label: "упорядочен, режим соблюдает", Prompt: "Поведение упорядоченное, режим соблюдает."},
				{Value: "minor_remarks", Label: "режим на негрубых замечаниях", Prompt: "Режим соблюдает на негрубых замечаниях."},
				{Value: "violates", Label: "нарушает режим", Prompt: "Нарушает режим отделения."},
				{Value: "restless", Label: "двигательно беспокоен", Prompt: "Двигательно беспокоен."},
			},
			Conditional: []Conditional{{IfValue: "violates", Show: []string{"behavior_detail"}}},
		},
		{
			ID: "behavior_detail", Label: "Характер нарушений режима", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpBehavior,
			Options: []Option{
				{Value: "conflict", Label: "конфликтность", Prompt: "конфликтность"},
				{Value: "protest", Label: "протестные реакции", Prompt: "протестные реакции"},
				{Value: "refusal", Label: "отказ от режима", Prompt: "отказ от соблюдения режима"},
				{Value: "aggression", Label: "агрессия", Prompt: "агрессивные проявления"},
			},
		},
		{
			ID: "contact", Label: "Общение и контакт", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpBehavior,
			Options: []Option{
				{Value: "calm_distance", Label: "в беседе спокоен, дистанцию соблюдает", Prompt: "в беседе с врачом спокоен, дистанцию соблюдает"},
				{Value: "productive", Label: "доступен продуктивному контакту", Prompt: "доступен продуктивному контакту"},
				{Value: "selective_children", Label: "общается с детьми избирательно", Prompt: "общается с детьми избирательно"},
				{Value: "isolated", Label: "держится обособленно", Prompt: "держится обособленно"},
				{Value: "polite_staff", Label: "с персоналом вежлив", Prompt: "с персоналом вежлив"},
				{Value: "negativistic", Label: "негативистичен", Prompt: "негативистичен"},
				{Value: "does_not_disclose", Label: "переживаний не раскрывает", Prompt: "переживаний в полном объеме не раскрывает"},
				{Value: "staff_remarks", Label: "замечания персонала (без причины)", Prompt: "со слов персонала получал замечания, на них реагировал непродолжительно, пререкался/спорил/повышал голос; вербальной коррекции поддавался слабо, быстро возвращался к исходному рисунку поведения"},
			},
		},

		// ─── Группа «Сон и аппетит» ──────────────────────────────────────────
		{
			ID: "sleep", Label: "Сон", Type: "select",
			Required: true, AllowCustom: true, Default: "not_disturbed", Group: grpSleep,
			Options: []Option{
				{Value: "not_disturbed", Label: "не нарушен", Prompt: "Сон не нарушен."},
				{Value: "hard_to_fall_asleep", Label: "трудности засыпания", Prompt: "Сон с трудностями засыпания."},
				{Value: "superficial", Label: "поверхностный", Prompt: "Сон поверхностный."},
				{Value: "sufficient", Label: "достаточен", Prompt: "Сон достаточный."},
			},
			Conditional: []Conditional{
				{IfValue: "hard_to_fall_asleep", Show: []string{"sleep_detail"}},
				{IfValue: "superficial", Show: []string{"sleep_detail"}},
			},
		},
		{
			ID: "sleep_detail", Label: "Характер нарушения сна", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpSleep,
			Options: []Option{
				{Value: "hard_to_fall_asleep", Label: "трудности засыпания", Prompt: "трудности засыпания"},
				{Value: "frequent_awakenings", Label: "частые пробуждения", Prompt: "частые пробуждения"},
				{Value: "superficial", Label: "поверхностный сон", Prompt: "поверхностный сон"},
				{Value: "no_rest", Label: "отсутствие чувства отдыха", Prompt: "отсутствие чувства отдыха после сна"},
			},
		},
		{
			ID: "appetite", Label: "Аппетит", Type: "select",
			Required: true, AllowCustom: true, Default: "preserved", Group: grpSleep,
			Options: []Option{
				{Value: "preserved", Label: "достаточен, избирателен", Prompt: "Аппетит достаточен, избирателен."},
				{Value: "decreased", Label: "снижен (в период болезни)", Prompt: "Аппетит снижен (в период болезни)."},
				{Value: "selective", Label: "избирателен", Prompt: "Аппетит избирателен."},
				{Value: "increased", Label: "повышен", Prompt: "Аппетит повышен."},
			},
		},

		// ─── Группа «Терапия и жалобы» ───────────────────────────────────────
		{
			ID: "tolerance", Label: "Переносимость терапии", Type: "select",
			Required: true, AllowCustom: true, Default: "good", Group: grpTherapy,
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
			Required: false, AllowCustom: true, Group: grpTherapy,
			Help: "Опишите нежелательные явления (без персональных данных).",
		},
		{
			ID: "complaints", Label: "Жалобы", Type: "select",
			Required: true, AllowCustom: true, Default: "none", Group: grpTherapy,
			Options: []Option{
				{Value: "none", Label: "не предъявляет", Prompt: "Жалобы не предъявляет."},
				{Value: "cannot_formulate", Label: "самостоятельно не формирует", Prompt: "Жалобы самостоятельно не формирует."},
				{Value: "present", Label: "есть жалобы", Prompt: "Предъявляет жалобы."},
			},
			Conditional: []Conditional{{IfValue: "present", Show: []string{"complaints_detail"}}},
		},
		{
			ID: "complaints_detail", Label: "Какие жалобы", Type: "text",
			Required: false, AllowCustom: true, Group: grpTherapy,
			Help: "Опишите жалобы полностью (без персональных данных).",
		},
		{
			ID: "anamnesis_disease", Label: "Анамнез заболевания (дополнения)", Type: "select",
			Required: false, AllowCustom: true, Default: "no_additions", Group: grpTherapy,
			Options: []Option{
				{Value: "no_additions", Label: "без дополнений", Prompt: "Анамнез заболевания: без дополнений."},
				{Value: "present", Label: "есть дополнения", Prompt: "Имеются дополнения к анамнезу заболевания."},
			},
			Conditional: []Conditional{{IfValue: "present", Show: []string{"anamnesis_detail"}}},
		},
		{
			ID: "anamnesis_detail", Label: "Дополнения к анамнезу", Type: "text",
			Required: false, AllowCustom: true, Group: grpTherapy,
			Help: "Опишите дополнения к анамнезу (без персональных данных).",
		},

		// ─── Группа «События дня» ────────────────────────────────────────────
		{
			ID: "events", Label: "События дня", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpEvents,
			Help: "Коррекция терапии, обследования, выходные. Консультации специалистов сюда не включаем.",
			Options: []Option{
				{Value: "therapy_correction", Label: "коррекция терапии", Prompt: "проведена коррекция терапии"},
				{Value: "somatic", Label: "соматическое заболевание", Prompt: "отмечается сопутствующее соматическое заболевание"},
				{Value: "examination", Label: "обследование (ЭКГ/ЭЭГ/УЗИ/лаб.)", Prompt: "выполнено обследование"},
				{Value: "weekend_duty", Label: "выходной / дежурный персонал", Prompt: "детали выходного дня / наблюдения дежурного персонала"},
				{Value: "relative_visit", Label: "визит родственника / прогулка", Prompt: "визит родственника / прогулка (только если реально было)"},
			},
			Conditional: []Conditional{
				{IfValue: "therapy_correction", Show: []string{"events_detail"}},
				{IfValue: "somatic", Show: []string{"events_detail"}},
				{IfValue: "examination", Show: []string{"events_detail"}},
				{IfValue: "weekend_duty", Show: []string{"events_detail", "additional_info"}},
				{IfValue: "relative_visit", Show: []string{"events_detail"}},
			},
		},
		{
			ID: "events_detail", Label: "Детали событий дня", Type: "text",
			Required: false, AllowCustom: true, Group: grpEvents,
			Help: "Изменение дозы, результаты обследований и т.п. (без персональных данных).",
		},
		{
			ID: "additional_info", Label: "Дополнительные сведения", Type: "text",
			Required: false, AllowCustom: true, Group: grpEvents,
			Help: "Выходной день или блок «за период выходных» в понедельник (без персональных данных).",
		},
		{
			ID: "exam_plan", Label: "План обследования", Type: "select",
			Required: false, AllowCustom: true, Default: "no_change", Group: grpEvents,
			Options: []Option{
				{Value: "no_change", Label: "без дополнений", Prompt: "План обследования: без дополнений."},
				{Value: "adjusted", Label: "была корректировка", Prompt: "План обследования: в связи с изменением состояния проведена корректировка."},
			},
			Conditional: []Conditional{{IfValue: "adjusted", Show: []string{"exam_plan_detail"}}},
		},
		{
			ID: "exam_plan_detail", Label: "Причина корректировки плана обследования", Type: "text",
			Required: false, AllowCustom: true, Group: grpEvents,
			Help: "В связи с чем проведена корректировка (без персональных данных).",
		},
		{
			ID: "prescriptions", Label: "Назначения", Type: "select",
			Required: false, AllowCustom: true, Default: "see_list", Group: grpEvents,
			Help: "Только препараты. Режим отделения в назначения не включаем.",
			Options: []Option{
				{Value: "see_list", Label: "препараты по листу назначений", Prompt: "Назначения: лекарственная терапия согласно листу назначений (только препараты, без режима отделения)."},
				{Value: "no_change", Label: "без изменений (только препараты)", Prompt: "Назначения без изменений (только препараты, без режима отделения)."},
			},
		},
	}
}

// examQuestions builds the extra sections of the 10-day exam (docs/06 §5).
func examQuestions() []Question {
	return []Question{
		// Анамнез заболевания — в dailyQuestions (общий для всех режимов).

		// ─── Соматический и неврологический статус ───────────────────────────
		{
			ID: "physical_status", Label: "Физикальный статус", Type: "select",
			Required: false, AllowCustom: true, Default: "unremarkable", Group: grpSomatic,
			Options: []Option{
				{Value: "unremarkable", Label: "без особенностей", Prompt: "Физикальное исследование: состояние удовлетворительное, без особенностей."},
				{Value: "changes", Label: "есть изменения", Prompt: "В физикальном статусе отмечаются изменения."},
			},
			Conditional: []Conditional{{IfValue: "changes", Show: []string{"physical_detail"}}},
		},
		{
			ID: "physical_detail", Label: "Физикальный статус (изменения)", Type: "text",
			Required: false, AllowCustom: true, Group: grpSomatic,
			Help: "Опишите изменения, при необходимости укажите витальные показатели (без персональных данных).",
		},
		{
			ID: "neuro_status", Label: "Неврологический статус", Type: "select",
			Required: false, AllowCustom: true, Default: "no_acute", Group: grpSomatic,
			Options: []Option{
				{Value: "no_acute", Label: "без острой неврологической симптоматики", Prompt: "Неврологический статус: без острой неврологической симптоматики."},
				{Value: "detailed_normal", Label: "развёрнутый нормальный блок", Prompt: "Неврологический статус: очаговой и менингеальной симптоматики не выявлено."},
				{Value: "changes", Label: "есть изменения", Prompt: "В неврологическом статусе отмечаются изменения."},
			},
			Conditional: []Conditional{{IfValue: "changes", Show: []string{"neuro_detail"}}},
		},
		{
			ID: "neuro_detail", Label: "Неврологический статус (изменения)", Type: "text",
			Required: false, AllowCustom: true, Group: grpSomatic,
			Help: "Опишите неврологические изменения (без персональных данных).",
		},

		// ─── Психический статус (E5, docs/06 §5.2) ───────────────────────────
		{
			ID: "orientation", Label: "Ориентировка", Type: "select",
			Required: false, AllowCustom: true, Default: "partial_typical", Group: grpPsych,
			Options: []Option{
				{Value: "partial_typical", Label: "частично (место, время, личность)", Prompt: "Ориентирован(а) частично (в месте, времени, собственной личности)."},
				{Value: "correct", Label: "верно", Prompt: "Ориентирован(а) верно в месте, времени, собственной личности."},
				{Value: "impaired", Label: "нарушена", Prompt: "Ориентировка нарушена."},
			},
		},
		{
			ID: "criticism", Label: "Критика к состоянию", Type: "select",
			Required: false, AllowCustom: true, Default: "formal", Group: grpPsych,
			Options: []Option{
				{Value: "absent", Label: "отсутствует", Prompt: "Критика к своему состоянию отсутствует."},
				{Value: "formal", Label: "формальная", Prompt: "Критика к состоянию формальная."},
				{Value: "conciliatory", Label: "соглашательская", Prompt: "Критика к состоянию соглашательская."},
				{Value: "forming", Label: "формируется", Prompt: "Критика к состоянию формируется."},
				{Value: "preserved", Label: "сохранна", Prompt: "Критика к состоянию сохранна."},
			},
		},
		{
			ID: "thinking", Label: "Мышление", Type: "select",
			Required: false, AllowCustom: true, Default: "no_gross", Group: grpPsych,
			Options: []Option{
				{Value: "no_gross", Label: "без грубых нарушений", Prompt: "Мышление без грубых нарушений."},
				{Value: "concrete", Label: "конкретное", Prompt: "Мышление конкретное."},
				{Value: "detailed", Label: "обстоятельное", Prompt: "Мышление обстоятельное."},
				{Value: "slowed", Label: "замедленное", Prompt: "Мышление замедленное."},
			},
		},
		{
			ID: "attention_memory", Label: "Внимание и память", Type: "select",
			Required: false, AllowCustom: true, Default: "no_gross", Group: grpPsych,
			Options: []Option{
				{Value: "no_gross", Label: "без грубых нарушений", Prompt: "Внимание и память без грубых нарушений."},
				{Value: "reduced", Label: "снижены", Prompt: "Внимание и память снижены."},
				{Value: "exhausted", Label: "истощаемы", Prompt: "Внимание истощаемо."},
			},
		},
		{
			ID: "intellect", Label: "Интеллект", Type: "select",
			Required: false, AllowCustom: true, Default: "age_norm", Group: grpPsych,
			Options: []Option{
				{Value: "age_norm", Label: "возрастная норма", Prompt: "Интеллектуально представляется на уровне возрастной нормы, запас сведений неравномерен. На вопросы из школьной программы отвечает выборочно."},
				{Value: "low_norm", Label: "низкая возрастная норма", Prompt: "Интеллектуально представляется на уровне низкой возрастной нормы, запас сведений неравномерен. На вопросы из школьной программы отвечает выборочно."},
				{Value: "mild_id", Label: "лёгкая УО", Prompt: "Интеллектуально представляется сниженным до уровня легкой УО, запас сведений неравномерен. На вопросы из школьной программы отвечает выборочно."},
				{Value: "reduced", Label: "интеллектуально-мнестически снижен", Prompt: "Интеллектуально-мнестически снижен, запас сведений неравномерен."},
			},
			Conditional: []Conditional{
				{IfValue: "age_norm", Show: []string{"intellect_example"}},
				{IfValue: "low_norm", Show: []string{"intellect_example"}},
				{IfValue: "mild_id", Show: []string{"intellect_example"}},
			},
		},
		{
			ID: "intellect_example", Label: "Пример из школьной программы", Type: "text",
			Required: false, AllowCustom: true, Group: grpPsych,
			Help: "Краткий пример выборочного ответа (без персональных данных).",
		},
		{
			ID: "suicidal", Label: "Суицидальные тенденции", Type: "select",
			Required: false, AllowCustom: false, Default: "not_detected", Group: grpPsych,
			Options: []Option{
				{Value: "not_detected", Label: "не выявлены", Prompt: "Суицидальных тенденций не выявлено."},
				{Value: "detected", Label: "выявлены", Prompt: "Выявлены суицидальные тенденции."},
			},
			Conditional: []Conditional{{IfValue: "detected", Show: []string{"suicidal_detail"}}},
		},
		{
			ID: "suicidal_detail", Label: "Уточнение суицидальных тенденций", Type: "text",
			Required: false, AllowCustom: true, Group: grpPsych,
			Help: "Опишите характер тенденций (без персональных данных).",
		},

		// ─── Диагноз ─────────────────────────────────────────────────────────
		{
			ID: "diagnosis", Label: "Основное заболевание (МКБ-10)", Type: "text",
			Required: false, AllowCustom: true, Group: grpDiagnosis,
			Help: "Код и название по МКБ-10 (напр. F84.0). Без персональных данных.",
		},
		{
			ID: "syndrome", Label: "Синдром", Type: "select",
			Required: false, AllowCustom: true, Group: grpDiagnosis,
			Options: []Option{
				{Value: "behavioral", Label: "поведенческих нарушений", Prompt: "синдром поведенческих нарушений"},
				{Value: "anxious", Label: "тревожный", Prompt: "тревожный синдром"},
				{Value: "depressive", Label: "депрессивный", Prompt: "депрессивный синдром"},
				{Value: "psychomotor_aggression", Label: "психомоторной расторможенности (с агрессией)", Prompt: "синдром психомоторной расторможенности (с агрессией)"},
				{Value: "psychomotor_autoaggression", Label: "психомоторной расторможенности (с аутоагрессией)", Prompt: "синдром психомоторной расторможенности (с аутоагрессией)"},
				{Value: "affective_volitional", Label: "аффективно-волевой неустойчивости", Prompt: "синдром аффективно-волевой неустойчивости"},
				{Value: "psychopathic", Label: "психопатоподобный", Prompt: "психопатоподобный синдром"},
				{Value: "asthenic", Label: "астенический", Prompt: "астенический синдром"},
			},
		},
		{
			ID: "comorbidities", Label: "Сопутствующие заболевания", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpDiagnosis,
			Help: "Выберите из списка или добавьте свой вариант (код/название МКБ).",
			Options: []Option{
				{Value: "j00", Label: "J00 — ОРВИ/острый назофарингит", Prompt: "J00 — острый назофарингит"},
				{Value: "r51", Label: "R51 — головная боль", Prompt: "R51 — головная боль"},
				{Value: "e66_9", Label: "E66.9 — ожирение", Prompt: "E66.9 — ожирение"},
				{Value: "none", Label: "не выявлены", Prompt: "Сопутствующих заболеваний не выявлено"},
			},
		},

		// ─── Вмешательства (без консультаций доп. специалистов) ──────────────
		// Назначения — в dailyQuestions (только препараты).
		{
			ID: "interventions", Label: "Выполненные вмешательства", Type: "multiselect",
			Required: false, AllowCustom: true, Group: grpOrders,
			Help: "Инструментальные/лабораторные. Консультации специалистов — вручную вне генерации.",
			Options: []Option{
				{Value: "ecg", Label: "ЭКГ", Prompt: "ЭКГ"},
				{Value: "eeg", Label: "ЭЭГ", Prompt: "ЭЭГ"},
				{Value: "ultrasound", Label: "УЗИ", Prompt: "УЗИ"},
				{Value: "lab", Label: "лаборатория", Prompt: "лабораторное обследование"},
			},
			Conditional: []Conditional{
				{IfValue: "ecg", Show: []string{"interventions_detail"}},
				{IfValue: "eeg", Show: []string{"interventions_detail"}},
				{IfValue: "ultrasound", Show: []string{"interventions_detail"}},
				{IfValue: "lab", Show: []string{"interventions_detail"}},
			},
		},
		{
			ID: "interventions_detail", Label: "Заключения по вмешательствам", Type: "text",
			Required: false, AllowCustom: true, Group: grpOrders,
			Help: "Краткие заключения/результаты (без персональных данных).",
		},

		// ─── Динамика и эпикриз ──────────────────────────────────────────────
		{
			ID: "period_dynamics", Label: "Динамика за период (эпикриз)", Type: "select",
			Required: false, AllowCustom: true, Default: "improvement", Group: grpEpicrisis,
			Options: []Option{
				{Value: "improvement", Label: "с улучшением в условиях отделения", Prompt: "Психическое состояние с улучшением в условиях отделения."},
				{Value: "slight_improvement", Label: "с незначительным улучшением", Prompt: "Психическое состояние с незначительным улучшением."},
				{Value: "no_improvement", Label: "без заметного улучшения", Prompt: "Психическое состояние без заметного улучшения."},
				{Value: "no_change", Label: "без существенных изменений", Prompt: "Психическое состояние без существенных изменений."},
			},
		},
		{
			ID: "discharge", Label: "Выписка?", Type: "boolean",
			Required: false, AllowCustom: false, Group: grpEpicrisis,
			Help:        "Отметьте «Да», если оформляется выписка — откроется блок заключения.",
			Conditional: []Conditional{{IfValue: "true", Show: []string{"discharge_detail"}}},
		},
		{
			ID: "discharge_detail", Label: "Заключение и рекомендации при выписке", Type: "text",
			Required: false, AllowCustom: true, Group: grpEpicrisis,
			Help: "Рекомендации, что выдано на руки и т.п. (без персональных данных).",
		},
	}
}
