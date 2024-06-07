package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Project struct {
	Name      string                 `json:"name"`
	Hooks     map[string]Hook        `json:"hooks"`
	Workflows map[string]interface{} `json:"workflows"`
	Metadata  *Metadata              `json:"metadata"`
	Services  map[string]Service     `json:"services"`
	Raw       string                 `json:"-"`
}

type Metadata struct {
	Template string `json:"template"`
}

type Hook struct {
	Run     string `json:"run"`
	Shell   string `json:"shell"`
	Posix   *Hook  `json:"posix"`
	Windows *Hook  `json:"windows"`
}

type Service struct {
	Host         string          `json:"host"`
	Language     string          `json:"language"`
	Hooks        map[string]Hook `json:"hooks"`
	RelativePath string          `json:"project"`
}

func Load(path string) (*Project, error) {
	azureYamlPath := filepath.Join(path, "azure.yaml")

	_, err := os.Stat(azureYamlPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("azure.yaml file not found in repo root @ %s, %w", azureYamlPath, err)
	}

	projectBytes, err := os.ReadFile(azureYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read azure.yaml file %s: %w", azureYamlPath, err)
	}

	var azdProject Project
	if err := yaml.Unmarshal(projectBytes, &azdProject); err != nil {
		return nil, fmt.Errorf("failed to unmarshal azure.yaml file %s: %w", azureYamlPath, err)
	}

	azdProject.Raw = string(projectBytes)

	return &azdProject, nil
}
