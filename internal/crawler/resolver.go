package crawler

import "context"

// Resolver detects and resolves dependencies of a specific type.
type Resolver interface {
	// Detect finds dependency references in file contents.
	Detect(ctx context.Context, contents FileTree) ([]DepRef, error)

	// Resolve takes a reference and returns the project identity, version,
	// and a FileTree of its contents (for recursive scanning).
	Resolve(ctx context.Context, ref DepRef) (*ResolvedDep, error)
}

// AuthProvider returns the appropriate token for a given host.
type AuthProvider interface {
	TokenForHost(host string) string
}
