package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/wbreza/azd-template-analysis/cmd"
)

func main() {
	if os.Getenv("AZD_DEBUG") == "true" {
		fmt.Printf("Press any key to continue debugging (%d)", os.Getpid())
		_, err := fmt.Scanln()
		if err != nil {
			color.Red("ERROR: %v", err)
			os.Exit(1)
		}
	}

	ctx := context.Background()
	rootCmd := cmd.NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		color.Red("ERROR: %v", err)
	}
}
