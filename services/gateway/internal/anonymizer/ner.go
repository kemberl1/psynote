package anonymizer

import "context"

// NEREntity is a named entity returned by an external NER backend (level 4
// docs/04 — Natasha / SpaCy ru). Offsets are byte offsets into the text passed
// to Recognize.
type NEREntity struct {
	Start int
	End   int
	Type  EntityType // обычно EntityPerson / EntityInstitution / EntityAddress
}

// NERClient is the boundary for the optional Python NER side-car (Natasha).
// It MUST run inside the local docker network only — PII never leaves the
// perimeter (docs/04 §6, §9). The Go pipeline works fully without it (MVP);
// when configured, NER хits are merged as additional candidate spans.
type NERClient interface {
	// Recognize returns person/location/org spans for the given text.
	Recognize(ctx context.Context, text string) ([]NEREntity, error)
}

// noopNER is the default backend: it recognises nothing. The Go-only layers
// (regex + dictionaries + morphology) remain the working MVP detector, and the
// fail-closed gate compensates for anything they miss.
type noopNER struct{}

func (noopNER) Recognize(_ context.Context, _ string) ([]NEREntity, error) { return nil, nil }
