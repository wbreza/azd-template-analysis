package cmd

import (
	"github.com/spf13/cobra"
)

func newAnalyzeCmd(root *cobra.Command) {
	analyze := &cobra.Command{
		Use: "analyze",
		Run: func(cmd *cobra.Command, args []string) {
			println("analyze")
		},
	}

	root.AddCommand(analyze)
}
