package anonymizer

import "regexp"

// regexDetector pairs a compiled pattern with the entity type it produces and a
// human-readable name (used only for documentation/logging of categories, never
// of values). This is level 2 of the pipeline (docs/04 §3, уровень 2).
type regexDetector struct {
	name string
	re   *regexp.Regexp
	typ  EntityType
	// group selects which capture group is the actual PII span. 0 = whole match.
	group int
}

// Month names (any case) for textual dates like «06 октября 2025 г.».
const monthAlt = `(?:янв(?:ар[ья])?|фев(?:рал[ья])?|мар(?:та?)?|апр(?:ел[яья])?|ма[йя]|июн[ья]|июл[ья]|авг(?:уст[а]?)?|сен(?:тябр[яья])?|окт(?:ябр[яья])?|ноя(?:бр[яья])?|дек(?:абр[яья])?)`

// buildRegexDetectors returns the ordered list of structured-PII detectors.
// Order matters only for documentation; overlap resolution is handled by the
// span engine, where longer/period spans win over single dates.
func buildRegexDetectors() []regexDetector {
	return []regexDetector{
		// --- ПЕРИОДЫ (диапазоны дат) — раньше одиночных дат, span длиннее ---
		{
			name: "date_range_numeric",
			// 30.11—01.12.2025 / 30.11.2025 - 01.12.2025 / 30.11 — 01.12
			re:  regexp.MustCompile(`\b\d{1,2}\.\d{1,2}(?:\.\d{2,4})?\s*[—–-]\s*\d{1,2}\.\d{1,2}(?:\.\d{2,4})?\b`),
			typ: EntityPeriod,
		},
		{
			name: "date_range_textual",
			re:   regexp.MustCompile(`(?i)\bс\s+\d{1,2}\s+` + monthAlt + `\s+по\s+\d{1,2}\s+` + monthAlt + `(?:\s+\d{4})?(?:\s*г\.?)?`),
			typ:  EntityPeriod,
		},

		// --- ДАТЫ ---
		{
			name: "date_numeric",
			// 19.09.2025, 19/09/2025, 19-09-2025, 2025-09-19
			re:  regexp.MustCompile(`\b(?:\d{1,2}[./-]\d{1,2}[./-]\d{2,4}|\d{4}-\d{2}-\d{2})\b`),
			typ: EntityDate,
		},
		{
			name: "date_textual",
			// «06» октября 2025 г. / 6 октября 2025 года
			re:  regexp.MustCompile(`(?i)[«"]?\d{1,2}[»"]?\s+` + monthAlt + `\s+\d{4}(?:\s*(?:г\.?|года))?`),
			typ: EntityDate,
		},
		{
			name: "month_year",
			re:   regexp.MustCompile(`(?i)\b` + monthAlt + `\s+\d{4}\s*(?:г\.?|года)`),
			typ:  EntityDate,
		},

		// --- ВРЕМЯ ---
		{
			name: "time_clinical",
			// время: 10 час. 18 мин. / 10:18 / в 10 ч 18 мин
			re:  regexp.MustCompile(`(?i)\b\d{1,2}\s*(?:час\.?|ч\.?)\s*\d{1,2}\s*(?:мин\.?|м\.?)`),
			typ: EntityTime,
		},
		{
			name: "time_colon",
			re:   regexp.MustCompile(`\b\d{1,2}:\d{2}(?::\d{2})?\b`),
			typ:  EntityTime,
		},

		// --- ТЕЛЕФОНЫ ---
		{
			name: "phone",
			// +7 (812) 123-45-67, 8 812 1234567, +7-911-123-45-67
			re:  regexp.MustCompile(`(?:\+7|\b8)[\s\-]?\(?\d{3,4}\)?[\s\-]?\d{2,3}[\s\-]?\d{2}[\s\-]?\d{2}\b`),
			typ: EntityPhone,
		},

		// --- НОМЕРА ДОКУМЕНТОВ ---
		{
			name: "doc_card_number",
			// медицинская карта № 20252184 / история болезни № 1234 / протокол ВК № 442.3
			re:  regexp.MustCompile(`(?i)(?:мед(?:ицинск(?:ая|ой))?\s+карт[аы]|истори[ияй]\s+болезни|протокол(?:а)?(?:\s+ВК)?|карт[аы]|амбулаторн\w+\s+карт\w*)\s*№\s*[\d.\-/]+`),
			typ: EntityDocNumber,
		},
		{
			name: "bare_long_id",
			// длинный числовой идентификатор (>= 7 цифр), напр. 20252184
			re:  regexp.MustCompile(`\b\d{7,}\b`),
			typ: EntityDocNumber,
		},

		// --- ДОКУМЕНТЫ, УДОСТОВЕРЯЮЩИЕ ЛИЧНОСТЬ ---
		{
			name: "snils",
			re:   regexp.MustCompile(`\b\d{3}-\d{3}-\d{3}\s?\d{2}\b`),
			typ:  EntityIDDoc,
		},
		{
			name: "passport",
			re:   regexp.MustCompile(`(?i)\bпаспорт\b[^\n]{0,20}?\d{2}\s?\d{2}\s?\d{6}`),
			typ:  EntityIDDoc,
		},
		{
			name: "policy_oms",
			re:   regexp.MustCompile(`(?i)\bполис\b[^\n]{0,20}?\d{6,16}`),
			typ:  EntityIDDoc,
		},

		// --- ВОЗРАСТ ---
		{
			name: "age_numeric",
			// 14 лет, 7-ми лет, в возрасте 15 лет.
			// ВНИМАНИЕ: Go RE2 \b распознаёт только ASCII-границы слов, поэтому
			// границы вокруг кириллических «лет/год» НЕ задаём через \b.
			re:  regexp.MustCompile(`(?i)\b\d{1,2}(?:-?(?:ти|ми|и|х))?[\s-]+(?:лет|год[аов]?)`),
			typ: EntityAge,
		},
		{
			name: "age_verbal",
			// в возрасте до пятнадцати лет / в возрасте пятнадцати лет
			re:  regexp.MustCompile(`(?i)в\s+возрасте\s+(?:до\s+)?[а-яё]+(?:\s+[а-яё]+)?\s+лет`),
			typ: EntityAge,
		},

		// --- АДРЕСА ---
		{
			name: "address_full",
			// проживающий по адресу: г. ..., ул. ..., д. N, кв. N
			re:  regexp.MustCompile(`(?i)(?:прожива\w+\s+по\s+)?адрес[у:]*[^\n]{0,200}?(?:кв\.?\s*\d+|д\.?\s*\d+[а-я]?(?:\s*к(?:орп)?\.?\s*\d+)?)`),
			typ: EntityAddress,
		},
		{
			name: "address_markers",
			// г. Санкт-Петербург, пр-кт Ленина, д. 5, к. 2, кв. 17, литера А
			re:  regexp.MustCompile(`(?i)(?:\bг\.\s*[А-ЯЁ][а-яё-]+|\b(?:ул|пр-кт|пр|просп|пер|наб|бул|ш|пл)\.\s*[А-ЯЁ][а-яё-]+)(?:[^\n]{0,80}?(?:д\.?\s*\d+[а-я]?|к(?:орп)?\.?\s*\d+|кв\.?\s*\d+|литера?\s*[А-ЯA-Z]))*`),
			typ: EntityAddress,
		},
	}
}

// runRegexDetectors applies all level-2 detectors and records spans.
func runRegexDetectors(detectors []regexDetector, text string, set *spanSet) {
	for _, d := range detectors {
		for _, loc := range d.re.FindAllStringIndex(text, -1) {
			set.add(loc[0], loc[1], d.typ)
		}
	}
}
