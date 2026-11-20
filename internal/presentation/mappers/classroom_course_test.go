package mappers_test

import (
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/presentation/mappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("classroom course DTO mapper", func() {
	It("formats course timestamps in UTC", func() {
		created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.FixedZone("offset", 2*60*60))
		updated := created.Add(time.Hour)
		result := mappers.ClassroomCourseToDTO(application.ClassroomCourseResult{
			ExternalID: "900423945297", SourceReference: "source", Status: "active",
			CreatedAt: created, UpdatedAt: updated,
		})

		Expect(result.ExternalID).To(Equal("900423945297"))
		Expect(result.SourceReference).To(Equal("source"))
		Expect(result.Status).To(Equal("active"))
		Expect(result.CreatedAt).To(Equal(created.UTC().Format(time.RFC3339Nano)))
		Expect(result.UpdatedAt).To(Equal(updated.UTC().Format(time.RFC3339Nano)))
	})
})
