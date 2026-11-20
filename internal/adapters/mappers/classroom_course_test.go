package mappers_test

import (
	"time"

	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/classroom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("classroom course row mapper", func() {
	created := time.Date(2026, 2, 3, 4, 5, 6, 123000000, time.UTC)
	updated := created.Add(time.Hour)
	row := func() models.ClassroomCourseRow {
		return models.ClassroomCourseRow{ID: 4, ExternalID: "123", SourceReference: "source", Status: "active", CreatedAt: created.Format(time.RFC3339Nano), UpdatedAt: updated.Format(time.RFC3339Nano)}
	}

	It("maps persisted fields", func() {
		result, err := mappers.ToClassroomCourse(row())
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(classroom.Course{ID: 4, ExternalID: "123", SourceReference: "source", Status: classroom.Active, CreatedAt: created, UpdatedAt: updated}))
	})

	It("reports malformed timestamps with field context", func() {
		input := row()
		input.CreatedAt = "bad"
		_, err := mappers.ToClassroomCourse(input)
		Expect(err).To(MatchError(ContainSubstring("parse course created_at")))

		input = row()
		input.UpdatedAt = "bad"
		_, err = mappers.ToClassroomCourse(input)
		Expect(err).To(MatchError(ContainSubstring("parse course updated_at")))
	})
})
