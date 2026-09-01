// Package localhistory persists bounded, privacy-conscious scan summaries for
// local CLI workflows. It never stores repository source, finding messages,
// credentials, environment values, or SaaS ownership identifiers.
package localhistory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	CurrentVersion             = 1
	DefaultRetention           = 90 * 24 * time.Hour
	DefaultMaximumRecords      = 200
	DefaultMaximumFileBytes    = 2 << 20
	maximumRepositoryNameRunes = 120
	historyDirectoryName       = "debtdrone"
	historyFileName            = "history.json"
)

type Outcome string

const (
	OutcomeCompleted Outcome = "completed"
	OutcomePartial   Outcome = "partial"
)

// Summary contains only aggregate scan information. Severity counts may total
// less than Findings when a scanner reports an unrecognized severity.
type Summary struct {
	Findings           int     `json:"findings"`
	Critical           int     `json:"critical"`
	High               int     `json:"high"`
	Medium             int     `json:"medium"`
	Low                int     `json:"low"`
	TechnicalDebtHours float64 `json:"technical_debt_hours"`
	Warnings           int     `json:"warnings"`
	AnalyzerFailures   int     `json:"analyzer_failures"`
}

type Record struct {
	ID          string    `json:"id"`
	Repository  string    `json:"repository"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Outcome     Outcome   `json:"outcome"`
	Summary     Summary   `json:"summary"`
}

type RecordInput struct {
	RepositoryPath string
	StartedAt      time.Time
	CompletedAt    time.Time
	Outcome        Outcome
	Summary        Summary
}

type Options struct {
	Retention      time.Duration
	MaximumRecords int
	MaximumBytes   int
	Now            func() time.Time
	NewID          func() string
}

type Store struct {
	path    string
	options Options
}

type historyFile struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

func DefaultOptions() Options {
	return Options{
		Retention:      DefaultRetention,
		MaximumRecords: DefaultMaximumRecords,
		MaximumBytes:   DefaultMaximumFileBytes,
		Now:            time.Now,
		NewID:          uuid.NewString,
	}
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return PathIn(configDir), nil
}

func PathIn(userConfigDir string) string {
	return filepath.Join(userConfigDir, historyDirectoryName, historyFileName)
}

func New(path string) (*Store, error) {
	return NewWithOptions(path, DefaultOptions())
}

func NewWithOptions(path string, options Options) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("history path is required")
	}
	if options.Retention <= 0 {
		return nil, errors.New("history retention must be greater than zero")
	}
	if options.MaximumRecords <= 0 {
		return nil, errors.New("history maximum records must be greater than zero")
	}
	if options.MaximumBytes <= 0 {
		return nil, errors.New("history maximum bytes must be greater than zero")
	}
	if options.Now == nil {
		return nil, errors.New("history clock is required")
	}
	if options.NewID == nil {
		return nil, errors.New("history ID generator is required")
	}
	return &Store{path: path, options: options}, nil
}

// RecordScan adds one successful or partial scan summary through the shared
// persistence path and returns the stored record.
func (s *Store) RecordScan(ctx context.Context, input RecordInput) (Record, error) {
	if err := validateInput(input); err != nil {
		return Record{}, err
	}

	record := Record{
		ID:          s.options.NewID(),
		Repository:  RepositoryDisplayName(input.RepositoryPath),
		StartedAt:   input.StartedAt.UTC(),
		CompletedAt: input.CompletedAt.UTC(),
		Outcome:     input.Outcome,
		Summary:     input.Summary,
	}
	if _, err := uuid.Parse(record.ID); err != nil {
		return Record{}, fmt.Errorf("generate history ID: %w", err)
	}

	err := s.withLock(ctx, func() error {
		file, _, err := s.load()
		if err != nil {
			return err
		}
		for _, existing := range file.Records {
			if existing.ID == record.ID {
				return fmt.Errorf("generated history ID %q already exists", record.ID)
			}
		}
		file.Records = append(file.Records, record)
		file.Records = s.prune(file.Records)
		retained := false
		for _, existing := range file.Records {
			if existing.ID == record.ID {
				retained = true
				break
			}
		}
		if !retained {
			return fmt.Errorf("scan completed at %s falls outside the configured history bounds", record.CompletedAt.Format(time.RFC3339))
		}
		return s.write(file, record.ID)
	})
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

// List returns newest-first summaries after applying retention.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	var records []Record
	err := s.withLock(ctx, func() error {
		file, found, err := s.load()
		if err != nil {
			return err
		}
		if !found {
			records = []Record{}
			return nil
		}

		pruned := s.prune(file.Records)
		if len(pruned) != len(file.Records) {
			file.Records = pruned
			if err := s.write(file, ""); err != nil {
				return err
			}
		}
		records = append([]Record(nil), pruned...)
		return nil
	})
	return records, err
}

func (s *Store) Get(ctx context.Context, id string) (Record, bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Record{}, false, fmt.Errorf("invalid history ID %q: %w", id, err)
	}
	records, err := s.List(ctx)
	if err != nil {
		return Record{}, false, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, true, nil
		}
	}
	return Record{}, false, nil
}

func (s *Store) Delete(ctx context.Context, id string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, fmt.Errorf("invalid history ID %q: %w", id, err)
	}

	deleted := false
	err := s.withLock(ctx, func() error {
		file, found, err := s.load()
		if err != nil || !found {
			return err
		}
		records := make([]Record, 0, len(file.Records))
		for _, record := range file.Records {
			if record.ID == id {
				deleted = true
				continue
			}
			records = append(records, record)
		}
		if !deleted {
			return nil
		}
		file.Records = records
		return s.write(file, "")
	})
	return deleted, err
}

func (s *Store) Clear(ctx context.Context) error {
	return s.withLock(ctx, func() error {
		_, found, err := s.load()
		if err != nil || !found {
			return err
		}
		return s.write(historyFile{Version: CurrentVersion, Records: []Record{}}, "")
	})
}

// RepositoryDisplayName returns a bounded basename and deliberately drops
// parent directories, URL user information, query strings, and fragments.
func RepositoryDisplayName(repositoryPath string) string {
	value := strings.TrimSpace(repositoryPath)
	if parsed, err := url.Parse(value); err == nil && strings.Contains(value, "://") {
		value = parsed.Path
		if strings.Trim(value, "/") == "" {
			value = parsed.Hostname()
		}
	} else if at := strings.LastIndex(value, "@"); at >= 0 {
		if colon := strings.Index(value[at:], ":"); colon >= 0 {
			value = value[at+colon+1:]
		}
	}
	if delimiter := strings.IndexAny(value, "?#"); delimiter >= 0 {
		value = value[:delimiter]
	}
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimRight(value, "/")
	value = path.Base(value)
	if value == "" || value == "." || value == "/" {
		value = "repository"
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if value == "" {
		value = "repository"
	}
	if utf8.RuneCountInString(value) > maximumRepositoryNameRunes {
		value = string([]rune(value)[:maximumRepositoryNameRunes])
	}
	return value
}

func validateInput(input RecordInput) error {
	if input.StartedAt.IsZero() {
		return errors.New("history scan start time is required")
	}
	if input.CompletedAt.IsZero() {
		return errors.New("history scan completion time is required")
	}
	if input.CompletedAt.Before(input.StartedAt) {
		return errors.New("history scan completion time cannot be before its start time")
	}
	if input.Outcome != OutcomeCompleted && input.Outcome != OutcomePartial {
		return fmt.Errorf("history outcome must be %q or %q, got %q", OutcomeCompleted, OutcomePartial, input.Outcome)
	}
	return validateSummary(input.Summary)
}

func validateSummary(summary Summary) error {
	counts := []struct {
		name  string
		value int
	}{
		{"findings", summary.Findings},
		{"critical", summary.Critical},
		{"high", summary.High},
		{"medium", summary.Medium},
		{"low", summary.Low},
		{"warnings", summary.Warnings},
		{"analyzer_failures", summary.AnalyzerFailures},
	}
	for _, count := range counts {
		if count.value < 0 {
			return fmt.Errorf("history summary %s cannot be negative", count.name)
		}
	}
	remainingFindings := summary.Findings
	for _, severity := range []int{summary.Critical, summary.High, summary.Medium, summary.Low} {
		if severity > remainingFindings {
			return fmt.Errorf("history severity total cannot exceed findings %d", summary.Findings)
		}
		remainingFindings -= severity
	}
	if summary.TechnicalDebtHours < 0 || math.IsNaN(summary.TechnicalDebtHours) || math.IsInf(summary.TechnicalDebtHours, 0) {
		return errors.New("history technical debt hours must be a finite non-negative number")
	}
	return nil
}

func validateRecord(record Record) error {
	if _, err := uuid.Parse(record.ID); err != nil {
		return fmt.Errorf("record has invalid ID %q: %w", record.ID, err)
	}
	if record.Repository == "" || RepositoryDisplayName(record.Repository) != record.Repository {
		return fmt.Errorf("record %s has an unsafe repository display name", record.ID)
	}
	return validateInput(RecordInput{
		RepositoryPath: record.Repository,
		StartedAt:      record.StartedAt,
		CompletedAt:    record.CompletedAt,
		Outcome:        record.Outcome,
		Summary:        record.Summary,
	})
}

func (s *Store) withLock(ctx context.Context, action func() error) error {
	if err := ensureDirectory(filepath.Dir(s.path)); err != nil {
		return err
	}
	lock, err := acquireHistoryLock(ctx, s.path+".lock")
	if err != nil {
		return fmt.Errorf("lock history %q: %w", s.path, err)
	}
	defer lock.release()
	return action()
}

func ensureDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create history directory %q: %w", directory, err)
	}
	return nil
}

func (s *Store) load() (historyFile, bool, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return historyFile{Version: CurrentVersion, Records: []Record{}}, false, nil
	}
	if err != nil {
		return historyFile{}, false, fmt.Errorf("read history %q: %w", s.path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return historyFile{}, true, fmt.Errorf("inspect history %q: %w", s.path, err)
	}
	if info.Size() > int64(s.options.MaximumBytes) {
		return historyFile{}, true, fmt.Errorf("history file %q exceeds the %d-byte limit; move it aside and rerun DebtDrone", s.path, s.options.MaximumBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(s.options.MaximumBytes)+1))
	if err != nil {
		return historyFile{}, true, fmt.Errorf("read history %q: %w", s.path, err)
	}

	decoded, err := decodeHistory(data)
	if err != nil {
		return historyFile{}, true, fmt.Errorf("invalid history file %q: %w", s.path, err)
	}
	return decoded, true, nil
}

func decodeHistory(data []byte) (historyFile, error) {
	var header struct {
		Version *int `json:"version"`
	}
	if err := decodeJSON(data, &header, false); err != nil {
		if errors.Is(err, io.EOF) {
			return historyFile{}, errors.New("history file is empty; move it aside and rerun DebtDrone")
		}
		return historyFile{}, fmt.Errorf("decode version: %w", err)
	}
	if header.Version == nil {
		return historyFile{}, fmt.Errorf("version is required; expected version %d", CurrentVersion)
	}
	if *header.Version > CurrentVersion {
		return historyFile{}, fmt.Errorf("schema version %d is newer than supported version %d; upgrade DebtDrone before using this history", *header.Version, CurrentVersion)
	}
	if *header.Version < CurrentVersion {
		return historyFile{}, fmt.Errorf("schema version %d is no longer supported; migrate or move the history file aside", *header.Version)
	}

	var file historyFile
	if err := decodeJSON(data, &file, true); err != nil {
		return historyFile{}, fmt.Errorf("decode JSON: %w", err)
	}
	if file.Records == nil {
		file.Records = []Record{}
	}
	seen := make(map[string]struct{}, len(file.Records))
	for index, record := range file.Records {
		if err := validateRecord(record); err != nil {
			return historyFile{}, fmt.Errorf("record %d: %w", index, err)
		}
		if _, exists := seen[record.ID]; exists {
			return historyFile{}, fmt.Errorf("record %d duplicates history ID %q", index, record.ID)
		}
		seen[record.ID] = struct{}{}
	}
	return file, nil
}

func decodeJSON(data []byte, target any, strict bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("history file must contain one JSON document")
	}
	return nil
}

func (s *Store) prune(records []Record) []Record {
	cutoff := s.options.Now().UTC().Add(-s.options.Retention)
	kept := make([]Record, 0, len(records))
	for _, record := range records {
		if !record.CompletedAt.Before(cutoff) {
			kept = append(kept, record)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		return kept[i].CompletedAt.After(kept[j].CompletedAt)
	})
	if len(kept) > s.options.MaximumRecords {
		kept = kept[:s.options.MaximumRecords]
	}
	return kept
}

func (s *Store) write(file historyFile, requiredID string) error {
	file.Version = CurrentVersion
	if file.Records == nil {
		file.Records = []Record{}
	}

	var data []byte
	for {
		encoded, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			return fmt.Errorf("encode history: %w", err)
		}
		data = append(encoded, '\n')
		if len(data) <= s.options.MaximumBytes {
			break
		}
		if len(file.Records) <= 1 {
			return fmt.Errorf("history record exceeds the %d-byte file limit", s.options.MaximumBytes)
		}
		if file.Records[len(file.Records)-1].ID == requiredID {
			return errors.New("new history record falls outside the configured file-size bound")
		}
		file.Records = file.Records[:len(file.Records)-1]
	}
	if err := writeFileAtomically(s.path, data); err != nil {
		return fmt.Errorf("write history %q: %w", s.path, err)
	}
	return nil
}

func writeFileAtomically(destination string, data []byte) error {
	return writeFileAtomicallyWithReplace(destination, data, replaceFile)
}

func writeFileAtomicallyWithReplace(destination string, data []byte, replace func(string, string) error) error {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".history-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary history file: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary history file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary history file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary history file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary history file: %w", err)
	}
	closed = true
	if err := replace(temporaryPath, destination); err != nil {
		return fmt.Errorf("replace history file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync history directory: %w", err)
	}
	return nil
}
