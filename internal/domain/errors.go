package domain

import "fmt"

type ErrorCode string

const (
	CodeValidation    ErrorCode = "validation_error"
	CodeStateConflict ErrorCode = "state_conflict"
	CodeDutyConflict  ErrorCode = "duty_conflict"
	CodeNotFound      ErrorCode = "not_found"
	CodeArchived      ErrorCode = "case_archived"
	CodeCorrupt       ErrorCode = "data_corrupt"
)

type RuleError struct {
	Code    ErrorCode
	Field   string
	Message string
}

func (e *RuleError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func invalid(field, message string) error {
	return &RuleError{Code: CodeValidation, Field: field, Message: message}
}
func conflict(message string) error { return &RuleError{Code: CodeStateConflict, Message: message} }

func ErrorDetails(err error) (ErrorCode, string, string) {
	if e, ok := err.(*RuleError); ok {
		return e.Code, e.Field, e.Message
	}
	return "internal_error", "", "内部错误"
}
