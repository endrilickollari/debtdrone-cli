package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestCCppAnalyzer_AnalyzeFile(t *testing.T) {
	analyzer := NewCCppAnalyzer(models.ComplexityThresholds{})

	code := `
	// Simple function
	int add(int a, int b) {
		return a + b;
	}

	// Template function
	template <typename T>
	T max(T a, T b) {
		if (a > b) {
			return a;
		}
		return b;
	}

	// Function with pointers and complexity
	void complex_logic(int* ptr) {
		if (ptr == nullptr) {
			return;
		}

		for (int i = 0; i < 10; i++) {
			if (i % 2 == 0) {
				// nested
				while (true) {
					break;
				}
			} else if (i == 5) {
				continue;
			}
		}

		int x = (ptr != nullptr) ? *ptr : 0;
	}
	
	// Weird signature
	void* (*func_ptr_ret)(int arg) {
		return nullptr;
	}
	`

	metrics, err := analyzer.AnalyzeFile("test.cpp", []byte(code))
	assert.NoError(t, err)
	assert.Len(t, metrics, 4)

	// Helper to find metric by function name
	findMetric := func(name string) *models.ComplexityMetric {
		for _, m := range metrics {
			if m.FunctionName == name {
				return &m
			}
		}
		return nil
	}

	mAdd := findMetric("add")
	assert.NotNil(t, mAdd)
	if mAdd != nil {
		assert.Equal(t, 2, mAdd.ParameterCount)
		assert.Equal(t, 1, mAdd.CyclomaticComplexity)
	}

	mMax := findMetric("max")
	assert.NotNil(t, mMax)
	if mMax != nil {
		assert.Equal(t, 2, mMax.ParameterCount)
		assert.Equal(t, 2, mMax.CyclomaticComplexity)
	}

	mComplex := findMetric("complex_logic")
	assert.NotNil(t, mComplex)
	if mComplex != nil {
		assert.Equal(t, 1, mComplex.ParameterCount)
		assert.Equal(t, 7, mComplex.CyclomaticComplexity, "Cyclomatic complexity mismatch for complex_logic")
	}

	mPtr := findMetric("func_ptr_ret")
	if mPtr == nil {
		t.Logf("Found functions: %v", getFunctionNames(metrics))
	}
	assert.NotNil(t, mPtr)
}

func getFunctionNames(metrics []models.ComplexityMetric) []string {
	var names []string
	for _, m := range metrics {
		names = append(names, m.FunctionName)
	}
	return names
}
