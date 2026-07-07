package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// GetQueryEmbedding computes the query embedding using the system default
// embedding model. Callers can pre-compute and reuse the result across
// multiple KBs to avoid redundant embedding API calls.
func (s *knowledgeBaseService) GetQueryEmbedding(ctx context.Context, kbID string, queryText string) ([]float32, error) {
	// Use system default embedding model (EmbeddingModelID removed from KB)
	defaultModelID, err := s.getDefaultEmbeddingModelID(ctx)
	if err != nil {
		logger.Errorf(ctx, "GetQueryEmbedding: failed to get default embedding model: %v", err)
		return nil, err
	}

	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, defaultModelID)
	if err != nil {
		logger.Errorf(ctx, "GetQueryEmbedding: failed to get embedding model %s: %v", defaultModelID, err)
		return nil, err
	}

	return embeddingModel.Embed(ctx, queryText)
}

// getDefaultEmbeddingModelID returns the system default embedding model ID.
func (s *knowledgeBaseService) getDefaultEmbeddingModelID(ctx context.Context) (string, error) {
	models, err := s.modelService.ListModels(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list models: %w", err)
	}
	for _, m := range models {
		if m.Type == types.ModelTypeEmbedding && m.IsDefault {
			return m.ID, nil
		}
	}
	// Fallback: first embedding model
	for _, m := range models {
		if m.Type == types.ModelTypeEmbedding {
			return m.ID, nil
		}
	}
	return "", fmt.Errorf("no embedding model configured")
}

// SearchKnowledgeSummaries performs a semantic search over the global
// weknora_summary collection. The query is embedded using the embedding
// model of referenceKBID (which must be one of the KBs being searched)
// so all KBs in the search scope share the same vector space.
func (s *knowledgeBaseService) SearchKnowledgeSummaries(
	ctx context.Context,
	referenceKBID string,
	query string,
	topK int,
	filter types.SummaryFilter,
) ([]*types.SummaryHit, error) {
	if s.summaryIndex == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}
	vec, err := s.GetQueryEmbedding(ctx, referenceKBID, query)
	if err != nil {
		return nil, err
	}
	return s.summaryIndex.SearchSummaries(ctx, vec, topK, filter)
}

// ResolveEmbeddingModelKeys resolves embedding model IDs to their actual model
// identity key (name + endpoint). Since EmbeddingModelID is now system-level,
// all KBs share the same default embedding model.
func (s *knowledgeBaseService) ResolveEmbeddingModelKeys(ctx context.Context, kbs []*types.KnowledgeBase) map[string]string {
	// All KBs use the system default embedding model
	defaultModelID, err := s.getDefaultEmbeddingModelID(ctx)
	if err != nil {
		logger.Warnf(ctx, "ResolveEmbeddingModelKeys: cannot get default embedding model: %v", err)
		result := make(map[string]string, len(kbs))
		for _, kb := range kbs {
			result[kb.ID] = "default"
		}
		return result
	}

	model, err := s.modelService.GetModelByID(ctx, defaultModelID)
	modelKey := defaultModelID
	if err == nil && model != nil {
		modelKey = model.Name + "|" + model.Parameters.BaseURL
	}

	result := make(map[string]string, len(kbs))
	for _, kb := range kbs {
		result[kb.ID] = modelKey
	}
	return result
}

// HybridSearch performs hybrid search, including vector retrieval and keyword retrieval.
//
// id is the "primary" knowledge base ID used to resolve the embedding model and
// determine the KB type (e.g. FAQ). When params.KnowledgeBaseIDs is set, those
// IDs are used for the actual retrieval scope instead of id alone, allowing a
// single call to span multiple KBs that share the same embedding model. In that
// case id should be any one of those KBs (typically the first) so that its
// embedding model and type configuration are used for the search.
func (s *knowledgeBaseService) HybridSearch(ctx context.Context,
	id string,
	params types.SearchParams,
) ([]*types.SearchResult, error) {
	// Determine the set of KB IDs to search
	searchKBIDs := params.KnowledgeBaseIDs
	if len(searchKBIDs) == 0 {
		searchKBIDs = []string{id}
	}

	logger.Infof(ctx, "Hybrid search parameters, knowledge base IDs: %v, query text: %s", searchKBIDs, params.QueryText)

	tenantInfo, _ := types.TenantInfoFromContext(ctx)

	// Create a composite retrieval engine with tenant's configured retrievers
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
	if err != nil {
		logger.Errorf(ctx, "Failed to create retrieval engine: %v", err)
		return nil, err
	}

	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	// Use 5x over-retrieval to ensure sufficient candidates for RRF fusion and reranking.
	// Scale proportionally when searching multiple KBs to maintain per-KB recall quality.
	matchCount := max(params.MatchCount*5, 50) * len(searchKBIDs)
	if matchCount > 1000 {
		matchCount = 1000
	}

	// Build retrieval parameters for vector and keyword engines
	retrieveParams, err := s.buildRetrievalParams(ctx, retrieveEngine, kb, params, searchKBIDs, matchCount)
	if err != nil {
		return nil, err
	}
	if len(retrieveParams) == 0 {
		logger.Error(ctx, "No retrieval parameters available")
		return nil, errors.New("no retrieve params")
	}

	// Execute retrieval using the configured engines
	logger.Infof(ctx, "Starting retrieval, parameter count: %d", len(retrieveParams))
	retrieveResults, err := retrieveEngine.Retrieve(ctx, retrieveParams)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_ids": searchKBIDs,
			"query_text":         params.QueryText,
		})
		return nil, err
	}

	// Separate and fuse retrieval results
	vectorResults, keywordResults := classifyRetrievalResults(ctx, retrieveResults)
	if len(vectorResults) == 0 && len(keywordResults) == 0 {
		logger.Info(ctx, "No search results found")
		return nil, nil
	}
	logger.Infof(ctx, "Result count before fusion: vector=%d, keyword=%d", len(vectorResults), len(keywordResults))

	deduplicatedChunks := fuseOrDeduplicate(ctx, vectorResults, keywordResults)

	kb.EnsureDefaults()

	// FAQ-specific post-processing: iterative retrieval or negative question filtering
	deduplicatedChunks = s.applyFAQPostProcessing(ctx, kb, deduplicatedChunks, vectorResults, retrieveEngine, retrieveParams, params, matchCount)

	// Limit to MatchCount
	if len(deduplicatedChunks) > params.MatchCount {
		deduplicatedChunks = deduplicatedChunks[:params.MatchCount]
	}

	return s.processSearchResults(ctx, deduplicatedChunks, params.SkipContextEnrichment)
}

