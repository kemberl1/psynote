package anonymizer

import (
	"regexp"
	"strings"
)

// patronymicPattern matches Russian patronymics (отчества) in ALL grammatical
// cases. It is the SINGLE SOURCE OF TRUTH shared by the L5 detector (fio.go) and
// the L7 validation gate (below), so the detector cleans exactly what the gate
// flags as residual_patronymic — no more, no less.
//
// ВАЖНО (ЭТАП 3.2): паттерн применяется ТОЛЬКО к ЦЕЛОМУ ТОКЕНУ (см.
// isWholePatronymic) — и детектором, и гейтом. Подстрочное применение в гейте
// (FindAllStringIndex) на Этапе 3.1 ловило ПРЕФИКСЫ нарицательных слов с
// заглавной буквы («Налич» в «Наличие», «Различ» в «Различие») → ложный
// fail-closed на ~100% дневников. Теперь определение «целый токен» одно на оба.
//
// Masculine (полные): stem -ович/-евич/-ьевич + optional case ending
//
//	(им. ∅, род. -а, дат. -у, твор. -ем/-ём, предл. -е).
//
// Masculine (короткие на -ич): голое «-ич» НЕ выводится из общего шаблона
// `[А-ЯЁ][а-яё]+ич` — иначе как целый токен совпадают нарицательные
// «Паралич/Москвич/Кулич» и т.п. Вместо этого — ЗАКРЫТЫЙ список реальных
// коротких отчеств (Ильич/Кузьмич/Лукич/Фомич/Саввич/Никитич/Фокич) с падежами
// (твор. -ом/-ём, род. -а, дат. -у, предл. -е). Так «Кузьмич» ловится, а
// «паралич» — нет, без опоры на бесконечный стоп-лист.
//
// Feminine: stem -овн/-евн/-ьевн/-ичн/-иничн + REQUIRED case ending
//
//	(им. -а, род. -ы, дат./предл. -е, вин. -у, твор. -ой/-ою).
//
// КЛЮЧЕВОЙ ФИКС 3.1 (сохранён): женский суффикс кодируется как «овн/евн/...» с
// ОТДЕЛЬНЫМ окончанием, поэтому косвенные «Петровны/Петровне/Петровну/Петровной»
// ловятся (раньше «овна|евна» с гласной «а» их пропускал).
const patronymicPattern = `(?:` +
	`[А-ЯЁ][а-яё]+(?:ович|евич|ьевич)(?:ем|ём|а|у|е)?` + // полные мужские + падежи
	`|(?:Ильич|Кузьмич|Лукич|Фомич|Саввич|Никитич|Фокич)(?:ом|ём|ем|а|у|е)?` + // короткие мужские + падежи
	`|[А-ЯЁ][а-яё]+(?:овн|евн|ьевн|иничн|ичн)(?:ою|ой|а|ы|е|у)` + // женские + обязательное окончание
	`)`

// gate is level 7 (docs/04 §3): an INDEPENDENT, more paranoid re-scan of the
// already-anonymized text. If it finds anything that still looks like PII it
// returns suspicions; the pipeline then reports Clean=false (fail-closed).
type gate struct {
	wl        *whitelist
	skip      map[string]struct{}
	dict      *dictionaries
	residual  []*regexp.Regexp
	patronym  *regexp.Regexp
	placeMask *regexp.Regexp
}

func newGate(wl *whitelist, d *dictionaries) *gate {
	return &gate{
		wl:   wl,
		skip: d.fioSkipWords,
		dict: d,
		residual: []*regexp.Regexp{
			regexp.MustCompile(`\b\d{1,2}[./-]\d{1,2}[./-]\d{2,4}\b`),                                     // остаточная дата
			regexp.MustCompile(`(?:\+7|\b8)[\s\-]?\(?\d{3,4}\)?[\s\-]?\d{2,3}[\s\-]?\d{2}[\s\-]?\d{2}\b`), // телефон
			regexp.MustCompile(`\b\d{7,}\b`),                                                              // длинный ID
			regexp.MustCompile(`(?i)\b\d{1,2}\s+(?:лет|год[аов]?)`),                                       // возраст
		},
		patronym:  regexp.MustCompile(patronymicPattern),
		placeMask: regexp.MustCompile(`\[[А-ЯA-Z_]+\]`),
	}
}

// Suspicion describes a residual PII finding (category only, NEVER the value —
// docs/04 §7). Position помогает отладке без раскрытия значения.
type Suspicion struct {
	Type     EntityType
	Position int
	Reason   string
}

// inspect scans anonymized text and returns residual suspicions. The text is
// first masked at placeholder positions so валидатор не реагирует на сами
// плейсхолдеры.
func (g *gate) inspect(text string) []Suspicion {
	masked := g.placeMask.ReplaceAllStringFunc(text, func(m string) string {
		return strings.Repeat(" ", len(m))
	})

	var out []Suspicion

	for _, re := range g.residual {
		for _, loc := range re.FindAllStringIndex(masked, -1) {
			out = append(out, Suspicion{Type: EntityDate, Position: loc[0], Reason: "residual structured pattern"})
		}
	}

	// ЭТАП 3.2 — устранение асимметрии гейт↔детектор. И отчество, и словарные
	// ФИО проверяются ПО ЦЕЛОМУ ТОКЕНУ той же токенизацией (tokenize) и тем же
	// определением «целый токен» (isWholePatronymic / isFullToken), что и
	// детектор fio.go. Раньше гейт применял patronymicPattern подстрокой
	// (FindAllStringIndex) и ловил ПРЕФИКСЫ нарицательных слов с заглавной буквы
	// («Налич» в «Наличие», «Различ» в «Различие») → ложный fail-closed.
	for _, tk := range tokenize(masked) {
		if !startsUpperCyrillic(tk.text) {
			continue
		}
		w := strings.TrimRight(tk.text, ".")
		if _, ok := g.skip[normalizeWord(w)]; ok {
			continue
		}

		// Остаточное отчество — почти всегда непойманное ФИО. Проверяем РОВНО
		// ту же логику целого токена, что и детектор, поэтому гейт реагирует
		// только на то, что детектор обязан был вычистить.
		if isWholePatronymic(g.patronym, tk.text) {
			out = append(out, Suspicion{Type: EntityPerson, Position: tk.start, Reason: "residual patronymic"})
			continue
		}

		// Остаточные словарные ФИО (заглавные слова из справочника).
		if g.wl.isWhitelisted(tk.text) {
			continue
		}
		if g.dict.surnames.match(tk.text) || g.dict.firstNames.match(tk.text) {
			out = append(out, Suspicion{Type: EntityPerson, Position: tk.start, Reason: "residual dictionary name"})
		}
	}

	return out
}
