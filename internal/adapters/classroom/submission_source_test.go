package classroom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

func TestHTTPClassroomClientListsCourseWorkAcrossPagesWithAuth(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI()+" "+r.Header.Get("Authorization"))
		if r.URL.EscapedPath() != "/v1/courses/course%2F1/courseWork" {
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "next" {
			_ = json.NewEncoder(w).Encode(map[string]any{"courseWork": []CourseWork{{ID: "task-2", Title: "Second"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"courseWork": []CourseWork{{ID: "task-1", Title: "First"}}, "nextPageToken": "next"})
	}))
	defer server.Close()

	client := NewHTTPClassroomClient(HTTPConfig{BaseURL: server.URL + "/v1", AccessToken: "token"}, server.Client())
	items, err := client.ListCourseWork(context.Background(), "course/1")
	if err != nil {
		t.Fatalf("ListCourseWork() error = %v", err)
	}
	if got := []string{items[0].ID, items[1].ID}; !reflect.DeepEqual(got, []string{"task-1", "task-2"}) {
		t.Fatalf("coursework IDs = %v", got)
	}
	if len(requests) != 2 || !strings.Contains(requests[0], "pageSize=100") || !strings.Contains(requests[0], "Bearer token") || !strings.Contains(requests[1], "pageToken=next") {
		t.Fatalf("requests = %v", requests)
	}
}

func TestHTTPClassroomClientListsSubmissionsAcrossPages(t *testing.T) {
	var tokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens = append(tokens, r.URL.Query().Get("pageToken"))
		if r.URL.Path != "/courses/course-1/courseWork/task-1/studentSubmissions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if len(tokens) == 1 {
			_, _ = w.Write([]byte(`{"studentSubmissions":[{"id":"submission-1"}],"nextPageToken":"page-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"studentSubmissions":[{"id":"submission-2"}]}`))
	}))
	defer server.Close()

	client := NewHTTPClassroomClient(HTTPConfig{BaseURL: server.URL}, server.Client())
	items, err := client.ListStudentSubmissions(context.Background(), "course-1", "task-1")
	if err != nil {
		t.Fatalf("ListStudentSubmissions() error = %v", err)
	}
	if len(items) != 2 || items[0].ID != "submission-1" || items[1].ID != "submission-2" {
		t.Fatalf("submissions = %#v", items)
	}
	if !reflect.DeepEqual(tokens, []string{"", "page-2"}) {
		t.Fatalf("page tokens = %v", tokens)
	}
}

func TestHTTPClassroomClientListsCourseParticipantsAcrossPages(t *testing.T) {
	var tokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens = append(tokens, r.URL.Query().Get("pageToken"))
		if r.URL.Path != "/courses/course-1/students" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if len(tokens) == 1 {
			_, _ = w.Write([]byte(`{"students":[{"userId":"user-1","profile":{"name":{"fullName":"First"},"emailAddress":"first@example.test"}}],"nextPageToken":"page-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"students":[{"userId":"user-2","profile":{"name":{"fullName":"Second"}}}]}`))
	}))
	defer server.Close()

	client := NewHTTPClassroomClient(HTTPConfig{BaseURL: server.URL}, server.Client())
	items, err := client.ListCourseParticipants(context.Background(), "course-1")
	if err != nil {
		t.Fatalf("ListCourseParticipants() error = %v", err)
	}
	if len(items) != 2 || items[0].UserID != "user-1" || items[1].Profile.Name.FullName != "Second" {
		t.Fatalf("participants = %#v", items)
	}
	if !reflect.DeepEqual(tokens, []string{"", "page-2"}) {
		t.Fatalf("page tokens = %v", tokens)
	}
}

func TestHTTPClassroomClientGetsUserProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userProfiles/user-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("pageSize") != "" {
			t.Fatalf("unexpected pageSize query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"id":"user-1","name":{"fullName":"Teacher One"},"emailAddress":"teacher@example.test"}`))
	}))
	defer server.Close()
	client := NewHTTPClassroomClient(HTTPConfig{BaseURL: server.URL}, server.Client())
	profile, err := client.GetUserProfile(context.Background(), "user-1")
	if err != nil || profile.ID != "user-1" || profile.Name.FullName != "Teacher One" || profile.EmailAddress != "teacher@example.test" {
		t.Fatalf("profile=%#v err=%v", profile, err)
	}
}

func TestSubmissionSourceGetsUserProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"user-1","name":{"fullName":"Teacher One"},"emailAddress":"teacher@example.test"}`))
	}))
	defer server.Close()
	source := NewSubmissionSourceWithClient(NewHTTPClassroomClient(HTTPConfig{BaseURL: server.URL}, server.Client()))
	profile, err := source.GetProfile(context.Background(), "user-1")
	if err != nil || profile.ClassroomID != "user-1" || profile.Name != "Teacher One" || profile.Email != "teacher@example.test" {
		t.Fatalf("profile=%#v err=%v", profile, err)
	}
}

func TestSubmissionSourceReportsUnsupportedProfileClient(t *testing.T) {
	source := NewSubmissionSourceWithClient(courseworkOnlyClient{})
	if _, err := source.GetProfile(context.Background(), "user-1"); err == nil || !strings.Contains(err.Error(), "profile client unavailable") {
		t.Fatalf("error=%v", err)
	}
}

func TestSubmissionSourceWrapsProfileLookupErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer server.Close()
	source := NewSubmissionSourceWithClient(NewHTTPClassroomClient(HTTPConfig{BaseURL: server.URL}, server.Client()))
	if _, err := source.GetProfile(context.Background(), "user-1"); err == nil || !strings.Contains(err.Error(), "get Classroom user profile") {
		t.Fatalf("error=%v", err)
	}
}

func TestHTTPClassroomClientReportsHTTPAndDecodeErrors(t *testing.T) {
	tests := []struct {
		name     string
		response string
		status   int
		want     string
	}{
		{name: "http status", response: "permission denied", status: http.StatusForbidden, want: "Classroom API returned 403 Forbidden: permission denied"},
		{name: "invalid json", response: "not-json", status: http.StatusOK, want: "decode Classroom response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()
			client := NewHTTPClassroomClient(HTTPConfig{BaseURL: server.URL}, server.Client())
			_, err := client.ListCourseWork(context.Background(), "course-1")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSubmissionSourceMapsTasksSubmissionsAndAttachmentMetadata(t *testing.T) {
	client := &fakeClassroomClient{
		coursework:  []CourseWork{{ID: "task-1", Title: "Report", Description: "Details", CreationTime: "created", DueDate: &Date{Year: 2026, Month: 11, Day: 20}, DueTime: &Time{Hours: 9, Minutes: 5}}},
		submissions: []StudentSubmission{{ID: "submission-1", UserID: "user-1", State: "TURNED_IN", CreationTime: "created", UpdateTime: "updated", Late: true, Assignment: Assignment{Attachments: []Attachment{{DriveFile: &DriveFileAttachment{File: DriveFile{ID: "file-1", Title: "report.docx", MIMEType: "application/docx", AlternateURL: "https://drive/file-1"}}}, {}}}}},
	}
	source := NewSubmissionSourceWithClientAndTimeZone(client, "Europe/Kyiv")
	tasks, err := source.ListTasks(context.Background(), "course-1")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("ListTasks() = %#v, %v", tasks, err)
	}
	if tasks[0].Provider != "google-classroom" || tasks[0].CourseID != "course-1" || tasks[0].CreatedAtExternal != "created" || tasks[0].DueDate != "2026-11-20" || tasks[0].DueTime != "09:05:00" || tasks[0].TimeZone != "Europe/Kyiv" || tasks[0].DueAt != "2026-11-20T09:05:00" {
		t.Fatalf("task mapping = %#v", tasks[0])
	}
	items, err := source.ListSubmissions(context.Background(), "task-1")
	if err != nil || len(items) != 1 {
		t.Fatalf("ListSubmissions() = %#v, %v", items, err)
	}
	item := items[0]
	if item.State != submission.Submitted || item.SubmitterID != "user-1" || !item.Late || item.SubmittedAt != "updated" || len(item.Materials) != 1 {
		t.Fatalf("submission mapping = %#v", item)
	}
	material := item.Materials[0]
	if material.ExternalID != "file-1" || material.Name != "report.docx" || material.MIMEType != "application/docx" || material.SourceURI != "https://drive/file-1" || material.Role != "supporting_evidence" || material.Status != submission.Pending {
		t.Fatalf("material mapping = %#v", material)
	}
}

func TestSubmissionSourceMapsCourseParticipants(t *testing.T) {
	var participant CourseParticipant
	if err := json.Unmarshal([]byte(`{"userId":"user-1","profile":{"name":{"fullName":"First"},"emailAddress":"first@example.test"}}`), &participant); err != nil {
		t.Fatal(err)
	}
	client := &fakeClassroomClient{participants: []CourseParticipant{participant}}
	source := NewSubmissionSourceWithClient(client)
	items, err := source.ListParticipants(context.Background(), "course-1")
	if err != nil || !reflect.DeepEqual(items, []task.Participant{{ExternalID: "user-1", Name: "First", Email: "first@example.test"}}) {
		t.Fatalf("participants = %#v, error = %v", items, err)
	}
}

func TestSubmissionSourceTreatsUnsupportedParticipantClientAsEmptyRoster(t *testing.T) {
	source := NewSubmissionSourceWithClient(courseworkOnlyClient{})
	items, err := source.ListParticipants(context.Background(), "course-1")
	if err != nil || len(items) != 0 {
		t.Fatalf("participants = %#v, error = %v", items, err)
	}
}

func TestSubmissionSourceListsTasksChangedSinceCheckpoint(t *testing.T) {
	client := &fakeClassroomClient{coursework: []CourseWork{
		{ID: "old", UpdateTime: "2026-01-01T00:00:00Z"},
		{ID: "new", UpdateTime: "2026-01-03T00:00:00Z"},
		{ID: "unknown", UpdateTime: "not-a-time"},
	}}
	source := NewSubmissionSourceWithClient(client)
	items, err := source.ListTasksSince(context.Background(), "course-1", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListTasksSince() error = %v", err)
	}
	if got := []string{items[0].ExternalID, items[1].ExternalID}; !reflect.DeepEqual(got, []string{"new", "unknown"}) {
		t.Fatalf("changed tasks = %#v", got)
	}
}

func TestSubmissionSourceListsAllTasksWithoutCheckpoint(t *testing.T) {
	client := &fakeClassroomClient{coursework: []CourseWork{{ID: "first"}, {ID: "second"}}}
	source := NewSubmissionSourceWithClient(client)
	items, err := source.ListTasksSince(context.Background(), "course-1", time.Time{})
	if err != nil {
		t.Fatalf("ListTasksSince() error = %v", err)
	}
	if got := []string{items[0].ExternalID, items[1].ExternalID}; !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("tasks = %#v", got)
	}
}

func TestSubmissionSourceMapsDateWithoutTime(t *testing.T) {
	client := &fakeClassroomClient{coursework: []CourseWork{{ID: "task-1", DueDate: &Date{Year: 2026, Month: 11, Day: 20}}}}
	source := NewSubmissionSourceWithClient(client)
	items, err := source.ListTasks(context.Background(), "course-1")
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if items[0].DueAt != "2026-11-20" {
		t.Fatalf("DueAt = %q", items[0].DueAt)
	}
}

func TestSubmissionSourceLeavesDueDateEmptyWhenNoDateIsProvided(t *testing.T) {
	source := NewSubmissionSourceWithClient(&fakeClassroomClient{coursework: []CourseWork{{ID: "task-1"}}})
	items, err := source.ListTasks(context.Background(), "course-1")
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if items[0].DueAt != "" {
		t.Fatalf("DueAt = %q, want empty", items[0].DueAt)
	}
}

func TestHTTPClassroomClientReportsMalformedBaseURL(t *testing.T) {
	client := NewHTTPClassroomClient(HTTPConfig{BaseURL: "://bad-url"}, http.DefaultClient)
	_, err := client.ListCourseWork(context.Background(), "course-1")
	if err == nil || !strings.Contains(err.Error(), "build Classroom request") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPClassroomClientReportsInvalidContext(t *testing.T) {
	client := NewHTTPClassroomClient(HTTPConfig{BaseURL: "https://classroom.example/v1"}, http.DefaultClient)
	_, err := client.ListCourseWork(nil, "course-1")
	if err == nil || !strings.Contains(err.Error(), "create Classroom request") {
		t.Fatalf("error = %v, want request creation error", err)
	}
}

func TestHTTPClassroomClientReportsSubmissionAndTransportErrors(t *testing.T) {
	client := NewHTTPClassroomClient(HTTPConfig{BaseURL: "http://example.test"}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	})})
	_, err := client.ListStudentSubmissions(context.Background(), "course-1", "task-1")
	if err == nil || !strings.Contains(err.Error(), "request Classroom API:") || !strings.Contains(err.Error(), "network unavailable") {
		t.Fatalf("submission error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSubmissionSourceFallsBackToCreationTimeForSubmittedAt(t *testing.T) {
	client := &fakeClassroomClient{
		coursework:  []CourseWork{{ID: "task-1"}},
		submissions: []StudentSubmission{{ID: "submission-1", CreationTime: "created", UpdateTime: "updated"}, {ID: "submission-2", CreationTime: "created-only"}},
	}
	source := NewSubmissionSourceWithClient(client)
	if _, err := source.ListTasks(context.Background(), "course-1"); err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	items, err := source.ListSubmissions(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ListSubmissions() error = %v", err)
	}
	if len(items) != 2 || items[0].SubmittedAt != "updated" || items[1].SubmittedAt != "created-only" {
		t.Fatalf("SubmittedAt mapping = %#v", items)
	}
}

func TestSubmissionSourceReturnsContextualErrors(t *testing.T) {
	clientErr := errors.New("upstream unavailable")
	source := NewSubmissionSourceWithClient(&fakeClassroomClient{err: clientErr})
	if _, err := source.ListTasks(context.Background(), "course-1"); !strings.Contains(err.Error(), "list Classroom coursework: upstream unavailable") {
		t.Fatalf("ListTasks error = %v", err)
	}
	if _, err := source.ListSubmissions(context.Background(), "unknown-task"); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("unknown task error = %v", err)
	}

	source = NewSubmissionSourceWithClient(&fakeClassroomClient{coursework: []CourseWork{{ID: "task-1"}}, err: clientErr})
	if _, err := source.ListTasks(context.Background(), "course-1"); err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if _, err := source.ListSubmissions(context.Background(), "task-1"); !strings.Contains(err.Error(), "list Classroom submissions: upstream unavailable") {
		t.Fatalf("ListSubmissions error = %v", err)
	}
}

func TestHTTPConfigFromEnvironmentUsesConfiguredAndDefaultValues(t *testing.T) {
	t.Setenv("GOOGLE_CLASSROOM_BASE_URL", "https://example.test/")
	t.Setenv("GOOGLE_CLASSROOM_ACCESS_TOKEN", "secret")
	if got := HTTPConfigFromEnvironment(); got.BaseURL != "https://example.test/" || got.AccessToken != "secret" {
		t.Fatalf("configured HTTP config = %#v", got)
	}
	t.Setenv("GOOGLE_CLASSROOM_BASE_URL", "")
	t.Setenv("GOOGLE_CLASSROOM_ACCESS_TOKEN", "")
	if got := HTTPConfigFromEnvironment(); got.BaseURL != defaultBaseURL || got.AccessToken != "" {
		t.Fatalf("default HTTP config = %#v", got)
	}
	t.Setenv("GOOGLE_CLIENT_ID", "client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("GOOGLE_REFRESH_TOKEN", "refresh")
	t.Setenv("GOOGLE_CLASSROOM_TIMEZONE", "Europe/Kyiv")
	if got := HTTPConfigFromEnvironment(); got.ClientID != "client" || got.ClientSecret != "secret" || got.RefreshToken != "refresh" || got.TimeZone != "Europe/Kyiv" {
		t.Fatalf("OAuth config = %#v", got)
	}
	if got := NewHTTPClassroomClient(HTTPConfig{}, nil); got == nil {
		t.Fatal("NewHTTPClassroomClient() returned nil")
	}
}

func TestNewSubmissionSourceUsesDefaultHTTPClient(t *testing.T) {
	if source := NewSubmissionSource(); source == nil {
		t.Fatal("NewSubmissionSource() returned nil")
	}
}

type fakeClassroomClient struct {
	coursework   []CourseWork
	submissions  []StudentSubmission
	participants []CourseParticipant
	err          error
}

type courseworkOnlyClient struct{}

func (courseworkOnlyClient) ListCourseWork(context.Context, string) ([]CourseWork, error) {
	return nil, nil
}

func (courseworkOnlyClient) ListStudentSubmissions(context.Context, string, string) ([]StudentSubmission, error) {
	return nil, nil
}

func (f *fakeClassroomClient) ListCourseWork(context.Context, string) ([]CourseWork, error) {
	if f.err != nil && f.coursework == nil {
		return nil, f.err
	}
	return f.coursework, nil
}

func (f *fakeClassroomClient) ListStudentSubmissions(context.Context, string, string) ([]StudentSubmission, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.submissions, nil
}

func (f *fakeClassroomClient) ListCourseParticipants(context.Context, string) ([]CourseParticipant, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.participants, nil
}
