package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"

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

			allResults := []*analyze.TemplateWithResults{}

			analysisCtx := analyze.AnalysisContext{
				WorkingDirectory: flags.filePath,
			}

			for _, template := range templateList {
				if flags.template == "" || flags.template == template.Source {
					templateDir := filepath.Join(flags.filePath, filepath.Base(template.Source))
					var templateAnalysis *analyze.Segment

					templateAnalysis, err = analyze.AnalyzeTemplate(analysisCtx, template)
					if err != nil {
						templateAnalysis = &analyze.Segment{
							Errors: []string{err.Error()},
						}

						color.Red("Failed to analyze template '%s': %w", templateDir, err)
					} else {
						color.Green("Template '%s' analyzed successfully.", templateDir)
					}

					allResults = append(allResults, &analyze.TemplateWithResults{
						Template: template,
						Analysis: templateAnalysis,
					})
				}
			}

			resultBytes, err := json.MarshalIndent(allResults, "", " ")
			if err != nil {
				return fmt.Errorf("failed to marshal results: %w", err)
			}

			if err := os.WriteFile(filepath.Join(flags.filePath, "results.json"), resultBytes, 0644); err != nil {
				return fmt.Errorf("failed to write results: %w", err)
			}

			rootFilePath := filepath.Join(flags.filePath, "results.csv")
			rootMetrics, err := writeAnalysisToCsv(rootFilePath, allResults, "", false)
			if err != nil {
				return fmt.Errorf("failed to write root analysis to csv: %w", err)
			}

			writeMetrics("Root", "Based on all templates", rootMetrics)

			hooksFilePath := filepath.Join(flags.filePath, "hooks.csv")
			hookMetrics, err := writeAnalysisToCsv(hooksFilePath, allResults, "hooks", true)
			if err != nil {
				return fmt.Errorf("failed to write hooks analysis to csv: %w", err)
			}

			writeMetrics("Hooks", "Based on templates that use hooks", hookMetrics)

			return nil
		},
	}

	analyze.Flags().StringVarP(&flags.template, "template", "t", "", "Template to analyze.")
	analyze.Flags().StringVarP(&flags.filePath, "file", "f", "", "Path to the template sync directory.")

	root.AddCommand(analyze)
}

func writeMetrics(title string, description string, metrics map[string]string) {
	sortedKeys := []string{}
	for k := range metrics {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	fmt.Println()
	color.HiWhite("%s Metrics: (%s)", title, description)
	for _, key := range sortedKeys {
		color.HiBlack("%s: %s", key, metrics[key])
	}
}

func writeAnalysisToCsv(filePath string, allResults []*analyze.TemplateWithResults, segmentFilter string, recursive bool) (map[string]string, error) {
	csvFile, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create csv file: %w", err)
	}

	csvWriter := csv.NewWriter(csvFile)
	allInsightKeys := []string{}

	segmentCount := 0

	for _, result := range allResults {
		segment := result.Analysis
		if segmentFilter != "" {
			if analyze.HasSegment(result.Analysis, segmentFilter) {
				segment = result.Analysis.Segments[segmentFilter]
			} else {
				continue
			}
		}

		segmentCount++
		resultKeys := getInsightKeys(segment, recursive)
		for _, key := range resultKeys {
			if !slices.Contains(allInsightKeys, key) {
				allInsightKeys = append(allInsightKeys, key)
			}
		}
	}

	sort.Strings(allInsightKeys)

	headers := []string{"Template", "Repo", "Author"}
	headers = append(headers, allInsightKeys...)

	csvWriter.Write(headers)

	for _, result := range allResults {
		segment := result.Analysis
		if segmentFilter != "" {
			if analyze.HasSegment(result.Analysis, segmentFilter) {
				segment = result.Analysis.Segments[segmentFilter]
			} else {
				continue
			}
		}

		values := []string{
			result.Template.Title,
			result.Template.Source,
			result.Template.Author,
		}

		for _, insightKey := range allInsightKeys {
			insightValue := analyze.GetInsight(segment, insightKey)
			values = append(values, fmt.Sprint(insightValue))
		}

		csvWriter.Write(values)
	}

	csvWriter.Flush()
	csvFile.Close()

	insightMetrics := map[string]string{}

	for _, insightKey := range allInsightKeys {
		count := 0

		for _, result := range allResults {
			value := analyze.GetInsight(result.Analysis, insightKey)
			boolVal, ok := value.(bool)
			if ok && boolVal {
				count++
			}
		}

		// Calculate the percentage
		if segmentCount > 0 {
			insightMetrics[insightKey] = fmt.Sprintf("%d%%", int(math.Round((float64(count)/float64(segmentCount))*100)))
		} else {
			insightMetrics[insightKey] = "N/A"
		}
	}

	return insightMetrics, nil
}

func getInsightKeys(analysis *analyze.Segment, recursive bool) []string {
	allKeys := []string{}
	for key := range analysis.Insights {
		if !slices.Contains(allKeys, key) {
			allKeys = append(allKeys, key)
		}
	}

	if recursive {
		for _, segment := range analysis.Segments {
			segmentKeys := getInsightKeys(segment, recursive)
			for _, key := range segmentKeys {
				if !slices.Contains(allKeys, key) {
					allKeys = append(allKeys, key)
				}
			}
		}
	}

	sort.Strings(allKeys)

	return allKeys
}
