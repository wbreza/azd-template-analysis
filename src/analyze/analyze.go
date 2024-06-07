package analyze

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/wbreza/azd-template-analysis/project"
	"github.com/wbreza/azd-template-analysis/templates"
)

type Segment struct {
	Errors   []string            `json:"errors"`
	Data     map[string]any      `json:"data"`
	Insights map[string]*Insight `json:"insights"`
	Segments map[string]*Segment `json:"segments"`
}

func NewSegment() *Segment {
	return &Segment{
		Errors:   []string{},
		Data:     map[string]any{},
		Insights: map[string]*Insight{},
		Segments: map[string]*Segment{},
	}
}

type TemplateWithResults struct {
	Template *templates.Template `json:"template"`
	Analysis *Segment            `json:"analysis"`
}

type AnalysisContext struct {
	WorkingDirectory string
}

type analysisFunc func(ctx AnalysisContext, template *templates.Template, analysis *Segment) error

var heuristicMap = map[string]regexp.Regexp{
	"usesAzCli":      *regexp.MustCompile(`az\s`),
	"usesAzCliLogin": *regexp.MustCompile(`az\slogin`),
	"usesAzd":        *regexp.MustCompile(`azd\s`),
}

func AnalyzeTemplate(ctx AnalysisContext, template *templates.Template) (*Segment, error) {
	root := NewSegment()

	analysisFuncs := []analysisFunc{
		analyzeHooks,
		analyzeProject,
		analyzeTemplate,
	}

	for _, analyzeFunc := range analysisFuncs {
		if err := analyzeFunc(ctx, template, root); err != nil {
			root.Errors = append(root.Errors, err.Error())
		}
	}

	return root, nil
}

func HasSegment(analysis *Segment, key string) bool {
	_, has := analysis.Segments[key]
	if !has {
		for _, segment := range analysis.Segments {
			has = HasSegment(segment, key)
			if has {
				break
			}
		}
	}

	return has
}

func HasInsightValue[T comparable](analysis *Segment, key string, value T) bool {
	results, has := GetInsight[T](analysis, key)
	if !has {
		return false
	}

	return slices.Contains(results, value)
}

func GetTopInsight[T comparable](segment *Segment, key string) (T, bool) {
	var zero T

	values, has := GetInsight[any](segment, key)
	if !has {
		return zero, false
	}

	return values[0].(T), true
}

func GetInsight[T comparable](analysis *Segment, key string) ([]T, bool) {
	results := []T{}

	insight, has := analysis.Insights[key]
	if has {
		value, ok := insight.Value.(T)
		if ok {
			results = append(results, value)
		}
	}

	for _, segment := range analysis.Segments {
		childResults, has := GetInsight[T](segment, key)
		if has {
			results = append(results, childResults...)
		}
	}

	return results, len(results) > 0
}

func analyzeTemplate(ctx AnalysisContext, template *templates.Template, analysis *Segment) error {
	templateSegment := NewSegment()
	analysis.Segments["template"] = templateSegment

	templateSegment.Insights["tagCount"] = NewInsight(NumberInsight, len(template.Author))
	templateSegment.Insights["isCommunity"] = NewInsight(BoolInsight, slices.Contains(template.Tags, "community"))
	templateSegment.Insights["isMsft"] = NewInsight(BoolInsight, slices.Contains(template.Tags, "msft"))

	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	azdProject, err := project.Load(templatePath)
	templateSegment.Insights["hasAzureYaml"] = NewInsight(BoolInsight, azdProject != nil && err == nil)

	analyzeFileSystem(ctx, template, templateSegment)

	return nil
}

