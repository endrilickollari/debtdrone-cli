package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestRustAnalyzer_AnalyzeFile(t *testing.T) {
	thresholds := models.DefaultComplexityThresholds()
	analyzer := NewRustAnalyzer(thresholds)

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
fn simple() {
    println!("Hello");
}

pub fn add(a: i32, b: i32) -> i32 {
    a + b
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
			name: "Generics and Traits",
			code: `
fn execute<T: Clone + Debug>(input: T) where T: Display {
    println!("{:?}", input);
}
`,
			expectedFunctions: []string{"execute"},
			expectedCyclomatic: map[string]int{
				"execute": 1,
			},
			expectedParams: map[string]int{
				"execute": 1,
			},
		},
		{
			name: "Methods and Self",
			code: `
impl Rectangle {
    fn area(&self) -> u32 {
        self.width * self.height
    }
    
    fn new(width: u32, height: u32) -> Self {
        Self { width, height }
    }
}
`,
			expectedFunctions: []string{"area", "new"},
			expectedCyclomatic: map[string]int{
				"area": 1,
				"new":  1,
			},
			expectedParams: map[string]int{
				"area": 0, // &self skipped
				"new":  2,
			},
		},
		{
			name: "Async Await",
			code: `
async fn fetch_data() -> Result<(), Error> {
    let _ = client.get().await?;
    Ok(())
}
`,
			expectedFunctions: []string{"fetch_data"},
			expectedCyclomatic: map[string]int{
				"fetch_data": 2, // 1 (base) + 1 (?)
			},
			expectedParams: map[string]int{
				"fetch_data": 0,
			},
		},
		{
			name: "Complexity Calculation",
			code: `
fn complex_logic(x: i32) -> i32 {
    if x > 0 {
        match x {
            1 => println!("one"),
            2 | 3 => println!("two or three"),
            _ => println!("other"),
        }
    } else if x < 0 {
        return -1;
    }
    
    // Loops
    for i in 0..10 {
        if i % 2 == 0 {
             continue;
        }
    }
    
    // Binary
    if a && b || c {
         // ...
    }
    
    // ? operator
    let y = some_func()?;
    
    0
}
`,
			expectedFunctions: []string{"complex_logic"},
			expectedCyclomatic: map[string]int{
				"complex_logic": 14,
			},
			expectedParams: map[string]int{
				"complex_logic": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := analyzer.AnalyzeFile("test.rs", []byte(tt.code))
			assert.NoError(t, err)
			assert.Len(t, metrics, len(tt.expectedFunctions))

			for _, m := range metrics {
				if expectedCyc, ok := tt.expectedCyclomatic[m.FunctionName]; ok {
					assert.Equal(t, expectedCyc, m.CyclomaticComplexity, "Cyclomatic complexity for %s", m.FunctionName)
				}
				if expectedParam, ok := tt.expectedParams[m.FunctionName]; ok {
					assert.Equal(t, expectedParam, m.ParameterCount, "Parameter count for %s", m.FunctionName)
				}
			}
		})
	}
}
