package web

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/application"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkbenchAndCreateRoute(t *testing.T) {
	repo, _ := store.New(t.TempDir())
	h := New(application.New(repo))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<body>") {
		t.Fatalf("工作台响应无效: %d", rec.Code)
	}
	body := `{"request_id":"r","expected_revision":0,"case_id":"C","building_name":"楼宇","created_by":"u"}`
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cases", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("建档接口失败: %d %s", rec.Code, rec.Body.String())
	}
}
