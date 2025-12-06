package analysis

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/store"
	"github.com/google/uuid"
)

// Job represents an analysis job
type Job struct {
	AnalysisRunID uuid.UUID
	RepositoryID  uuid.UUID
	UserID        uuid.UUID
	RepoURL       string
	RepoToken     string // Optional
	Branch        string // Optional
}

// Engine manages analysis jobs and workers
type Engine struct {
	workers       int
	jobQueue      chan Job
	quit          chan bool
	wg            sync.WaitGroup
	gitService    *git.Service
	analysisStore store.AnalysisRunStoreInterface
	issueStore    store.TechnicalDebtIssueStoreInterface
	repoStore     store.RepositoryStoreInterface
	metricsStore  store.MetricsStoreInterface
	analyzers     []Analyzer
}

// NewEngine creates a new analysis engine
func NewEngine(
	workers int,
	gitService *git.Service,
	analysisStore store.AnalysisRunStoreInterface,
	issueStore store.TechnicalDebtIssueStoreInterface,
	repoStore store.RepositoryStoreInterface,
	metricsStore store.MetricsStoreInterface,
	analyzers []Analyzer,
) *Engine {
	return &Engine{
		workers:       workers,
		jobQueue:      make(chan Job, 100), // Buffer size
		quit:          make(chan bool),
		gitService:    gitService,
		analysisStore: analysisStore,
		issueStore:    issueStore,
		repoStore:     repoStore,
		metricsStore:  metricsStore,
		analyzers:     analyzers,
	}
}

// Start starts the worker pool
func (e *Engine) Start() {
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}
	log.Printf("🚀 Analysis Engine started with %d workers", e.workers)
}

// Stop stops the worker pool
func (e *Engine) Stop() {
	close(e.quit)
	e.wg.Wait()
	log.Println("🛑 Analysis Engine stopped")
}

// SubmitJob submits a job to the queue
func (e *Engine) SubmitJob(job Job) {
	select {
	case e.jobQueue <- job:
		log.Printf("📥 Job submitted: %s", job.AnalysisRunID)
	default:
		log.Printf("⚠️ Job queue full, dropping job: %s", job.AnalysisRunID)
	}
}

func (e *Engine) worker(id int) {
	defer e.wg.Done()
	log.Printf("👷 Worker %d started", id)

	for {
		select {
		case job := <-e.jobQueue:
			log.Printf("👷 Worker %d processing job: %s", id, job.AnalysisRunID)
			e.processJob(job)
		case <-e.quit:
			log.Printf("👷 Worker %d stopping", id)
			return
		}
	}
}

