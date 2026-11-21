package application_test

import (
	"context"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("polling connected materials", func() {
	var (
		source      *mocks.MockSubmissionSource
		retriever   *mocks.MockMaterialRetriever
		submissions *mocks.MockSubmissionRepository
		state       *mocks.MockPollingStateRepository
		service     *application.PollingService
	)
	const courseID = "course-1"
	activeTask := task.Task{ID: 42, ExternalID: "task-1", CourseID: courseID}
	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		courses := mocks.NewMockClassroomCourseRepository(ctrl)
		tasks := mocks.NewMockTaskRepository(ctrl)
		bindings := mocks.NewMockTaskBindingRepository(ctrl)
		source = mocks.NewMockSubmissionSource(ctrl)
		retriever = mocks.NewMockMaterialRetriever(ctrl)
		submissions = mocks.NewMockSubmissionRepository(ctrl)
		state = mocks.NewMockPollingStateRepository(ctrl)
		courses.EXPECT().List(gomock.Any()).Return([]classroom.Course{{ExternalID: courseID, Status: classroom.Active}}, nil)
		source.EXPECT().ListTasks(gomock.Any(), courseID).Return([]task.Task{activeTask}, nil)
		tasks.EXPECT().List(gomock.Any()).Return([]task.Task{}, nil)
		tasks.EXPECT().Register(gomock.Any(), gomock.Any()).Return(nil)
		bindings.EXPECT().Active(gomock.Any()).Return([]task.Binding{{Task: activeTask}}, nil)
		service = application.NewPollingServiceWithMaterials(courses, source, tasks, bindings, submissions, state, retriever)
	})

	It("stores retrieval results only for submitted work without mutating provider materials", func(ctx SpecContext) {
		pending := submission.Material{ExternalID: "file-1", Name: "report.pdf", Status: submission.Pending}
		input := []submission.Submission{
			{ExternalID: "report-1", State: submission.Submitted, Materials: []submission.Material{pending}},
			{ExternalID: "draft-1", State: submission.Created, Materials: []submission.Material{pending}},
		}
		source.EXPECT().ListSubmissions(ctx, activeTask.ExternalID).Return(input, nil)
		denied := pending
		denied.Status = submission.AccessDenied
		retriever.EXPECT().Retrieve(ctx, pending).Return(denied)
		submissions.EXPECT().Upsert(ctx, submission.Submission{TaskID: activeTask.ID, TaskExternalID: activeTask.ExternalID, ExternalID: "report-1", State: submission.Submitted, Materials: []submission.Material{denied}}).Return(nil)
		submissions.EXPECT().Upsert(ctx, submission.Submission{TaskID: activeTask.ID, TaskExternalID: activeTask.ExternalID, ExternalID: "draft-1", State: submission.Created, Materials: []submission.Material{pending}}).Return(nil)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(input[0].Materials).To(Equal([]submission.Material{pending}))
	})

	It("does not fetch remaining materials, save a partial submission, or advance its checkpoint after cancellation", func(ctx SpecContext) {
		pollCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		first := submission.Material{ExternalID: "first", Status: submission.Pending}
		state.EXPECT().LastCheckpoint(pollCtx).Return(time.Time{}, nil)
		source.EXPECT().ListSubmissions(pollCtx, activeTask.ExternalID).Return([]submission.Submission{{
			ExternalID: "report-1", State: submission.Submitted, Materials: []submission.Material{first, {ExternalID: "second", Status: submission.Pending}},
		}}, nil)
		retriever.EXPECT().Retrieve(pollCtx, first).DoAndReturn(func(_ context.Context, item submission.Material) submission.Material {
			cancel()
			item.Status = submission.Failed
			return item
		})
		Expect(service.Run(pollCtx, time.Hour)).To(MatchError(context.Canceled))
	})
})
