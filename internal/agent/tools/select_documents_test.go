package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fakeKBSvc satisfies interfaces.KnowledgeBaseService by embedding a nil
// interface value. The SelectDocumentsTool only calls SearchKnowledgeSummaries,
// so the remaining methods are never invoked; any accidental call will panic
// with a nil pointer dereference, which is acceptable for a unit test.
type fakeKBSvc struct {
	interfaces.KnowledgeBaseService

	// Configurable behaviour.
	hits []*types.SummaryHit
	err  error

	// Captured arguments from the most recent call.
	gotReferenceKB string
	gotQuery       string
	gotTopK        int
	gotFilter      types.SummaryFilter
	callCount      int
}

func (f *fakeKBSvc) SearchKnowledgeSummaries(
	_ context.Context,
	referenceKBID string,
	query string,
	topK int,
	filter types.SummaryFilter,
) ([]*types.SummaryHit, error) {
	f.callCount++
	f.gotReferenceKB = referenceKBID
	f.gotQuery = query
	f.gotTopK = topK
	f.gotFilter = filter
	return f.hits, f.err
}

func newTestTool(svc *fakeKBSvc, targets types.SearchTargets) *SelectDocumentsTool {
	return NewSelectDocumentsTool(svc, targets, types.SummaryFilter{})
}

// newTestToolWithDefaults 构造带上下文预选 filter 的工具，仅在验证预选回落时使用。
func newTestToolWithDefaults(
	svc *fakeKBSvc,
	targets types.SearchTargets,
	defaults types.SummaryFilter,
) *SelectDocumentsTool {
	return NewSelectDocumentsTool(svc, targets, defaults)
}

func defaultTargets() types.SearchTargets {
	return types.SearchTargets{
		{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-1"},
		{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-2"},
	}
}

func TestSelectDocuments_EmptyQuery(t *testing.T) {
	svc := &fakeKBSvc{}
	tool := newTestTool(svc, defaultTargets())

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"   "}`))
	if err == nil {
		t.Fatalf("expected error for empty query, got nil")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed ToolResult, got %+v", result)
	}
	if !strings.Contains(result.Error, "query 参数必填") {
		t.Fatalf("unexpected error message: %q", result.Error)
	}
	if svc.callCount != 0 {
		t.Fatalf("SearchKnowledgeSummaries should not be invoked, got %d calls", svc.callCount)
	}
}

func TestSelectDocuments_InvalidJSON(t *testing.T) {
	svc := &fakeKBSvc{}
	tool := newTestTool(svc, defaultTargets())

	result, err := tool.Execute(context.Background(), json.RawMessage(`{not-json`))
	if err == nil {
		t.Fatalf("expected unmarshal error, got nil")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed ToolResult, got %+v", result)
	}
}

func TestSelectDocuments_NoSearchTargets(t *testing.T) {
	svc := &fakeKBSvc{}
	tool := newTestTool(svc, types.SearchTargets{})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"hello"}`))
	if err == nil {
		t.Fatalf("expected error for no targets, got nil")
	}
	if result.Success {
		t.Fatalf("expected failed result, got %+v", result)
	}
}

func TestSelectDocuments_KBIDsFilteredToEmpty(t *testing.T) {
	svc := &fakeKBSvc{}
	tool := newTestTool(svc, defaultTargets())

	// Requested KB ID is not in the available search targets -> filtered scope is empty.
	args := json.RawMessage(`{"query":"hello","knowledge_base_ids":["kb-not-exist"]}`)
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed ToolResult, got %+v", result)
	}
	if svc.callCount != 0 {
		t.Fatalf("expected service not called, got %d", svc.callCount)
	}
}

func TestSelectDocuments_Success(t *testing.T) {
	svc := &fakeKBSvc{
		hits: []*types.SummaryHit{
			// Unsorted on purpose to exercise the tool's sort step.
			{ID: "s-1", KnowledgeID: "k-1", KnowledgeBaseID: "kb-1", FileName: "doc1", Content: "first doc", Score: 0.42},
			{ID: "s-2", KnowledgeID: "k-2", KnowledgeBaseID: "kb-1", FileName: "doc2", Content: "second doc", Score: 0.91, TagIDs: []string{"tag-product", "tag-year"}},
			{ID: "s-3", KnowledgeID: "k-3", KnowledgeBaseID: "kb-2", FileName: "doc3", Content: "third doc", Score: 0.77},
		},
	}
	tool := newTestTool(svc, defaultTargets())

	args := json.RawMessage(`{
		"query": "what is pension?",
		"tag_ids": ["tag-product"],
		"top_k": 2
	}`)
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	// referenceKBID should be the first KB in the effective search scope.
	if svc.gotReferenceKB != "kb-1" {
		t.Fatalf("reference KB = %q, want %q", svc.gotReferenceKB, "kb-1")
	}
	if svc.gotQuery != "what is pension?" {
		t.Fatalf("query = %q", svc.gotQuery)
	}
	if svc.gotTopK != 2 {
		t.Fatalf("topK = %d, want 2", svc.gotTopK)
	}
	if len(svc.gotFilter.KnowledgeBaseIDs) != 2 {
		t.Fatalf("filter KBs = %v, want 2 entries", svc.gotFilter.KnowledgeBaseIDs)
	}
	if len(svc.gotFilter.TagIDs) != 1 || svc.gotFilter.TagIDs[0] != "tag-product" {
		t.Fatalf("filter TagIDs = %v", svc.gotFilter.TagIDs)
	}

	// Data payload validation.
	data := result.Data
	if data == nil {
		t.Fatalf("expected Data payload, got nil")
	}
	if count, _ := data["count"].(int); count != 3 {
		t.Fatalf("count = %v, want 3", data["count"])
	}
	ids, _ := data["knowledge_ids"].([]string)
	if len(ids) != 3 {
		t.Fatalf("knowledge_ids = %v", ids)
	}
	// The tool sorts by descending score; first id must correspond to the highest score hit.
	if ids[0] != "k-2" {
		t.Fatalf("top-ranked id = %q, want %q", ids[0], "k-2")
	}
	if display, _ := data["display_type"].(string); display != "document_shortlist" {
		t.Fatalf("display_type = %q", display)
	}
}

