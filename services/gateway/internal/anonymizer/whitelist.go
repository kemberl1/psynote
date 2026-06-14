package anonymizer

import "regexp"

// whitelist implements level 6 (docs/04 §3, уровень 6 — «не навреди»). It marks
// clinically valuable regions that detectors must NEVER replace: ICD-10 codes,
// Latin drug names with dosages, units, and common status abbreviations.
//
// Whitelist hits are registered as protected spans; the span engine then drops
// any PII candidate that overlaps them (см. span.resolve).
type whitelist struct {
	patterns []*regexp.Regexp
}

func newWhitelist() *whitelist {
	return &whitelist{
		patterns: []*regexp.Regexp{
			// МКБ-10: латинская буква + 2 цифры + опц. ".<цифры>" (F92.8, R51, J00, F06.68).
			regexp.MustCompile(`\b[A-ZА-Я]\d{2}(?:\.\d{1,2})?\b`),
			// Дозировки и единицы (кроме граммов): "25 мг", "10 мл", "120/80 мм рт.ст.".
			regexp.MustCompile(`(?i)\b\d+(?:[.,]\d+)?\s*(?:мг|мкг|мл|ме|ед|таб|капс|%|мм\s*рт\.?\s*ст\.?)\b`),
			// Граммы — ОТДЕЛЬНО и УЗКО (ЭТАП 3.1, fix residual_date): только дозо-
			// подобное число (≤3 цифр или с дробной частью). Это не даёт юниту «г»
			// поглотить 4-значный ГОД в «2025 г.» (сокращение «год»), из-за чего
			// дата раньше защищалась как «доза», выпадала из замены и просачивалась
			// мимо детектора прямо на валидатор-гейт (residual_date).
			regexp.MustCompile(`(?i)\b\d{1,3}(?:[.,]\d+)?\s*г\b`),
			// Латинские лекарственные формы/названия: "Tab.", "Sol.", "Ibuprofeni".
			regexp.MustCompile(`\b(?:Tab|Sol|Caps|Susp|Ung|Dragee|Rp)\.?\s+[A-Za-z]+`),
			regexp.MustCompile(`\b[A-Z][a-z]+i\b`), // латинский genitivus препаратов (Ibuprofeni)
			// Витальные показатели и инструментальные аббревиатуры.
			regexp.MustCompile(`\b(?:ЭКГ|ЭЭГ|УЗИ|МРТ|КТ|QTc|АД|ЧСС|ЧДД|Ps|D=S|T)\b`),
		},
	}
}

// mark registers all whitelist matches in text as protected spans.
func (w *whitelist) mark(text string, set *spanSet) {
	for _, re := range w.patterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			set.addProtected(loc[0], loc[1])
		}
	}
}

// isWhitelisted reports whether the entire token equals an ICD-like or clinical
// token. Used by the validation gate to avoid flagging clinical tokens as
// residual PII (precision, docs/04 §7).
func (w *whitelist) isWhitelisted(token string) bool {
	for _, re := range w.patterns {
		if loc := re.FindStringIndex(token); loc != nil && loc[0] == 0 && loc[1] == len(token) {
			return true
		}
	}
	return false
}
