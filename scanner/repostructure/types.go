package repostructure

import "context"

// NativeDependency is a build-time system dependency required by a build root.
type NativeDependency struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// BuildRoot describes one independently buildable project within a repository.
type BuildRoot struct {
	Dir          string             `json:"dir"`
	Language     string             `json:"language"`
	Tool         string             `json:"tool"`
	ManifestFile string             `json:"manifest_file"`
	TestRunner   string             `json:"test_runner"`
	DockerImage  string             `json:"docker_image,omitempty"`
	NativeDeps   []NativeDependency `json:"native_deps,omitempty"`
}

// Structure is the static build and directory layout detected for a repository.
type Structure struct {
	BuildRoots  []BuildRoot `json:"build_roots"`
	SourceRoots []string    `json:"source_roots"`
	TestDirs    []string    `json:"test_dirs"`
	EntryPoints []string    `json:"entry_points"`
	DocDirs     []string    `json:"doc_dirs"`
	RepoRoot    string      `json:"repo_root"`
	IsMonorepo  bool        `json:"is_monorepo"`
	Warnings    []string    `json:"warnings,omitempty"`
}

type contextKey struct{}

// WithContext makes precomputed repository metadata available to analyzers.
func WithContext(ctx context.Context, structure *Structure) context.Context {
	return context.WithValue(ctx, contextKey{}, structure)
}

// FromContext returns repository metadata previously attached with WithContext.
func FromContext(ctx context.Context) (*Structure, bool) {
	structure, ok := ctx.Value(contextKey{}).(*Structure)
	return structure, ok && structure != nil
}
