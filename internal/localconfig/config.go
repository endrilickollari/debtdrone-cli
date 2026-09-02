// Package localconfig defines DebtDrone's durable, local user configuration.
// It is intentionally independent from Cobra, the TUI, MCP, and the scanner so
// every entry point can resolve the same settings without owning the schema.
package localconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion       = 1
	MaximumComplexity    = 10_000
	MaximumDisplayResult = 100_000
	configDirectoryName  = "debtdrone"
	configFileName       = "config.yaml"
)

type Key string

const (
	KeyOutputFormat    Key = "scan.output_format"
	KeyFailOn          Key = "scan.fail_on"
	KeyMaxComplexity   Key = "scan.max_complexity"
	KeySecurityScan    Key = "scan.security_scan"
	KeyCoverage        Key = "scan.coverage"
	KeyUpdateChecks    Key = "update.checks"
	KeyShowLineNumbers Key = "ui.show_line_numbers"
	KeyMaxResults      Key = "ui.max_results"
	KeyHistoryEnabled  Key = "history.enabled"
)

type Source string

const (
	SourceDefault     Source = "default"
	SourceConfigFile  Source = "config_file"
	SourceEnvironment Source = "environment"
	SourceFlag        Source = "flag"
)

type Definition struct {
	Key         Key
	Environment string
	Type        string
	Default     string
	Description string
}

var definitions = []Definition{
	{KeyOutputFormat, "DEBTDRONE_OUTPUT_FORMAT", "string", "text", "Output format: text or json"},
	{KeyFailOn, "DEBTDRONE_FAIL_ON", "string", "none", "Minimum severity that fails a scan: none, low, medium, high, or critical"},
	{KeyMaxComplexity, "DEBTDRONE_MAX_COMPLEXITY", "integer", "15", "High cyclomatic-complexity threshold per function"},
	{KeySecurityScan, "DEBTDRONE_SECURITY_SCAN", "boolean", "true", "Enable Trivy vulnerability scanning"},
	{KeyCoverage, "DEBTDRONE_COVERAGE", "boolean", "false", "Parse existing coverage artifacts without running tests"},
	{KeyUpdateChecks, "DEBTDRONE_UPDATE_CHECKS", "boolean", "true", "Check for newer releases on startup"},
	{KeyShowLineNumbers, "DEBTDRONE_SHOW_LINE_NUMBERS", "boolean", "true", "Show line information in interactive results"},
	{KeyMaxResults, "DEBTDRONE_MAX_RESULTS", "integer", "500", "Maximum interactive results to render; zero is unlimited"},
	{KeyHistoryEnabled, "DEBTDRONE_HISTORY_ENABLED", "boolean", "true", "Persist privacy-conscious local scan history"},
}

type Values struct {
	OutputFormat    string
	FailOn          string
	MaxComplexity   int
	SecurityScan    bool
	Coverage        bool
	UpdateChecks    bool
	ShowLineNumbers bool
	MaxResults      int
	HistoryEnabled  bool
}

type Overrides struct {
	OutputFormat    *string
	FailOn          *string
	MaxComplexity   *int
	SecurityScan    *bool
	Coverage        *bool
	UpdateChecks    *bool
	ShowLineNumbers *bool
	MaxResults      *int
	HistoryEnabled  *bool
}

type Resolved struct {
	Values  Values
	Sources map[Key]Source
}

type document struct {
	Version int             `yaml:"version"`
	Scan    scanDocument    `yaml:"scan"`
	Update  updateDocument  `yaml:"update"`
	UI      uiDocument      `yaml:"ui"`
	History historyDocument `yaml:"history"`
}

type scanDocument struct {
	OutputFormat  *string `yaml:"output_format"`
	FailOn        *string `yaml:"fail_on"`
	MaxComplexity *int    `yaml:"max_complexity"`
	SecurityScan  *bool   `yaml:"security_scan"`
	Coverage      *bool   `yaml:"coverage"`
}

type updateDocument struct {
	Checks *bool `yaml:"checks"`
}

type uiDocument struct {
	ShowLineNumbers *bool `yaml:"show_line_numbers"`
	MaxResults      *int  `yaml:"max_results"`
}

type historyDocument struct {
	Enabled *bool `yaml:"enabled"`
}

func Definitions() []Definition {
	return append([]Definition(nil), definitions...)
}

