package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// MigrateFuncs holds the database migration entry points wired by the
// process entrypoint. Up applies all up migrations; DownOne rolls back the
// most recently applied version.
type MigrateFuncs struct {
	Up      func(context.Context) error
	DownOne func(context.Context) error
}

func NewRoot(version string, serve func(context.Context) error, migrate MigrateFuncs) *cobra.Command {
	root := &cobra.Command{
		Use:          "platform-api",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
	}
	root.AddCommand(
		&cobra.Command{
			Use:   "serve",
			Short: "Serve the platform HTTP API",
			RunE: func(command *cobra.Command, _ []string) error {
				return serve(command.Context())
			},
		},
		&cobra.Command{
			Use:   "version",
			Short: "Print the platform API version",
			Run: func(command *cobra.Command, _ []string) {
				fmt.Fprintln(command.OutOrStdout(), version)
			},
		},
		newMigrateCommand(migrate),
	)
	return root
}

func newMigrateCommand(migrate MigrateFuncs) *cobra.Command {
	command := &cobra.Command{
		Use:          "migrate",
		Short:        "Run database migrations",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
	}
	command.AddCommand(
		&cobra.Command{
			Use:          "up",
			Short:        "Apply all up migrations",
			SilenceUsage: true,
			Args:         cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				return runMigrate(command.Context(), migrate.Up)
			},
		},
		&cobra.Command{
			Use:          "down-one",
			Short:        "Roll back the most recent migration",
			SilenceUsage: true,
			Args:         cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				return runMigrate(command.Context(), migrate.DownOne)
			},
		},
	)
	return command
}

// runMigrate requires DATABASE_URL before handing control to the wired
// migration func; the error propagates to the process entrypoint, which exits
// non-zero.
func runMigrate(ctx context.Context, run func(context.Context) error) error {
	if os.Getenv("DATABASE_URL") == "" {
		return errors.New("migrate requires the DATABASE_URL environment variable to be set")
	}
	return run(ctx)
}
