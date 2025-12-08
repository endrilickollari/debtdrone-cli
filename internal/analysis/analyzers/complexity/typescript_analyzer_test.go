package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestTypeScriptAnalyzer_AnalyzeFile(t *testing.T) {
	thresholds := models.DefaultComplexityThresholds()
	analyzer := NewTypeScriptAnalyzer(thresholds)

	tests := []struct {
		name               string
		code               string
		expectedFunctions  []string
		expectedCyclomatic map[string]int
		expectedParams     map[string]int
	}{
		{
			name: "Typed Functions",
			code: `
function add(a: number, b: number): number {
    return a + b;
}

const multiply = (x: number, y: number): number => {
    return x * y;
}
`,
			expectedFunctions: []string{"add", "multiply"},
			expectedCyclomatic: map[string]int{
				"add":      1,
				"multiply": 1,
			},
			expectedParams: map[string]int{
				"add":      2,
				"multiply": 2,
			},
		},
		{
			name: "Classes and Decorators",
			code: `
@Component({
  selector: 'app-root'
})
class AppComponent {
    @Input()
    title: string = 'app';

    constructor(private service: AppService) {}

    updateTitle(newTitle: string): void {
        if (newTitle) {
            this.title = newTitle;
        }
    }
}
`,
			expectedFunctions: []string{"updateTitle"},
			expectedCyclomatic: map[string]int{
				"updateTitle": 2,
			},
			expectedParams: map[string]int{
				"updateTitle": 1,
			},
		},
		{
			name: "Generics and Complexity",
			code: `
function processData<T extends object>(data: T | null): void {
    if (!data) {
        return;
    }

    for (const key in data) {
        if (Object.prototype.hasOwnProperty.call(data, key)) {
            const val = data[key];
            if (val && (typeof val === 'string' || typeof val === 'number')) {
                console.log(val);
            }
        }
    }
    
    const y = x ? true : false;
}
`,
			expectedFunctions: []string{"processData"},
			expectedCyclomatic: map[string]int{
				"processData": 9,
			},
			expectedParams: map[string]int{
				"processData": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := analyzer.AnalyzeFile("test.ts", []byte(tt.code))
			assert.NoError(t, err)

			findMetric := func(name string) *models.ComplexityMetric {
				for _, m := range metrics {
					if m.FunctionName == name {
						return &m
					}
				}
				return nil
			}

			for _, fn := range tt.expectedFunctions {
				m := findMetric(fn)
				if assert.NotNil(t, m, "Function %s not found", fn) {
					if expectedCyc, ok := tt.expectedCyclomatic[fn]; ok {
						assert.Equal(t, expectedCyc, m.CyclomaticComplexity, "Cyclomatic complexity for %s", fn)
					}
					if expectedParam, ok := tt.expectedParams[fn]; ok {
						assert.Equal(t, expectedParam, m.ParameterCount, "Parameter count for %s", fn)
					}
				}
			}
		})
	}
}