func ParseKey(value string) (Key, error) {
	key := Key(strings.TrimSpace(value))
	for _, definition := range definitions {
		if definition.Key == key {
			return key, nil
		}
	}
	supported := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		supported = append(supported, string(definition.Key))
	}
	return "", fmt.Errorf("unknown configuration key %q; supported keys: %s", value, strings.Join(supported, ", "))
}

func ParseOverride(key Key, value string) (Overrides, error) {
	var overrides Overrides
	if err := overrides.Set(key, value); err != nil {
		return Overrides{}, err
	}
	return overrides, nil
}

// Set parses and validates one value into an override layer. It is used by
// adapters to collect only explicitly supplied flags or session settings.
func (overrides *Overrides) Set(key Key, value string) error {
	if overrides == nil {
		return errors.New("configuration overrides are required")
	}
	return setStringValue(overrides, key, value)
}

func Value(values Values, key Key) string {
	switch key {
	case KeyOutputFormat:
		return values.OutputFormat
	case KeyFailOn:
		return values.FailOn
	case KeyMaxComplexity:
		return strconv.Itoa(values.MaxComplexity)
	case KeySecurityScan:
		return strconv.FormatBool(values.SecurityScan)
	case KeyCoverage:
		return strconv.FormatBool(values.Coverage)
	case KeyUpdateChecks:
		return strconv.FormatBool(values.UpdateChecks)
	case KeyShowLineNumbers:
		return strconv.FormatBool(values.ShowLineNumbers)
	case KeyMaxResults:
		return strconv.Itoa(values.MaxResults)
	case KeyHistoryEnabled:
		return strconv.FormatBool(values.HistoryEnabled)
	default:
		return ""
	}
}

func OverrideValue(overrides Overrides, key Key) (string, bool) {
	switch key {
	case KeyOutputFormat:
		if overrides.OutputFormat != nil {
			return *overrides.OutputFormat, true
		}
	case KeyFailOn:
		if overrides.FailOn != nil {
			return *overrides.FailOn, true
		}
	case KeyMaxComplexity:
		if overrides.MaxComplexity != nil {
			return strconv.Itoa(*overrides.MaxComplexity), true
		}
	case KeySecurityScan:
		if overrides.SecurityScan != nil {
			return strconv.FormatBool(*overrides.SecurityScan), true
		}
	case KeyCoverage:
		if overrides.Coverage != nil {
			return strconv.FormatBool(*overrides.Coverage), true
		}
	case KeyUpdateChecks:
		if overrides.UpdateChecks != nil {
			return strconv.FormatBool(*overrides.UpdateChecks), true
		}
	case KeyShowLineNumbers:
		if overrides.ShowLineNumbers != nil {
			return strconv.FormatBool(*overrides.ShowLineNumbers), true
		}
	case KeyMaxResults:
		if overrides.MaxResults != nil {
			return strconv.Itoa(*overrides.MaxResults), true
		}
	case KeyHistoryEnabled:
		if overrides.HistoryEnabled != nil {
			return strconv.FormatBool(*overrides.HistoryEnabled), true
		}
	}
	return "", false
}

func Defaults() Values {
	return Values{
		OutputFormat:    "text",
		FailOn:          "none",
		MaxComplexity:   15,
		SecurityScan:    true,
		Coverage:        false,
		UpdateChecks:    true,
		ShowLineNumbers: true,
		MaxResults:      500,
		HistoryEnabled:  true,
	}
}

// DefaultPath returns the OS-native per-user configuration path. UserConfigDir
// follows XDG_CONFIG_HOME on Unix, Library/Application Support on macOS, and
// AppData on Windows.
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return PathIn(configDir), nil
}

func PathIn(userConfigDir string) string {
	return filepath.Join(userConfigDir, configDirectoryName, configFileName)
}

// Load reads one strict, versioned YAML document. A missing file is equivalent
// to an empty configuration layer; all other read and validation failures are
// returned to the caller.
func Load(path string) (Overrides, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Overrides{}, false, nil
	}
	if err != nil {
		return Overrides{}, false, fmt.Errorf("read configuration %q: %w; check file permissions", path, err)
	}

	overrides, err := Parse(data)
	if err != nil {
		return Overrides{}, true, fmt.Errorf("invalid configuration %q: %w; fix it or move it aside before retrying", path, err)
	}
	return overrides, true, nil
}

