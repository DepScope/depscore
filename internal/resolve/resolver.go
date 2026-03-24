package resolve

import "context"

type ManifestFile struct {
	Path    string
	Content []byte
}

type Resolver interface {
	Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error)
}
