package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
	"mooc-manus/internal/domains/models/tracing"
)

type stubSvc struct {
	detail *dtos.TraceDetailDTO
	err    error
}

func (s *stubSvc) GetTraceDetail(context.Context, string) (*dtos.TraceDetailDTO, error) {
	return s.detail, s.err
}

func (s *stubSvc) ListTraces(context.Context, dtos.TraceListRequest) (*dtos.TraceListDTO, error) {
	return &dtos.TraceListDTO{Total: 0, Traces: nil}, s.err
}

func setup(svc services.TraceApplicationService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTraceHandler(svc)
	r.GET("/api/trace/:trace_id", h.GetDetail)
	r.GET("/api/traces", h.List)
	return r
}

func TestTraceHandler_GetDetail_200(t *testing.T) {
	svc := &stubSvc{detail: &dtos.TraceDetailDTO{TraceID: "t1", Root: &tracing.SpanNode{SpanID: 0, ParentSpanID: -1}}}
	r := setup(svc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/trace/t1", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	var body dtos.TraceDetailDTO
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "t1", body.TraceID)
}

func TestTraceHandler_GetDetail_404(t *testing.T) {
	svc := &stubSvc{err: services.ErrTraceNotFound}
	r := setup(svc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/trace/nope", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 404, w.Code)
}

func TestTraceHandler_List_200(t *testing.T) {
	svc := &stubSvc{}
	r := setup(svc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/traces?page=1&page_size=20", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}
