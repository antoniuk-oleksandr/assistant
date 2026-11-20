package main

import (
	"io"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAssistent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Assistent Suite")
}

var _ = Describe("the application entry point", func() {
	It("prints the greeting", func() {
		originalStdout := os.Stdout
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		os.Stdout = writer
		main()

		Expect(writer.Close()).To(Succeed())
		os.Stdout = originalStdout

		output, err := io.ReadAll(reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(output)).To(Equal("Hello, world!\n"))
		Expect(reader.Close()).To(Succeed())
	})
})
