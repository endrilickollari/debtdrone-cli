package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/mcpserver"
	"github.com/spf13/cobra"
)

type mcpRunFunc func(context.Context, string, string, localconfig.Values) error

func newMCPCmd() *cobra.Command {
	return newMCPCommandWithResolver(version, mcpserver.RunStdio, defaultConfigurationResolver)
}

func newMCPCommand(serverVersion string, run mcpRunFunc) *cobra.Command {
	return newMCPCommandWithResolver(serverVersion, run, func(flags localconfig.Overrides) (localconfig.Resolved, error) {
		return localconfig.Resolve(localconfig.Overrides{}, nil, flags)
	})
}

func newMCPCommandWithResolver(serverVersion string, run mcpRunFunc, resolve configurationResolver) *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the local DebtDrone MCP server over stdio",
		Long: `Run a local Model Context Protocol server over stdin and stdout.
The configured root defines the repository boundary available to MCP tools,
and scans do not modify repository contents.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedRoot, err := resolveMCPRoot(root)
			if err != nil {
				return err
			}
			resolved, err := resolve(localconfig.Overrides{})
			if err != nil {
				return fmt.Errorf("resolve MCP configuration: %w", err)
			}

			err = run(cmd.Context(), resolvedRoot, serverVersion, resolved.Values)
			if errors.Is(err, context.Canceled) && cmd.Context().Err() != nil {
				return nil
			}
			if err != nil {
				return fmt.Errorf("MCP server failed: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&root, "root", "", "Repository root exposed to the MCP server")
	if err := cmd.MarkFlagRequired("root"); err != nil {
		panic(err)
	}

	return cmd
}

func resolveMCPRoot(root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve MCP root %q: %w", root, err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("inspect MCP root %q: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("MCP root %q is not a directory", root)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolve MCP root symlinks %q: %w", root, err)
	}

	return filepath.Clean(resolvedRoot), nil
}
