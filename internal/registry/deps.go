package registry

// Dependency represents a single dependency entry from a registry.
type Dependency struct {
	Name       string
	Constraint string
}

// DependencyFetcher extends Fetcher with the ability to retrieve a package's
// dependency list from the registry. Used by the discover command to resolve
// transitive dependencies when no lockfile is available.
type DependencyFetcher interface {
	Fetcher
	FetchDependencies(name, version string) ([]Dependency, error)
}