func TestSelectDocuments_NoHits(t *testing.T) {
	svc := &fakeKBSvc{hits: nil}
	tool := newTestTool(svc, defaultTargets())

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"empty"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success with empty hits, got %+v", result)
	}
	if count, _ := result.Data["count"].(int); count != 0 {
		t.Fatalf("count = %v, want 0", result.Data["count"])
	}
	if !strings.Contains(result.Output, "未找到与查询") {
		t.Fatalf("output missing empty-state marker: %q", result.Output)
	}
}

func TestSelectDocuments_TopKClamp(t *testing.T) {
	svc := &fakeKBSvc{}
	tool := newTestTool(svc, defaultTargets())

	// top_k beyond 20 should be clamped to 20 before calling the service.
	_, _ = tool.Execute(context.Background(), json.RawMessage(`{"query":"x","top_k":999}`))
	if svc.gotTopK != 20 {
		t.Fatalf("topK upper clamp = %d, want 20", svc.gotTopK)
	}

	// top_k <= 0 should default to 5.
	svc2 := &fakeKBSvc{}
	tool2 := newTestTool(svc2, defaultTargets())
	_, _ = tool2.Execute(context.Background(), json.RawMessage(`{"query":"x","top_k":0}`))
	if svc2.gotTopK != 5 {
		t.Fatalf("topK default = %d, want 5", svc2.gotTopK)
	}
}

func TestSelectDocuments_ServiceError(t *testing.T) {
	svc := &fakeKBSvc{err: errors.New("milvus down")}
	tool := newTestTool(svc, defaultTargets())

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"x"}`))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed result, got %+v", result)
	}
	if !strings.Contains(result.Error, "摘要检索失败") {
		t.Fatalf("unexpected error message: %q", result.Error)
	}
}

// TestSelectDocuments_ScalarFilterPassThrough 验证文件名关键词能按原样透传到 SummaryFilter。
func TestSelectDocuments_ScalarFilterPassThrough(t *testing.T) {
	svc := &fakeKBSvc{}
	tool := newTestTool(svc, defaultTargets())

	args := json.RawMessage(`{
		"query": "报告",
		"filename_keywords": ["Q3", "财报"]
	}`)
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := svc.gotFilter
	if len(got.FileNameKeywords) != 2 || got.FileNameKeywords[0] != "Q3" || got.FileNameKeywords[1] != "财报" {
		t.Fatalf("FileNameKeywords = %v", got.FileNameKeywords)
	}
}

// TestSelectDocuments_DefaultFilterFallback 验证入参缺省时能回落到 defaultFilter，
// 以及入参优先级高于 defaultFilter。
func TestSelectDocuments_DefaultFilterFallback(t *testing.T) {
	defaults := types.SummaryFilter{
		TagIDs: []string{"tag-default"},
	}

	// Case A：入参没传过滤→回落到 defaultFilter。
	svcA := &fakeKBSvc{}
	toolA := newTestToolWithDefaults(svcA, defaultTargets(), defaults)
	if _, err := toolA.Execute(context.Background(), json.RawMessage(`{"query":"报告"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svcA.gotFilter.TagIDs) != 1 || svcA.gotFilter.TagIDs[0] != "tag-default" {
		t.Fatalf("fallback TagIDs = %v", svcA.gotFilter.TagIDs)
	}

	// Case B：入参明确指定→覆盖 defaultFilter。
	svcB := &fakeKBSvc{}
	toolB := newTestToolWithDefaults(svcB, defaultTargets(), defaults)
	args := json.RawMessage(`{"query":"报告","tag_ids":["tag-override"]}`)
	if _, err := toolB.Execute(context.Background(), args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svcB.gotFilter.TagIDs[0] != "tag-override" {
		t.Fatalf("override TagIDs = %v", svcB.gotFilter.TagIDs)
	}
}
