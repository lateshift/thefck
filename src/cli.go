package main

import "github.com/spf13/cobra"

type cliOptions struct {
	dbPath           string
	reportChanges    bool
	reportDuplicates bool
	serveAddress     string
}

// newRootCommand builds the command tree. The default command scans a
// directory, and the "serve" subcommand exposes the indexed records over HTTP.
func newRootCommand() *cobra.Command {
	opts := cliOptions{
		dbPath:           "checksums.db",
		reportDuplicates: true,
		serveAddress:     "127.0.0.1:8080",
	}

	cmd := &cobra.Command{
		Use:   "thefck [directory]",
		Short: "Index files and flag duplicate content",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return RunScan(root, ScanOptions{
				DBPath:           opts.dbPath,
				ReportChanges:    opts.reportChanges,
				ReportDuplicates: opts.reportDuplicates,
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.dbPath, "db", opts.dbPath, "bbolt database path")
	flags.BoolVar(&opts.reportChanges, "report-changes", false, "print new, changed, and missing files during the scan")
	flags.BoolVar(&opts.reportDuplicates, "report-duplicates", opts.reportDuplicates, "print duplicate matches discovered during the scan")

	cmd.AddCommand(newServeCommand(&opts))
	return cmd
}

func newServeCommand(opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the indexed files SPA",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ServeIndex(ServeOptions{
				DBPath:  opts.dbPath,
				Address: opts.serveAddress,
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.dbPath, "db", opts.dbPath, "bbolt database path")
	flags.StringVar(&opts.serveAddress, "addr", opts.serveAddress, "HTTP listen address")

	return cmd
}
