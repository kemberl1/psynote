package anonymizer

// EntityType enumerates the categories of personal / identifying data (ПДн)
// recognised by the pipeline. Each category maps to a typed placeholder so the
// downstream LLM keeps the sentence structure while never seeing real data
// (см. docs/04_anonymization.md §4).
type EntityType string

const (
	// EntityPatient — ФИО пациента (ребёнка). Плейсхолдер [ПАЦИЕНТ].
	EntityPatient EntityType = "patient"
	// EntityDoctor — ФИО врача / медперсонала / члена ВК. Плейсхолдер [ФИО_ВРАЧА].
	EntityDoctor EntityType = "doctor"
	// EntityParent — ФИО родителя / законного представителя. Плейсхолдер [РОДИТЕЛЬ].
	EntityParent EntityType = "parent"
	// EntityPerson — ФИО без явной роли (кандидат-персона). Плейсхолдер [ФИО].
	EntityPerson EntityType = "person"
	// EntityDate — единичная дата. Плейсхолдер [ДАТА].
	EntityDate EntityType = "date"
	// EntityPeriod — диапазон дат. Плейсхолдер [ПЕРИОД].
	EntityPeriod EntityType = "period"
	// EntityTime — время суток. Плейсхолдер [ВРЕМЯ].
	EntityTime EntityType = "time"
	// EntityAge — возраст. Плейсхолдер [ВОЗРАСТ].
	EntityAge EntityType = "age"
	// EntityAddress — адрес проживания. Плейсхолдер [АДРЕС].
	EntityAddress EntityType = "address"
	// EntityInstitution — название учреждения (больница, ЦВЛ, ПНД). Плейсхолдер [УЧРЕЖДЕНИЕ].
	EntityInstitution EntityType = "institution"
	// EntityDocNumber — номер карты / истории болезни / протокола ВК. Плейсхолдер [НОМЕР_ДОКУМЕНТА].
	EntityDocNumber EntityType = "doc_number"
	// EntityPhone — телефонный номер. Плейсхолдер [ТЕЛЕФОН].
	EntityPhone EntityType = "phone"
	// EntityIDDoc — СНИЛС / паспорт / полис. Плейсхолдер [ДОКУМЕНТ].
	EntityIDDoc EntityType = "id_doc"
)

// placeholders maps every entity category to its typed, irreversible placeholder.
// The map of placeholder → real value is NEVER persisted (см. docs/04 §5).
var placeholders = map[EntityType]string{
	EntityPatient:     "[ПАЦИЕНТ]",
	EntityDoctor:      "[ФИО_ВРАЧА]",
	EntityParent:      "[РОДИТЕЛЬ]",
	EntityPerson:      "[ФИО]",
	EntityDate:        "[ДАТА]",
	EntityPeriod:      "[ПЕРИОД]",
	EntityTime:        "[ВРЕМЯ]",
	EntityAge:         "[ВОЗРАСТ]",
	EntityAddress:     "[АДРЕС]",
	EntityInstitution: "[УЧРЕЖДЕНИЕ]",
	EntityDocNumber:   "[НОМЕР_ДОКУМЕНТА]",
	EntityPhone:       "[ТЕЛЕФОН]",
	EntityIDDoc:       "[ДОКУМЕНТ]",
}

// Placeholder returns the typed placeholder string for an entity type.
// Unknown types fall back to a generic, still-non-reversible marker.
func (e EntityType) Placeholder() string {
	if p, ok := placeholders[e]; ok {
		return p
	}
	return "[УДАЛЕНО]"
}

// priority defines which entity wins when two candidate spans overlap.
// Higher value = kept. Person/FIO categories rank highest because a missed
// ФИО is the most dangerous leak (fail-closed bias, docs/04 §1).
func (e EntityType) priority() int {
	switch e {
	case EntityPatient, EntityDoctor, EntityParent, EntityPerson:
		return 100
	case EntityAddress:
		return 90
	case EntityInstitution:
		return 80
	case EntityPeriod:
		return 70
	case EntityDate, EntityTime:
		return 60
	case EntityDocNumber, EntityIDDoc, EntityPhone:
		return 50
	case EntityAge:
		return 40
	default:
		return 10
	}
}

// AllPlaceholders returns the full set of placeholder tokens. Used by the
// validation gate to mask already-anonymized regions before re-scanning.
func AllPlaceholders() []string {
	out := make([]string, 0, len(placeholders))
	for _, v := range placeholders {
		out = append(out, v)
	}
	return out
}
