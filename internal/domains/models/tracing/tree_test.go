package tracing

import (
	"errors"
	"testing"
)

// makeNode 构造一个 SpanNode 便于测试
func makeNode(spanID, parentSpanID int32) *SpanNode {
	return &SpanNode{
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
	}
}

func TestBuildSpanTree_HappyPath(t *testing.T) {
	// 构造 1 (root) -> 2, 3；2 -> 4 的树
	nodes := []*SpanNode{
		makeNode(1, -1),
		makeNode(2, 1),
		makeNode(3, 1),
		makeNode(4, 2),
	}
	root, err := BuildSpanTree(nodes)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if root == nil {
		t.Fatalf("root is nil")
	}
	if root.SpanID != 1 {
		t.Fatalf("expected root SpanID=1, got %d", root.SpanID)
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected root have 2 children, got %d", len(root.Children))
	}
	// 找到 span 2 并验证其子节点
	var span2 *SpanNode
	for _, c := range root.Children {
		if c.SpanID == 2 {
			span2 = c
			break
		}
	}
	if span2 == nil {
		t.Fatalf("span 2 not found under root")
	}
	if len(span2.Children) != 1 {
		t.Fatalf("expected span2 have 1 child, got %d", len(span2.Children))
	}
	if span2.Children[0].SpanID != 4 {
		t.Fatalf("expected span2.Children[0].SpanID=4, got %d", span2.Children[0].SpanID)
	}
}

func TestBuildSpanTree_EmptyInput(t *testing.T) {
	root, err := BuildSpanTree(nil)
	if !errors.Is(err, ErrEmptyTrace) {
		t.Fatalf("expected ErrEmptyTrace, got %v", err)
	}
	if root != nil {
		t.Fatalf("expected nil root, got %+v", root)
	}
	root, err = BuildSpanTree([]*SpanNode{})
	if !errors.Is(err, ErrEmptyTrace) {
		t.Fatalf("expected ErrEmptyTrace on empty slice, got %v", err)
	}
	if root != nil {
		t.Fatalf("expected nil root on empty slice, got %+v", root)
	}
}

func TestBuildSpanTree_NoRoot(t *testing.T) {
	// 全部节点都有父节点、无 ParentSpanID=-1
	nodes := []*SpanNode{
		makeNode(2, 1),
		makeNode(3, 2),
	}
	root, err := BuildSpanTree(nodes)
	if !errors.Is(err, ErrNoRoot) {
		t.Fatalf("expected ErrNoRoot, got %v", err)
	}
	if root != nil {
		t.Fatalf("expected nil root, got %+v", root)
	}
}

func TestBuildSpanTree_MultipleRoots(t *testing.T) {
	nodes := []*SpanNode{
		makeNode(1, -1),
		makeNode(2, -1),
	}
	root, err := BuildSpanTree(nodes)
	if !errors.Is(err, ErrMultipleRoots) {
		t.Fatalf("expected ErrMultipleRoots, got %v", err)
	}
	if root != nil {
		t.Fatalf("expected nil root, got %+v", root)
	}
}

func TestBuildSpanTree_OrphanNode(t *testing.T) {
	// span 99 的 parent_span_id=42 不在集合中，应作为 orphan 挂到 root
	nodes := []*SpanNode{
		makeNode(1, -1),
		makeNode(2, 1),
		makeNode(99, 42),
	}
	root, err := BuildSpanTree(nodes)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if root == nil {
		t.Fatalf("root is nil")
	}
	if root.SpanID != 1 {
		t.Fatalf("expected root SpanID=1, got %d", root.SpanID)
	}
	// root 下面应有 span 2 + orphan span 99 共 2 个子节点
	if len(root.Children) != 2 {
		t.Fatalf("expected root have 2 children (span 2 + orphan 99), got %d", len(root.Children))
	}
	var orphan *SpanNode
	for _, c := range root.Children {
		if c.SpanID == 99 {
			orphan = c
			break
		}
	}
	if orphan == nil {
		t.Fatalf("orphan span 99 not attached to root")
	}
	if orphan.Tags == nil {
		t.Fatalf("orphan.Tags is nil")
	}
	if v, ok := orphan.Tags["_orphan"].(bool); !ok || !v {
		t.Fatalf("expected orphan._orphan=true, got %v", orphan.Tags["_orphan"])
	}
	if v, ok := orphan.Tags["_original_parent"].(int32); !ok || v != 42 {
		t.Fatalf("expected orphan._original_parent=42, got %v", orphan.Tags["_original_parent"])
	}
}
