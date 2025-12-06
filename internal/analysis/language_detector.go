package analysis

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/go-enry/go-enry/v2"
)

type LanguageStats struct {
	Breakdown       map[string]int64 `json:"breakdown"`
	PrimaryLanguage string           `json:"primary_language"`
	TotalBytes      int64            `json:"total_bytes"`
}

func DetectLanguages(repoPath string) (*LanguageStats, error) {
	log.Printf("🔍 Detecting languages in: %s", repoPath)

	breakdown := make(map[string]int64)
	var totalBytes int64

	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirName := d.Name()
			if dirName == ".git" || dirName == "node_modules" || dirName == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if enry.IsVendor(path) || enry.IsGenerated(path, content) {
			return nil
		}
		language := enry.GetLanguage(filepath.Base(path), content)
		if language == "" {
			return nil
		}

		breakdown[language] += info.Size()
		totalBytes += info.Size()

		return nil
	})

	if err != nil {
		log.Printf("❌ Failed to walk repository: %v", err)
		return nil, err
	}

	var primaryLanguage string
	var maxBytes int64
	for lang, bytes := range breakdown {
		if bytes > maxBytes {
			maxBytes = bytes
			primaryLanguage = lang
		}
	}

	stats := &LanguageStats{
		Breakdown:       breakdown,
		PrimaryLanguage: primaryLanguage,
		TotalBytes:      totalBytes,
	}

	log.Printf("✅ Detected %d languages, primary: %s, total bytes: %d", len(breakdown), primaryLanguage, totalBytes)

	return stats, nil
}

type ConfigFile struct {
	Path     string `json:"path"`
	Category string `json:"category"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
}

func DetectConfigFiles(repoPath string) ([]ConfigFile, error) {
	log.Printf("🔍 Detecting config files in: %s", repoPath)

	var configFiles []ConfigFile

	configPatterns := map[string]struct {
		category string
		fileType string
	}{
		"package.json":      {"dependencies", "npm"},
		"package-lock.json": {"dependencies", "npm"},
		"yarn.lock":         {"dependencies", "yarn"},
		"pnpm-lock.yaml":    {"dependencies", "pnpm"},
		"requirements.txt":  {"dependencies", "pip"},
		"Pipfile":           {"dependencies", "pipenv"},
		"poetry.lock":       {"dependencies", "poetry"},
		"setup.py":          {"dependencies", "python"},
		"go.mod":            {"dependencies", "go"},
		"go.sum":            {"dependencies", "go"},
		"Gemfile":           {"dependencies", "ruby"},
		"Gemfile.lock":      {"dependencies", "ruby"},
		"Cargo.toml":        {"dependencies", "rust"},
		"Cargo.lock":        {"dependencies", "rust"},
		"composer.json":     {"dependencies", "php"},
		"composer.lock":     {"dependencies", "php"},

		"Makefile":          {"build", "make"},
		"CMakeLists.txt":    {"build", "cmake"},
		"build.gradle":      {"build", "gradle"},
		"build.gradle.kts":  {"build", "gradle"},
		"pom.xml":           {"build", "maven"},
		"webpack.config.js": {"build", "webpack"},
		"vite.config.js":    {"build", "vite"},
		"vite.config.ts":    {"build", "vite"},
		"rollup.config.js":  {"build", "rollup"},

		"Dockerfile":          {"containerization", "docker"},
		"docker-compose.yml":  {"containerization", "docker-compose"},
		"docker-compose.yaml": {"containerization", "docker-compose"},
		".dockerignore":       {"containerization", "docker"},
		".env":                {"environment", "dotenv"},
		".env.example":        {"environment", "dotenv"},
		".env.local":          {"environment", "dotenv"},
		"config.yml":          {"environment", "yaml"},
		"config.yaml":         {"environment", "yaml"},
		"appsettings.json":    {"environment", "json"},

		"tsconfig.json":       {"typescript", "typescript"},
		"tsconfig.build.json": {"typescript", "typescript"},
		"jsconfig.json":       {"typescript", "javascript"},
		".eslintrc":           {"linting", "eslint"},
		".eslintrc.js":        {"linting", "eslint"},
		".eslintrc.json":      {"linting", "eslint"},
		".prettierrc":         {"linting", "prettier"},
		".prettierrc.json":    {"linting", "prettier"},
		".editorconfig":       {"linting", "editorconfig"},

		"jest.config.js":   {"testing", "jest"},
		"vitest.config.js": {"testing", "vitest"},
		"pytest.ini":       {"testing", "pytest"},
		"phpunit.xml":      {"testing", "phpunit"},
	}

	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirName := d.Name()
			if dirName == ".git" || dirName == "node_modules" || dirName == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(repoPath, path)
		if err != nil {
			return nil
		}

		fileName := filepath.Base(path)

		if pattern, exists := configPatterns[fileName]; exists {
			info, err := d.Info()
			if err != nil {
				return nil
			}

			configFiles = append(configFiles, ConfigFile{
				Path:     relPath,
				Category: pattern.category,
				Type:     pattern.fileType,
				Size:     info.Size(),
			})
		}

		if filepath.Dir(relPath) == ".github/workflows" && filepath.Ext(fileName) == ".yml" {
			info, err := d.Info()
			if err != nil {
				return nil
			}

			configFiles = append(configFiles, ConfigFile{
				Path:     relPath,
				Category: "ci_cd",
				Type:     "github-actions",
				Size:     info.Size(),
			})
		}

		return nil
	})

	if err != nil {
		log.Printf("❌ Failed to detect config files: %v", err)
		return nil, err
	}

	log.Printf("✅ Detected %d config files", len(configFiles))

	return configFiles, nil
}
