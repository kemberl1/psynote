package anonymizer

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// token is a word with its byte range in the source text.
type token struct {
	text  string
	start int
	end   int
}

// fioDetector implements levels 3 & 5 (docs/04): Russian full-name detection
// with case (падеж) awareness, patronymic suffixes, initials, and gazetteers.
// It is the working Go MVP for ФИО; an external NER (level 4) can augment it.
type fioDetector struct {
	dict *dictionaries

	// patronymic suffixes (отчества) — сильный сигнал ФИО (docs/04 §5).
	patronymicRe *regexp.Regexp
	// "Фамилия И.О." / "Фамилия И. О."
	surnameInitialsRe *regexp.Regexp
	// "И.О. Фамилия"
	initialsSurnameRe *regexp.Regexp
	// role markers that precede medical staff names.
	doctorMarkerRe *regexp.Regexp
	// role markers for parents / legal representatives.
	parentMarkerRe *regexp.Regexp
	// patient markers.
	patientMarkerRe *regexp.Regexp
}

func newFIODetector(d *dictionaries) *fioDetector {
	return &fioDetector{
		dict: d,
		// Отчества: -вич/-вича/-вичу/-вичем (м.р.) и -вна/-вны/-вне/-вной (ж.р.),
		// а также -ьевич/-ьевна, -ич/-инична.
		patronymicRe:      regexp.MustCompile(`(?:[А-ЯЁ][а-яё]+(?:ович|евич|ьевич|ич)(?:а|у|ем|е)?|[А-ЯЁ][а-яё]+(?:овна|евна|ьевна|инична)(?:ы|е|ой|у)?)`),
		surnameInitialsRe: regexp.MustCompile(`[А-ЯЁ][а-яё]+(?:ов|ев|ёв|ин|ын|ский|ская|цкий|цкая|ова|ева|ёва|ина|ына)?\s+[А-ЯЁ]\.\s?[А-ЯЁ]\.`),
		initialsSurnameRe: regexp.MustCompile(`[А-ЯЁ]\.\s?[А-ЯЁ]\.\s+[А-ЯЁ][а-яё]+`),
		doctorMarkerRe:    regexp.MustCompile(`(?i)(?:врач(?:-[а-яё]+)?|психиатр|педиатр|невролог|психолог|заведующ(?:ий|ая)|зав\.\s*отделением|председател[ья]|член\s+ВК|секретар[ья]|доктор|медсестра|медбрат)`),
		parentMarkerRe:    regexp.MustCompile(`(?i)(?:мать|отец|матери|отца|родител[ья]|опекун|представител[ья]|законн\w+\s+представител\w+)`),
		patientMarkerRe:   regexp.MustCompile(`(?i)(?:пациент(?:а|у|ке|ки)?|пациентк[аи]|больн(?:ой|ого|ому)|ребёнк\w*|ребенк\w*|на\s+имя|освидетельствовани\w+)`),
	}
}