// buildRetrievalParams constructs the vector and keyword retrieval parameters
// based on the knowledge base type, engine capabilities, and search params.
func (s *knowledgeBaseService) buildRetrievalParams(
	ctx context.Context,
	retrieveEngine *retriever.CompositeRetrieveEngine,
	kb *types.KnowledgeBase,
	params types.SearchParams,
	searchKBIDs []string,
	matchCount int,
) ([]types.RetrieveParams, error) {
	var retrieveParams []types.RetrieveParams

	// Resolve KB metadata for the search set so the underlying retrieve engine
	// (e.g. Milvus) can route to the correct collection without an extra DB
	// round-trip. Failure is non-fatal: the engine will fall back to its own
	// KBLookup if needed.
	var kbMetas []*types.KnowledgeBase
	if len(searchKBIDs) == 1 && kb != nil && searchKBIDs[0] == kb.ID {
		kbMetas = []*types.KnowledgeBase{kb}
	} else if len(searchKBIDs) > 0 {
		if fetched, err := s.repo.GetKnowledgeBaseByIDs(ctx, searchKBIDs); err == nil {
			kbMetas = fetched
		} else {
			logger.Warnf(ctx, "Failed to fetch KB metadata for retrieve params, falling back to lookup: %v", err)
		}
	}

	// Add vector retrieval params if supported
	if retrieveEngine.SupportRetriever(types.VectorRetrieverType) && !params.DisableVectorMatch {
		logger.Info(ctx, "Vector retrieval supported, preparing vector retrieval parameters")

		var queryEmbedding []float32

		if len(params.QueryEmbedding) > 0 {
			queryEmbedding = params.QueryEmbedding
			logger.Infof(ctx, "Using pre-computed query embedding, vector length: %d", len(queryEmbedding))
		} else {
			// Use system default embedding model (EmbeddingModelID removed from KB)
			defaultModelID, err := s.getDefaultEmbeddingModelID(ctx)
			if err != nil {
				logger.Errorf(ctx, "Failed to get default embedding model: %v", err)
				return nil, err
			}
			logger.Infof(ctx, "Getting embedding model, model ID: %s", defaultModelID)

			embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, defaultModelID)
			if err != nil {
				logger.Errorf(ctx, "Failed to get embedding model, model ID: %s, error: %v", defaultModelID, err)
				return nil, err
			}
			logger.Infof(ctx, "Embedding model retrieved: %v", embeddingModel)

			logger.Info(ctx, "Starting to generate query embedding")
			queryEmbedding, err = embeddingModel.Embed(ctx, params.QueryText)
			if err != nil {
				logger.Errorf(ctx, "Failed to embed query text, query text: %s, error: %v", params.QueryText, err)
				return nil, err
			}
			logger.Infof(ctx, "Query embedding generated successfully, embedding vector length: %d", len(queryEmbedding))
		}

		vectorParams := types.RetrieveParams{
			Query:            params.QueryText,
			Embedding:        queryEmbedding,
			KnowledgeBaseIDs: searchKBIDs,
			KnowledgeBases:   kbMetas,
			TopK:             matchCount,
			Threshold:        params.VectorThreshold,
			RetrieverType:    types.VectorRetrieverType,
			KnowledgeIDs:     params.KnowledgeIDs,
			TagIDs:           params.TagIDs,
		}

		// For FAQ knowledge base, use FAQ index
		if kb.Type == types.KnowledgeBaseTypeFAQ {
			vectorParams.KnowledgeType = types.KnowledgeTypeFAQ
		}

		retrieveParams = append(retrieveParams, vectorParams)
		logger.Info(ctx, "Vector retrieval parameters setup completed")
	}

	// Add keyword retrieval params if supported and not FAQ
	if retrieveEngine.SupportRetriever(types.KeywordsRetrieverType) && !params.DisableKeywordsMatch &&
		kb.Type != types.KnowledgeBaseTypeFAQ {
		logger.Info(ctx, "Keyword retrieval supported, preparing keyword retrieval parameters")
		retrieveParams = append(retrieveParams, types.RetrieveParams{
			Query:            params.QueryText,
			KnowledgeBaseIDs: searchKBIDs,
			KnowledgeBases:   kbMetas,
			TopK:             matchCount,
			Threshold:        params.KeywordThreshold,
			RetrieverType:    types.KeywordsRetrieverType,
			KnowledgeIDs:     params.KnowledgeIDs,
			TagIDs:           params.TagIDs,
		})
		logger.Info(ctx, "Keyword retrieval parameters setup completed")
	}

	return retrieveParams, nil
}
