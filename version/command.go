package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCommand returns a CLI command to print the application binary version information.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the application binary version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			verInfo := NewInfo()

			_, err := fmt.Fprintln(cmd.OutOrStdout(), verInfo.String())
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}
