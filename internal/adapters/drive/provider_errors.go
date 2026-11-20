package drive

import (
	"encoding/json"
	"io"
	"strings"
)

const maxErrorBody = 32 << 10

func isQuotaError(body io.Reader) bool {
	data, err := io.ReadAll(io.LimitReader(body, maxErrorBody+1))
	if err != nil || len(data) > maxErrorBody {
		return false
	}
	var envelope struct {
		Error struct {
			Status string `json:"status"`
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return false
	}
	if envelope.Error.Status == "RESOURCE_EXHAUSTED" {
		return true
	}
	for _, entry := range envelope.Error.Errors {
		if quotaReason(entry.Reason) {
			return true
		}
	}
	for _, entry := range envelope.Error.Details {
		if quotaReason(entry.Reason) {
			return true
		}
	}
	return false
}

func quotaReason(reason string) bool {
	switch strings.ToLower(strings.ReplaceAll(reason, "_", "")) {
	case "ratelimitexceeded", "userratelimitexceeded", "dailylimitexceeded",
		"dailylimitexceededunreg", "quotaexceeded", "downloadquotaexceeded",
		"storagequotaexceeded", "sharingratelimitexceeded", "resourceexhausted":
		return true
	default:
		return false
	}
}
