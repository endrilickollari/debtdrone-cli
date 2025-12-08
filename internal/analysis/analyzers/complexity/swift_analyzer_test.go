package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestSwiftAnalyzer_AnalyzeFile(t *testing.T) {
	thresholds := models.DefaultComplexityThresholds()
	analyzer := NewSwiftAnalyzer(thresholds)

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
func sayHello() {
    print("Hello")
}

public func add(a: Int, b: Int) -> Int {
    return a + b
}
`,
			expectedFunctions: []string{"sayHello", "add"},
			expectedCyclomatic: map[string]int{
				"sayHello": 1,
				"add":      1,
			},
			expectedParams: map[string]int{
				"sayHello": 0,
				"add":      2,
			},
		},
		{
			name: "Attributes and Generics",
			code: `
@available(iOS 13.0, *)
@discardableResult
func request<T: Decodable>(url: URL) async throws -> T {
    let (data, _) = try await URLSession.shared.data(from: url)
    return try JSONDecoder().decode(T.self, from: data)
}
`,
			expectedFunctions: []string{"request"},
			expectedCyclomatic: map[string]int{
				"request": 1,
			},
			expectedParams: map[string]int{
				"request": 1,
			},
		},
		{
			name: "Init and Deinit",
			code: `
class User {
    var name: String
    
    init(name: String) {
        self.name = name
    }
    
    deinit {
        print("Goodbye \(name)")
    }
}
`,
			expectedFunctions: []string{"init", "deinit"},
			expectedCyclomatic: map[string]int{
				"init":   1,
				"deinit": 1,
			},
			expectedParams: map[string]int{
				"init":   1,
				"deinit": 0,
			},
		},
		{
			name: "Complexity Calculation",
			code: `
func complexLogic(x: Int?) {
    guard let x = x else { return }
    
    if x > 0 {
        print("positive")
    } else if x < 0 {
        print("negative")
    }
    
    switch x {
    case 1:
        print("one")
    case 2:
        print("two")
    default:
        break
    }
    
    for i in 0..<10 {
        if i % 2 == 0 { continue }
    }
    
    // Optional chaining
    let c = someObj?.params?.count
    
    // Nil coalescing
    let d = c ?? 0
    
    // Ternary
    let e = d > 5 ? true : false
    
    // Binary
    if a && b || c {
         // ...
    }
}
`,
			expectedFunctions: []string{"complexLogic"},
			expectedCyclomatic: map[string]int{
				"complexLogic": 14,
			},
			expectedParams: map[string]int{
				"complexLogic": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := analyzer.AnalyzeFile("test.swift", []byte(tt.code))
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
