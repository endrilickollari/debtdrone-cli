package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ComplexityMetric struct {
	ID            uuid.UUID `json:"id" db:"id"`
	UserID        uuid.UUID `json:"user_id" db:"user_id"`
	RepositoryID  uuid.UUID `json:"repository_id" db:"repository_id"`
	AnalysisRunID uuid.UUID `json:"analysis_run_id" db:"analysis_run_id"`
	FilePath      string    `json:"file_path" db:"file_path"`
	FunctionName  string    `json:"function_name" db:"function_name"`
	StartLine     int       `json:"start_line" db:"start_line"`
	EndLine       int       `json:"end_line" db:"end_line"`
	StartColumn   *int      `json:"start_column,omitempty" db:"start_column"`
	EndColumn     *int      `json:"end_column,omitempty" db:"end_column"`

	CyclomaticComplexity int  `json:"cyclomatic_complexity" db:"cyclomatic_complexity"`
	CognitiveComplexity  *int `json:"cognitive_complexity,omitempty" db:"cognitive_complexity"`
	NestingDepth         int  `json:"nesting_depth" db:"nesting_depth"`
	ParameterCount       int  `json:"parameter_count" db:"parameter_count"`
	LinesOfCode          int  `json:"lines_of_code" db:"lines_of_code"`

	HalsteadVolume     *float64 `json:"halstead_volume,omitempty" db:"halstead_volume"`
	HalsteadDifficulty *float64 `json:"halstead_difficulty,omitempty" db:"halstead_difficulty"`
	HalsteadEffort     *float64 `json:"halstead_effort,omitempty" db:"halstead_effort"`
	HalsteadTime       *float64 `json:"halstead_time,omitempty" db:"halstead_time"`
	HalsteadBugs       *float64 `json:"halstead_bugs,omitempty" db:"halstead_bugs"`

	Severity           string  `json:"severity" db:"severity"`
	ComplexityCategory *string `json:"complexity_category,omitempty" db:"complexity_category"`

	TechnicalDebtMinutes int     `json:"technical_debt_minutes" db:"technical_debt_minutes"`
	CodeSnippet          *string `json:"code_snippet,omitempty" db:"code_snippet"`

	RefactoringSuggestions []RefactoringSuggestion `json:"refactoring_suggestions,omitempty" db:"refactoring_suggestions"`
	Language               string                  `json:"language" db:"language"`
	Metadata               *string                 `json:"metadata,omitempty" db:"metadata"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type RefactoringSuggestion struct {
	Type        string `json:"type"`
	Priority    string `json:"priority"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

type FileComplexitySummary struct {
	RepositoryID              uuid.UUID `json:"repository_id" db:"repository_id"`
	AnalysisRunID             uuid.UUID `json:"analysis_run_id" db:"analysis_run_id"`
	FilePath                  string    `json:"file_path" db:"file_path"`
	Language                  string    `json:"language" db:"language"`
	FunctionCount             int       `json:"function_count" db:"function_count"`
	AvgCyclomaticComplexity   float64   `json:"avg_cyclomatic_complexity" db:"avg_cyclomatic_complexity"`
	MaxCyclomaticComplexity   int       `json:"max_cyclomatic_complexity" db:"max_cyclomatic_complexity"`
	AvgCognitiveComplexity    *float64  `json:"avg_cognitive_complexity,omitempty" db:"avg_cognitive_complexity"`
	MaxCognitiveComplexity    *int      `json:"max_cognitive_complexity,omitempty" db:"max_cognitive_complexity"`
	AvgNestingDepth           float64   `json:"avg_nesting_depth" db:"avg_nesting_depth"`
	MaxNestingDepth           int       `json:"max_nesting_depth" db:"max_nesting_depth"`
	TotalLinesOfCode          int       `json:"total_lines_of_code" db:"total_lines_of_code"`
	TotalTechnicalDebtMinutes int       `json:"total_technical_debt_minutes" db:"total_technical_debt_minutes"`
	CriticalFunctions         int       `json:"critical_functions" db:"critical_functions"`
	HighComplexityFunctions   int       `json:"high_complexity_functions" db:"high_complexity_functions"`
	MediumComplexityFunctions int       `json:"medium_complexity_functions" db:"medium_complexity_functions"`
	LowComplexityFunctions    int       `json:"low_complexity_functions" db:"low_complexity_functions"`
}

type RepositoryComplexitySummary struct {
	RepositoryID             uuid.UUID `json:"repository_id" db:"repository_id"`
	AnalysisRunID            uuid.UUID `json:"analysis_run_id" db:"analysis_run_id"`
	AnalyzedFilesCount       int       `json:"analyzed_files_count" db:"analyzed_files_count"`
	TotalFunctions           int       `json:"total_functions" db:"total_functions"`
	AvgCyclomaticComplexity  float64   `json:"avg_cyclomatic_complexity" db:"avg_cyclomatic_complexity"`
	MaxCyclomaticComplexity  int       `json:"max_cyclomatic_complexity" db:"max_cyclomatic_complexity"`
	AvgCognitiveComplexity   *float64  `json:"avg_cognitive_complexity,omitempty" db:"avg_cognitive_complexity"`
	MaxCognitiveComplexity   *int      `json:"max_cognitive_complexity,omitempty" db:"max_cognitive_complexity"`
	TotalComplexityDebtHours float64   `json:"total_complexity_debt_hours" db:"total_complexity_debt_hours"`
	CriticalComplexityCount  int       `json:"critical_complexity_count" db:"critical_complexity_count"`
	HighComplexityCount      int       `json:"high_complexity_count" db:"high_complexity_count"`
	DeepNestingCount         int       `json:"deep_nesting_count" db:"deep_nesting_count"`
	LongParameterListCount   int       `json:"long_parameter_list_count" db:"long_parameter_list_count"`
	CriticalIssues           int       `json:"critical_issues" db:"critical_issues"`
	HighIssues               int       `json:"high_issues" db:"high_issues"`
	MediumIssues             int       `json:"medium_issues" db:"medium_issues"`
	LowIssues                int       `json:"low_issues" db:"low_issues"`
}

type ComplexityThresholds struct {
	CyclomaticHigh      int `json:"cyclomatic_high"`
	CyclomaticCritical  int `json:"cyclomatic_critical"`
	CognitiveHigh       int `json:"cognitive_high"`
	CognitiveCritical   int `json:"cognitive_critical"`
	NestingWarning      int `json:"nesting_warning"`
	NestingCritical     int `json:"nesting_critical"`
	ParameterWarning    int `json:"parameter_warning"`
	ParameterCritical   int `json:"parameter_critical"`
	LinesOfCodeWarning  int `json:"lines_of_code_warning"`
	LinesOfCodeCritical int `json:"lines_of_code_critical"`
}

func DefaultComplexityThresholds() ComplexityThresholds {
	return ComplexityThresholds{
		CyclomaticHigh:      10,
		CyclomaticCritical:  20,
		CognitiveHigh:       15,
		CognitiveCritical:   25,
		NestingWarning:      4,
		NestingCritical:     6,
		ParameterWarning:    5,
		ParameterCritical:   7,
		LinesOfCodeWarning:  150,
		LinesOfCodeCritical: 300,
	}
}

func (t ComplexityThresholds) DetermineSeverity(cyclomatic, cognitive, nesting, params int) string {
	if cyclomatic > t.CyclomaticCritical ||
		nesting > t.NestingCritical+1 ||
		cognitive > t.CognitiveCritical {
		return "critical"
	}

	if cyclomatic > t.CyclomaticHigh ||
		nesting > t.NestingWarning+1 ||
		params > t.ParameterCritical ||
		cognitive > t.CognitiveHigh {
		return "high"
	}

	if cyclomatic > t.CyclomaticHigh/2 ||
		nesting > t.NestingWarning ||
		params > t.ParameterWarning ||
		cognitive > t.CognitiveHigh/2 {
		return "medium"
	}

	return "low"
}

func CalculateTechnicalDebt(cyclomatic, cognitive, nesting, params, loc int) int {
	debtMinutes := 0

	if cyclomatic > 20 {
		debtMinutes += (cyclomatic - 20) * 15
	} else if cyclomatic > 10 {
		debtMinutes += (cyclomatic - 10) * 10
	} else if cyclomatic > 5 {
		debtMinutes += (cyclomatic - 5) * 5
	}

	if cognitive > 15 {
		debtMinutes += (cognitive - 15) * 8
	}
	if nesting > 5 {
		debtMinutes += (nesting - 5) * 20
	} else if nesting > 3 {
		debtMinutes += (nesting - 3) * 10
	}

	if params > 7 {
		debtMinutes += (params - 7) * 15
	} else if params > 5 {
		debtMinutes += (params - 5) * 10
	}

	if loc > 300 {
		debtMinutes += ((loc - 300) / 50) * 30
	} else if loc > 150 {
		debtMinutes += ((loc - 150) / 50) * 15
	}

	if debtMinutes < 0 {
		return 0
	}
	return debtMinutes
}

func GenerateRefactoringSuggestions(cyclomatic, cognitive, nesting, params, loc int) []RefactoringSuggestion {
	suggestions := []RefactoringSuggestion{}

	if cyclomatic > 20 {
		suggestions = append(suggestions, RefactoringSuggestion{
			Type:        "extract_method",
			Priority:    "high",
			Title:       "Extract Method",
			Description: "Break down this complex function into smaller, focused methods",
			Reason:      formatString("Cyclomatic complexity of %d exceeds critical threshold of 20", cyclomatic),
		})
	} else if cyclomatic > 10 {
		suggestions = append(suggestions, RefactoringSuggestion{
			Type:        "simplify_logic",
			Priority:    "medium",
			Title:       "Simplify Control Flow",
			Description: "Reduce the number of decision points and branches",
			Reason:      formatString("Cyclomatic complexity of %d exceeds recommended threshold of 10", cyclomatic),
		})
	}

	if nesting > 5 {
		suggestions = append(suggestions, RefactoringSuggestion{
			Type:        "reduce_nesting",
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use early returns, guard clauses, or extract nested logic into separate functions",
			Reason:      formatString("Nesting depth of %d makes code difficult to understand", nesting),
		})
	}

	if params > 7 {
		suggestions = append(suggestions, RefactoringSuggestion{
			Type:        "introduce_parameter_object",
			Priority:    "medium",
			Title:       "Introduce Parameter Object",
			Description: "Group related parameters into a configuration object or struct",
			Reason:      formatString("Function has %d parameters, making it hard to use and test", params),
		})
	}

	if loc > 300 {
		suggestions = append(suggestions, RefactoringSuggestion{
			Type:        "split_function",
			Priority:    "high",
			Title:       "Split Large Function",
			Description: "Break this large function into smaller, cohesive functions with single responsibilities",
			Reason:      formatString("Function length of %d lines exceeds maintainability threshold", loc),
		})
	}

	if cognitive > 15 {
		suggestions = append(suggestions, RefactoringSuggestion{
			Type:        "simplify_logic",
			Priority:    "high",
			Title:       "Reduce Cognitive Complexity",
			Description: "Simplify the mental model required to understand this code",
			Reason:      formatString("Cognitive complexity of %d makes code difficult to comprehend", cognitive),
		})
	}

	return suggestions
}

func formatString(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

type HalsteadMetrics struct {
	N1         int     `json:"n1"`
	N2         int     `json:"n2"`
	TotalN1    int     `json:"total_n1"`
	TotalN2    int     `json:"total_n2"`
	Vocabulary int     `json:"vocabulary"`
	Length     int     `json:"length"`
	Volume     float64 `json:"volume"`
	Difficulty float64 `json:"difficulty"`
	Effort     float64 `json:"effort"`
	Time       float64 `json:"time"`
	Bugs       float64 `json:"bugs"`
}

type ComplexityAnalysisRequest struct {
	RepositoryID uuid.UUID `json:"repository_id"`
	FilePath     string    `json:"file_path,omitempty"`
	Language     string    `json:"language,omitempty"`
	Threshold    int       `json:"threshold,omitempty"`
}

type ComplexityAnalysisResponse struct {
	AnalysisRunID uuid.UUID                   `json:"analysis_run_id"`
	Summary       RepositoryComplexitySummary `json:"summary"`
	Metrics       []ComplexityMetric          `json:"metrics"`
	FileSummaries []FileComplexitySummary     `json:"file_summaries,omitempty"`
}
