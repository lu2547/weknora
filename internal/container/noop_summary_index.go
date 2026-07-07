package container

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// noopSummaryIndex is the fallback SummaryIndex used when the configured
// retriever driver does not support a global summary collection (e.g. the
// build is running without milvus). All writes are silently ignored and
// SearchSummaries returns an empty result so the upstream flow keeps working.
type noopSummaryIndex struct{}

// NewNoopSummaryIndex returns a SummaryIndex whose methods are all no-ops.
func NewNoopSummaryIndex() interfaces.SummaryIndex {
	return &noopSummaryIndex{}
}

func (n *noopSummaryIndex) UpsertKnowledgeSummary(ctx context.Context, item *types.SummaryItem) error {
	return nil
}

func (n *noopSummaryIndex) DeleteKnowledgeSummary(ctx context.Context, knowledgeID string) error {
	return nil
}

func (n *noopSummaryIndex) DeleteKnowledgeBaseSummaries(ctx context.Context, knowledgeBaseID string) error {
	return nil
}

func (n *noopSummaryIndex) SearchSummaries(
	ctx context.Context,
	queryVector []float32,
	topK int,
	filter types.SummaryFilter,
) ([]*types.SummaryHit, error) {
	return nil, nil
}

func (n *noopSummaryIndex) DropSummaryCollection(ctx context.Context) error {
	return nil
}

func (n *noopSummaryIndex) InspectSummaryCollection(ctx context.Context) (bool, int, error) {
	// No backing store; report as non-existent.
	return false, 0, nil
}
