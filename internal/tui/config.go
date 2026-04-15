package tui

type configMode int

const (
	configNavigating configMode = iota
	configEditing
)

type configItem struct {
	Category    string
	Key         string
	Value       string
	Type        string
	Description string
	Options     []string // Predefined choices for "choice" type
	IsOption    bool     // If true, user cycles through Options instead of typing
}

func defaultConfigItems() []configItem {
	return []configItem{
		{
			Category:    "General",
			Key:         "Output Format",
			Value:       "text",
			Type:        "string",
			Description: "Render mode for scan results",
			Options:     []string{"text", "json"},
			IsOption:    true,
		},
		{
			Category:    "General",
			Key:         "Auto-Update Checks",
			Value:       "true",
			Type:        "bool",
			Description: "Check for a newer release on each startup",
		},
		{
			Category:    "Quality Gate",
			Key:         "Fail on Severity",
			Value:       "high",
			Type:        "string",
			Description: "Min severity for non-zero exit code",
			Options:     []string{"low", "medium", "high", "critical", "none"},
			IsOption:    true,
		},
		{
			Category:    "Quality Gate",
			Key:         "Max Complexity",
			Value:       "15",
			Type:        "int",
			Description: "Cyclomatic-complexity threshold per function",
		},
		{
			Category:    "Quality Gate",
			Key:         "Security Scan",
			Value:       "true",
			Type:        "bool",
			Description: "Run Trivy vulnerability and secret detection",
		},
		{
			Category:    "Display",
			Key:         "Show Line Numbers",
			Value:       "true",
			Type:        "bool",
			Description: "Include line:col in the results list",
		},
		{
			Category:    "Display",
			Key:         "Max Results",
			Value:       "500",
			Type:        "int",
			Description: "Cap on issues rendered per scan (0 = unlimited)",
		},
	}
}
