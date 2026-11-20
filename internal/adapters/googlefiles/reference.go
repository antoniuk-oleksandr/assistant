// Package googlefiles recognizes Google file references without following them.
package googlefiles

import (
	"net/url"
	"regexp"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func ValidID(id string) bool { return identifier.MatchString(id) }

type Reference struct {
	ID          string
	ResourceKey string
}

func ParseURL(raw string) Reference {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return Reference{}
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Reference{}
	}
	var id string
	switch u.Host {
	case "drive.google.com":
		if u.Path == "/open" || u.Path == "/uc" {
			id = singleValue(query, "id")
		} else {
			id = pathID(u.Path, "file")
		}
	case "docs.google.com":
		id = pathID(u.Path, "document", "spreadsheets")
	}
	if !ValidID(id) {
		return Reference{}
	}
	key := singleValue(query, "resourcekey")
	if !ValidID(key) {
		key = ""
	}
	return Reference{ID: id, ResourceKey: key}
}

func singleValue(query url.Values, name string) string {
	if values := query[name]; len(values) == 1 {
		return values[0]
	}
	return ""
}

func pathID(path string, kinds ...string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, kind := range kinds {
		if parts[0] != kind {
			continue
		}
		tail := parts[1:]
		if len(tail) >= 2 && tail[0] == "u" {
			tail = tail[2:]
		}
		// /d/e/ is a published view, not a Drive file reference.
		if len(tail) >= 2 && tail[0] == "d" && tail[1] != "e" {
			return tail[1]
		}
	}
	return ""
}
