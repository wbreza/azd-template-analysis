package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-template-analysis/templates"
)

type syncFlags struct {
	outputDir string
	template  string
}

func newSyncCmd(root *cobra.Command) {
	flags := &syncFlags{}

	sync := &cobra.Command{
		Use: "sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.outputDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get current working directory: %w", err)
				}
				flags.outputDir = filepath.Join(cwd, "templates")
			}

			// Sync all templates
			if flags.template == "" {
				templateList, err := templates.GetTemplates("https://azure.github.io/awesome-azd/templates.json")
				if err != nil {
					return fmt.Errorf("failed to get templates: %w", err)
				}

				var wg sync.WaitGroup
				// Only allow 10 concurrent downloads
				sem := make(chan bool, 10)

				for _, t := range templateList {
					wg.Add(1)
					sem <- true

					go func(source string) {
						defer wg.Done()
						defer func() { <-sem }()

						if err := templates.Sync(source, flags.outputDir); err != nil {
							color.Red("Template '%s' synced failed, %v.", source, err)
						} else {
							color.Green("Template '%s' synced successfully.", source)
						}
					}(t.Source)
				}

				wg.Wait()

				templateBytes, err := json.MarshalIndent(templateList, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal templates: %w", err)
				}

				templatesFilePath := filepath.Join(flags.outputDir, "templates.json")
				if err := os.WriteFile(templatesFilePath, templateBytes, 0644); err != nil {
					return fmt.Errorf("failed to write templates file: %w", err)
				}

			} else { // Sync a specific template
				if err := templates.Sync(flags.template, flags.outputDir); err != nil {
					return fmt.Errorf("failed to sync template '%s': %w", flags.template, err)
				}

				color.Green("Template '%s' synced successfully.", flags.template)
			}

			return nil
		},
	}

	sync.Flags().StringVarP(&flags.outputDir, "output", "o", "", "The output directory where templates will be downloaded.")
	sync.Flags().StringVarP(&flags.template, "template", "t", "", "The specific git repo template to sync.")

	root.AddCommand(sync)
}
