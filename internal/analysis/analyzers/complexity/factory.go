package complexity

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
)

// Analyzer is the interface for language-specific complexity analyzers
type Analyzer interface {
	AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error)
	Language() string
}

// Factory creates the appropriate complexity analyzer based on file extension
type Factory struct {
	thresholds models.ComplexityThresholds
}

// NewFactory creates a new analyzer factory
func NewFactory(thresholds models.ComplexityThresholds) *Factory {
	return &Factory{
		thresholds: thresholds,
	}
}

// GetAnalyzer returns the appropriate analyzer for the given file path
func (f *Factory) GetAnalyzer(filePath string) (Analyzer, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return NewGoAnalyzer(f.thresholds), nil
	case ".js", ".jsx":
		return NewJavaScriptAnalyzer(f.thresholds), nil
	case ".ts", ".tsx":
		return NewTypeScriptAnalyzer(f.thresholds), nil
	case ".py":
		return NewPythonAnalyzer(f.thresholds), nil
	case ".cs":
		return NewCSharpAnalyzer(f.thresholds), nil
	case ".php":
		return NewPHPAnalyzer(f.thresholds), nil
	case ".java":
		return NewJavaAnalyzer(f.thresholds), nil
	case ".rb":
		return NewRubyAnalyzer(f.thresholds), nil
	case ".rs":
		return NewRustAnalyzer(f.thresholds), nil
	case ".kt", ".kts":
		return NewKotlinAnalyzer(f.thresholds), nil
	case ".swift":
		return NewSwiftAnalyzer(f.thresholds), nil
	case ".c", ".cpp", ".cc", ".cxx", ".c++", ".h", ".hpp", ".hxx", ".h++":
		return NewCCppAnalyzer(f.thresholds), nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// IsSupported returns true if the file extension is supported for complexity analysis
func (f *Factory) IsSupported(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	supportedExts := []string{
		".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".java", ".cs", ".php",
		".rb", ".rs", ".kt", ".kts", ".swift",
		".c", ".cpp", ".cc", ".cxx", ".c++", ".h", ".hpp", ".hxx", ".h++",
	}

	for _, supported := range supportedExts {
		if ext == supported {
			return true
		}
	}
	return false
}
