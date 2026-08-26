package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

const maxBody = 1 << 20

type errorResponse struct {
	Error struct {
		Code    domain.ErrorCode `json:"code"`
		Field   string           `json:"field,omitempty"`
		Message string           `json:"message"`
	} `json:"error"`
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return &domain.RuleError{Code: domain.CodeValidation, Field: "Content-Type", Message: "必须为 application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return &domain.RuleError{Code: domain.CodeValidation, Field: "body", Message: "请求 JSON 无效"}
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return &domain.RuleError{Code: domain.CodeValidation, Field: "body", Message: "请求体只能包含一个 JSON 对象"}
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	code, field, message := domain.ErrorDetails(err)
	status := http.StatusInternalServerError
	if errors.Is(err, storage.ErrNotFound) {
		code = domain.CodeNotFound
		message = "案件或档案不存在"
		status = 404
	} else {
		switch code {
		case domain.CodeValidation:
			status = 400
		case domain.CodeNotFound:
			status = 404
		case domain.CodeStateConflict, domain.CodeDutyConflict, domain.CodeArchived:
			status = 409
		case domain.CodeCorrupt:
			status = 503
		}
	}
	var out errorResponse
	out.Error.Code = code
	out.Error.Field = field
	out.Error.Message = message
	writeJSON(w, status, out)
}