func Parse(data []byte) (Overrides, error) {
	root, err := decodeSingleDocument(data)
	if err != nil {
		return Overrides{}, err
	}

	var header struct {
		Version *int `yaml:"version"`
	}
	if err := root.Decode(&header); err != nil {
		return Overrides{}, fmt.Errorf("decode version: %w", err)
	}
	if header.Version == nil {
		return Overrides{}, fmt.Errorf("version is required; set version: %d", CurrentVersion)
	}
	if *header.Version > CurrentVersion {
		return Overrides{}, fmt.Errorf("schema version %d is newer than supported version %d; upgrade DebtDrone before using this file", *header.Version, CurrentVersion)
	}
	if *header.Version < CurrentVersion {
		return Overrides{}, fmt.Errorf("schema version %d is no longer supported; migrate the file to version %d", *header.Version, CurrentVersion)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var config document
	if err := decoder.Decode(&config); err != nil {
		return Overrides{}, fmt.Errorf("decode YAML: %w", err)
	}

	overrides := Overrides{
		OutputFormat:    config.Scan.OutputFormat,
		FailOn:          config.Scan.FailOn,
		MaxComplexity:   config.Scan.MaxComplexity,
		SecurityScan:    config.Scan.SecurityScan,
		Coverage:        config.Scan.Coverage,
		UpdateChecks:    config.Update.Checks,
		ShowLineNumbers: config.UI.ShowLineNumbers,
		MaxResults:      config.UI.MaxResults,
		HistoryEnabled:  config.History.Enabled,
	}
	if err := validateOverrides(overrides); err != nil {
		return Overrides{}, err
	}
	return overrides, nil
}

func decodeSingleDocument(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("configuration is empty; set version: %d", CurrentVersion)
		}
		return nil, fmt.Errorf("decode YAML: %w", err)
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("decode YAML: %w", err)
		}
		return nil, errors.New("only one YAML document is supported")
	}
	return &root, nil
}

// Resolve applies the documented precedence from lowest to highest: built-in
// defaults, the configuration file, DEBTDRONE_* environment variables, then
// explicitly supplied command flags.
func Resolve(file Overrides, environment map[string]string, flags Overrides) (Resolved, error) {
	if err := validateOverrides(file); err != nil {
		return Resolved{}, fmt.Errorf("config file: %w", err)
	}
	env, err := overridesFromEnvironment(environment)
	if err != nil {
		return Resolved{}, err
	}
	if err := validateOverrides(flags); err != nil {
		return Resolved{}, fmt.Errorf("flags: %w", err)
	}

	resolved := Resolved{Values: Defaults(), Sources: make(map[Key]Source, len(definitions))}
	for _, definition := range definitions {
		resolved.Sources[definition.Key] = SourceDefault
	}
	apply(&resolved, file, SourceConfigFile)
	apply(&resolved, env, SourceEnvironment)
	apply(&resolved, flags, SourceFlag)
	return resolved, nil
}

func overridesFromEnvironment(environment map[string]string) (Overrides, error) {
	known := make(map[string]struct{}, len(definitions))
	supported := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		known[definition.Environment] = struct{}{}
		supported = append(supported, definition.Environment)
	}

	var unknown []string
	for key := range environment {
		if strings.HasPrefix(key, "DEBTDRONE_") {
			if _, ok := known[key]; !ok {
				unknown = append(unknown, key)
			}
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		sort.Strings(supported)
		return Overrides{}, fmt.Errorf("unknown environment variable(s) %s; supported variables: %s", strings.Join(unknown, ", "), strings.Join(supported, ", "))
	}

	var result Overrides
	for _, definition := range definitions {
		value, ok := environment[definition.Environment]
		if !ok {
			continue
		}
		if value == "" {
			return Overrides{}, fmt.Errorf("environment variable %s cannot be empty", definition.Environment)
		}
		if err := setStringValue(&result, definition.Key, value); err != nil {
			return Overrides{}, fmt.Errorf("environment variable %s: %w", definition.Environment, err)
		}
	}
	return result, nil
}

