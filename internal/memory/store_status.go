package memory

import "context"

// StoreStatus reports whether a memory operation was served from a degraded
// backend, such as the in-memory fallback store, instead of the primary store.
type StoreStatus struct {
	Degraded bool   `json:"degraded"`
	Reason   string `json:"reason,omitempty"`
}

type storeStatusKey struct{}

// WithStoreStatus returns a context carrying a StoreStatus that callers can
// inspect after a store operation completes.
func WithStoreStatus(ctx context.Context) (context.Context, *StoreStatus) {
	status := &StoreStatus{}
	return context.WithValue(ctx, storeStatusKey{}, status), status
}

// MarkStoreDegraded records that the operation was served by a fallback path.
// It is a no-op when the context carries no status.
func MarkStoreDegraded(ctx context.Context, err error) {
	status, ok := ctx.Value(storeStatusKey{}).(*StoreStatus)
	if !ok || status == nil {
		return
	}
	status.Degraded = true
	if err != nil {
		status.Reason = err.Error()
	}
}

// StoreStatusFrom returns the StoreStatus attached to a context, if any.
func StoreStatusFrom(ctx context.Context) *StoreStatus {
	status, ok := ctx.Value(storeStatusKey{}).(*StoreStatus)
	if !ok {
		return nil
	}
	return status
}

// DegradedStatus returns the status for inclusion in a response, or nil when
// the operation was served by the primary store.
func DegradedStatus(status *StoreStatus) *StoreStatus {
	if status == nil || !status.Degraded {
		return nil
	}
	return status
}
