package tracing

import "errors"

var (
	ErrEmptyTrace    = errors.New("empty trace")
	ErrNoRoot        = errors.New("no root span")
	ErrMultipleRoots = errors.New("multiple root spans")
)

// SpanNode 专用于 GET /api/trace/:trace_id 返回的树状结构
type SpanNode struct {
	SpanID        int32                  `json:"span_id"`
	ParentSpanID  int32                  `json:"parent_span_id"`
	SpanType      string                 `json:"span_type"`
	OperationName string                 `json:"operation_name"`
	StartTime     int64                  `json:"start_time"`
	EndTime       int64                  `json:"end_time"`
	LatencyMs     int32                  `json:"latency_ms"`
	IsError       bool                   `json:"is_error"`
	Tags          map[string]interface{} `json:"tags"`
	Logs          []LogEntry             `json:"logs"`
	Children      []*SpanNode            `json:"children"`
}

// BuildSpanTree 把扁平数组还原成树
// 前置：nodes 已按 span_id ASC 排序
func BuildSpanTree(nodes []*SpanNode) (*SpanNode, error) {
	if len(nodes) == 0 {
		return nil, ErrEmptyTrace
	}
	idx := make(map[int32]*SpanNode, len(nodes))
	for _, n := range nodes {
		n.Children = make([]*SpanNode, 0)
		idx[n.SpanID] = n
	}
	var (
		root    *SpanNode
		orphans []*SpanNode
	)
	for _, n := range nodes {
		if n.ParentSpanID == -1 {
			if root != nil {
				return nil, ErrMultipleRoots
			}
			root = n
			continue
		}
		parent, ok := idx[n.ParentSpanID]
		if !ok {
			if n.Tags == nil {
				n.Tags = make(map[string]interface{})
			}
			n.Tags["_orphan"] = true
			n.Tags["_original_parent"] = n.ParentSpanID
			orphans = append(orphans, n)
			continue
		}
		parent.Children = append(parent.Children, n)
	}
	if root == nil {
		return nil, ErrNoRoot
	}
	for _, o := range orphans {
		root.Children = append(root.Children, o)
	}
	return root, nil
}
