package model

// TextAnchor preserves the actual selected text and UTF-16 offsets into the
// concatenated PDF text-layer spans. SourceHash binds it to an immutable source.
type AnnotationPatch struct {
	Color      *AnnotationColor `json:"color,omitempty"`
	Comment    *string          `json:"comment,omitempty"`
	SourceHash string           `json:"source_hash,omitempty"`
}

type TextAnchor struct {
	Version    int    `json:"version"`
	SourceHash string `json:"source_hash"`
	Exact      string `json:"exact"`
	Prefix     string `json:"prefix"`
	Suffix     string `json:"suffix"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
}
