package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wuxs/ssm/pkg/sshconfig"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Normalize ~/.ssh/ssm.config and include it from ~/.ssh/config",
	Long: `Normalize existing ~/.ssh/ssm.config entries to the current SSM SSH config layout.
The command also ensures ~/.ssh/config includes ~/.ssh/ssm.config.`,
	Run: runMigrateCommand,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}

func runMigrateCommand(cmd *cobra.Command, args []string) {
	result, err := sshconfig.MigrateLegacy()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Updated %d configs in %s\n", result.MigratedCount, result.SSMConfigPath)
	if result.IncludeAdded {
		fmt.Printf("Added Include ~/.ssh/ssm.config to %s\n", result.SSHConfigPath)
	} else {
		fmt.Printf("Include already present in %s\n", result.SSHConfigPath)
	}
}
