package anonymizer

import (
	"regexp"
	"strings"
)

// institutionDetector implements the gazetteer-driven part of level 3 (docs/04):
// when an institution marker (ГБУЗ, ЦВЛ, ПНД, «больница», ...) appears, the
// surrounding full name — including quoted parts and «им. ...» — is captured.
type institutionDetector struct {
	dict *dictionaries
	// quoted captures a quoted institution name «...» / "...".
	quoted *regexp.Regexp
	// named captures the «им. <Кого-то>» honorific tail.
	named *regexp.Regexp
}

func newInstitutionDetector(d *dictionaries) *institutionDetector {
	return &institutionDetector{
		dict:   d,
		quoted: regexp.MustCompile(`[«"][^»"]{0,120}[»"]`),
		named:  regexp.MustCompile(`(?i)\bим\.\s*[^,\n.]{0,60}`),
	}
}

// detect records institution spans. For each marker occurrence it expands the
// span to include an adjacent quoted name, a trailing «им. ...», and an
// optional «№ N».
func (det *institutionDetector) detect(text string, set *spanSet) {
	lower := strings.ToLower(text)
	numRe := regexp.MustCompile(`(?i)^\s*№\s*\d+`)

	for _, marker := range det.dict.institutions {
		from := 0
		for {
			idx := strings.Index(lower[from:], marker)
			if idx < 0 {
				break
			}
			start := from + idx
			end := start + len(marker)

			// Расширяем вправо: « номер », « кавычки », « им. ... ».
			rest := text[end:]
			grew := true
			for grew {
				grew = false
				trimmed := strings.TrimLeft(rest, " \t")
				lead := len(rest) - len(trimmed)
				if loc := numRe.FindStringIndex(rest); loc != nil && loc[0] == 0 {
					end += loc[1]
					rest = text[end:]
					grew = true
					continue
				}
				if loc := det.quoted.FindStringIndex(trimmed); loc != nil && loc[0] == 0 {
					end += lead + loc[1]
					rest = text[end:]
					grew = true
					continue
				}
				if loc := det.named.FindStringIndex(trimmed); loc != nil && loc[0] == 0 {
					end += lead + loc[1]
					rest = text[end:]
					grew = true
					continue
				}
			}

			// Расширяем влево к аббревиатуре-приставке «СПб», «ГКУЗ» уже учтены
			// как отдельные маркеры; добавим предшествующее «№»-словосочетание.
			set.add(start, end, EntityInstitution)
			from = end
		}
	}
}
