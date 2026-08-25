package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func NewRoot(version string, serve func(context.Context) error) *cobra.Command {
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
	)
	return root
}
