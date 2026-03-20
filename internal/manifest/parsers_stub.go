package manifest

// Stub constructors — replaced by real implementations in Tasks 6-8.
// These exist only so the package compiles while parsers are not yet implemented.

type stubParser struct{ eco Ecosystem }

func (s *stubParser) Parse(_ string) ([]Package, error) { return nil, nil }
func (s *stubParser) Ecosystem() Ecosystem              { return s.eco }

func NewGoModParser() Parser      { return &stubParser{eco: EcosystemGo} }
func NewPythonParser() Parser     { return &stubParser{eco: EcosystemPython} }
func NewRustParser() Parser       { return &stubParser{eco: EcosystemRust} }
func NewJavaScriptParser() Parser { return &stubParser{eco: EcosystemNPM} }
