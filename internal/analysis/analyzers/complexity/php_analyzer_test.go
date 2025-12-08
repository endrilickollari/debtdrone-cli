package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestPHPAnalyzer_AnalyzeFile(t *testing.T) {
	thresholds := models.DefaultComplexityThresholds()
	analyzer := NewPHPAnalyzer(thresholds)

	tests := []struct {
		name               string
		code               string
		expectedFunctions  []string
		expectedCyclomatic map[string]int
		expectedParams     map[string]int
	}{
		{
			name: "Basic Functions",
			code: `<?php
function simple() {
    echo "Hello";
}

function add($a, $b) {
    return $a + $b;
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
			name: "Class Methods",
			code: `<?php
class Calculator {
    public function multiply($x, $y) {
        return $x * $y;
    }
    
    private function divide(int $a, int $b): float {
        if ($b == 0) return 0;
        return $a / $b;
    }
}
`,
			expectedFunctions: []string{"multiply", "divide"},
			expectedCyclomatic: map[string]int{
				"multiply": 1,
				"divide":   2, // 1 base + 1 if
			},
			expectedParams: map[string]int{
				"multiply": 2,
				"divide":   2,
			},
		},
		{
			name: "PHP 8 Attributes and Types",
			code: `<?php
#[Route("/api")]
function handleRequest(#[Inject] Request $req): Response|null {
    return null;
}
`,
			expectedFunctions: []string{"handleRequest"},
			expectedCyclomatic: map[string]int{
				"handleRequest": 1,
			},
			expectedParams: map[string]int{
				"handleRequest": 1,
			},
		},
		{
			name: "Constructor Property Promotion",
			code: `<?php
class User {
    public function __construct(
        public string $name,
        private int $age,
        protected string $email,
    ) {}
}
`,
			expectedFunctions: []string{"__construct"},
			expectedCyclomatic: map[string]int{
				"__construct": 1,
			},
			expectedParams: map[string]int{
				"__construct": 3,
			},
		},
		{
			name: "Complexity Calculation",
			code: `<?php
function complex($x) {
    if ($x > 0) {
        for ($i = 0; $i < $x; $i++) {
             switch ($i) {
                 case 1: echo "one"; break;
                 case 2: echo "two"; break;
                 default: echo "other";
             }
        }
    }
    
    $y = ($x < 0) ? -1 : 1;
    $z = $x ?? 0;
    
    if ($a && $b || $c) {
        echo "logic";
    }
}
`,
			expectedFunctions: []string{"complex"},
			expectedCyclomatic: map[string]int{
				"complex": 11,
			},
			expectedParams: map[string]int{
				"complex": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := analyzer.AnalyzeFile("test.php", []byte(tt.code))
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
