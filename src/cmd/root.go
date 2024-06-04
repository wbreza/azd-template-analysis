package cmd

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use: "azdt",
	}

	newSyncCmd(root)
	newAnalyzeCmd(root)

	return root
}
