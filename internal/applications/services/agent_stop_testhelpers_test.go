package services

import (
	"bytes"
	"net/http"
	"sync"
)

// fakeSSEWriter 同时实现 http.ResponseWriter 与 http.Flusher，供 sse.StartChat 使用
// 与 sse 包内测试文件里的 fakeResponseWriter 结构对齐，独立一份避免跨包 export
type fakeSSEWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	header http.Header
	status int
}

func newFakeSSEWriter() *fakeSSEWriter {
	return &fakeSSEWriter{header: make(http.Header)}
}

func (w *fakeSSEWriter) Header() http.Header { return w.header }

func (w *fakeSSEWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *fakeSSEWriter) WriteHeader(status int) { w.status = status }

func (w *fakeSSEWriter) Flush() {}
