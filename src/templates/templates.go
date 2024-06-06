package templates

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

type Template struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Website     string   `json:"website"`
	Author      string   `json:"author"`
	Source      string   `json:"source"`
	Tags        []string `json:"tags"`
}

func Load(path string) ([]*Template, error) {
	templateBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	var templates []*Template
	if err := json.Unmarshal(templateBytes, &templates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file %s: %w", path, err)
	}

	slices.SortFunc(templates, func(a *Template, b *Template) int {
		if a.Title < b.Title {
			return -1
		} else if a.Title > b.Title {
			return 1
		}

		return 0
	})

	return templates, nil
}

func GetTemplates(url string) ([]*Template, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download templates: %w", err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading response body: %w", err)
	}

	var templates []*Template
	if err := json.Unmarshal(body, &templates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal templates: %w", err)
	}

	return templates, nil
}

func Sync(source string, outputDir string) error {
	_, err := os.Stat(outputDir)
	if err != nil {
		os.MkdirAll(outputDir, 0755)
	}

	repoRoot := filepath.Join(outputDir, filepath.Base(source))
	_, err = os.Stat(repoRoot)
	if err == nil {
		pullCmd := exec.Command("git", "pull")
		pullCmd.Dir = repoRoot
		if err := pullCmd.Run(); err != nil {
			return fmt.Errorf("failed to pull repo: %w", err)
		}
	} else {
		cloneCmd := exec.Command("git", "clone", source)
		cloneCmd.Dir = outputDir
		if err := cloneCmd.Run(); err != nil {
			return fmt.Errorf("failed to clone repo: %w", err)
		}
	}

	return nil
}
