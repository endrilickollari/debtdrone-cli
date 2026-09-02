package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/spf13/cobra"
)

type configStore interface {
	Path() string
	Load() (localconfig.Overrides, bool, error)
	Set(context.Context, localconfig.Key, string) error
	Unset(context.Context, localconfig.Key) (bool, error)
}

type configStoreFactory func() (configStore, error)
type environmentSource func() map[string]string
type configurationResolver func(localconfig.Overrides) (localconfig.Resolved, error)

type configOutput struct {
	Key         localconfig.Key    `json:"key"`
	Value       string             `json:"value"`
	Type        string             `json:"type"`
	Source      localconfig.Source `json:"source"`
	Description string             `json:"description"`
}

func defaultConfigStore() (configStore, error) {
	path, err := localconfig.DefaultPath()
	if err != nil {
		return nil, err
	}
	return localconfig.NewStore(path)
}

func processEnvironment() map[string]string {
	values := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, found := strings.Cut(item, "=")
		if found {
			values[key] = value
		}
	}
	return values
}

func newConfigCmd() *cobra.Command {
	return newConfigCommand(defaultConfigStore, processEnvironment)
}

func defaultConfigurationResolver(flags localconfig.Overrides) (localconfig.Resolved, error) {
	return resolveConfiguration(defaultConfigStore, processEnvironment, flags)
}

func newConfigCommand(open configStoreFactory, environment environmentSource) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage DebtDrone configuration",
		Args:  cobra.NoArgs,
	}

	var listFormat string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List effective configuration values and sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := configFormat(listFormat)
			if err != nil {
				return err
			}
			resolved, err := resolveConfiguration(open, environment, localconfig.Overrides{})
			if err != nil {
				return err
			}
			items := configOutputs(resolved)
			if format == "json" {
				return writeConfigJSON(cmd, items)
			}
			return printConfigTable(cmd, items)
		},
	}
	listCmd.Flags().StringVarP(&listFormat, "format", "f", "text", "Output format: text or json")

	var getFormat string
	getCmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get one effective configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := configFormat(getFormat)
			if err != nil {
				return err
			}
			key, err := localconfig.ParseKey(args[0])
			if err != nil {
				return err
			}
			resolved, err := resolveConfiguration(open, environment, localconfig.Overrides{})
			if err != nil {
				return err
			}
			item := configOutputFor(resolved, key)
			if format == "json" {
				return writeConfigJSON(cmd, item)
			}
			fmt.Fprintln(cmd.OutOrStdout(), item.Value)
			return nil
		},
	}
	getCmd.Flags().StringVarP(&getFormat, "format", "f", "text", "Output format: text or json")

	setCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Persist one configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := localconfig.ParseKey(args[0])
			if err != nil {
				return err
			}
			store, err := openConfigStore(open)
			if err != nil {
				return err
			}
			if err := store.Set(cmd.Context(), key, args[1]); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s in %s.\n", key, store.Path())
			return nil
		},
	}

	unsetCmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove one persisted configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := localconfig.ParseKey(args[0])
			if err != nil {
				return err
			}
			store, err := openConfigStore(open)
			if err != nil {
				return err
			}
			removed, err := store.Unset(cmd.Context(), key)
			if err != nil {
				return fmt.Errorf("unset %s: %w", key, err)
			}
			if !removed {
				fmt.Fprintf(cmd.OutOrStdout(), "No config-file value is set for %s.\n", key)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unset %s in %s.\n", key, store.Path())
			return nil
		},
	}

	cmd.AddCommand(listCmd, getCmd, setCmd, unsetCmd)
	return cmd
}

func openConfigStore(open configStoreFactory) (configStore, error) {
	if open == nil {
		return nil, errors.New("configuration store factory is required")
	}
	store, err := open()
	if err != nil {
		return nil, fmt.Errorf("open local configuration: %w", err)
	}
	if store == nil {
		return nil, errors.New("open local configuration: store is unavailable")
	}
	return store, nil
}

func resolveConfiguration(open configStoreFactory, environment environmentSource, flags localconfig.Overrides) (localconfig.Resolved, error) {
	store, err := openConfigStore(open)
	if err != nil {
		return localconfig.Resolved{}, err
	}
	file, _, err := store.Load()
	if err != nil {
		return localconfig.Resolved{}, fmt.Errorf("load local configuration: %w", err)
	}
	environmentValues := map[string]string{}
	if environment != nil {
		environmentValues = environment()
	}
	resolved, err := localconfig.Resolve(file, environmentValues, flags)
	if err != nil {
		return localconfig.Resolved{}, fmt.Errorf("resolve local configuration: %w", err)
	}
	return resolved, nil
}

func configOutputs(resolved localconfig.Resolved) []configOutput {
	definitions := localconfig.Definitions()
	items := make([]configOutput, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, configOutput{
			Key: definition.Key, Value: localconfig.Value(resolved.Values, definition.Key),
			Type: definition.Type, Source: resolved.Sources[definition.Key], Description: definition.Description,
		})
	}
	return items
}

func configOutputFor(resolved localconfig.Resolved, key localconfig.Key) configOutput {
	for _, item := range configOutputs(resolved) {
		if item.Key == key {
			return item
		}
	}
	return configOutput{Key: key}
}

func configFormat(value string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(value))
	if format != "text" && format != "json" {
		return "", fmt.Errorf("invalid config format %q (valid: text, json)", value)
	}
	return format, nil
}

func writeConfigJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printConfigTable(cmd *cobra.Command, items []configOutput) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "KEY\tVALUE\tTYPE\tSOURCE\tDESCRIPTION")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", item.Key, item.Value, item.Type, item.Source, item.Description)
	}
	return w.Flush()
}
