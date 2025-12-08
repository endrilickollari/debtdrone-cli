package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestRubyAnalyzer_AnalyzeFile(t *testing.T) {
	thresholds := models.DefaultComplexityThresholds()
	analyzer := NewRubyAnalyzer(thresholds)

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
def simple
  puts "Hello"
end

def add(a, b)
  return a + b
end
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
			name: "Singleton Methods",
			code: `
class Calculator
  def self.multiply(x, y)
    x * y
  end
end
`,
			expectedFunctions: []string{"multiply"},
			expectedCyclomatic: map[string]int{
				"multiply": 1,
			},
			expectedParams: map[string]int{
				"multiply": 2,
			},
		},
		{
			name: "Optional Parentheses",
			code: `
def greet name, greeting = "Hello"
  puts "#{greeting}, #{name}"
end
`,
			expectedFunctions: []string{"greet"},
			expectedCyclomatic: map[string]int{
				"greet": 1,
			},
			expectedParams: map[string]int{
				"greet": 2,
			},
		},
		{
			name: "Keyword Arguments",
			code: `
def configure(host:, port: 8080, **options)
  puts host
end
`,
			expectedFunctions: []string{"configure"},
			expectedCyclomatic: map[string]int{
				"configure": 1,
			},
			expectedParams: map[string]int{
				"configure": 3,
			},
		},
		{
			name: "Complexity Calculation",
			code: `
def complex_logic(x)
  if x > 0
    puts "positive"
  elsif x < 0
    puts "negative"
  else
    puts "zero"
  end

  # Post-fix conditionals (modifiers)
  return unless x != 0
  
  # Loops
  while x < 10
    x += 1
  end

  # Case
  case x
  when 1
    puts "one"
  when 2
    puts "two"
  end

  # Iterators (blocks)
  [1, 2, 3].each do |i|
    puts i
  end

  # Binary logic
  if a && b || c
    puts "logic"
  end
end
`,
			expectedFunctions: []string{"complex_logic"},
			expectedCyclomatic: map[string]int{
				"complex_logic": 11,
			},
			expectedParams: map[string]int{
				"complex_logic": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := analyzer.AnalyzeFile("test.rb", []byte(tt.code))
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
