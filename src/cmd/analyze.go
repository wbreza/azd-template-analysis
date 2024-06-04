package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-template-analysis/analyze"
	"github.com/wbreza/azd-template-analysis/templates"
)

type analyzeFlags struct {
	template string
	filePath string
}

func newAnalyzeCmd(root *cobra.Command) {
	flags := &analyzeFlags{}

	analyze := &cobra.Command{
		Use: "analyze",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.filePath == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get current working directory: %w", err)
				}
				flags.filePath = filepath.Join(cwd, "templates")
			}

			templateList, err := templates.Load(filepath.Join(flags.filePath, "templates.json"))
			if err != nil {
				return fmt.Errorf("failed to load templates: %w", err)
			}

			resultMap := map[string]*analyze.TemplateWithResults{}

			analysisCtx := analyze.AnalysisContext{
				WorkingDirectory: flags.filePath,
			}

			for _, template := range templateList {
				if flags.template == "" || flags.template == template.Source {
					templateDir := filepath.Join(flags.filePath, filepath.Base(template.Source))
					var templateAnalysis *analyze.TemplateAnalysis

					templateAnalysis, err = analyze.AnalyzeTemplate(analysisCtx, template)
					if err != nil {
						templateAnalysis = &analyze.TemplateAnalysis{
							Errors: []string{err.Error()},
						}

						color.Red("Failed to analyze template '%s': %w", templateDir, err)
					} else {
						color.Green("Template '%s' analyzed successfully.", templateDir)
					}

					resultMap[templateDir] = &analyze.TemplateWithResults{
						Template: template,
						Analysis: templateAnalysis,
					}
				}
			}

			allResults := []*analyze.TemplateWithResults{}
			for _, v := range resultMap {
				allResults = append(allResults, v)
			}

			resultBytes, err := json.MarshalIndent(allResults, "", " ")
			if err != nil {
				return fmt.Errorf("failed to marshal results: %w", err)
			}

			if err := os.WriteFile(filepath.Join(flags.filePath, "results.json"), resultBytes, 0644); err != nil {
				return fmt.Errorf("failed to write results: %w", err)
			}

			csvFile, err := os.Create(filepath.Join(flags.filePath, "results.csv"))
			if err != nil {
				return fmt.Errorf("failed to create csv file: %w", err)
			}

			csvWriter := csv.NewWriter(csvFile)
			defer csvWriter.Flush()
			defer csvFile.Close()

			csvWriter.Write([]string{
				"Template",
				"Repo",
				"Author",
				"Missing azure.yaml",
				"Has Project Hooks",
				"Uses az login",
				"Uses azd",
				"Has Workflows",
				"Has Metadata",
				"Has Services",
				"App Service",
				"Container App",
				"Function App",
				"Spring App",
				"AKS",
				"Static Web App",
				"AI Endpoint",
			})

			for _, result := range allResults {
				csvWriter.Write([]string{
					result.Template.Title,
					result.Template.Source,
					result.Template.Author,
					fmt.Sprint(result.Analysis.Insights["missingAzureYamlAtRoot"]),
					fmt.Sprint(result.Analysis.Insights["hasProjectHooks"]),
					fmt.Sprint(result.Analysis.Insights["usesAzLogin"]),
					fmt.Sprint(result.Analysis.Insights["usesAzd"]),
					fmt.Sprint(result.Analysis.Insights["hasWorkflows"]),
					fmt.Sprint(result.Analysis.Insights["hasMetadata"]),
					fmt.Sprint(result.Analysis.Insights["hasServices"]),
					fmt.Sprint(result.Analysis.Insights["usesAppService"]),
					fmt.Sprint(result.Analysis.Insights["usesContainerApps"]),
					fmt.Sprint(result.Analysis.Insights["usesFunctionApps"]),
					fmt.Sprint(result.Analysis.Insights["usesSpringApps"]),
					fmt.Sprint(result.Analysis.Insights["usesAks"]),
					fmt.Sprint(result.Analysis.Insights["usesStaticWebApps"]),
					fmt.Sprint(result.Analysis.Insights["usesAiEndpoint"]),
				})
			}

			return nil
		},
	}

	analyze.Flags().StringVarP(&flags.template, "template", "t", "", "Template to analyze.")
	analyze.Flags().StringVarP(&flags.filePath, "file", "f", "", "Path to the template sync directory.")

	root.AddCommand(analyze)
}
