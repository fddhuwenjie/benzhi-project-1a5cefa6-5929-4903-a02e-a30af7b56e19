package web

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/application"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var assets embed.FS

type Handler struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Handler {
	h := &Handler{app: app, mux: http.NewServeMux()}
	h.routes()
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }
func (h *Handler) routes() {
	h.mux.HandleFunc("GET "+RouteWorkbench, h.Workbench)
	sub, _ := fs.Sub(assets, "assets")
	h.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(sub))))
	h.mux.HandleFunc("POST /api/cases", h.CreateCase)
	h.mux.HandleFunc("GET /api/cases/{id}", h.GetCase)
	h.mux.HandleFunc("POST /api/cases/{id}/protocol/precheck", h.PrecheckProtocol)
	h.mux.HandleFunc("POST /api/cases/{id}/freeze", h.FreezeProtocol)
	h.mux.HandleFunc("POST /api/cases/{id}/runs", h.SubmitRun)
	h.mux.HandleFunc("POST /api/cases/{id}/deviations", h.CorrectDeviations)
	h.mux.HandleFunc("POST /api/cases/{id}/review", h.ReviewCase)
	h.mux.HandleFunc("GET /api/cases/{id}/review-readiness", h.ReviewReadiness)
	h.mux.HandleFunc("GET /api/cases/{id}/requests/{request_id}", h.GetReceipt)
	h.mux.HandleFunc("GET /api/cases/{id}/verify", h.VerifyCertificate)
	h.mux.HandleFunc("GET /api/cases/{id}/audit", h.GetAuditTimeline)
}
func (h *Handler) Workbench(w http.ResponseWriter, r *http.Request) {
	b, err := assets.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "页面资源不可用", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeError(w, 400, "invalid_request", "请求格式无效: "+err.Error(), 0)
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, msg string, revision int64) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "current_revision": revision}})
}
func appError(w http.ResponseWriter, err error) {
	var conflict *application.ConflictError
	var missing *application.NotFoundError
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &conflict):
		writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "conflict", "message": err.Error(), "case_id": conflict.CaseID, "current_revision": conflict.CurrentRevision, "current_status": conflict.CurrentStatus, "receipt_summary": conflict.ReceiptSummary}})
	case errors.As(err, &missing):
		writeError(w, 404, "not_found", err.Error(), 0)
	case errors.As(err, &validation):
		writeJSON(w, 422, map[string]any{"error": map[string]any{"code": "validation_failed", "message": err.Error(), "issues": validation.Issues}})
	default:
		writeError(w, 422, "business_rule", err.Error(), 0)
	}
}

func (h *Handler) PrecheckProtocol(w http.ResponseWriter, r *http.Request) {
	var c application.PrecheckProtocolCommand
	if !decode(w, r, &c) {
		return
	}
	out, err := h.app.PrecheckProtocol(r.PathValue("id"), c)
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) CreateCase(w http.ResponseWriter, r *http.Request) {
	var c application.CreateCaseCommand
	if !decode(w, r, &c) {
		return
	}
	out, err := h.app.Create(c)
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 201, out)
}
func (h *Handler) GetCase(w http.ResponseWriter, r *http.Request) {
	c, err := h.app.Get(r.PathValue("id"))
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, ProjectCase(c))
}
func (h *Handler) FreezeProtocol(w http.ResponseWriter, r *http.Request) {
	var c application.FreezeProtocolCommand
	if !decode(w, r, &c) {
		return
	}
	out, err := h.app.Freeze(r.PathValue("id"), c)
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) SubmitRun(w http.ResponseWriter, r *http.Request) {
	var c application.SubmitRunCommand
	if !decode(w, r, &c) {
		return
	}
	out, err := h.app.SubmitRun(r.PathValue("id"), c)
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) CorrectDeviations(w http.ResponseWriter, r *http.Request) {
	var c application.CorrectCommand
	if !decode(w, r, &c) {
		return
	}
	out, err := h.app.Correct(r.PathValue("id"), c)
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) ReviewCase(w http.ResponseWriter, r *http.Request) {
	var c application.ReviewCommand
	if !decode(w, r, &c) {
		return
	}
	out, err := h.app.Review(r.PathValue("id"), c)
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) ReviewReadiness(w http.ResponseWriter, r *http.Request) {
	out, err := h.app.ReviewReadiness(r.PathValue("id"), r.URL.Query().Get("reviewer_id"))
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) GetReceipt(w http.ResponseWriter, r *http.Request) {
	out, err := h.app.Receipt(r.PathValue("id"), r.PathValue("request_id"))
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) VerifyCertificate(w http.ResponseWriter, r *http.Request) {
	v, err := h.app.Verify(r.PathValue("id"))
	if err != nil {
		appError(w, err)
		return
	}
	status := 200
	if !v.Valid {
		status = 409
	}
	writeJSON(w, status, v)
}
func (h *Handler) GetAuditTimeline(w http.ResponseWriter, r *http.Request) {
	timeline, err := h.app.AuditTimeline(r.PathValue("id"))
	if err != nil {
		appError(w, err)
		return
	}
	writeJSON(w, 200, timeline)
}
func HealthText(addr string) string {
	return fmt.Sprintf("排烟联动演练资格工作台监听 %s", addr)
}
