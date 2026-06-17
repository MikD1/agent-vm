package cli

import (
	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/spf13/cobra"
)

// newLimaClient builds a lima.Client honoring the persistent --verbose flag.
// Persistent flags from the root are merged into cmd.Flags() before RunE runs,
// so this resolves correctly from any subcommand; if the flag is somehow absent
// GetBool returns false, which is the safe (normal-mode) default.
func newLimaClient(cmd *cobra.Command) *lima.Client {
	verbose, _ := cmd.Flags().GetBool("verbose")
	return lima.New(lima.ExecRunner{Verbose: verbose})
}
