package classroom_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/alex/assistent/internal/domain/classroom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClassroomCourse(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Classroom Course")
}

var _ = Describe("Classroom course reference normalization", func() {
	It("normalizes numeric IDs, Classroom URLs, and Base64URL segments", func() {
		encoded := base64.RawURLEncoding.EncodeToString([]byte("900423945297"))

		for _, reference := range []string{
			"900423945297",
			"  900423945297  ",
			"https://classroom.google.com/c/" + encoded,
			encoded,
		} {
			id, err := classroom.NormalizeReference(reference)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal("900423945297"))
		}
	})

	It("rejects malformed, non-Classroom, and non-numeric references", func() {
		for _, reference := range []string{
			"",
			"not-a-course",
			"900423945297/extra",
			"https://classroom.google.com/u/0/h",
			"https://example.com/c/900423945297",
			"https://classroom.google.com/c/not-a-course",
			"%%%",
		} {
			_, err := classroom.NormalizeReference(reference)
			Expect(err).To(MatchError(ContainSubstring(classroom.ErrInvalidCourseReference.Error())))
			Expect(errors.Is(err, classroom.ErrInvalidCourseReference)).To(BeTrue())
		}
	})

	It("rejects Classroom URLs with unsupported schemes, hosts, or URL components", func() {
		for _, reference := range []string{
			"http://classroom.google.com/c/900423945297",
			"https://example.com/c/900423945297",
			"https://classroom.google.com:443/c/900423945297",
			"https://user@classroom.google.com/c/900423945297",
			"https://classroom.google.com/c/900423945297?course=1",
			"https://classroom.google.com/c/900423945297#course",
		} {
			_, err := classroom.NormalizeReference(reference)
			Expect(err).To(MatchError(ContainSubstring(classroom.ErrInvalidCourseReference.Error())))
			Expect(errors.Is(err, classroom.ErrInvalidCourseReference)).To(BeTrue())
		}
	})

	It("rejects Classroom URLs with arbitrary or malformed paths", func() {
		for _, reference := range []string{
			"https://classroom.google.com/u/0/h",
			"https://classroom.google.com/c",
			"https://classroom.google.com/c/",
			"https://classroom.google.com/c/900423945297/extra",
		} {
			_, err := classroom.NormalizeReference(reference)
			Expect(err).To(MatchError(ContainSubstring(classroom.ErrInvalidCourseReference.Error())))
			Expect(errors.Is(err, classroom.ErrInvalidCourseReference)).To(BeTrue())
		}
	})
})
