package anonymizer

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

//go:embed dictionaries/*.txt
var embeddedDicts embed.FS

// dictionaries holds the gazetteers (level 3 docs/04). They are loaded once at
// construction. Files are embedded by default so the binary is self-contained,
// but an operator may override the directory via config to extend lists without
// recompiling the detection logic.
type dictionaries struct {
	firstNames   *stemSet // основы имён (для сопоставления по префиксу)
	surnames     *stemSet // основы фамилий
	institutions []string // маркеры учреждений (lower-case)
	fioSkipWords map[string]struct{}
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
		d.institutions = append(d.institutions, strings.ToLower(w))
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
