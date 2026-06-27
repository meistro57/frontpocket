package memoryloop

import (
	"fmt"
	"sort"
	"strings"

	"github.com/meistro57/frontpocket/internal/memory"
)

type MemoryClusterer struct {
	MaxClusterSize int
}

func (c MemoryClusterer) Cluster(points []memory.MemoryPoint) []Cluster {
	if len(points) == 0 {
		return nil
	}
	maxSize := c.MaxClusterSize
	if maxSize <= 0 {
		maxSize = 25
	}
	groups := make(map[string][]memory.MemoryPoint)
	for _, point := range points {
		key := clusterKey(point)
		groups[key] = append(groups[key], point)
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	clusters := make([]Cluster, 0, len(keys))
	for _, key := range keys {
		items := dedupeBySnippet(groups[key])
		for idx := 0; idx < len(items); idx += maxSize {
			end := idx + maxSize
			if end > len(items) {
				end = len(items)
			}
			chunk := items[idx:end]
			clusters = append(clusters, Cluster{
				ClusterID: fmt.Sprintf("%s_%02d", sanitizeClusterKey(key), len(clusters)+1),
				Label:     key,
				Points:    chunk,
			})
		}
	}
	return clusters
}

func clusterKey(point memory.MemoryPoint) string {
	project := strings.TrimSpace(point.Project)
	if project == "" {
		project = "general"
	}
	kind := strings.TrimSpace(point.MemoryKind)
	if kind == "" {
		kind = memory.KindFact
	}
	speaker := strings.TrimSpace(point.Speaker)
	if speaker == "" {
		speaker = "unknown"
	}
	return fmt.Sprintf("%s | %s | %s", project, kind, speaker)
}

func dedupeBySnippet(points []memory.MemoryPoint) []memory.MemoryPoint {
	seen := make(map[string]struct{}, len(points))
	out := make([]memory.MemoryPoint, 0, len(points))
	for _, point := range points {
		snippet := strings.ToLower(strings.TrimSpace(point.SourceQuote))
		if snippet == "" {
			snippet = strings.ToLower(strings.TrimSpace(point.Summary))
		}
		if snippet == "" {
			snippet = strings.ToLower(strings.TrimSpace(point.Text))
		}
		if snippet == "" {
			snippet = point.MemoryID
		}
		if _, ok := seen[snippet]; ok {
			continue
		}
		seen[snippet] = struct{}{}
		out = append(out, point)
	}
	return out
}

func sanitizeClusterKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	replacer := strings.NewReplacer(" ", "_", "|", "_", "/", "_", "\\", "_", ":", "_", "-", "_")
	clean := replacer.Replace(key)
	for strings.Contains(clean, "__") {
		clean = strings.ReplaceAll(clean, "__", "_")
	}
	return strings.Trim(clean, "_")
}