func setStringValue(overrides *Overrides, key Key, value string) error {
	switch key {
	case KeyOutputFormat:
		overrides.OutputFormat = pointer(strings.ToLower(value))
	case KeyFailOn:
		overrides.FailOn = pointer(strings.ToLower(value))
	case KeyMaxComplexity:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer, got %q", key, value)
		}
		overrides.MaxComplexity = &parsed
	case KeySecurityScan:
		parsed, err := parseBoolean(value)
		if err != nil {
			return fmt.Errorf("%s must be true or false, got %q", key, value)
		}
		overrides.SecurityScan = &parsed
	case KeyCoverage:
		parsed, err := parseBoolean(value)
		if err != nil {
			return fmt.Errorf("%s must be true or false, got %q", key, value)
		}
		overrides.Coverage = &parsed
	case KeyUpdateChecks:
		parsed, err := parseBoolean(value)
		if err != nil {
			return fmt.Errorf("%s must be true or false, got %q", key, value)
		}
		overrides.UpdateChecks = &parsed
	case KeyShowLineNumbers:
		parsed, err := parseBoolean(value)
		if err != nil {
			return fmt.Errorf("%s must be true or false, got %q", key, value)
		}
		overrides.ShowLineNumbers = &parsed
	case KeyMaxResults:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer, got %q", key, value)
		}
		overrides.MaxResults = &parsed
	case KeyHistoryEnabled:
		parsed, err := parseBoolean(value)
		if err != nil {
			return fmt.Errorf("%s must be true or false, got %q", key, value)
		}
		overrides.HistoryEnabled = &parsed
	default:
		return fmt.Errorf("unknown configuration key %q", key)
	}
	return validateOverrides(*overrides)
}

func parseBoolean(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func validateOverrides(overrides Overrides) error {
	if overrides.OutputFormat != nil && *overrides.OutputFormat != "text" && *overrides.OutputFormat != "json" {
		return fmt.Errorf("%s must be one of text or json, got %q", KeyOutputFormat, *overrides.OutputFormat)
	}
	if overrides.FailOn != nil {
		switch *overrides.FailOn {
		case "none", "low", "medium", "high", "critical":
		default:
			return fmt.Errorf("%s must be one of none, low, medium, high, or critical, got %q", KeyFailOn, *overrides.FailOn)
		}
	}
	if overrides.MaxComplexity != nil && (*overrides.MaxComplexity < 1 || *overrides.MaxComplexity > MaximumComplexity) {
		return fmt.Errorf("%s must be between 1 and %d, got %d", KeyMaxComplexity, MaximumComplexity, *overrides.MaxComplexity)
	}
	if overrides.MaxResults != nil && (*overrides.MaxResults < 0 || *overrides.MaxResults > MaximumDisplayResult) {
		return fmt.Errorf("%s must be between 0 and %d, got %d", KeyMaxResults, MaximumDisplayResult, *overrides.MaxResults)
	}
	return nil
}

func apply(resolved *Resolved, overrides Overrides, source Source) {
	if overrides.OutputFormat != nil {
		resolved.Values.OutputFormat = *overrides.OutputFormat
		resolved.Sources[KeyOutputFormat] = source
	}
	if overrides.FailOn != nil {
		resolved.Values.FailOn = *overrides.FailOn
		resolved.Sources[KeyFailOn] = source
	}
	if overrides.MaxComplexity != nil {
		resolved.Values.MaxComplexity = *overrides.MaxComplexity
		resolved.Sources[KeyMaxComplexity] = source
	}
	if overrides.SecurityScan != nil {
		resolved.Values.SecurityScan = *overrides.SecurityScan
		resolved.Sources[KeySecurityScan] = source
	}
	if overrides.Coverage != nil {
		resolved.Values.Coverage = *overrides.Coverage
		resolved.Sources[KeyCoverage] = source
	}
	if overrides.UpdateChecks != nil {
		resolved.Values.UpdateChecks = *overrides.UpdateChecks
		resolved.Sources[KeyUpdateChecks] = source
	}
	if overrides.ShowLineNumbers != nil {
		resolved.Values.ShowLineNumbers = *overrides.ShowLineNumbers
		resolved.Sources[KeyShowLineNumbers] = source
	}
	if overrides.MaxResults != nil {
		resolved.Values.MaxResults = *overrides.MaxResults
		resolved.Sources[KeyMaxResults] = source
	}
	if overrides.HistoryEnabled != nil {
		resolved.Values.HistoryEnabled = *overrides.HistoryEnabled
		resolved.Sources[KeyHistoryEnabled] = source
	}
}

func pointer[T any](value T) *T { return &value }