func analyzeFileSystem(ctx AnalysisContext, template *templates.Template, root *Segment) error {
	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	infraPath := filepath.Join(templatePath, "infra")

	root.Insights["hasInfra"] = NewInsight(BoolInsight, hasDir(templatePath, "infra"))
	root.Insights["hasGithub"] = NewInsight(BoolInsight, hasDir(templatePath, ".github"))
	root.Insights["hasAzdo"] = NewInsight(BoolInsight, hasDir(templatePath, ".azdo"))
	root.Insights["hasDevcontainer"] = NewInsight(BoolInsight, hasDir(templatePath, ".devcontainer"))

	root.Insights["infraBicep"] = NewInsight(BoolInsight, hasFilePattern(infraPath, "*.bicep"))
	root.Insights["infraTerraform"] = NewInsight(BoolInsight, hasFilePattern(infraPath, "*.tf"))

	return nil
}

func analyzeProject(ctx AnalysisContext, template *templates.Template, root *Segment) error {
	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	azdProject, err := project.Load(templatePath)
	if err != nil {
		return err
	}

	if azdProject == nil {
		return nil
	}

	projectSegment := NewSegment()
	root.Segments["project"] = projectSegment

	projectSegment.Insights["hasHooks"] = NewInsight(BoolInsight, HasInsightValue(root, "hasProjectHooks", true) || HasInsightValue(root, "hasServiceHooks", true))
	projectSegment.Insights["hasWorkflows"] = NewInsight(BoolInsight, len(azdProject.Workflows) > 0)
	projectSegment.Insights["hasMetadata"] = NewInsight(BoolInsight, azdProject.Metadata != nil)
	projectSegment.Insights["hasServices"] = NewInsight(BoolInsight, len(azdProject.Services) > 0)

	if azdProject != nil {
		projectSegment.Insights["serviceCount"] = NewInsight(NumberInsight, len(azdProject.Services))
	}

	hostTypes := []string{"appservice", "containerapp", "function", "springapp", "aks", "staticwebapp", "ai.endpoint"}
	for _, hostType := range hostTypes {
		projectSegment.Insights[fmt.Sprintf("host-%s", hostType)] = NewInsight(BoolInsight, hasHostType(*azdProject, hostType))
	}

	languages := map[string][]string{
		"dotnet":     {"csharp", "dotnet", "fsharp"},
		"java":       {"java"},
		"javascript": {"javascript", "node", "ts"},
		"python":     {"python", "py"},
	}
	for key, languageSet := range languages {
		projectSegment.Insights[fmt.Sprintf("lang-%s", key)] = NewInsight(BoolInsight, hasLanguage(*azdProject, languageSet))
	}

	return nil
}

func analyzeHooks(ctx AnalysisContext, template *templates.Template, root *Segment) error {
	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	azdProject, err := project.Load(templatePath)
	if err != nil {
		return err
	}

	hooksRootSegment := NewSegment()
	hasProjectHooks := len(azdProject.Hooks) > 0

	if hasProjectHooks {
		projectHooks := NewSegment()
		hooksRootSegment.Segments["project"] = projectHooks

		// Project Hooks
		analyzeHooksMap(azdProject.Hooks, projectHooks, templatePath)
	}

	hasServiceHooks := false
	serviceHooks := NewSegment()

	// Service Hooks
	for serviceName, service := range azdProject.Services {
		if len(service.Hooks) == 0 {
			continue
		}

		serviceSegment := NewSegment()
		serviceHooks.Segments[serviceName] = serviceSegment
		hasServiceHooks = true

		servicePath := filepath.Join(templatePath, service.RelativePath)
		analyzeHooksMap(service.Hooks, serviceSegment, servicePath)
	}

	if hasServiceHooks {
		hooksRootSegment.Segments["services"] = serviceHooks
	}

	hooksRootSegment.Insights["hasProjectHooks"] = NewInsight(BoolInsight, hasProjectHooks)
	hooksRootSegment.Insights["hasServiceHooks"] = NewInsight(BoolInsight, hasServiceHooks)

	if hasProjectHooks || hasServiceHooks {
		root.Segments["hooks"] = hooksRootSegment
	}

	return nil
}

func hasFilePattern(path string, pattern string) bool {
	matches := []string{}

	err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() {
			matched, err := filepath.Match(pattern, entry.Name())
			if err != nil {
				return err
			}
			if matched {
				matches = append(matches, path)
			}
		}

		return nil
	})

	if err != nil {
		return false
	}

	return len(matches) > 0
}

