package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"icecoreverdict/internal/application"
	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

type createRequest struct {
	RequestID             string    `json:"request_id"`
	CaseID                string    `json:"case_id"`
	Title                 string    `json:"title"`
	TransferBatch         string    `json:"transfer_batch"`
	IncidentSummary       string    `json:"incident_summary"`
	LeadActorID           string    `json:"lead_actor_id"`
	CreatedBy             string    `json:"created_by"`
	CreatedAt             time.Time `json:"created_at"`
	AllowedResearchScopes []string  `json:"allowed_research_scopes,omitempty"`
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok", "service": "IceCoreVerdict"})
}
func (s *Server) CreateCase(w http.ResponseWriter, r *http.Request) {
	var in createRequest
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := application.ValidateRequestID(in.RequestID); err != nil {
		writeError(w, err)
		return
	}
	cmd := domain.CreateCase{CaseID: in.CaseID, Title: in.Title, TransferBatch: in.TransferBatch, IncidentSummary: in.IncidentSummary, LeadActorID: in.LeadActorID, CreatedBy: in.CreatedBy, CreatedAt: in.CreatedAt, AllowedResearchScopes: in.AllowedResearchScopes}
	result, err := s.app.Create(r.Context(), in.RequestID, cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, result)
}
func (s *Server) SubmitCommand(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("case_id")
	var cmd application.Command
	if err := readJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if err := application.ValidateRequestID(cmd.RequestID); err != nil {
		writeError(w, err)
		return
	}
	if err := application.ValidateActor(cmd.ActorID); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.Execute(r.Context(), caseID, cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) GetCase(w http.ResponseWriter, r *http.Request) {
	view, err := s.app.Get(r.PathValue("case_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, view)
}
func (s *Server) GetEvents(w http.ResponseWriter, r *http.Request) {
	offset, err := parseInt(r, "offset", 0)
	if err != nil {
		writeError(w, err)
		return
	}
	limit, err := parseInt(r, "limit", 50)
	if err != nil {
		writeError(w, err)
		return
	}
	if limit < 1 || limit > 100 {
		writeError(w, &domain.RuleError{Code: domain.CodeValidation, Field: "limit", Message: "必须为 1 到 100"})
		return
	}
	fromRevision, err := parseUint(r, "from_revision")
	if err != nil {
		writeError(w, err)
		return
	}
	toRevision, err := parseUint(r, "to_revision")
	if err != nil {
		writeError(w, err)
		return
	}
	if toRevision > 0 && fromRevision > toRevision {
		writeError(w, &domain.RuleError{Code: domain.CodeValidation, Field: "revision_range", Message: "起始修订号不得大于结束修订号"})
		return
	}
	fromTime, err := parseTime(r, "occurred_from")
	if err != nil {
		writeError(w, err)
		return
	}
	toTime, err := parseTime(r, "occurred_to")
	if err != nil {
		writeError(w, err)
		return
	}
	if !fromTime.IsZero() && !toTime.IsZero() && fromTime.After(toTime) {
		writeError(w, &domain.RuleError{Code: domain.CodeValidation, Field: "occurred_range", Message: "起始时间不得晚于结束时间"})
		return
	}
	eventType := r.URL.Query().Get("event_type")
	if eventType != "" && !domain.IsKnownEventType(eventType) {
		writeError(w, &domain.RuleError{Code: domain.CodeValidation, Field: "event_type", Message: "未知事件类型"})
		return
	}
	page, err := s.app.AuditEvents(r.PathValue("case_id"), storage.EventFilter{ActorID: r.URL.Query().Get("actor_id"), EventType: eventType, FromRevision: fromRevision, ToRevision: toRevision, OccurredFrom: fromTime, OccurredTo: toTime, Limit: limit, Cursor: r.URL.Query().Get("cursor"), Offset: offset})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

func parseUint(r *http.Request, key string) (uint64, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || n == 0 {
		return 0, &domain.RuleError{Code: domain.CodeValidation, Field: key, Message: "必须为正整数"}
	}
	return n, nil
}
func parseTime(r *http.Request, key string) (time.Time, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return time.Time{}, nil
	}
	v, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, &domain.RuleError{Code: domain.CodeValidation, Field: key, Message: "必须为 RFC3339 时间"}
	}
	return v.UTC(), nil
}
func (s *Server) DownloadArchive(w http.ResponseWriter, r *http.Request) {
	doc, data, err := s.app.Archive(r.PathValue("case_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", "\""+doc.ContentDigest+"\"")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (s *Server) VerifyArchive(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.VerifyArchive(r.PathValue("case_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	status := 200
	if !result.Valid {
		status = 409
	}
	writeJSON(w, status, result)
}
func parseInt(r *http.Request, key string, def int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return def, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, &domain.RuleError{Code: domain.CodeValidation, Field: key, Message: "必须为非负整数"}
	}
	return n, nil
}

var _ = json.RawMessage{}
