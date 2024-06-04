package analyze

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/wbreza/azd-template-analysis/project"
	"github.com/wbreza/azd-template-analysis/templates"
)

type TemplateAnalysis struct {
	Errors   []string                  `json:"errors"`
	Hooks    map[string]map[string]any `json:"hooks"`
	Insights map[string]any            `json:"insights"`
}

type TemplateWithResults struct {
	Template *templates.Template `json:"template"`
	Analysis *TemplateAnalysis   `json:"analysis"`
}

type AnalysisContext struct {
	WorkingDirectory string
}

type analysisFunc func(ctx AnalysisContext, template *templates.Template, analysis *TemplateAnalysis) error

var heuristicMap = map[string]regexp.Regexp{
	"usesAzLogin": *regexp.MustCompile(`az\slogin`),
	"usesAzd":     *regexp.MustCompile(`azd\s`),
}

func AnalyzeTemplate(ctx AnalysisContext, template *templates.Template) (*TemplateAnalysis, error) {
	analysis := &TemplateAnalysis{
		Errors:   []string{},
		Insights: map[string]any{},
	}

	analysisFuncs := []analysisFunc{
		analyzeHooks,
		analyzeProject,
	}

	for _, f := range analysisFuncs {
		if err := f(ctx, template, analysis); err != nil {
			analysis.Errors = append(analysis.Errors, err.Error())
		}
	}

	return analysis, nil
}

func analyzeProject(ctx AnalysisContext, template *templates.Template, analysis *TemplateAnalysis) error {
	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	azdProject, err := project.Load(templatePath)

	analysis.Insights["missingAzureYamlAtRoot"] = errors.Is(err, fs.ErrNotExist)
	analysis.Insights["hasProjectHooks"] = azdProject != nil && len(azdProject.Hooks) > 0
	analysis.Insights["usesAzLogin"] = azdProject != nil && hasHookHeuristic(analysis, "usesAzLogin")
	analysis.Insights["usesAzd"] = azdProject != nil && hasHookHeuristic(analysis, "usesAzd")
	analysis.Insights["hasWorkflows"] = azdProject != nil && len(azdProject.Workflows) > 0
	analysis.Insights["hasMetadata"] = azdProject != nil && azdProject.Metadata != nil
	analysis.Insights["hasServices"] = azdProject != nil && len(azdProject.Services) > 0
	analysis.Insights["usesAppService"] = azdProject != nil && hasHostType(*azdProject, "appservice")
	analysis.Insights["usesContainerApps"] = azdProject != nil && hasHostType(*azdProject, "containerapp")
	analysis.Insights["usesFunctionApps"] = azdProject != nil && hasHostType(*azdProject, "function")
	analysis.Insights["usesSpringApps"] = azdProject != nil && hasHostType(*azdProject, "springapp")
	analysis.Insights["usesAks"] = azdProject != nil && hasHostType(*azdProject, "aks")
	analysis.Insights["usesStaticWebApps"] = azdProject != nil && hasHostType(*azdProject, "staticwebapp")
	analysis.Insights["usesAiEndpoint"] = azdProject != nil && hasHostType(*azdProject, "ai.endpoint")

	return nil
}

func hasHookHeuristic(analysis *TemplateAnalysis, key string) bool {
	if len(analysis.Hooks) == 0 {
		return false
	}

	for _, features := range analysis.Hooks {
		if features[key] == true {
			return true
		}
	}

	return false

}

func hasHostType(azdProject project.Project, hostType string) bool {
	if len(azdProject.Services) == 0 {
		return false
	}

	for _, service := range azdProject.Services {
		if service.Host == hostType {
			return true
		}
	}

	return false
}

func analyzeHooks(ctx AnalysisContext, template *templates.Template, analysis *TemplateAnalysis) error {
	if analysis.Hooks == nil {
		analysis.Hooks = map[string]map[string]any{}
	}

	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	azdProject, err := project.Load(templatePath)
	if err != nil {
		return err
	}

	for hookName, hook := range azdProject.Hooks {
		analysis.Hooks[hookName] = map[string]any{}

		hookRun := hook.Run
		if hookRun == "" {
			hookRun = hook.Posix.Run
		}
		if hookRun == "" {
			hookRun = hook.Windows.Run
		}
		if hookRun == "" {
			analysis.Errors = append(analysis.Errors, fmt.Sprintf("%s hook missing run command", hookName))
			return nil
		}

		var hookScript string
		scriptPath := filepath.Join(templatePath, hookRun)
		_, err := os.Stat(scriptPath)

		// Inline script
		if err != nil {
			analysis.Hooks[hookName]["usesInlineScript"] = true
			hookScript = hookRun
		} else { // File script
			analysis.Hooks[hookName]["usesInlineScript"] = false
			hookBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				analysis.Errors = append(analysis.Errors, fmt.Sprintf("Failed reading hook file '%s': %v", scriptPath, err))
			}
			hookScript = string(hookBytes)
		}

		for heuristicKey, heuristic := range heuristicMap {
			analysis.Hooks[hookName]["script"] = hookScript
			analysis.Hooks[hookName][heuristicKey] = heuristic.MatchString(hookScript)
		}
	}

	return nil
}
