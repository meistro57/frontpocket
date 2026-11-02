package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func asStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(asString(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if strings.Contains(trimmed, ",") {
			parts := strings.Split(trimmed, ",")
			return uniqueTrimmed(parts)
		}
		return []string{trimmed}
	default:
		return nil
	}
}

func strategyStrings(strategies []DeepDrillStrategy) []string {
	if len(strategies) == 0 {
		return nil
	}
	out := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		if trimmed := strings.TrimSpace(string(strategy)); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseDeepDrillStrategies(value any) []DeepDrillStrategy {
	parts := asStringSlice(value)
	if len(parts) == 0 {
		return nil
	}
	out := make([]DeepDrillStrategy, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, DeepDrillStrategy(trimmed))
		}
	}
	return out
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, entry := range typed {
			out[key] = entry
		}
		return out
	default:
		return nil
	}
}

func asFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func asBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func ParseDeepDrillThoughtType(value string) DeepDrillThoughtType {
	return DeepDrillThoughtType(strings.ToUpper(strings.TrimSpace(value)))
}

func ParseDeepDrillInfoGain(value string) DeepDrillInfoGain {
	return DeepDrillInfoGain(strings.ToUpper(strings.TrimSpace(value)))
}

func NormalizeDeepDrillArtifactStatus(thought DeepDrillThought) string {
	return normalizeArtifactStatus(thought)
}

func ParseDeepDrillUncertaintiesCSV(value string) []DeepDrillUncertainty {
	parts := strings.Split(value, ",")
	rows := make([]DeepDrillUncertainty, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.ToUpper(part))
		if trimmed == "" {
			continue
		}
		rows = append(rows, DeepDrillUncertainty(trimmed))
	}
	return normalizeUncertainties(rows)
}

func ParseDeepDrillHypothesesCSV(value string) []DeepDrillHypothesisState {
	parts := strings.Split(value, ";")
	rows := make([]DeepDrillHypothesisState, 0, len(parts))
	for idx, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		id := ""
		statement := trimmed
		if split := strings.SplitN(trimmed, "|", 2); len(split) == 2 {
			id = strings.TrimSpace(split[0])
			statement = strings.TrimSpace(split[1])
		}
		if id == "" {
			id = fmt.Sprintf("H%d", idx+1)
		}
		rows = append(rows, DeepDrillHypothesisState{ID: id, Statement: statement, Rank: idx + 1, Confidence: 0.42, EvidenceStrength: 0.2, Status: "active"})
	}
	return normalizeHypothesisSeeds(rows)
}

func ParseDeepDrillFreezeAfter(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}
