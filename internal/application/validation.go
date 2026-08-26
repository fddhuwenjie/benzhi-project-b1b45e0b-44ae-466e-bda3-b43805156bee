package application

import (
	"strings"

	"icecoreverdict/internal/domain"
)

func ValidateRequestID(value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 8 || len(value) > 128 {
		return &domain.RuleError{Code: domain.CodeValidation, Field: "request_id", Message: "长度必须为 8 到 128 个字符"}
	}
	for _, r := range value {
		if r < 33 || r > 126 {
			return &domain.RuleError{Code: domain.CodeValidation, Field: "request_id", Message: "仅允许可打印 ASCII 字符"}
		}
	}
	return nil
}
func ValidateActor(value string) error {
	if strings.TrimSpace(value) == "" {
		return &domain.RuleError{Code: domain.CodeValidation, Field: "actor_id", Message: "不能为空"}
	}
	return nil
}
