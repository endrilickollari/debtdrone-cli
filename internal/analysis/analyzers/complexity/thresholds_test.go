package complexity

import (
	"strings"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonAnalyzerUsesConfiguredComplexityThreshold(t *testing.T) {
	code := "def classify(value):\n" + strings.Repeat("    if value:\n        value -= 1\n", 16)
	thresholds := models.DefaultComplexityThresholds()
	thresholds.CyclomaticHigh = 10
	thresholds.CyclomaticCritical = 20
	metrics, err := NewPythonAnalyzer(thresholds).AnalyzeFile("test.py", []byte(code))
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, "high", metrics[0].Severity)

	thresholds.CyclomaticHigh = 40
	thresholds.CyclomaticCritical = 80
	metrics, err = NewPythonAnalyzer(thresholds).AnalyzeFile("test.py", []byte(code))
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, "medium", metrics[0].Severity)
}

// The tree-sitter analyzers share one classifier with the Go analyzer's
// thresholds: at the default limit of 10, a function scoring 11–15 is a
// high-severity finding in every language, not only in Go. Before the
// classifier took the configured thresholds these languages only reported
// from 16 upwards, so this pins the behaviour change the SaaS engine inherits.
func TestConfiguredCyclomaticLimitAppliesToEveryLanguage(t *testing.T) {
	const branches = 12 // cyclomatic 13: above the limit, below the old fixed 15
	cBranches := strings.Repeat("  if (x) { x--; }\n", branches)
	cases := []struct {
		name     string
		file     string
		code     string
		analyzer func(models.ComplexityThresholds) Analyzer
	}{
		{"python", "a.py", "def f(x):\n" + strings.Repeat("    if x:\n        x -= 1\n", branches),
			func(t models.ComplexityThresholds) Analyzer { return NewPythonAnalyzer(t) }},
		{"javascript", "a.js", "function f(x) {\n" + cBranches + "}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewJavaScriptAnalyzer(t) }},
		{"typescript", "a.ts", "function f(x: number) {\n" + cBranches + "}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewTypeScriptAnalyzer(t) }},
		{"java", "A.java", "class A {\n void f(int x) {\n" + cBranches + " }\n}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewJavaAnalyzer(t) }},
		{"csharp", "A.cs", "class A {\n void F(int x) {\n" + cBranches + " }\n}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewCSharpAnalyzer(t) }},
		{"c", "a.c", "void f(int x) {\n" + cBranches + "}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewCCppAnalyzer(t) }},
		{"php", "a.php", "<?php\nfunction f($x) {\n" + strings.Repeat("  if ($x) { $x--; }\n", branches) + "}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewPHPAnalyzer(t) }},
		{"ruby", "a.rb", "def f(x)\n" + strings.Repeat("  if x\n    x -= 1\n  end\n", branches) + "end\n",
			func(t models.ComplexityThresholds) Analyzer { return NewRubyAnalyzer(t) }},
		{"rust", "a.rs", "fn f(mut x: i32) {\n" + strings.Repeat("  if x > 0 { x -= 1; }\n", branches) + "}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewRustAnalyzer(t) }},
		{"kotlin", "a.kt", "fun f(y: Int) {\n var x = y\n" + strings.Repeat("  if (x > 0) { x-- }\n", branches) + "}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewKotlinAnalyzer(t) }},
		{"swift", "a.swift", "func f(y: Int) {\n var x = y\n" + strings.Repeat("  if x > 0 { x -= 1 }\n", branches) + "}\n",
			func(t models.ComplexityThresholds) Analyzer { return NewSwiftAnalyzer(t) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metrics, err := tc.analyzer(models.DefaultComplexityThresholds()).AnalyzeFile(tc.file, []byte(tc.code))
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			m := metrics[0]
			require.Greater(t, m.CyclomaticComplexity, 10)
			require.LessOrEqual(t, m.CyclomaticComplexity, 15)
			assert.Equal(t, "high", m.Severity, "cyclomatic %d at limit 10", m.CyclomaticComplexity)

			raised := models.DefaultComplexityThresholds()
			raised.CyclomaticHigh, raised.CyclomaticCritical = 40, 80
			metrics, err = tc.analyzer(raised).AnalyzeFile(tc.file, []byte(tc.code))
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.NotContains(t, []string{"high", "critical"}, metrics[0].Severity,
				"raising the limit clears the finding")
		})
	}
}
