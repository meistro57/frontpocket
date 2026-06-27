package memoryloop

import (
	"context"

	"github.com/meistro57/frontpocket/internal/memory"
)

type Runner struct {
	Harvester    *MemoryHarvester
	Clusterer    MemoryClusterer
	Extractor    FactExtractor
	Scout        ContradictionScout
	Timeline     TimelineBuilder
	Curator      CanonCurator
	ReviewQueue  *FileReviewQueue
	CanonicalRef memory.MemoryStore
}

func (r Runner) Run(ctx context.Context, filter HarvestFilter, writeCandidates bool) (LoopResult, error) {
	batches := make([]memory.MemoryPoint, 0)
	if r.Harvester != nil {
		_, err := r.Harvester.Harvest(ctx, filter, func(points []memory.MemoryPoint) error {
			batches = append(batches, points...)
			return nil
		})
		if err != nil {
			return LoopResult{}, err
		}
	}

	clusters := r.Clusterer.Cluster(batches)
	candidates := make([]Candidate, 0)
	for _, cluster := range clusters {
		candidates = append(candidates, r.Extractor.Extract(cluster)...)
	}
	candidates = append(candidates, r.Timeline.Build(batches)...)
	candidates, contradictionCount := r.Scout.Check(candidates)

	existingCanonical := make([]memory.SearchResult, 0)
	if r.CanonicalRef != nil {
		results, err := r.CanonicalRef.Search(ctx, memory.SearchRequest{Query: "memory canonical", Limit: 200, IncludeRejected: true})
		if err == nil {
			existingCanonical = results
		}
	}
	curated, skipped := r.Curator.Curate(candidates, existingCanonical)
	if writeCandidates && r.ReviewQueue != nil {
		if err := r.ReviewQueue.Upsert(curated); err != nil {
			return LoopResult{}, err
		}
	}
	return LoopResult{
		Candidates:          curated,
		SkippedDuplicates:   skipped,
		ContradictionMarked: contradictionCount,
	}, nil
}
