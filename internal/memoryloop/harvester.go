package memoryloop

import (
	"context"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type RawMemorySource interface {
	ScrollRaw(ctx context.Context, limit int, offset string, filters memory.SearchFilters, since, until time.Time, includeCanonical bool) ([]memory.MemoryPoint, string, error)
}

type MemoryHarvester struct {
	source RawMemorySource
}

func NewMemoryHarvester(source RawMemorySource) *MemoryHarvester {
	return &MemoryHarvester{source: source}
}

func (h *MemoryHarvester) Harvest(ctx context.Context, filter HarvestFilter, onBatch func([]memory.MemoryPoint) error) (int, error) {
	if h == nil || h.source == nil {
		return 0, nil
	}
	batchSize := filter.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	limit := filter.Limit
	processed := 0
	cursor := ""

	for {
		if limit > 0 && processed >= limit {
			break
		}
		remaining := batchSize
		if limit > 0 && limit-processed < remaining {
			remaining = limit - processed
		}
		if remaining <= 0 {
			break
		}

		points, nextCursor, err := h.source.ScrollRaw(ctx, remaining, cursor, memory.SearchFilters{
			Project:    filter.Project,
			Speaker:    filter.Speaker,
			MemoryKind: filter.MemoryKind,
		}, filter.Since, filter.Until, filter.IncludeExistingCanon)
		if err != nil {
			return processed, err
		}
		if len(points) == 0 {
			break
		}
		if err := onBatch(points); err != nil {
			return processed, err
		}
		processed += len(points)
		if nextCursor == "" || nextCursor == cursor {
			break
		}
		cursor = nextCursor
	}

	return processed, nil
}
