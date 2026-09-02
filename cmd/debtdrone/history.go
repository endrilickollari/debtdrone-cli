package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/spf13/cobra"
)

type historyStore interface {
	List(context.Context) ([]localhistory.Record, error)
	Get(context.Context, string) (localhistory.Record, bool, error)
	Delete(context.Context, string) (bool, error)
	Clear(context.Context) error
}

type historyStoreFactory func() (historyStore, error)

type historyListOptions struct {
	format string
	limit  int
}

func defaultHistoryStore() (historyStore, error) {
	path, err := localhistory.DefaultPath()
	if err != nil {
		return nil, err
	}
	return localhistory.New(path)
}

func newHistoryCmd() *cobra.Command {
	return newHistoryCommand(defaultHistoryStore)
}

func newHistoryCommand(open historyStoreFactory) *cobra.Command {
	rootOptions := historyListOptions{format: "text", limit: 10}
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Inspect and manage local scan history",
		Args:  cobra.NoArgs,
		RunE:  runHistoryList(open, &rootOptions),
	}
	addHistoryListFlags(cmd, &rootOptions)

	listOptions := historyListOptions{format: "text", limit: 10}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent scan summaries",
		Args:  cobra.NoArgs,
		RunE:  runHistoryList(open, &listOptions),
	}
	addHistoryListFlags(listCmd, &listOptions)

	var showFormat string
	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one scan summary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := historyFormat(showFormat)
			if err != nil {
				return err
			}
			store, err := openHistoryStore(open)
			if err != nil {
				return err
			}
			record, found, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("show history record %q: %w", args[0], err)
			}
			if !found {
				return fmt.Errorf("history record %q was not found", args[0])
			}
			if format == "json" {
				return writeHistoryJSON(cmd, record)
			}
			return printHistoryRecord(cmd, record)
		},
	}
	showCmd.Flags().StringVarP(&showFormat, "format", "f", "text", "Output format: text or json")

	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete one scan summary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openHistoryStore(open)
			if err != nil {
				return err
			}
			deleted, err := store.Delete(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("delete history record %q: %w", args[0], err)
			}
			if !deleted {
				return fmt.Errorf("history record %q was not found", args[0])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted history record %s.\n", args[0])
			return nil
		},
	}

	var forceClear bool
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Delete all scan summaries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := openHistoryStore(open)
			if err != nil {
				return err
			}
			records, err := store.List(cmd.Context())
			if err != nil {
				return fmt.Errorf("inspect history before clearing: %w", err)
			}
			if len(records) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Local scan history is already empty.")
				return nil
			}
			if !forceClear {
				confirmed, err := confirmHistoryClear(cmd)
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(cmd.OutOrStdout(), "History clear cancelled.")
					return nil
				}
			}
			if err := store.Clear(cmd.Context()); err != nil {
				return fmt.Errorf("clear history: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Local scan history cleared.")
			return nil
		},
	}
	clearCmd.Flags().BoolVar(&forceClear, "force", false, "Clear history without an interactive confirmation")

	cmd.AddCommand(listCmd, showCmd, deleteCmd, clearCmd)
	return cmd
}

func addHistoryListFlags(cmd *cobra.Command, options *historyListOptions) {
	cmd.Flags().StringVarP(&options.format, "format", "f", "text", "Output format: text or json")
	cmd.Flags().IntVar(&options.limit, "limit", 10, "Maximum number of history records to return")
}

func runHistoryList(open historyStoreFactory, options *historyListOptions) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		format, err := historyFormat(options.format)
		if err != nil {
			return err
		}
		if options.limit < 1 || options.limit > localhistory.DefaultMaximumRecords {
			return fmt.Errorf("history limit must be between 1 and %d, got %d", localhistory.DefaultMaximumRecords, options.limit)
		}
		store, err := openHistoryStore(open)
		if err != nil {
			return err
		}
		records, err := store.List(cmd.Context())
		if err != nil {
			return fmt.Errorf("list history: %w", err)
		}
		if len(records) > options.limit {
			records = records[:options.limit]
		}
		if format == "json" {
			if records == nil {
				records = []localhistory.Record{}
			}
			return writeHistoryJSON(cmd, records)
		}
		return printHistoryTable(cmd, records)
	}
}

func openHistoryStore(open historyStoreFactory) (historyStore, error) {
	if open == nil {
		return nil, errors.New("history store factory is required")
	}
	store, err := open()
	if err != nil {
		return nil, fmt.Errorf("open local history: %w", err)
	}
	if store == nil {
		return nil, errors.New("open local history: store is unavailable")
	}
	return store, nil
}

func historyFormat(value string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(value))
	if format != "text" && format != "json" {
		return "", fmt.Errorf("invalid history format %q (valid: text, json)", value)
	}
	return format, nil
}

func writeHistoryJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printHistoryTable(cmd *cobra.Command, records []localhistory.Record) error {
	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No local scan history found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tCOMPLETED (UTC)\tREPOSITORY\tOUTCOME\tFINDINGS\tC/H/M/L\tDEBT (H)")
	for _, record := range records {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d/%d/%d/%d\t%.2f\n",
			record.ID,
			record.CompletedAt.UTC().Format("2006-01-02 15:04:05"),
			record.Repository,
			record.Outcome,
			record.Summary.Findings,
			record.Summary.Critical,
			record.Summary.High,
			record.Summary.Medium,
			record.Summary.Low,
			record.Summary.TechnicalDebtHours,
		)
	}
	return w.Flush()
}

func printHistoryRecord(cmd *cobra.Command, record localhistory.Record) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID:\t%s\n", record.ID)
	fmt.Fprintf(w, "Repository:\t%s\n", record.Repository)
	fmt.Fprintf(w, "Outcome:\t%s\n", record.Outcome)
	fmt.Fprintf(w, "Started (UTC):\t%s\n", record.StartedAt.UTC().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "Completed (UTC):\t%s\n", record.CompletedAt.UTC().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "Findings:\t%d\n", record.Summary.Findings)
	fmt.Fprintf(w, "Critical / High / Medium / Low:\t%d / %d / %d / %d\n",
		record.Summary.Critical, record.Summary.High, record.Summary.Medium, record.Summary.Low)
	fmt.Fprintf(w, "Technical debt hours:\t%.2f\n", record.Summary.TechnicalDebtHours)
	fmt.Fprintf(w, "Warnings:\t%d\n", record.Summary.Warnings)
	fmt.Fprintf(w, "Analyzer failures:\t%d\n", record.Summary.AnalyzerFailures)
	return w.Flush()
}

func confirmHistoryClear(cmd *cobra.Command) (bool, error) {
	fmt.Fprint(cmd.ErrOrStderr(), "Type \"yes\" to clear all local scan history: ")
	response, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read history confirmation: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(response), "yes"), nil
}
