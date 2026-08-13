package export

import "strings"

// Obvious LLM misspellings in clinical wording (Сидрос → Синдром).
var obviousTypos = [][2]string{
	{"Сидрос", "Синдром"},
	{"сидрос", "синдром"},
	{"СИДРОС", "СИНДРОМ"},
	{"психмоторн", "психомоторн"},
	{"Психмоторн", "Психомоторн"},
	{"ПСИХМОТОРН", "ПСИХОМОТОРН"},
	{"самостоятельно не предъявляет", "не предъявляет"},
	{"Самостоятельно не предъявляет", "не предъявляет"},
	{"самостоятельно не формирует", "не предъявляет"},
	{"Самостоятельно не формирует", "не предъявляет"},
}

func fixObviousTypos(text string) string {
	if text == "" {
		return text
	}
	out := text
	for _, pair := range obviousTypos {
		if strings.Contains(out, pair[0]) {
			out = strings.ReplaceAll(out, pair[0], pair[1])
		}
	}
	return out
}
