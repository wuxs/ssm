package cmd

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wuxs/ssm/pkg/sshconfig"
)

//go:embed completion.bash
var bashHistoryCompletion string

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:       "completion bash",
		Short:     "Generate Bash history completion with a fixed cursor",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"bash"},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), bashHistoryCompletion)
			return err
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:    "__history-complete prefix",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			candidates, err := historyCandidates(args[0])
			if err != nil {
				return err
			}
			for _, candidate := range candidates {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), candidate); err != nil {
					return err
				}
			}
			return nil
		},
	})
}

func historyCandidates(prefix string) ([]string, error) {
	configs, err := sshconfig.List()
	if err != nil {
		return nil, err
	}
	var candidates []string
	seen := make(map[string]bool)
	for _, cfg := range configs {
		// Qualify the alias, rather than HostName, to preserve SSH authentication and jump options.
		qualified := cfg.GetKey()
		if cfg.Alias != "" {
			qualified = cfg.Username + "@" + cfg.Alias + ":" + cfg.Port
		}
		for _, target := range []string{cfg.Alias, qualified} {
			// Insert literal shell words; metadata and credentials never become candidates.
			if target == "" || strings.ContainsAny(target, " \t\r\n\\\"'`$;&|<>(){}!*?#") || !strings.HasPrefix(target, prefix) || seen[target] {
				continue
			}
			seen[target] = true
			candidates = append(candidates, target)
		}
	}

	sort.Strings(candidates)
	return candidates, nil
}
