package manifest

// Stub parsers — replaced by real implementations in Tasks 6-8.

type stubParser struct{ eco Ecosystem }

func (s *stubParser) Parse(dir string) ([]Package, error) { return nil, nil }
func (s *stubParser) Ecosystem() Ecosystem                { return s.eco }

func NewPythonParser() Parser     { return &stubParser{eco: EcosystemPython} }
func NewGoModParser() Parser      { return &stubParser{eco: EcosystemGo} }
func NewRustParser() Parser       { return &stubParser{eco: EcosystemRust} }
func NewJavaScriptParser() Parser { return &stubParser{eco: EcosystemNPM} }
