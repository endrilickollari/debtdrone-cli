package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestKotlinAnalyzer_AnalyzeFile(t *testing.T) {
	thresholds := models.DefaultComplexityThresholds()
	analyzer := NewKotlinAnalyzer(thresholds)

	tests := []struct {
		name               string
		code               string
		expectedFunctions  []string
		expectedCyclomatic map[string]int
		expectedParams     map[string]int
	}{
		{
			name: "Basic Functions",
			code: `
fun simple() {
    println("Hello")
}

fun add(a: Int, b: Int): Int {
    return a + b
}
`,
			expectedFunctions: []string{"simple", "add"},
			expectedCyclomatic: map[string]int{
				"simple": 1,
				"add":    1,
			},
			expectedParams: map[string]int{
				"simple": 0,
				"add":    2,
			},
		},
		{
			name: "Expression Body",
			code: `
fun square(x: Int) = x * x

fun max(a: Int, b: Int) = if (a > b) a else b
`,
			expectedFunctions: []string{"square", "max"},
			expectedCyclomatic: map[string]int{
				"square": 1,
				"max":    2, // 1 base + 1 if
			},
			expectedParams: map[string]int{
				"square": 1,
				"max":    2,
			},
		},
		{
			name: "Extension Function",
			code: `
fun String.shout(): String {
    return this.toUpperCase() + "!"
}
`,
			expectedFunctions: []string{"shout"},
			expectedCyclomatic: map[string]int{
				"shout": 1,
			},
			expectedParams: map[string]int{
				"shout": 0,
			},
		},
		{
			name: "Complexity Calculation",
			code: `
fun complex(x: Int) {
    if (x > 0) {
        for (i in 0..x) {
             when (i) {
                 1 -> println("one")
                 2 -> println("two")
                 else -> println("other")
             }
        }
    }
    
    val y = if (x < 0) -1 else 1
    
    x?.let {
       println(it)
    }
}
`,
			expectedFunctions: []string{"complex"},
			expectedCyclomatic: map[string]int{
				"complex": 9,
			},
			expectedParams: map[string]int{
				"complex": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := analyzer.AnalyzeFile("test.kt", []byte(tt.code))
			assert.NoError(t, err)
			assert.Len(t, metrics, len(tt.expectedFunctions))

			foundFuncs := make(map[string]bool)
			for _, m := range metrics {
				foundFuncs[m.FunctionName] = true
				if expectedCyc, ok := tt.expectedCyclomatic[m.FunctionName]; ok {
					assert.Equal(t, expectedCyc, m.CyclomaticComplexity, "Cyclomatic complexity for %s", m.FunctionName)
				}
				if expectedParam, ok := tt.expectedParams[m.FunctionName]; ok {
					assert.Equal(t, expectedParam, m.ParameterCount, "Parameter count for %s", m.FunctionName)
				}
			}

			for _, fn := range tt.expectedFunctions {
				assert.True(t, foundFuncs[fn], "Function %s not found", fn)
			}
		})
	}
}
