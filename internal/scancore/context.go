package scancore

import "context"

type targetSelectionKey struct{}

type targetSelection struct {
	files       []string
	incremental bool
}

// WithTargetFiles binds the scanner's canonical, prefiltered file selection to
// legacy analyzers without exposing public scanner types or string context keys.
func WithTargetFiles(ctx context.Context, files []string, incremental bool) context.Context {
	selection := targetSelection{files: append([]string(nil), files...), incremental: incremental}
	return context.WithValue(ctx, targetSelectionKey{}, selection)
}

// TargetFiles returns the explicit target set, whether it represents an
// incremental scan, and whether a selection was bound at all.
func TargetFiles(ctx context.Context) (files []string, incremental bool, ok bool) {
	selection, ok := ctx.Value(targetSelectionKey{}).(targetSelection)
	if !ok {
		return nil, false, false
	}
	return selection.files, selection.incremental, true
}
