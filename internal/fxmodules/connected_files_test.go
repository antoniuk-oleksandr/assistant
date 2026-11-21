package fxmodules

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex/assistent/internal/adapters/drive"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/platform/config"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"golang.org/x/oauth2"
)

func TestPollingPersistsMixedConnectedFileOutcomes(t *testing.T) {
	t.Setenv("REPORT_RENDERER", "local-xlsx")
	g := NewWithT(t)
	const pdfLink = "https://drive.google.com/file/d/pdf/view?resourcekey=fixture-key"
	const docsLink = "https://docs.google.com/document/d/denied/edit"
	const sheetsLink = "https://docs.google.com/spreadsheets/d/missing/edit"
	const content = "fixture PDF contents"
	const replacement = "replacement PDF contents"
	var returned atomic.Bool
	var pdfRevision atomic.Int32
	var driveRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/courses/123/courseWork":
			fmt.Fprint(w, `{"courseWork":[{"id":"task-1","title":"Report"}]}`)
		case "/v1/courses/123/students":
			fmt.Fprint(w, `{"students":[{"userId":"teacher-1","profile":{"name":{"fullName":"Teacher One"},"emailAddress":"teacher@example.test"}}]}`)
		case "/v1/courses/123/courseWork/task-1/studentSubmissions":
			state := "TURNED_IN"
			if returned.Load() {
				state = "RETURNED"
			}
			fmt.Fprintf(w, `{"studentSubmissions":[
				{"id":"report","state":%q,"assignmentSubmission":{"attachments":[
					{"driveFile":{"id":"pdf","title":"Classroom name","alternateLink":%q}},
					{"link":{"title":"Doc","url":%q}},
					{"link":{"title":"Sheet","url":%q}},
					{"driveFile":{"driveFile":{"id":"failed","title":"Nested attachment"}}},
					{"link":{"title":"Unrelated website","url":"https://example.invalid/file/d/ignored/view"}}
				]}},
				{"id":"draft","state":"RECLAIMED_BY_STUDENT","assignmentSubmission":{"attachments":[{"driveFile":{"id":"draft-file","title":"Draft"}}]}},
				{"id":"empty","state":"TURNED_IN"}
			]}`, state, pdfLink, docsLink, sheetsLink)
		case "/drive/v3/files/pdf":
			driveRequests.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer fixture-drive-token" {
				t.Errorf("Drive authorization = %q", got)
			}
			if got := r.Header.Get("X-Goog-Drive-Resource-Keys"); got != "pdf/fixture-key" {
				t.Errorf("Drive resource keys = %q", got)
			}
			body, name := content, "Report.pdf"
			switch pdfRevision.Load() {
			case 1:
				body, name = replacement, "Updated.pdf"
			case 2:
				body, name = "unavailable body", "Unavailable.pdf"
			}
			if r.URL.Query().Get("alt") == "media" {
				if pdfRevision.Load() == 2 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, body)
				return
			}
			fmt.Fprintf(w, `{"id":"pdf","name":%q,"mimeType":"application/pdf","size":"%d","webViewLink":"https://drive.google.com/open?id=pdf"}`, name, len(body))
		case "/drive/v3/files/denied":
			driveRequests.Add(1)
			w.WriteHeader(http.StatusForbidden)
		case "/drive/v3/files/missing":
			driveRequests.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case "/drive/v3/files/failed":
			driveRequests.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	t.Setenv("SQLITE_DATABASE_PATH", filepath.Join(dir, "state.db"))
	t.Setenv("GOOGLE_CLASSROOM_BASE_URL", server.URL+"/v1")
	t.Setenv("GOOGLE_CLASSROOM_ACCESS_TOKEN", "fixture-classroom-token")
	t.Setenv("GOOGLE_DRIVE_BASE_URL", server.URL+"/drive/v3")
	t.Setenv("GOOGLE_DRIVE_OAUTH_CLIENT_ID", "")
	t.Setenv("GOOGLE_DRIVE_OAUTH_CLIENT_SECRET", "")
	t.Setenv("GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN", "")
	t.Setenv("GOOGLE_DRIVE_MATERIAL_DIR", filepath.Join(dir, "attachments"))
	var (
		poller         *application.PollingService
		courses        cli.ClassroomService
		scenarios      cli.ScenarioService
		bindings       ports.TaskBindingRepository
		courseBindings ports.ScenarioCourseBindingRepository
		repo           ports.SubmissionRepository
	)
	app := fx.New(fx.NopLogger, PlatformModule, StorageModule, ClassroomStorageModule, ApplicationModule, PollingModule,
		fx.Decorate(func(_ ports.MaterialRetriever, cfg config.DriveConfig) (ports.MaterialRetriever, error) {
			tokens := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fixture-drive-token"})
			return drive.NewMaterialRetrieverWithTokenSource(cfg, tokens)
		}),
		fx.Populate(&poller, &courses, &scenarios, &bindings, &courseBindings, &repo))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	g.Expect(app.Start(ctx)).To(Succeed())
	defer func() { g.Expect(app.Stop(context.Background())).To(Succeed()) }()
	g.Expect(courses.Add(ctx, "123")).To(Succeed())
	g.Expect(scenarios.Add(ctx, "science-report")).To(Succeed())
	g.Expect(courseBindings.Bind(ctx, "science-report", "123")).To(Succeed())
	g.Expect(poller.Refresh(ctx)).To(Succeed())
	g.Expect(driveRequests.Load()).To(BeZero())
	g.Expect(bindings.Bind(ctx, "science-report", "task-1")).To(Succeed())
	active, err := bindings.Active(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(active).To(HaveLen(1))

	readSubmissions := func() map[string]submission.Submission {
		items, err := repo.ListByTask(ctx, active[0].Task.ID)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(items).To(HaveLen(3))
		byID := make(map[string]submission.Submission)
		for _, item := range items {
			byID[item.ExternalID] = item
		}
		g.Expect(byID).To(HaveKey("report"))
		g.Expect(byID).To(HaveKey("draft"))
		g.Expect(byID).To(HaveKey("empty"))
		return byID
	}
	g.Expect(poller.Refresh(ctx)).To(Succeed())
	g.Expect(driveRequests.Load()).To(Equal(int32(5)))
	g.Expect(poller.Refresh(ctx)).To(Succeed())
	items := readSubmissions()
	g.Expect(items["draft"].State).To(Equal(submission.Created))
	g.Expect(items["draft"].Materials).To(HaveLen(1))
	g.Expect(items["draft"].Materials[0].Status).To(Equal(submission.Pending))
	g.Expect(items["draft"].Materials[0].LocalPath).To(BeEmpty())
	g.Expect(items["empty"].State).To(Equal(submission.Submitted))
	g.Expect(items["empty"].Materials).To(BeEmpty())

	report := items["report"]
	g.Expect(report.State).To(Equal(submission.Submitted))
	g.Expect(report.Materials).To(HaveLen(4))
	materials := make(map[string]submission.Material)
	for _, material := range report.Materials {
		materials[material.ExternalID] = material
	}
	pdf := materials["pdf"]
	g.Expect(pdf.Status).To(Equal(submission.Pending))
	g.Expect(pdf.Name).To(Equal("Report.pdf"))
	g.Expect(pdf.MIMEType).To(Equal("application/pdf"))
	g.Expect(pdf.SizeBytes).To(HaveValue(Equal(int64(len(content)))))
	g.Expect(pdf.SourceURI).To(Equal(pdfLink))
	g.Expect(filepath.Dir(pdf.LocalPath)).To(Equal(filepath.Join(dir, "attachments")))
	contents, err := os.ReadFile(pdf.LocalPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(contents).To(Equal([]byte(content)))
	for _, want := range []struct {
		id, link, name string
		status         submission.MaterialStatus
	}{
		{"denied", docsLink, "Doc", submission.AccessDenied},
		{"missing", sheetsLink, "Sheet", submission.NotFound},
		{"failed", "", "Nested attachment", submission.Failed},
	} {
		got := materials[want.id]
		g.Expect(got.Status).To(Equal(want.status), want.id)
		g.Expect(got.SourceURI).To(Equal(want.link), want.id)
		g.Expect(got.Name).To(Equal(want.name), want.id)
		g.Expect(got.LocalPath).To(BeEmpty(), want.id)
	}

	requestsBeforeReturn := driveRequests.Load()
	returned.Store(true)
	g.Expect(poller.Refresh(ctx)).To(Succeed())
	g.Expect(driveRequests.Load()).To(Equal(requestsBeforeReturn), "returned submissions should not fetch attachments")
	updated := readSubmissions()["report"]
	g.Expect(updated.State).To(Equal(submission.Returned))
	g.Expect(updated.Materials).To(HaveLen(4))
	for _, material := range updated.Materials {
		previous := materials[material.ExternalID]
		g.Expect(material.Status).To(Equal(previous.Status), material.ExternalID)
		g.Expect(material.SizeBytes).To(Equal(previous.SizeBytes), material.ExternalID)
		g.Expect(material.SourceURI).To(Equal(previous.SourceURI), material.ExternalID)
		g.Expect(material.LocalPath).To(Equal(previous.LocalPath), material.ExternalID)
	}

	readPDF := func() submission.Material {
		report := readSubmissions()["report"]
		g.Expect(report.State).To(Equal(submission.Submitted))
		g.Expect(report.Materials).To(HaveLen(4))
		for _, material := range report.Materials {
			if material.ExternalID == pdf.ExternalID {
				return material
			}
		}
		t.Fatal("PDF material no longer linked to the submission")
		return submission.Material{}
	}
	returned.Store(false)
	pdfRevision.Store(1)
	g.Expect(poller.Refresh(ctx)).To(Succeed())
	newPDF := readPDF()
	g.Expect(newPDF.ID).To(Equal(pdf.ID))
	g.Expect(newPDF.SourceURI).To(Equal(pdfLink))
	g.Expect(newPDF.Name).To(Equal("Updated.pdf"))
	g.Expect(newPDF.SizeBytes).To(HaveValue(Equal(int64(len(replacement)))))
	g.Expect(newPDF.Status).To(Equal(submission.Pending))
	g.Expect(newPDF.LocalPath).NotTo(Equal(pdf.LocalPath))

	pdfRevision.Store(2)
	g.Expect(poller.Refresh(ctx)).To(Succeed())
	want := newPDF
	want.Status = submission.Failed
	g.Expect(readPDF()).To(Equal(want), "a failed download must preserve the last good snapshot")
	for path, body := range map[string]string{pdf.LocalPath: content, newPDF.LocalPath: replacement} {
		data, err := os.ReadFile(path)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(data).To(Equal([]byte(body)))
	}
}

func TestFxPollingAnalyzesUnknownMaterialAndPersistsLowConfidence(t *testing.T) {
	t.Setenv("REPORT_RENDERER", "local-xlsx")
	g := NewWithT(t)
	const materialBody = "opaque evidence payload"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/courses/123/courseWork":
			fmt.Fprint(w, `{"courseWork":[{"id":"unknown-task","title":"Report"}]}`)
		case "/v1/courses/123/students":
			fmt.Fprint(w, `{"students":[{"userId":"teacher-1","profile":{"name":{"fullName":"Teacher One"},"emailAddress":"teacher@example.test"}}]}`)
		case "/v1/courses/123/courseWork/unknown-task/studentSubmissions":
			fmt.Fprint(w, `{"studentSubmissions":[{"id":"unknown-submission","state":"TURNED_IN","assignmentSubmission":{"attachments":[{"driveFile":{"id":"unknown-material","title":"mystery.bin","alternateLink":"https://drive.google.com/file/d/unknown-material/view"}}]}}]}`)
		case "/drive/v3/files/unknown-material":
			if r.URL.Query().Get("alt") == "media" {
				fmt.Fprint(w, materialBody)
				return
			}
			fmt.Fprint(w, `{"id":"unknown-material","name":"mystery.bin","mimeType":"application/octet-stream","size":"22","webViewLink":"https://drive.google.com/open?id=unknown-material"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "state.db")
	t.Setenv("SQLITE_DATABASE_PATH", databasePath)
	t.Setenv("GOOGLE_CLASSROOM_BASE_URL", server.URL+"/v1")
	t.Setenv("GOOGLE_CLASSROOM_ACCESS_TOKEN", "fixture-classroom-token")
	t.Setenv("GOOGLE_DRIVE_BASE_URL", server.URL+"/drive/v3")
	t.Setenv("GOOGLE_DRIVE_OAUTH_CLIENT_ID", "")
	t.Setenv("GOOGLE_DRIVE_OAUTH_CLIENT_SECRET", "")
	t.Setenv("GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN", "")
	t.Setenv("GOOGLE_DRIVE_MATERIAL_DIR", filepath.Join(dir, "attachments"))

	newApp := func() (*fx.App, *application.PollingService, cli.ClassroomService, cli.ScenarioService, ports.TaskBindingRepository, ports.SubmissionRepository, ports.ScenarioCourseBindingRepository) {
		var (
			poller         *application.PollingService
			courses        cli.ClassroomService
			scenarios      cli.ScenarioService
			bindings       ports.TaskBindingRepository
			courseBindings ports.ScenarioCourseBindingRepository
			repo           ports.SubmissionRepository
		)
		app := fx.New(fx.NopLogger, PlatformModule, StorageModule, ClassroomStorageModule, ApplicationModule, PollingModule,
			fx.Decorate(func(_ ports.MaterialRetriever, cfg config.DriveConfig) (ports.MaterialRetriever, error) {
				tokens := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fixture-drive-token"})
				return drive.NewMaterialRetrieverWithTokenSource(cfg, tokens)
			}),
			fx.Populate(&poller, &courses, &scenarios, &bindings, &courseBindings, &repo))
		return app, poller, courses, scenarios, bindings, repo, courseBindings
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app, poller, courses, scenarios, bindings, repo, courseBindings := newApp()
	g.Expect(app.Start(ctx)).To(Succeed())
	g.Expect(courses.Add(ctx, "123")).To(Succeed())
	g.Expect(scenarios.Add(ctx, "science-report")).To(Succeed())
	g.Expect(courseBindings.Bind(ctx, "science-report", "123")).To(Succeed())
	g.Expect(poller.Refresh(ctx)).To(Succeed())
	g.Expect(bindings.Bind(ctx, "science-report", "unknown-task")).To(Succeed())
	g.Expect(poller.Refresh(ctx)).To(Succeed())

	active, err := bindings.Active(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(active).To(HaveLen(1))
	stored, err := repo.ListByTask(ctx, active[0].Task.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stored).To(HaveLen(1))
	g.Expect(stored[0].Materials).To(HaveLen(1))
	material := stored[0].Materials[0]
	g.Expect(material.ParseOutcomes).To(HaveLen(1))
	outcome := material.ParseOutcomes[0]
	g.Expect(outcome.Source.ScenarioName).To(Equal("science-report"))
	g.Expect(outcome.Source.ScenarioType).To(Equal("ScienceReportScenario"))
	g.Expect(outcome.Result.Issue).To(Equal(submission.AnalysisLowConfidence))
	g.Expect(outcome.Result.Analysis).NotTo(BeNil())
	g.Expect(outcome.Result.Analysis.Proposal.DocumentType).To(Equal("supporting_evidence"))
	g.Expect(outcome.Result.Analysis.Proposal.Fields["document_meaning"]).To(Equal("supporting evidence for the bound scenario"))
	g.Expect(outcome.Result.Analysis.Proposal.Fields["scenario_name"]).To(Equal("science-report"))
	g.Expect(outcome.Result.Analysis.LowConfidence).To(BeTrue())
	g.Expect(outcome.EligibleForAggregation()).To(BeFalse())
	g.Expect(app.Stop(ctx)).To(Succeed())

	app, _, _, _, _, repo, _ = newApp()
	g.Expect(app.Start(ctx)).To(Succeed())
	defer func() { g.Expect(app.Stop(context.Background())).To(Succeed()) }()
	reloaded, err := repo.ListByTask(ctx, active[0].Task.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reloaded).To(HaveLen(1))
	g.Expect(reloaded[0].Materials).To(HaveLen(1))
	reloadedOutcome := reloaded[0].Materials[0].ParseOutcomes[0]
	g.Expect(reloadedOutcome.Source.ScenarioType).To(Equal("ScienceReportScenario"))
	g.Expect(reloadedOutcome.Result.Analysis.Proposal.DocumentType).To(Equal("supporting_evidence"))
	g.Expect(reloadedOutcome.Result.Analysis.Proposal.Fields["scenario_name"]).To(Equal("science-report"))
	g.Expect(reloadedOutcome.Result.Analysis.LowConfidence).To(BeTrue())
	g.Expect(reloadedOutcome.EligibleForAggregation()).To(BeFalse())
}