// detect records ФИО spans into set. Strategy (defence in depth):
//  1. initials patterns ("Фамилия И.О." / "И.О. Фамилия");
//  2. patronymic-anchored blocks (отчество + соседние заглавные слова);
//  3. dictionary-anchored capitalised sequences (имя/фамилия из справочника).
//
// Role is inferred from nearby markers (врач/мать/пациент) to pick the precise
// placeholder; otherwise the generic [ФИО] is used.
func (f *fioDetector) detect(text string, set *spanSet) {
	// 1. Initials-based patterns (highly specific).
	for _, re := range []*regexp.Regexp{f.surnameInitialsRe, f.initialsSurnameRe} {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			set.add(loc[0], loc[1], f.roleAt(text, loc[0]))
		}
	}

	toks := tokenize(text)

	// 2. Patronymic-anchored blocks.
	for i, tk := range toks {
		if !f.patronymicRe.MatchString(tk.text) || !isFullToken(f.patronymicRe, tk.text) {
			continue
		}
		start, end := tk.start, tk.end
		// поглощаем соседние заглавные кириллические слова (фамилия+имя в падеже)
		j := i - 1
		for j >= 0 && f.isNameLike(toks[j].text) {
			start = toks[j].start
			j--
		}
		k := i + 1
		for k < len(toks) && f.isNameLike(toks[k].text) {
			end = toks[k].end
			k++
		}
		set.add(start, end, f.roleAt(text, start))
	}

	// 3. Dictionary-anchored capitalised sequences.
	i := 0
	for i < len(toks) {
		tk := toks[i]
		if !startsUpperCyrillic(tk.text) || f.isSkip(tk.text) {
			i++
			continue
		}
		isName := f.dict.surnames.match(tk.text) || f.dict.firstNames.match(tk.text)
		if !isName {
			i++
			continue
		}
		start, end := tk.start, tk.end
		j := i + 1
		// расширяем на последовательность заглавных имя/фамилия/отчество
		for j < len(toks) && f.isNameLike(toks[j].text) {
			end = toks[j].end
			j++
		}
		set.add(start, end, f.roleAt(text, start))
		i = j
		if i == 0 {
			i++
		}
	}
}

// isNameLike reports whether a token can be part of a full name: a capitalised
// Cyrillic word that is not a stop-word, OR a single-letter initial "И."
func (f *fioDetector) isNameLike(word string) bool {
	if word == "" {
		return false
	}
	if f.isSkip(word) {
		return false
	}
	// инициал вида "И." или "И"
	if r, size := utf8.DecodeRuneInString(word); isUpperCyrillic(r) {
		rest := word[size:]
		if rest == "." || rest == "" {
			return true
		}
	}
	if !startsUpperCyrillic(word) {
		return false
	}
	// явная подсказка: отчество / словарное имя или фамилия
	if isFullToken(f.patronymicRe, word) {
		return true
	}
	if f.dict.firstNames.match(word) || f.dict.surnames.match(word) {
		return true
	}
	// иначе — заглавное кириллическое слово (кандидат), но не стоп-слово
	return len([]rune(word)) >= 3
}

func (f *fioDetector) isSkip(word string) bool {
	_, ok := f.dict.fioSkipWords[normalizeWord(strings.TrimRight(word, "."))]
	return ok
}

// roleAt inspects a small window of text preceding pos to assign the precise
// placeholder (doctor / parent / patient) and falls back to a generic person.
func (f *fioDetector) roleAt(text string, pos int) EntityType {
	from := pos - 80
	if from < 0 {
		from = 0
	}
	ctx := text[from:pos]
	switch {
	case f.doctorMarkerRe.MatchString(ctx):
		return EntityDoctor
	case f.parentMarkerRe.MatchString(ctx):
		return EntityParent
	case f.patientMarkerRe.MatchString(ctx):
		return EntityPatient
	default:
		return EntityPerson
	}
}

// isFullToken reports whether re matches the whole token exactly.
func isFullToken(re *regexp.Regexp, tok string) bool {
	loc := re.FindStringIndex(tok)
	return loc != nil && loc[0] == 0 && loc[1] == len(tok)
}

// tokenize splits text into word tokens keeping byte offsets. A token is a run
// of Cyrillic/Latin letters with optional trailing dot (for initials like "И.").
func tokenize(text string) []token {
	var toks []token
	i := 0
	n := len(text)
	for i < n {
		r, size := utf8.DecodeRuneInString(text[i:])
		if isLetterRune(r) {
			start := i
			i += size
			for i < n {
				r2, s2 := utf8.DecodeRuneInString(text[i:])
				if isLetterRune(r2) || r2 == '-' {
					i += s2
					continue
				}
				break
			}
			// прихватываем точку инициала
			end := i
			if i < n && text[i] == '.' {
				end = i + 1
			}
			toks = append(toks, token{text: text[start:end], start: start, end: end})
			i = end
			continue
		}
		i += size
	}
	return toks
}

func isLetterRune(r rune) bool {
	return isCyrillicLetter(r) || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}
