package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestCSharpAnalyzer_AnalyzeFile_Complex(t *testing.T) {
	analyzer := NewCSharpAnalyzer(models.ComplexityThresholds{})

	code := `
	using System;

	public class TestClass
	{
		// 1. Standard method
		public void StandardMethod(int a)
		{
			if (a > 0)
			{
				Console.WriteLine("Positive");
			}
		}

		// 2. Attribute
		[HttpGet]
		public int AttributeMethod()
		{
			return 42;
		}

		// 3. Generics
		public void GenericMethod<T>(T items) where T : IEnumerable
		{
			foreach (var item in items)
			{
				if (item != null) continue;
			}
		}

		// 4. Expression-bodied (might be parsed as method_declaration with arrow_expression_clause)
		public int ExpressionBodied(int x) => x > 0 ? x : -x;

		// 5. Local function
		public void Outer()
		{
			void Inner(int y)
			{
				if (y == 0) return;
			}
			Inner(5);
		}
		
		// 6. Constructor
		public TestClass() {
			int x = 0;
			while(x < 10) x++;
		}
	}
	`

	metrics, err := analyzer.AnalyzeFile("Test.cs", []byte(code))
	assert.NoError(t, err)

	functionNames := make(map[string]*models.ComplexityMetric)
	for _, m := range metrics {
		functionNames[m.FunctionName] = &m
		val := m
		functionNames[m.FunctionName] = &val
	}

	if m, ok := functionNames["StandardMethod"]; ok {
		assert.Equal(t, 2, m.CyclomaticComplexity)
		assert.Equal(t, 1, m.ParameterCount)
	} else {
		t.Errorf("StandardMethod not found")
	}

	if m, ok := functionNames["AttributeMethod"]; ok {
		assert.Equal(t, 1, m.CyclomaticComplexity)
	} else {
		t.Errorf("AttributeMethod not found")
	}

	if m, ok := functionNames["GenericMethod"]; ok {
		assert.Equal(t, 3, m.CyclomaticComplexity)
	} else {
		t.Errorf("GenericMethod not found")
	}

	if m, ok := functionNames["ExpressionBodied"]; ok {
		assert.Equal(t, 2, m.CyclomaticComplexity)
	} else {
		t.Errorf("ExpressionBodied not found")
	}

	if _, ok := functionNames["Outer"]; !ok {
		t.Errorf("Outer method not found")
	}
	if m, ok := functionNames["Inner"]; ok {
		assert.Equal(t, 2, m.CyclomaticComplexity)
	} else {
		t.Log("Local function Inner not found as separate metric, which might be acceptable if treated as part of Outer's body complexity, but ideally we capture it.")
	}

	if m, ok := functionNames["TestClass"]; ok {
		assert.Equal(t, 2, m.CyclomaticComplexity)
	} else {
		t.Errorf("Constructor TestClass not found")
	}
}
