package types

import (
	"reflect"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestChunksSearchRequest_ToSearchParams_Defaults(t *testing.T) {
	r := &ChunksSearchRequest{
		Query:            "hello",
		KnowledgeBaseIDs: []string{"kb-1"},
	}
	got := r.ToSearchParams()

	if got.QueryText != "hello" {
		t.Errorf("QueryText = %q, want %q", got.QueryText, "hello")
	}
	// TopK <= 0 should default to 10.
	if got.MatchCount != 10 {
		t.Errorf("default MatchCount = %d, want 10", got.MatchCount)
	}
	// UseBM25 unset => keyword match enabled (DisableKeywordsMatch=false).
	if got.DisableKeywordsMatch {
		t.Error("DisableKeywordsMatch must be false when UseBM25 is unset")
	}
	if !reflect.DeepEqual(got.KnowledgeBaseIDs, []string{"kb-1"}) {
		t.Errorf("KnowledgeBaseIDs = %v, want [kb-1]", got.KnowledgeBaseIDs)
	}
}

func TestChunksSearchRequest_ToSearchParams_TopKPropagated(t *testing.T) {
	r := &ChunksSearchRequest{
		Query:            "q",
		KnowledgeBaseIDs: []string{"kb-1"},
		TopK:             25,
	}
	got := r.ToSearchParams()
	if got.MatchCount != 25 {
		t.Errorf("MatchCount = %d, want 25", got.MatchCount)
	}
}

func TestChunksSearchRequest_ToSearchParams_UseBM25TriState(t *testing.T) {
	cases := []struct {
		name              string
		useBM25           *bool
		wantDisableKwords bool
	}{
		{"unset_defaults_to_enabled", nil, false},
		{"explicit_true_keeps_enabled", boolPtr(true), false},
		{"explicit_false_disables", boolPtr(false), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &ChunksSearchRequest{
				Query:            "q",
				KnowledgeBaseIDs: []string{"kb-1"},
				UseBM25:          tc.useBM25,
			}
			got := r.ToSearchParams()
			if got.DisableKeywordsMatch != tc.wantDisableKwords {
				t.Errorf("DisableKeywordsMatch = %v, want %v", got.DisableKeywordsMatch, tc.wantDisableKwords)
			}
		})
	}
}

func TestChunksSearchRequest_ToSearchParams_FilterPassthrough(t *testing.T) {
	r := &ChunksSearchRequest{
		Query:            "q",
		KnowledgeBaseIDs: []string{"kb-a", "kb-b"},
		KnowledgeIDs:     []string{"k-1"},
		TagIDs:           []string{"t-1", "t-2"},
		TopK:             5,
		VectorThreshold:  0.7,
		KeywordThreshold: 0.5,
	}
	got := r.ToSearchParams()

	if !reflect.DeepEqual(got.KnowledgeBaseIDs, r.KnowledgeBaseIDs) {
		t.Errorf("KnowledgeBaseIDs not propagated: %v", got.KnowledgeBaseIDs)
	}
	if !reflect.DeepEqual(got.KnowledgeIDs, r.KnowledgeIDs) {
		t.Errorf("KnowledgeIDs not propagated: %v", got.KnowledgeIDs)
	}
	if !reflect.DeepEqual(got.TagIDs, r.TagIDs) {
		t.Errorf("TagIDs not propagated: %v", got.TagIDs)
	}
	if got.VectorThreshold != 0.7 || got.KeywordThreshold != 0.5 {
		t.Errorf("threshold passthrough failed: vec=%v kw=%v", got.VectorThreshold, got.KeywordThreshold)
	}
	if got.MatchCount != 5 {
		t.Errorf("MatchCount = %d, want 5", got.MatchCount)
	}
}

func TestChunksSearchRequest_ToSearchParams_NegativeTopKDefaults(t *testing.T) {
	r := &ChunksSearchRequest{Query: "q", KnowledgeBaseIDs: []string{"kb"}, TopK: -3}
	got := r.ToSearchParams()
	if got.MatchCount != 10 {
		t.Errorf("negative TopK should default to 10, got %d", got.MatchCount)
	}
}
