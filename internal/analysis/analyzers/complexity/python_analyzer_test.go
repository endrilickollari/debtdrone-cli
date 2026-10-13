package complexity

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pythonSpan is what the analyzer reports for one function.
type pythonSpan struct {
	start, end, loc, cyclomatic int
}

func analyzePython(t *testing.T, code string) map[string]pythonSpan {
	t.Helper()
	metrics, err := NewPythonAnalyzer(models.DefaultComplexityThresholds()).AnalyzeFile("app/module.py", []byte(code))
	require.NoError(t, err)
	spans := map[string]pythonSpan{}
	for _, m := range metrics {
		_, seen := spans[m.FunctionName]
		require.False(t, seen, "one %s per test", m.FunctionName)
		spans[m.FunctionName] = pythonSpan{m.StartLine, m.EndLine, m.LinesOfCode, m.CyclomaticComplexity}
	}
	return spans
}

// LinesOfCode is the physical line span of the definition, from its def line
// to its last line, as the Go analyzer counts it; it was left 0 (END-175).
// Start and end lines and complexity are unchanged by it.
func TestPythonLinesOfCodeIsTheDefinitionsLineSpan(t *testing.T) {
	cases := []struct {
		name string
		code string
		want map[string]pythonSpan
	}{
		{"single-line function", "def one(): return 1\n",
			map[string]pythonSpan{"one": {1, 1, 1, 1}}},
		{"multiline function with branches", `import os


def run_question(question, history):
    if not question:
        return None
    for turn in history:
        if turn.answer:
            continue
    return question
`, map[string]pythonSpan{"run_question": {4, 10, 7, 4}}},
		{"async function", `async def render_message(message):
    if message.partial:
        await message.flush()
    return message.text
`, map[string]pythonSpan{"render_message": {1, 4, 4, 2}}},
		{"decorated function counts from its def line, as its start line does", `@cache
@trace(name="load")
def load(path):
    with open(path) as f:
        return f.read()
`, map[string]pythonSpan{"load": {3, 5, 3, 1}}},
		{"nested functions each count their own span", `def outer(items):
    def inner(item):
        if item:
            return item
        return None
    return [inner(i) for i in items]
`, // Complexity is unchanged: the parent's count already includes the nested branch.
			map[string]pythonSpan{"outer": {1, 6, 6, 2}, "inner": {2, 5, 4, 2}}},
		{"method in a class", `class Engine:
    def process(self, job):
        if job.done:
            return
        job.run()
`, map[string]pythonSpan{"process": {2, 5, 4, 2}}},
		{"no final newline", "def tail(x):\n    return x", map[string]pythonSpan{"tail": {1, 2, 2, 1}}},
		{"trailing blank lines and a comment after the body are not counted", `def body(x):
    return x


# a comment about the next section
`, map[string]pythonSpan{"body": {1, 2, 2, 1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, analyzePython(t, c.code))
		})
	}
}

// The other tree-sitter analyzers that also left the count unset (END-175).
func TestJavaScriptAndJavaLinesOfCodeIsTheDefinitionsLineSpan(t *testing.T) {
	js, err := NewJavaScriptAnalyzer(models.DefaultComplexityThresholds()).AnalyzeFile("web/app.js", []byte(`function render(items) {
  if (!items) {
    return "";
  }
  return items.join(",");
}
`))
	require.NoError(t, err)
	require.NotEmpty(t, js)
	assert.Equal(t, "render", js[0].FunctionName)
	assert.Equal(t, 6, js[0].LinesOfCode)

	java, err := NewJavaAnalyzer(models.DefaultComplexityThresholds()).AnalyzeFile("src/Engine.java", []byte(`public class Engine {
    public int process(int job) {
        if (job > 0) {
            return job;
        }
        return 0;
    }
}
`))
	require.NoError(t, err)
	require.NotEmpty(t, java)
	assert.Equal(t, "process", java[0].FunctionName)
	assert.Equal(t, 6, java[0].LinesOfCode)
}
