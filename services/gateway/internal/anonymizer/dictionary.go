package anonymizer

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode"
)

//go:embed dictionaries/*.txt
var embeddedDicts embed.FS

// dictionaries holds the gazetteers (level 3 docs/04). They are loaded once at
// construction. Files are embedded by default so the binary is self-contained,
// but an operator may override the directory via config to extend lists without
// recompiling the detection logic.
type dictionaries struct {
	firstNames   *stemSet            // основы имён (для сопоставления по префиксу)
	surnames     *stemSet            // основы фамилий
	institutions []institutionMarker // маркеры учреждений (с режимом регистра)
	fioSkipWords map[string]struct{}
}

// institutionMarker is a single institution gazetteer entry (БАГ №1).
//
// caseSensitive markers are abbreviations (НИИ, ЦВЛ, ПНД, ГБУЗ …) that in real
// data are ALWAYS written in capitals; matching them case-sensitively prevents
// the lower-case substring «нии» inside ordinary words («улучшении») from being
// treated as a marker. Full-word markers («больница», «диспансер») stay
// case-insensitive but MUST still respect word boundaries (см. institutions.go).
type institutionMarker struct {
	// text is the literal searched for. For caseSensitive markers it is the
	// original (upper-case) form; otherwise it is lower-cased.
	text          string
	caseSensitive bool
}

// isAbbreviationMarker reports whether a raw dictionary line is an
// abbreviation-style marker: it contains letters and ALL of its letters are
// upper-case (e.g. «ГБУЗ», «ЦВЛ», «ПНД», «НИИ»). Such markers are matched
// case-sensitively. Mixed/lower-case full words («больница») return false.
func isAbbreviationMarker(raw string) bool {
	hasLetter := false
	for _, r := range raw {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
}

// loadDictionaries reads all gazetteers. If dir is non-empty, files are read
// from that directory on disk; otherwise the embedded copies are used.
func loadDictionaries(dir string) (*dictionaries, error) {
	read := func(name string) ([]string, error) {
		if dir != "" {
			return readLinesFile(os.DirFS(dir), name)
		}
		return readLinesFile(embeddedDicts, "dictionaries/"+name)
	}

	first, err := read("first_names.txt")
	if err != nil {
		return nil, fmt.Errorf("load first_names: %w", err)
	}
	sur, err := read("surnames.txt")
	if err != nil {
		return nil, fmt.Errorf("load surnames: %w", err)
	}
	inst, err := read("institutions.txt")
	if err != nil {
		return nil, fmt.Errorf("load institutions: %w", err)
	}
	skip, err := read("fio_skip_words.txt")
	if err != nil {
		return nil, fmt.Errorf("load fio_skip_words: %w", err)
	}

	d := &dictionaries{
		firstNames:   newStemSet(),
		surnames:     newStemSet(),
		fioSkipWords: make(map[string]struct{}, len(skip)),
	}
	for _, w := range first {
		d.firstNames.addStem(w)
	}
	for _, w := range sur {
		d.surnames.addStem(w)
	}
	for _, w := range inst {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if isAbbreviationMarker(w) {
			d.institutions = append(d.institutions, institutionMarker{text: w, caseSensitive: true})
		} else {
			d.institutions = append(d.institutions, institutionMarker{text: strings.ToLower(w), caseSensitive: false})
		}
	}
	for _, w := range skip {
		d.fioSkipWords[normalizeWord(w)] = struct{}{}
	}
	return d, nil
}

// readLinesFile reads a dictionary file, dropping blank lines and `#` comments.
func readLinesFile(fsys fs.FS, name string) ([]string, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
