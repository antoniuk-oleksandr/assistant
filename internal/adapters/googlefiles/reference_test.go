package googlefiles_test

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	"testing"

	"github.com/alex/assistent/internal/adapters/googlefiles"
	. "github.com/onsi/gomega"
)

type referenceGinkgoTest struct{}

func (*referenceGinkgoTest) TempDir() string          { return GinkgoT().TempDir() }
func (*referenceGinkgoTest) Setenv(key, value string) { GinkgoT().Setenv(key, value) }
func (*referenceGinkgoTest) Cleanup(fn func())        { DeferCleanup(fn) }
func (*referenceGinkgoTest) Helper()                  {}
func (*referenceGinkgoTest) Fatal(args ...any)        { Expect(false).To(BeTrue(), fmt.Sprint(args...)) }
func (*referenceGinkgoTest) Fatalf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*referenceGinkgoTest) Errorf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*referenceGinkgoTest) Run(name string, fn func(*referenceGinkgoTest)) bool {
	By(name)
	fn(&referenceGinkgoTest{})
	return true
}

func testParseGoogleFileReference(t *referenceGinkgoTest) {
	const id = "file_1-abc"
	for _, tc := range []struct{ name, raw, id, key string }{
		{"Drive path", "https://drive.google.com/file/d/" + id + "/view?resourcekey=key-1", id, "key-1"},
		{"Drive open", "https://drive.google.com/open?id=" + id, id, ""},
		{"Drive download", "https://drive.google.com/uc?id=" + id, id, ""},
		{"Docs account path", "https://docs.google.com/document/u/1/d/" + id + "/edit", id, ""},
		{"Sheets path", "https://docs.google.com/spreadsheets/d/" + id + "/edit", id, ""},
		{"duplicate key", "https://drive.google.com/open?id=" + id + "&resourcekey=one&resourcekey=two", id, ""},
		{"invalid key", "https://drive.google.com/open?id=" + id + "&resourcekey=%2F", id, ""},
		{"duplicate ID", "https://drive.google.com/open?id=" + id + "&id=other", "", ""},
		{"invalid query escape", "https://drive.google.com/open?id=%zz", "", ""},
		{"invalid path escape", "https://drive.google.com/file/d/%zz", "", ""},
		{"invalid ID", "https://drive.google.com/open?id=file%2Fother", "", ""},
		{"published view", "https://docs.google.com/spreadsheets/d/e/" + id + "/pub", "", ""},
		{"foreign host", "https://drive.google.com.example.invalid/file/d/" + id + "/view", "", ""},
		{"embedded credentials", "https://fixture@drive.google.com/file/d/" + id + "/view", "", ""},
		{"insecure scheme", "http://drive.google.com/file/d/" + id + "/view", "", ""},
	} {
		t.Run(tc.name, func(t *referenceGinkgoTest) {
			NewWithT(t).Expect(googlefiles.ParseURL(tc.raw)).To(Equal(googlefiles.Reference{ID: tc.id, ResourceKey: tc.key}))
		})
	}
}

func TestReferenceGinkgoSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "reference")
}

var _ = Describe("reference legacy tests", func() {
	It("runs the converted standard-library cases", func() {
		t := &referenceGinkgoTest{}
		testParseGoogleFileReference(t)
	})
})