func (e *Engine) processJob(job Job) {
	ctx := context.Background()

	// Update status to running
	err := e.analysisStore.UpdateStatus(ctx, job.AnalysisRunID, "running", nil)
	if err != nil {
		log.Printf("❌ Failed to update status to running: %v", err)
		return
	}

	// Clone Repository
	cloneOpts := git.CloneOptions{
		URL:          job.RepoURL,
		Branch:       job.Branch,
		Token:        job.RepoToken,
		UseInMemory:  false, // Default to disk for stability
		SingleBranch: true,
		Depth:        100, // Need history for churn analysis
	}

	repo, err := e.gitService.Clone(ctx, cloneOpts)
	if err != nil {
		log.Printf("❌ Clone failed: %v", err)
		e.analysisStore.UpdateStatus(ctx, job.AnalysisRunID, "failed", map[string]interface{}{"error": err.Error()})
		return
	}
	defer repo.Cleanup()

	// Detect Languages
	log.Printf("🔍 Detecting languages for repository: %s", job.RepositoryID)
	languageStats, err := DetectLanguages(repo.Path)
	if err != nil {
		log.Printf("⚠️  Language detection failed: %v (continuing with analysis)", err)
	} else {
		// Update repository with language stats
		repository, err := e.repoStore.GetByID(job.RepositoryID.String())
		if err != nil {
			log.Printf("⚠️  Failed to fetch repository for language update: %v", err)
		} else {
			// Convert breakdown map to JSONB format
			repository.PrimaryLanguage = &languageStats.PrimaryLanguage

			// Use proper JSON encoding
			breakdownBytes, err := json.Marshal(languageStats.Breakdown)
			if err != nil {
				log.Printf("⚠️  Failed to marshal language breakdown: %v", err)
			} else {
				breakdownJSON := string(breakdownBytes)
				repository.LanguageBreakdown = &breakdownJSON

				err = e.repoStore.Update(repository)
				if err != nil {
					log.Printf("⚠️  Failed to update repository with language stats: %v", err)
				} else {
					log.Printf("✅ Updated repository with language stats: primary=%s, languages=%d",
						languageStats.PrimaryLanguage, len(languageStats.Breakdown))
				}
			}
		}
	}

	// Detect Config Files
	log.Printf("🔍 Detecting config files for repository: %s", job.RepositoryID)
	configFiles, err := DetectConfigFiles(repo.Path)
	if err != nil {
		log.Printf("⚠️  Config file detection failed: %v (continuing with analysis)", err)
	} else if len(configFiles) > 0 {
		// Fetch repository if needed (might already be fetched from language detection)
		repository, err := e.repoStore.GetByID(job.RepositoryID.String())
		if err != nil {
			log.Printf("⚠️  Failed to fetch repository for config update: %v", err)
		} else {
			// Marshal config files to JSON
			configJSON, err := json.Marshal(configFiles)
			if err != nil {
				log.Printf("⚠️  Failed to marshal config files: %v", err)
			} else {
				configJSONStr := string(configJSON)
				repository.ConfigFiles = &configJSONStr

				err = e.repoStore.Update(repository)
				if err != nil {
					log.Printf("⚠️  Failed to update repository with config files: %v", err)
				} else {
					log.Printf("✅ Updated repository with %d config files", len(configFiles))
				}
			}
		}
	}

	// Run Analyzers
	var allIssues []models.TechnicalDebtIssue
	allMetrics := make(map[string]interface{})

	// Add job info to context for analyzers that need it
	ctx = context.WithValue(ctx, "analysisRunID", job.AnalysisRunID)
	ctx = context.WithValue(ctx, "repositoryID", job.RepositoryID)
	ctx = context.WithValue(ctx, "userID", job.UserID)

	for _, analyzer := range e.analyzers {
		log.Printf("🔍 Running analyzer: %s", analyzer.Name())
		result, err := analyzer.Analyze(ctx, repo)
		if err != nil {
			log.Printf("⚠️ Analyzer %s failed: %v", analyzer.Name(), err)
			continue
		}
		allIssues = append(allIssues, result.Issues...)
		for k, v := range result.Metrics {
			allMetrics[k] = v
		}
	}

	// Save Results
	// Save issues to database using batch insert
	if len(allIssues) > 0 {
		err = e.issueStore.BatchCreate(allIssues)
		if err != nil {
			log.Printf("❌ Failed to save issues to database: %v", err)
			// Don't fail the entire job, just log the error
		} else {
			log.Printf("✅ Saved %d issues to database", len(allIssues))
		}
	} else {
		log.Printf("✅ Found %d issues", len(allIssues))
	}

	// Update Analysis Run with success
	summary := calculateSummary(allIssues)

	// Merge summary into allMetrics
	for k, v := range summary {
		allMetrics[k] = v
	}

	log.Printf("🔄 Calling UpdateStatus with metrics: %+v", allMetrics)
	err = e.analysisStore.UpdateStatus(ctx, job.AnalysisRunID, "completed", allMetrics)
	if err != nil {
		log.Printf("❌ Failed to update status to completed: %v", err)
	} else {
		log.Printf("✅ Job completed: %s", job.AnalysisRunID)

		// Update repository metrics
		var debt float64
		if v, ok := allMetrics["total_debt_hours"].(float64); ok {
			debt = v
		} else if v, ok := allMetrics["total_debt_hours"].(int); ok {
			debt = float64(v)
		}

		var coverage float64
		if v, ok := allMetrics["test_coverage_percentage"].(float64); ok {
			coverage = v
		}

		var complexity float64
		if v, ok := allMetrics["complexity_avg_cyclomatic"].(float64); ok {
			complexity = v
		}

		var critical, high, medium, low int
		if v, ok := allMetrics["critical_count"].(int); ok {
			critical = v
		}
		if v, ok := allMetrics["high_count"].(int); ok {
			high = v
		}
		if v, ok := allMetrics["medium_count"].(int); ok {
			medium = v
		}
		if v, ok := allMetrics["low_count"].(int); ok {
			low = v
		}

		err = e.repoStore.UpdateMetrics(job.RepositoryID.String(), debt, coverage, complexity, critical, high, medium, low)
		if err != nil {
			log.Printf("❌ Failed to update repository metrics: %v", err)
		} else {
			log.Printf("✅ Updated repository metrics for %s", job.RepositoryID)
		}
	}

	// Create metrics snapshot after successful analysis
	log.Printf("📸 Creating metrics snapshot for repository: %s", job.RepositoryID)
	if err := e.metricsStore.CreateSnapshot(job.RepositoryID.String()); err != nil {
		log.Printf("⚠️ Failed to create metrics snapshot: %v", err)
		// Don't fail the job, just log it
	}
}

func calculateSummary(issues []models.TechnicalDebtIssue) map[string]interface{} {
	summary := make(map[string]interface{})
	summary["total_issues_found"] = len(issues)

	// Count by severity
	criticalCount := 0
	highCount := 0
	mediumCount := 0
	lowCount := 0

	// Count by category
	categoryCount := make(map[string]int)

	// Calculate total debt
	totalDebtHours := 0.0

	// Track unique files
	fileSet := make(map[string]bool)

	for _, issue := range issues {
		// Count by severity
		switch issue.Severity {
		case "critical":
			criticalCount++
		case "high":
			highCount++
		case "medium":
			mediumCount++
		case "low":
			lowCount++
		}

		// Count by category
		if issue.Category != "" {
			categoryCount[issue.Category]++
		}

		// Sum technical debt
		totalDebtHours += issue.TechnicalDebtHours

		// Track unique files
		if issue.FilePath != "" {
			fileSet[issue.FilePath] = true
		}
	}

	// Convert hours to minutes for additional metric
	totalDebtMinutes := int(totalDebtHours * 60)

	// Add severity counts
	summary["critical_count"] = criticalCount
	summary["high_count"] = highCount
	summary["medium_count"] = mediumCount
	summary["low_count"] = lowCount

	// Add category breakdown
	summary["category_breakdown"] = categoryCount

	// Add debt metrics
	summary["total_debt_hours"] = totalDebtHours
	summary["total_debt_minutes"] = totalDebtMinutes

	// Add file metrics
	summary["affected_files"] = len(fileSet)

	// Calculate average debt per issue
	if len(issues) > 0 {
		summary["avg_debt_hours_per_issue"] = totalDebtHours / float64(len(issues))
	} else {
		summary["avg_debt_hours_per_issue"] = 0.0
	}

	// Add priority statistics (critical + high)
	highPriorityCount := criticalCount + highCount
	summary["high_priority_count"] = highPriorityCount

	if len(issues) > 0 {
		summary["high_priority_percentage"] = float64(highPriorityCount) / float64(len(issues)) * 100
	} else {
		summary["high_priority_percentage"] = 0.0
	}

	return summary
}
