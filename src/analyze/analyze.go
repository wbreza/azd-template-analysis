package analyze

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/wbreza/azd-template-analysis/project"
	"github.com/wbreza/azd-template-analysis/templates"
)

type Segment struct {
	Errors   []string            `json:"errors"`
	Data     map[string]any      `json:"data"`
	Insights map[string]any      `json:"insights"`
	Segments map[string]*Segment `json:"segments"`
}

func NewSegment() *Segment {
	return &Segment{
		Errors:   []string{},
		Data:     map[string]any{},
		Insights: map[string]any{},
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
	}

	for _, f := range analysisFuncs {
		if err := f(ctx, template, root); err != nil {
			root.Errors = append(root.Errors, err.Error())
		}
	}

	return root, nil
}

func analyzeProject(ctx AnalysisContext, template *templates.Template, analysis *Segment) error {
	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	azdProject, err := project.Load(templatePath)

	analysis.Insights["missingAzureYamlAtRoot"] = errors.Is(err, fs.ErrNotExist)
	analysis.Insights["hasProjectHooks"] = HasSegment(analysis, "hooks")
	analysis.Insights["hasWorkflows"] = azdProject != nil && len(azdProject.Workflows) > 0
	analysis.Insights["hasMetadata"] = azdProject != nil && azdProject.Metadata != nil
	analysis.Insights["hasServices"] = azdProject != nil && len(azdProject.Services) > 0

	if azdProject != nil {
		analysis.Insights["serviceCount"] = len(azdProject.Services)
	}

	hostTypes := []string{"appservice", "containerapp", "function", "springapp", "aks", "staticwebapp", "ai.endpoint"}
	for _, hostType := range hostTypes {
		analysis.Insights[fmt.Sprintf("host-%s", hostType)] = azdProject != nil && hasHostType(*azdProject, hostType)
	}

	languages := map[string][]string{
		"dotnet":     {"csharp", "dotnet", "fsharp"},
		"java":       {"java"},
		"javascript": {"javascript", "node", "ts"},
		"python":     {"python", "py"},
	}
	for key, languageSet := range languages {
		analysis.Insights[fmt.Sprintf("lang-%s", key)] = azdProject != nil && hasLanguage(*azdProject, languageSet)
	}

	return nil
}

func HasSegment(analysis *Segment, key string) bool {
	_, has := analysis.Segments[key]
	return has
}

func GetInsight(analysis *Segment, key string) any {
	value, has := analysis.Insights[key]
	if has {
		return value
	}

	for _, segment := range analysis.Segments {
		return GetInsight(segment, key)
	}

	return ""
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

func analyzeHooks(ctx AnalysisContext, template *templates.Template, analysis *Segment) error {
	templatePath := filepath.Join(ctx.WorkingDirectory, filepath.Base(template.Source))
	azdProject, err := project.Load(templatePath)
	if err != nil {
		return err
	}

	if len(azdProject.Hooks) == 0 {
		return nil
	}

	hooksRootSegment := NewSegment()
	analysis.Segments["hooks"] = hooksRootSegment

	for hookName, hook := range azdProject.Hooks {
		hookSegment := NewSegment()
		hooksRootSegment.Segments[hookName] = hookSegment

		hookRun := hook.Run
		if hookRun == "" {
			hookRun = hook.Posix.Run
		}
		if hookRun == "" {
			hookRun = hook.Windows.Run
		}
		if hookRun == "" {
			hookSegment.Errors = append(hooksRootSegment.Errors, fmt.Sprintf("%s hook missing run command", hookName))
			return nil
		}

		hasWindowsScript := hook.Windows != nil && hook.Windows.Run != ""
		hasPosixScript := hook.Posix != nil && hook.Posix.Run != ""

		usesOsVariantScripts := hasWindowsScript && hasPosixScript
		hookSegment.Insights["usesOsVariantScripts"] = usesOsVariantScripts

		var hookScript string
		scriptPath := filepath.Join(templatePath, hookRun)
		_, err := os.Stat(scriptPath)

		// Inline script
		if err != nil {
			hookSegment.Insights["usesInlineScript"] = true
			hookScript = hookRun
		} else { // File script
			hookSegment.Insights["usesInlineScript"] = false
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
			embeddedScriptPath := filepath.Join(templatePath, scriptPath)
			scriptBytes, err := os.ReadFile(embeddedScriptPath)
			if err != nil {
				hookSegment.Errors = append(hookSegment.Errors, fmt.Sprintf("Failed reading embedded script '%s': %v", embeddedScriptPath, err))
			} else {
				allScripts[scriptPath] = string(scriptBytes)
			}
		}

		for heuristicKey, heuristic := range heuristicMap {
			for key, script := range allScripts {
				hookSegment.Data[key] = script
				hookSegment.Insights[heuristicKey] = heuristic.MatchString(script)
			}
		}
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
			hooksRootSegment.Insights[fmt.Sprintf("type-%s", hookName)] = HasSegment(hooksRootSegment, hookName)
		}
	}

	return nil
}

var scriptRegex = regexp.MustCompile(`([a-zA-Z]:[\\/]|[\\/])?((?:[a-zA-Z0-9_\-\.]+[\\/])*[a-zA-Z0-9_\-\.]+\.(sh|ps1))`)
