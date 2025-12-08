package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestJavaAnalyzer_AnalyzeFile(t *testing.T) {
	thresholds := models.DefaultComplexityThresholds()
	analyzer := NewJavaAnalyzer(thresholds)

	tests := []struct {
		name               string
		code               string
		expectedFunctions  []string
		expectedCyclomatic map[string]int
		expectedParams     map[string]int
	}{
		{
			name: "Basic Methods",
			code: `
public class Example {
    public void simple() {
        System.out.println("Hello");
    }

    public int add(int a, int b) {
        return a + b;
    }
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
			name: "Annotations and Generics",
			code: `
import java.util.List;
import java.util.Map;

public class Complex {
    @Override
    public void process(@Nullable String s, Map<String, List<Integer>> data) {
        if (s != null) {
            System.out.println(s);
        }
    }
}
`,
			expectedFunctions: []string{"process"},
			expectedCyclomatic: map[string]int{
				"process": 2, // 1 base + 1 if
			},
			expectedParams: map[string]int{
				"process": 2,
			},
		},
		{
			name: "Records",
			code: `
public record Point(int x, int y) {
    public Point {
        if (x < 0) x = 0;
        if (y < 0) y = 0;
    }

    public int diff() {
       return x - y;
    }
}
`,
			expectedFunctions: []string{"Point", "diff"},
			expectedCyclomatic: map[string]int{
				"Point": 3,
				"diff":  1,
			},
			expectedParams: map[string]int{
				"Point": 0,
				"diff":  0,
			},
		},
		{
			name: "Complexity Calculation",
			code: `
public class Calc {
    public void complexLogic(int x) {
        if (x > 0) {
            for (int i = 0; i < x; i++) {
                if (i % 2 == 0 && i > 5) {
                    System.out.println(i);
                } else if (i < 0 || x > 100) {
                     // logic
                }
            }
        }
        
        switch (x) {
            case 1: break;
            case 2: break;
            default: break;
        }

        int y = (x > 10) ? 1 : 0;
    }
}
`,
			expectedFunctions: []string{"complexLogic"},
			expectedCyclomatic: map[string]int{
				"complexLogic": 11,
			},
			expectedParams: map[string]int{
				"complexLogic": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := analyzer.AnalyzeFile("test.java", []byte(tt.code))
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
