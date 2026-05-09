package reports

import (
	"fmt"
	"sort"
	"strings"

	"withdraw-bot/internal/core"
)

func RenderStats(results map[core.MonitorModuleID]core.MonitorResult) string {
	statuses := make([]core.MonitorStatus, 0, len(results))
	keys := make([]string, 0, len(results))
	for id, result := range results {
		statuses = append(statuses, result.Status)
		keys = append(keys, string(id))
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Status: %s\n", core.WorstKnownStatus(statuses)))
	for _, key := range keys {
		result := results[core.MonitorModuleID(key)]
		builder.WriteString(fmt.Sprintf("\n%s: %s\n", key, result.Status))
		for _, metric := range result.Metrics {
			builder.WriteString(fmt.Sprintf("- %s: %s %s\n", metric.Key, metric.Value, metric.Unit))
		}
		for _, finding := range result.Findings {
			builder.WriteString(fmt.Sprintf("- %s: %s\n", finding.Severity, finding.Message))
		}
	}
	return strings.TrimSpace(builder.String())
}