func hasDir(root string, dirName string) bool {
	dirPath := filepath.Join(root, dirName)
	_, err := os.Stat(dirPath)

	return err == nil
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

func hasLanguage(azdProject project.Project, languageSet []string) bool {
	if len(azdProject.Services) == 0 {
		return false
	}

	for _, service := range azdProject.Services {
		if slices.Contains(languageSet, service.Language) {
			return true
		}
	}

	return false
}

func analyzeHooksMap(hooks map[string]project.Hook, root *Segment, filePath string) {
	totalLocCount := 0

	for hookName, hook := range hooks {
		locCount := 0

		hookSegment := NewSegment()
		root.Segments[hookName] = hookSegment

		hookRun := hook.Run
		if hookRun == "" {
			hookRun = hook.Posix.Run
		}
		if hookRun == "" {
			hookRun = hook.Windows.Run
		}
		if hookRun == "" {
			hookSegment.Errors = append(root.Errors, fmt.Sprintf("%s hook missing run command", hookName))
			return
		}

		hasWindowsScript := hook.Windows != nil && hook.Windows.Run != ""
		hasPosixScript := hook.Posix != nil && hook.Posix.Run != ""

		usesOsVariantScripts := hasWindowsScript && hasPosixScript
		hookSegment.Insights["usesOsVariantScripts"] = NewInsight(BoolInsight, usesOsVariantScripts)

		var hookScript string
		scriptPath := filepath.Join(filePath, hookRun)
		_, err := os.Stat(scriptPath)

		// Inline script
		if err != nil {
			hookSegment.Insights["usesInlineScript"] = NewInsight(BoolInsight, true)
			hookScript = hookRun
		} else { // File script
			hookSegment.Insights["usesInlineScript"] = NewInsight(BoolInsight, false)
			hookBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				hookSegment.Errors = append(hookSegment.Errors, fmt.Sprintf("Failed reading hook file '%s': %v", scriptPath, err))
			}
			hookScript = string(hookBytes)
		}

		allScripts := map[string]string{
			"script": hookScript,
		}

		embeddedScripts := scriptRegex.FindAllString(hookScript, -1)
		for _, scriptPath := range embeddedScripts {
			embeddedScriptPath := filepath.Join(filePath, scriptPath)
			scriptBytes, err := os.ReadFile(embeddedScriptPath)
			if err != nil {
				hookSegment.Errors = append(hookSegment.Errors, fmt.Sprintf("Failed reading embedded script '%s': %v", embeddedScriptPath, err))
			} else {
				hookScript := string(scriptBytes)
				allScripts[scriptPath] = hookScript
			}
		}

		for heuristicKey, heuristic := range heuristicMap {
			for key, script := range allScripts {
				hookSegment.Data[key] = script
				hookSegment.Insights[heuristicKey] = NewInsight(BoolInsight, heuristic.MatchString(script))
			}
		}

		for _, script := range allScripts {
			locCount += len(strings.Split(script, "\n"))
		}

		hookSegment.Insights["hooks-loc"] = NewInsight(NumberInsight, locCount)
		totalLocCount += locCount
	}

	hookNames := []string{
		"restore",
		"build",
		"provision",
		"package",
		"deploy",
		"up",
		"down",
	}

	phases := []string{"pre", "post"}

	for _, hookName := range hookNames {
		for _, phase := range phases {
			hookName := fmt.Sprintf("%s%s", phase, hookName)
			root.Insights[fmt.Sprintf("type-%s", hookName)] = NewInsight(BoolInsight, HasSegment(root, hookName))
		}
	}

	root.Insights["hooks-loc"] = NewInsight(NumberInsight, totalLocCount)
}

var scriptRegex = regexp.MustCompile(`([a-zA-Z]:[\\/]|[\\/])?((?:[a-zA-Z0-9_\-\.]+[\\/])*[a-zA-Z0-9_\-\.]+\.(sh|ps1))`)
