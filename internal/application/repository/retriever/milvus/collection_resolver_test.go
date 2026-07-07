package milvus

import (
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// fixedTime is used so enterprise collection names are deterministic.
var fixedTime = time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)

func newKB(id, category string) *types.KnowledgeBase {
	return &types.KnowledgeBase{
		ID:        id,
		Category:  category,
		CreatedAt: fixedTime,
	}
}

func TestEmbeddingCollection_PersonalAndPublic(t *testing.T) {
	r := NewCollectionResolver()

	personal, err := r.EmbeddingCollection(newKB("kb-1", types.KnowledgeBaseCategoryPersonal))
	if err != nil {
		t.Fatalf("personal: unexpected error: %v", err)
	}
	if personal != PersonalCollectionName {
		t.Errorf("personal collection = %q, want %q", personal, PersonalCollectionName)
	}

	public, err := r.EmbeddingCollection(newKB("kb-2", types.KnowledgeBaseCategoryPublic))
	if err != nil {
		t.Fatalf("public: unexpected error: %v", err)
	}
	if public != PublicCollectionName {
		t.Errorf("public collection = %q, want %q", public, PublicCollectionName)
	}
}

func TestEmbeddingCollection_EnterpriseConvention(t *testing.T) {
	r := NewCollectionResolver()
	kb := newKB("ent_abcdef_uuid_suffix", types.KnowledgeBaseCategoryEnterprise)

	got, err := r.EmbeddingCollection(kb)
	if err != nil {
		t.Fatalf("enterprise: unexpected error: %v", err)
	}

	wantPrefix := "enterprise_"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("enterprise collection %q must start with %q", got, wantPrefix)
	}
	// New convention: enterprise_<lower(knowledge_base_id)> (no sanitize).
	expected := "enterprise_ent_abcdef_uuid_suffix"
	if got != expected {
		t.Errorf("enterprise collection = %q, want %q", got, expected)
	}
}

func TestEmbeddingCollection_EnterpriseShortID(t *testing.T) {
	// IDs of any length use the full id as suffix (lowercased only).
	r := NewCollectionResolver()
	kb := newKB("ab_c", types.KnowledgeBaseCategoryEnterprise)

	got, err := r.EmbeddingCollection(kb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "enterprise_ab_c" {
		t.Errorf("short id collection = %q, want %q", got, "enterprise_ab_c")
	}
}

func TestEmbeddingCollection_EnterpriseLowercasesUpperID(t *testing.T) {
	// Uppercase characters in KB ID must be lowercased (no other char replacement).
	r := NewCollectionResolver()
	kb := newKB("KB_ABC_XYZ_Def", types.KnowledgeBaseCategoryEnterprise)

	got, err := r.EmbeddingCollection(kb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "enterprise_kb_abc_xyz_def"
	if got != want {
		t.Errorf("uppercase id collection = %q, want %q", got, want)
	}
}

func TestEmbeddingCollection_UnknownCategory(t *testing.T) {
	r := NewCollectionResolver()
	_, err := r.EmbeddingCollection(newKB("kb-x", "weird"))
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
	// Should wrap ErrUnknownCategory.
	if !strings.Contains(err.Error(), "unknown knowledge base category") {
		t.Errorf("error should mention unknown category, got: %v", err)
	}
}

func TestEmbeddingCollection_NilKB(t *testing.T) {
	r := NewCollectionResolver()
	_, err := r.EmbeddingCollection(nil)
	if err == nil {
		t.Fatal("expected error for nil KB")
	}
}

func TestSummaryCollection_AlwaysGlobal(t *testing.T) {
	r := NewCollectionResolver()
	got := r.SummaryCollection(newKB("any", types.KnowledgeBaseCategoryEnterprise))
	if got != SummaryCollectionName {
		t.Errorf("summary collection = %q, want %q", got, SummaryCollectionName)
	}
	// Even for nil, must return the global summary collection.
	if r.SummaryCollection(nil) != SummaryCollectionName {
		t.Error("summary collection should be global even for nil KB")
	}
}

func TestEmbeddingCollectionByMeta_MatchesEnterpriseHelper(t *testing.T) {
	kb := newKB("zzzzzz-suffix", types.KnowledgeBaseCategoryEnterprise)
	info := FromKnowledgeBase(kb)

	byKB := EnterpriseCollectionName(kb)
	byMeta, err := EmbeddingCollectionByMeta(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if byKB != byMeta {
		t.Errorf("EnterpriseCollectionName=%q vs EmbeddingCollectionByMeta=%q", byKB, byMeta)
	}
}

func TestGroupByCollection_MultiCategory(t *testing.T) {
	r := NewCollectionResolver()
	kbs := []*types.KnowledgeBase{
		newKB("p1", types.KnowledgeBaseCategoryPersonal),
		newKB("p2", types.KnowledgeBaseCategoryPersonal),
		newKB("pub-1", types.KnowledgeBaseCategoryPublic),
		newKB("ent-1xxxxx", types.KnowledgeBaseCategoryEnterprise),
	}

	groups, err := r.GroupByCollection(kbs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Personal: p1, p2 share the same collection.
	personalIDs := groups[PersonalCollectionName]
	if len(personalIDs) != 2 {
		t.Errorf("personal group size = %d, want 2", len(personalIDs))
	}
	if !contains(personalIDs, "p1") || !contains(personalIDs, "p2") {
		t.Errorf("personal group missing ids: %v", personalIDs)
	}

	// Public: pub-1 alone.
	publicIDs := groups[PublicCollectionName]
	if len(publicIDs) != 1 || publicIDs[0] != "pub-1" {
		t.Errorf("public group = %v, want [pub-1]", publicIDs)
	}

	// Enterprise gets its own collection.
	entCol := EnterpriseCollectionName(newKB("ent-1xxxxx", types.KnowledgeBaseCategoryEnterprise))
	if ids := groups[entCol]; len(ids) != 1 || ids[0] != "ent-1xxxxx" {
		t.Errorf("enterprise group for %q = %v, want [ent-1xxxxx]", entCol, ids)
	}
}

func TestGroupByCollection_PropagatesError(t *testing.T) {
	r := NewCollectionResolver()
	_, err := r.GroupByCollection([]*types.KnowledgeBase{
		newKB("p1", types.KnowledgeBaseCategoryPersonal),
		newKB("oops", "weird"),
	})
	if err == nil {
		t.Fatal("expected error from group with unknown category")
	}
}

func TestSanitizeCollectionSuffix(t *testing.T) {
	cases := map[string]string{
		"":             "default",
		"abc":          "abc",
		"a-b":          "a_b",
		"a.b/c":        "a_b_c",
		"-only-dashes": "_only_dashes",
	}
	for in, want := range cases {
		if got := sanitizeCollectionSuffix(in); got != want {
			t.Errorf("sanitizeCollectionSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

// helpers

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
