package main

import (
	"context"

	"github.com/fatih/color"
	"github.com/wbreza/azd-template-analysis/cmd"
)

func main() {
	ctx := context.Background()
	rootCmd := cmd.NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		color.Red("ERROR: %v", err)
	}
}
