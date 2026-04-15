package update

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/creativeprojects/go-selfupdate"
)

var (
	RepoOwner = "endrilickollari"
	RepoName  = "debtdrone-cli"
)

type UpdateInfo struct {
	Available    bool
	Version      string
	AssetURL     string
	AssetName    string
	// ReleaseNotes contains the markdown body from the GitHub release page.
	// It is populated by CheckForUpdate and displayed in the update prompt
	// modal. May be empty for releases that have no description.
	ReleaseNotes string
}

func CheckForUpdate(ctx context.Context, currentVersion string) (*UpdateInfo, error) {
	// Skip update check for development versions
	if currentVersion == "" || currentVersion == "dev" || currentVersion == "latest" {
		return &UpdateInfo{Available: false}, nil
	}

	repo := selfupdate.NewRepositorySlug(RepoOwner, RepoName)

	latest, found, err := selfupdate.DetectLatest(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	if !found || latest == nil {
		return &UpdateInfo{Available: false}, nil
	}

	if latest.LessOrEqual(currentVersion) {
		return &UpdateInfo{Available: false}, nil
	}

	return &UpdateInfo{
		Available:    true,
		Version:      latest.Version(),
		AssetURL:     latest.AssetURL,
		AssetName:    latest.AssetName,
		ReleaseNotes: latest.ReleaseNotes,
	}, nil
}

func isMatchingAsset(name string) bool {
	os := runtime.GOOS
	arch := runtime.GOARCH

	if !strings.Contains(name, os) {
		return false
	}
	if arch == "amd64" {
		return strings.Contains(name, "amd64") || strings.Contains(name, "x86_64") || strings.Contains(name, "64bit")
	} else if arch == "arm64" {
		return strings.Contains(name, "arm64") || strings.Contains(name, "aarch64")
	}
	return true
}

func PerformUpdate(ctx context.Context) error {
	repo := selfupdate.NewRepositorySlug(RepoOwner, RepoName)

	latest, found, err := selfupdate.DetectLatest(ctx, repo)
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}

	if !found || latest == nil {
		return fmt.Errorf("no releases found")
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return fmt.Errorf("could not locate executable: %w", err)
	}

	err = selfupdate.UpdateTo(ctx, latest.AssetURL, latest.AssetName, exe)
	if err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	return nil
}
