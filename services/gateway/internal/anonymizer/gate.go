package anonymizer

import (
	"regexp"
	"strings"
)

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
		patronym:  regexp.MustCompile(`(?:[А-ЯЁ][а-яё]+(?:ович|евич|ьевич)(?:а|у|ем|е)?|[А-ЯЁ][а-яё]+(?:овна|евна|ьевна)(?:ы|е|ой|у)?)`),
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

	// Остаточное отчество — почти всегда непойманное ФИО.
	for _, loc := range g.patronym.FindAllStringIndex(masked, -1) {
		out = append(out, Suspicion{Type: EntityPerson, Position: loc[0], Reason: "residual patronymic"})
	}

	// Остаточные словарные ФИО (заглавные слова из справочника).
	for _, tk := range tokenize(masked) {
		if !startsUpperCyrillic(tk.text) {
			continue
		}
		w := strings.TrimRight(tk.text, ".")
		if _, ok := g.skip[normalizeWord(w)]; ok {
			continue
		}
		if g.wl.isWhitelisted(tk.text) {
			continue
		}
		if g.dict.surnames.match(tk.text) || g.dict.firstNames.match(tk.text) {
			out = append(out, Suspicion{Type: EntityPerson, Position: tk.start, Reason: "residual dictionary name"})
		}
	}

	return out
}
