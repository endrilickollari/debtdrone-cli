package localconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePrecedence(t *testing.T) {
	tests := []struct {
		name        string
		file        Overrides
		environment map[string]string
		flags       Overrides
		want        Values
		wantSources map[Key]Source
	}{
		{
			name: "defaults",
			want: Defaults(),
			wantSources: map[Key]Source{
				KeyOutputFormat: SourceDefault, KeyFailOn: SourceDefault,
				KeyMaxComplexity: SourceDefault, KeySecurityScan: SourceDefault,
				KeyCoverage: SourceDefault, KeyUpdateChecks: SourceDefault,
				KeyShowLineNumbers: SourceDefault, KeyMaxResults: SourceDefault,
				KeyHistoryEnabled: SourceDefault,
			},
		},
		{
			name: "config file overrides defaults",
			file: Overrides{
				OutputFormat: pointer("json"), MaxComplexity: pointer(20),
				SecurityScan: pointer(false), HistoryEnabled: pointer(false),
			},
			want: Values{
				OutputFormat: "json", FailOn: "none", MaxComplexity: 20,
				SecurityScan: false, Coverage: false, UpdateChecks: true,
				ShowLineNumbers: true, MaxResults: 500, HistoryEnabled: false,
			},
			wantSources: map[Key]Source{
				KeyOutputFormat: SourceConfigFile, KeyFailOn: SourceDefault,
				KeyMaxComplexity: SourceConfigFile, KeySecurityScan: SourceConfigFile,
				KeyCoverage: SourceDefault, KeyUpdateChecks: SourceDefault,
				KeyShowLineNumbers: SourceDefault, KeyMaxResults: SourceDefault,
				KeyHistoryEnabled: SourceConfigFile,
			},
		},
		{
			name: "environment overrides config file",
			file: Overrides{
				FailOn: pointer("medium"), Coverage: pointer(false), MaxResults: pointer(50),
			},
			environment: map[string]string{
				"DEBTDRONE_FAIL_ON": "critical", "DEBTDRONE_COVERAGE": "true",
				"DEBTDRONE_MAX_RESULTS": "0",
			},
			want: Values{
				OutputFormat: "text", FailOn: "critical", MaxComplexity: 15,
				SecurityScan: true, Coverage: true, UpdateChecks: true,
				ShowLineNumbers: true, MaxResults: 0, HistoryEnabled: true,
			},
			wantSources: map[Key]Source{
				KeyOutputFormat: SourceDefault, KeyFailOn: SourceEnvironment,
				KeyMaxComplexity: SourceDefault, KeySecurityScan: SourceDefault,
				KeyCoverage: SourceEnvironment, KeyUpdateChecks: SourceDefault,
				KeyShowLineNumbers: SourceDefault, KeyMaxResults: SourceEnvironment,
				KeyHistoryEnabled: SourceDefault,
			},
		},
		{
			name: "flags have highest precedence",
			file: Overrides{MaxComplexity: pointer(20), SecurityScan: pointer(false)},
			environment: map[string]string{
				"DEBTDRONE_MAX_COMPLEXITY": "30", "DEBTDRONE_SECURITY_SCAN": "false",
			},
			flags: Overrides{MaxComplexity: pointer(40), SecurityScan: pointer(true)},
			want: Values{
				OutputFormat: "text", FailOn: "none", MaxComplexity: 40,
				SecurityScan: true, Coverage: false, UpdateChecks: true,
				ShowLineNumbers: true, MaxResults: 500, HistoryEnabled: true,
			},
			wantSources: map[Key]Source{
				KeyOutputFormat: SourceDefault, KeyFailOn: SourceDefault,
				KeyMaxComplexity: SourceFlag, KeySecurityScan: SourceFlag,
				KeyCoverage: SourceDefault, KeyUpdateChecks: SourceDefault,
				KeyShowLineNumbers: SourceDefault, KeyMaxResults: SourceDefault,
				KeyHistoryEnabled: SourceDefault,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := Resolve(test.file, test.environment, test.flags)
			require.NoError(t, err)
			assert.Equal(t, test.want, resolved.Values)
			assert.Equal(t, test.wantSources, resolved.Sources)
		})
	}
}

func TestParse(t *testing.T) {
	overrides, err := Parse([]byte(`version: 1
scan:
  output_format: json
  fail_on: high
  max_complexity: 25
  security_scan: false
  coverage: true
update:
  checks: false
ui:
  show_line_numbers: false
  max_results: 0
history:
  enabled: false
`))
	require.NoError(t, err)
	require.NotNil(t, overrides.OutputFormat)
	assert.Equal(t, "json", *overrides.OutputFormat)
	assert.Equal(t, "high", *overrides.FailOn)
	assert.Equal(t, 25, *overrides.MaxComplexity)
	assert.False(t, *overrides.SecurityScan)
	assert.True(t, *overrides.Coverage)
	assert.False(t, *overrides.UpdateChecks)
	assert.False(t, *overrides.ShowLineNumbers)
	assert.Equal(t, 0, *overrides.MaxResults)
	assert.False(t, *overrides.HistoryEnabled)
}

func TestParseRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty file", content: "", want: "configuration is empty; set version: 1"},
		{name: "missing version", content: "scan:\n  output_format: text\n", want: "version is required; set version: 1"},
		{name: "future version with new field", content: "version: 2\nfuture_mode: automatic\n", want: "newer than supported version 1; upgrade DebtDrone"},
		{name: "zero version", content: "version: 0\n", want: "schema version 0 is no longer supported; migrate"},
		{name: "old version", content: "version: -1\n", want: "no longer supported; migrate"},
		{name: "unknown key", content: "version: 1\nscan:\n  mystery_mode: true\n", want: "field mystery_mode not found"},
		{name: "unknown section", content: "version: 1\nnetwork:\n  enabled: true\n", want: "field network not found"},
		{name: "wrong type", content: "version: 1\nscan:\n  security_scan: sometimes\n", want: "cannot unmarshal"},
		{name: "invalid format", content: "version: 1\nscan:\n  output_format: xml\n", want: "scan.output_format must be one of text or json"},
		{name: "invalid severity", content: "version: 1\nscan:\n  fail_on: urgent\n", want: "scan.fail_on must be one of"},
		{name: "invalid complexity", content: "version: 1\nscan:\n  max_complexity: 0\n", want: "scan.max_complexity must be between 1 and 10000"},
		{name: "invalid result limit", content: "version: 1\nui:\n  max_results: -1\n", want: "ui.max_results must be between 0 and 100000"},
		{name: "multiple documents", content: "version: 1\n---\nversion: 1\n", want: "only one YAML document is supported"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.content))
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestResolveRejectsInvalidEnvironmentAndFlags(t *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		flags       Overrides
		want        string
	}{
		{name: "empty environment value", environment: map[string]string{"DEBTDRONE_FAIL_ON": ""}, want: "DEBTDRONE_FAIL_ON cannot be empty"},
		{name: "invalid environment boolean", environment: map[string]string{"DEBTDRONE_COVERAGE": "sometimes"}, want: "DEBTDRONE_COVERAGE: scan.coverage must be true or false"},
		{name: "numeric environment boolean", environment: map[string]string{"DEBTDRONE_COVERAGE": "1"}, want: "DEBTDRONE_COVERAGE: scan.coverage must be true or false"},
		{name: "invalid environment integer", environment: map[string]string{"DEBTDRONE_MAX_RESULTS": "many"}, want: "DEBTDRONE_MAX_RESULTS: ui.max_results must be an integer"},
		{name: "unknown environment key", environment: map[string]string{"DEBTDRONE_MAX_COMPLEXTY": "20"}, want: "unknown environment variable(s) DEBTDRONE_MAX_COMPLEXTY; supported variables:"},
		{name: "invalid flag", flags: Overrides{MaxComplexity: pointer(0)}, want: "flags: scan.max_complexity must be between"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Resolve(Overrides{}, test.environment, test.flags)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestResolveIgnoresUnrelatedEnvironmentVariables(t *testing.T) {
	resolved, err := Resolve(Overrides{}, map[string]string{"CI": "true", "PATH": "/bin"}, Overrides{})
	require.NoError(t, err)
	assert.Equal(t, Defaults(), resolved.Values)
}

func TestLoad(t *testing.T) {
	t.Run("missing file is an empty layer", func(t *testing.T) {
		_, found, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("error includes the path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("version: 2\n"), 0o600))
		_, found, err := Load(path)
		assert.True(t, found)
		require.ErrorContains(t, err, path)
		require.ErrorContains(t, err, "upgrade DebtDrone")
	})
}

func TestPathInUsesUserConfigDirectory(t *testing.T) {
	tests := []struct {
		name       string
		configRoot string
	}{
		{name: "Linux XDG", configRoot: filepath.Join("home", "developer", ".config")},
		{name: "macOS Application Support", configRoot: filepath.Join("Users", "developer", "Library", "Application Support")},
		{name: "Windows AppData", configRoot: filepath.Join("Users", "developer", "AppData", "Roaming")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, filepath.Join(test.configRoot, "debtdrone", "config.yaml"), PathIn(test.configRoot))
		})
	}
}

func TestDefinitionsAreReturnedAsACopy(t *testing.T) {
	items := Definitions()
	require.NotEmpty(t, items)
	items[0].Default = "changed"
	assert.Equal(t, "text", Definitions()[0].Default)
}
