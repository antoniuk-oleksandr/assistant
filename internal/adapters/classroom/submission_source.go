package classroom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

const defaultBaseURL = "https://classroom.googleapis.com/v1"

var ErrSourceUnavailable = errors.New("classroom submission source is unavailable")

// ClassroomClient is the transport boundary for the Classroom adapter. Keeping
// it separate from SubmissionSource makes the domain port testable without an
// OAuth token or a network connection.
type ClassroomClient interface {
	ListCourseWork(context.Context, string) ([]CourseWork, error)
	ListStudentSubmissions(context.Context, string, string) ([]StudentSubmission, error)
}

type CourseWork struct {
	ID           string `json:"id"`
	CourseID     string `json:"courseId"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CreationTime string `json:"creationTime"`
	UpdateTime   string `json:"updateTime"`
	DueDate      *Date  `json:"dueDate"`
	DueTime      *Time  `json:"dueTime"`
}

type Date struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type Time struct {
	Hours   int `json:"hours"`
	Minutes int `json:"minutes"`
}

type StudentSubmission struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	State        string     `json:"state"`
	Late         bool       `json:"late"`
	CreationTime string     `json:"creationTime"`
	UpdateTime   string     `json:"updateTime"`
	Assignment   Assignment `json:"assignmentSubmission"`
}

type Assignment struct {
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	DriveFile *DriveFileAttachment `json:"driveFile"`
}

type DriveFileAttachment struct {
	File DriveFile `json:"driveFile"`
}

type DriveFile struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	AlternateURL string `json:"alternateLink"`
	MIMEType     string `json:"mimeType"`
}

type SubmissionSource struct {
	client  ClassroomClient
	mu      sync.RWMutex
	courses map[string]string
}

// NewSubmissionSource is the Fx constructor. The HTTP transport is still
// replaceable through NewSubmissionSourceWithClient for unit and integration
// tests. Credentials are read only at the infrastructure boundary.
func NewSubmissionSource() ports.SubmissionSource {
	return NewSubmissionSourceWithClient(NewHTTPClassroomClient(HTTPConfigFromEnvironment(), http.DefaultClient))
}

func NewSubmissionSourceWithClient(client ClassroomClient) ports.SubmissionSource {
	return &SubmissionSource{client: client, courses: make(map[string]string)}
}

func (s *SubmissionSource) ListTasks(ctx context.Context, courseID string) ([]task.Task, error) {
	items, err := s.client.ListCourseWork(ctx, courseID)
	if err != nil {
		return nil, fmt.Errorf("list Classroom coursework: %w", err)
	}
	s.mu.Lock()
	for _, item := range items {
		s.courses[item.ID] = courseID
	}
	s.mu.Unlock()
	result := make([]task.Task, 0, len(items))
	for _, item := range items {
		result = append(result, toTask(item, courseID))
	}
	return result, nil
}

func (s *SubmissionSource) ListSubmissions(ctx context.Context, taskID string) ([]submission.Submission, error) {
	s.mu.RLock()
	courseID := s.courses[taskID]
	s.mu.RUnlock()
	if courseID == "" {
		return nil, fmt.Errorf("task %q: %w", taskID, ErrSourceUnavailable)
	}
	items, err := s.client.ListStudentSubmissions(ctx, courseID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list Classroom submissions: %w", err)
	}
	result := make([]submission.Submission, 0, len(items))
	for _, item := range items {
		result = append(result, toSubmission(item))
	}
	return result, nil
}

func toTask(item CourseWork, courseID string) task.Task {
	return task.Task{Provider: "google-classroom", ExternalID: item.ID, CourseID: courseID, Title: item.Title, Description: item.Description, CreatedAtExternal: item.CreationTime, DueAt: dueAt(item.DueDate, item.DueTime), Status: task.Discovered}
}

func toSubmission(item StudentSubmission) submission.Submission {
	submittedAt := item.UpdateTime
	if submittedAt == "" {
		submittedAt = item.CreationTime
	}
	result := submission.Submission{ExternalID: item.ID, SubmitterID: item.UserID, State: submission.State(strings.ToLower(item.State)), SubmittedAt: submittedAt, CreatedAt: item.CreationTime, UpdatedAt: item.UpdateTime, Late: item.Late}
	if result.State == "turned_in" {
		result.State = submission.Submitted
	}
	for _, attachment := range item.Assignment.Attachments {
		if attachment.DriveFile == nil {
			continue
		}
		file := attachment.DriveFile.File
		result.Materials = append(result.Materials, submission.Material{ExternalID: file.ID, Name: file.Title, MIMEType: file.MIMEType, SourceURI: file.AlternateURL, Role: "supporting_evidence", Status: submission.Pending})
	}
	return result
}

func dueAt(date *Date, clock *Time) string {
	if date == nil {
		return ""
	}
	if clock == nil {
		return fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day)
	}
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:00", date.Year, date.Month, date.Day, clock.Hours, clock.Minutes)
}

type HTTPConfig struct {
	BaseURL     string
	AccessToken string
}

func HTTPConfigFromEnvironment() HTTPConfig {
	baseURL := os.Getenv("GOOGLE_CLASSROOM_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return HTTPConfig{BaseURL: baseURL, AccessToken: os.Getenv("GOOGLE_CLASSROOM_ACCESS_TOKEN")}
}

type HTTPClassroomClient struct {
	baseURL     string
	accessToken string
	httpClient  *http.Client
}

func NewHTTPClassroomClient(cfg HTTPConfig, client *http.Client) ClassroomClient {
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &HTTPClassroomClient{baseURL: baseURL, accessToken: cfg.AccessToken, httpClient: client}
}

func (c *HTTPClassroomClient) ListCourseWork(ctx context.Context, courseID string) ([]CourseWork, error) {
	var result []CourseWork
	var pageToken string
	for {
		var page struct {
			Items []CourseWork `json:"courseWork"`
			Next  string       `json:"nextPageToken"`
		}
		if err := c.get(ctx, "/courses/"+url.PathEscape(courseID)+"/courseWork", pageToken, &page); err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if page.Next == "" {
			return result, nil
		}
		pageToken = page.Next
	}
}

func (c *HTTPClassroomClient) ListStudentSubmissions(ctx context.Context, courseID, taskID string) ([]StudentSubmission, error) {
	var result []StudentSubmission
	var pageToken string
	for {
		var page struct {
			Items []StudentSubmission `json:"studentSubmissions"`
			Next  string              `json:"nextPageToken"`
		}
		path := "/courses/" + url.PathEscape(courseID) + "/courseWork/" + url.PathEscape(taskID) + "/studentSubmissions"
		if err := c.get(ctx, path, pageToken, &page); err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if page.Next == "" {
			return result, nil
		}
		pageToken = page.Next
	}
}

func (c *HTTPClassroomClient) get(ctx context.Context, path, pageToken string, target any) error {
	requestURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("build Classroom request: %w", err)
	}
	query := requestURL.Query()
	query.Set("pageSize", "100")
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	requestURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create Classroom request: %w", err)
	}
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request Classroom API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Classroom API returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode Classroom response: %w", err)
	}
	return nil
}

var _ ports.SubmissionSource = (*SubmissionSource)(nil)
