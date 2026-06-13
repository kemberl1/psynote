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
//
// БАГ №1 (precision): сопоставление маркеров теперь учитывает ГРАНИЦЫ СЛОВА
// вручную (Cyrillic-aware, см. hasWordBoundaries), потому что RE2 `\b` работает
// только для ASCII. Маркеры-аббревиатуры (НИИ, ЦВЛ, ПНД, ГБУЗ …) сопоставляются
// РЕГИСТРОЗАВИСИМО, чтобы окончание «-нии» в обычных словах («улучшении»,
// «отделении», «линии») не принималось за маркер «НИИ».
func (det *institutionDetector) detect(text string, set *spanSet) {
	lower := strings.ToLower(text)
	numRe := regexp.MustCompile(`(?i)^\s*№\s*\d+`)

	for _, marker := range det.dict.institutions {
		// Регистрозависимые аббревиатуры ищем в исходном тексте, полнословные
		// маркеры — в lower-cased копии (регистронезависимо). Смещения совпадают:
		// strings.ToLower для кириллицы/латиницы здесь длину рун не меняет.
		hay := lower
		needle := marker.text
		if marker.caseSensitive {
			hay = text
		}

		from := 0
		for {
			idx := strings.Index(hay[from:], needle)
			if idx < 0 {
				break
			}
			start := from + idx
			end := start + len(needle)

			// Границы слова (Cyrillic-aware): маркер валиден, только если слева
			// и справа от него НЕ буква/цифра. Иначе это подстрока обычного
			// слова (НИИ в «улучшении», ПНД/ЦВЛ внутри слова) — пропускаем.
			if !hasWordBoundaries(text, start, end) {
				from = end
				continue
			}

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
