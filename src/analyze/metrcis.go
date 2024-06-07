package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
)

type MetricSection struct {
	Title       string
	Description string
	Metrics     map[string]string
}

func (m *MetricSection) String() string {
	sortedKeys := []string{}
	for k := range m.Metrics {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	var builder strings.Builder
	title := color.New(color.FgHiWhite)
	metric := color.New(color.FgHiBlack)

	builder.WriteString("\n")
	title.Fprintf(&builder, "%s Metrics: (%s)\n", m.Title, m.Description)

	for _, key := range sortedKeys {
		metric.Fprintf(&builder, "- %s: %s\n", key, m.Metrics[key])
	}

	return builder.String()
}

func (m *MetricSection) Markdown() string {
	sortedKeys := []string{}
	for k := range m.Metrics {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	builder := strings.Builder{}

	fmt.Fprintln(&builder)
	fmt.Fprintf(&builder, "# %s Metrics: (%s)\n", m.Title, m.Description)
	fmt.Fprintln(&builder)

	for _, key := range sortedKeys {
		fmt.Fprintf(&builder, "- **%s**: %s\n", key, m.Metrics[key])
	}

	return builder.String()
}
