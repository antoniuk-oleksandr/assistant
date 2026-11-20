package classroom

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidCourseReference = errors.New("InvalidClassroomCourseReference")

type Status string

const (
	Active   Status = "active"
	Disabled Status = "disabled"
)

type Course struct {
	ID              int64
	ExternalID      string
	SourceReference string
	Status          Status
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NormalizeReference(reference string) (string, error) {
	original := reference
	reference = strings.TrimSpace(reference)
	if parsed, err := url.Parse(reference); err == nil && parsed.IsAbs() {
		if parsed.Scheme != "https" || parsed.Host != "classroom.google.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(reference, "#") {
			return "", fmt.Errorf("%w: %q", ErrInvalidCourseReference, original)
		}
		segments := strings.Split(parsed.Path, "/")
		if len(segments) != 3 || segments[1] != "c" || segments[2] == "" {
			return "", fmt.Errorf("%w: %q", ErrInvalidCourseReference, original)
		}
		reference = segments[2]
	}
	if reference == original && strings.ContainsAny(reference, "/?#") {
		return "", fmt.Errorf("%w: %q", ErrInvalidCourseReference, original)
	}
	if numeric, err := strconv.ParseUint(reference, 10, 64); err == nil {
		return strconv.FormatUint(numeric, 10), nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(reference)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidCourseReference, original)
	}
	numeric, err := strconv.ParseUint(string(decoded), 10, 64)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidCourseReference, original)
	}
	return strconv.FormatUint(numeric, 10), nil
}
