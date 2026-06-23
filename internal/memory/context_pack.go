package memory

import "context"

type ContextPacker struct {
	Store MemoryStore
}

func (c ContextPacker) Build(ctx context.Context, req ContextPackRequest) (ContextPackResponse, error) {
	searchReq := SearchRequest{
		Query:   req.Query,
		Limit:   req.Limit,
		Filters: req.Filters,
	}
	results, err := c.Store.Search(ctx, searchReq)
	if err != nil {
		return ContextPackResponse{}, err
	}
	return ContextPackResponse{
		Query:      req.Query,
		MemoryPack: results,
	}, nil
}
