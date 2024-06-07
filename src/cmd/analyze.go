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
	template  string
	filePath  string
	outputDir string
}

func newAnalyzeCmd(root *cobra.Command) {
	flags := &analyzeFlags{}

	analyze := &cobra.Command{
		Use: "analyze",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current working directory: %w", err)
			}

			if flags.filePath == "" {
				flags.filePath = filepath.Join(cwd, "templates")
			}

			if flags.outputDir == "" {
				flags.outputDir = filepath.Join(cwd, "output")
			}

			if err := os.MkdirAll(flags.outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
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

			// Write raw results
			if err := os.WriteFile(filepath.Join(flags.outputDir, "raw.json"), resultBytes, 0644); err != nil {
				return fmt.Errorf("failed to write results: %w", err)
			}

			// Write template results
			templatesFilePath := filepath.Join(flags.outputDir, "templates.csv")
			templateMetrics, err := writeAnalysisToCsv(templatesFilePath, allResults, "template", false)
			if err != nil {
				return fmt.Errorf("failed to write root analysis to csv: %w", err)
			}

			templateSection := analyze.MetricSection{
				Title:       "Templates",
				Description: "Based on all templates",
				Metrics:     templateMetrics,
			}

			// Write project results
			projectsFilePath := filepath.Join(flags.outputDir, "projects.csv")
			projectMetrics, err := writeAnalysisToCsv(projectsFilePath, allResults, "project", false)
			if err != nil {
				return fmt.Errorf("failed to write root analysis to csv: %w", err)
			}

			projectSection := analyze.MetricSection{
				Title:       "Projects",
				Description: "Based on all templates with azure.yaml",
				Metrics:     projectMetrics,
			}

			// Write hook results
			hooksFilePath := filepath.Join(flags.outputDir, "hooks.csv")
			hookMetrics, err := writeAnalysisToCsv(hooksFilePath, allResults, "hooks", true)
			if err != nil {
				return fmt.Errorf("failed to write hooks analysis to csv: %w", err)
			}

			hookSection := analyze.MetricSection{
				Title:       "Hooks",
				Description: "Based on templates that use hooks",
				Metrics:     hookMetrics,
			}

			fmt.Print(templateSection.String())
			fmt.Print(projectSection.String())
			fmt.Print(hookSection.String())

			// Write markdown
			markdownFile, err := os.Create(filepath.Join(flags.outputDir, "output.md"))
			if err != nil {
				return fmt.Errorf("failed to create markdown file: %w", err)
			}

			defer markdownFile.Close()

			fmt.Fprint(markdownFile, templateSection.Markdown())
			fmt.Fprint(markdownFile, projectSection.Markdown())
			fmt.Fprint(markdownFile, hookSection.Markdown())

			return nil
		},
	}

	analyze.Flags().StringVarP(&flags.template, "template", "t", "", "Template to analyze.")
	analyze.Flags().StringVarP(&flags.filePath, "file", "f", "", "Path to the template sync directory.")
	analyze.Flags().StringVarP(&flags.outputDir, "output", "o", "", "Path to the output directory.")

	root.AddCommand(analyze)
}

func writeAnalysisToCsv(filePath string, allResults []*analyze.TemplateWithResults, segmentFilter string, recursive bool) (map[string]string, error) {
	csvFile, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create csv file: %w", err)
	}

	csvWriter := csv.NewWriter(csvFile)
	allInsightKeys := []string{}
	allInsights := map[string]*analyze.Insight{}
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
		resultInsights := getInsightKeys(segment, recursive)
		for key, insight := range resultInsights {
			if !slices.Contains(allInsightKeys, key) {
				allInsightKeys = append(allInsightKeys, key)
				allInsights[key] = insight
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
			insightValue, _ := analyze.GetTopInsight[any](segment, insightKey)
			values = append(values, fmt.Sprint(insightValue))
		}

		csvWriter.Write(values)
	}

	csvWriter.Flush()
	csvFile.Close()

	insightMetrics := map[string]string{}

	for key, insight := range allInsights {
		count := 0

		for _, result := range allResults {
			segment := result.Analysis
			if segmentFilter != "" {
				if analyze.HasSegment(result.Analysis, segmentFilter) {
					segment = result.Analysis.Segments[segmentFilter]
				} else {
					continue
				}
			}

			resolver := insight.Resolver()
			switch insight.Type {
			case analyze.BoolInsight:
				boolVal, ok := resolver.Value(segment, key).(bool)
				if ok && boolVal {
					count++
				}
			case analyze.NumberInsight:
				intVal, ok := resolver.Value(segment, key).(int)
				if ok {
					count += intVal
				}
			}
		}

		switch insight.Type {
		case analyze.BoolInsight:
			insightMetrics[key] = fmt.Sprintf("%d%%", int(math.Round((float64(count)/float64(segmentCount))*100)))
		case analyze.NumberInsight:
			insightMetrics[key] = fmt.Sprintf("%.2f (Avg)", float32(count)/float32(segmentCount))
		default:
			insightMetrics[key] = "N/A"
		}
	}

	return insightMetrics, nil
}

func getInsightKeys(analysis *analyze.Segment, recursive bool) map[string]*analyze.Insight {
	allInsights := map[string]*analyze.Insight{}

	for key, insight := range analysis.Insights {
		allInsights[key] = insight
	}

	if recursive {
		for _, segment := range analysis.Segments {
			segmentInsights := getInsightKeys(segment, recursive)
			for key, insight := range segmentInsights {
				allInsights[key] = insight
			}
		}
	}

	return allInsights
}
