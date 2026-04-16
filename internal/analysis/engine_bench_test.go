package analysis

import (
	"testing"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/google/uuid"
)

func BenchmarkProcessJobParallel(b *testing.B) {
	b.Skip("Requires full infrastructure setup - run manually with make bench")
}

func BenchmarkAnalyzersSequential(b *testing.B) {
	repo := &git.Repository{
		Path: b.TempDir(),
	}

	analyzers := []Analyzer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, analyzer := range analyzers {
			_ = analyzer.Name()
			_, _ = analyzer.Analyze(b.Context(), repo)
		}
	}
}

func BenchmarkHashFileContent(b *testing.B) {
	content := make([]byte, 10*1024) // 10KB file
	for i := range content {
		content[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HashFileContent(content)
	}
}

func BenchmarkCacheKeyGeneration(b *testing.B) {
	cache := NewAnalysisCache(nil)
	hash := "abc123def456abc123def456abc123def456abc123def456abc123def456abcd"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.cacheKey("complexity", hash)
	}
}

func BenchmarkUUIDGeneration(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid.New()
	}
}

func BenchmarkTimeNow(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = time.Now()
	}
}
