package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// SummaryIndex is the contract for reading and writing the global
// weknora_summary collection. It is deliberately separated from
// RetrieveEngineRepository because summaries are a single global index
// keyed by knowledge_id, not a per-KB retrieval engine.
//
// Implementations (Milvus) should be tolerant of dimension mismatches
// between the first-created collection and subsequent upserts: a warning
// is logged and the upsert is skipped, never aborts the upstream task.
type SummaryIndex interface {
	// UpsertKnowledgeSummary inserts or replaces the summary row for
	// item.KnowledgeID. The vector dimension must match the collection;
	// if it does not, the call should log a warning and return nil.
	UpsertKnowledgeSummary(ctx context.Context, item *types.SummaryItem) error

	// DeleteKnowledgeSummary removes the summary row for a single knowledge.
	DeleteKnowledgeSummary(ctx context.Context, knowledgeID string) error

	// DeleteKnowledgeBaseSummaries removes all summary rows for a KB.
	DeleteKnowledgeBaseSummaries(ctx context.Context, knowledgeBaseID string) error

	// SearchSummaries performs a dense vector search over the summary
	// collection, returning TopK knowledge-level hits.
	SearchSummaries(
		ctx context.Context,
		queryVector []float32,
		topK int,
		filter types.SummaryFilter,
	) ([]*types.SummaryHit, error)

	// DropSummaryCollection deletes the entire summary collection.
	// Intended for maintenance / reingest scripts.
	DropSummaryCollection(ctx context.Context) error

	// InspectSummaryCollection returns whether the global summary collection
	// exists and, if so, its vector dimension. Intended for startup self-check
	// and maintenance tooling; implementations MUST NOT create the collection
	// as a side effect.
	InspectSummaryCollection(ctx context.Context) (exists bool, dimension int, err error)
}
