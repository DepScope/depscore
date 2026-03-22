package manifest

// Stub parsers — replaced by real implementations in Tasks 6-8.
// Tasks 6 (Go) and 7 (Python) have real implementations in gomod.go and python.go.

type stubParser struct{ eco Ecosystem }

func (s *stubParser) Parse(dir string) ([]Package, error) { return nil, nil }
func (s *stubParser) Ecosystem() Ecosystem                { return s.eco }

func NewRustParser() Parser       { return &stubParser{eco: EcosystemRust} }
func NewJavaScriptParser() Parser { return &stubParser{eco: EcosystemNPM} }
