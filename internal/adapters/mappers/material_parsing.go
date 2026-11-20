package mappers

import (
	"encoding/json"

	"github.com/alex/assistent/internal/domain/submission"
)

func EncodeMaterialParsing(parsed *submission.ParseResult) (*string, error) {
	if parsed == nil {
		return nil, nil
	}
	data, err := json.Marshal(parsed)
	if err != nil {
		return nil, err
	}
	value := string(data)
	return &value, nil
}

func DecodeMaterialParsing(data *string) (*submission.ParseResult, error) {
	if data == nil {
		return nil, nil
	}
	var parsed submission.ParseResult
	if err := json.Unmarshal([]byte(*data), &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func EncodeMaterialParse(outcome submission.MaterialParse) (string, error) {
	data, err := json.Marshal(outcome)
	return string(data), err
}

func DecodeMaterialParse(data string) (submission.MaterialParse, error) {
	var outcome submission.MaterialParse
	err := json.Unmarshal([]byte(data), &outcome)
	return outcome, err
}
