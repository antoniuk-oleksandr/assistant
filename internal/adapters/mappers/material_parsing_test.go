package mappers

import (
	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Material parsing mapping", func() {
	It("round trips values and handles nil or invalid JSON", func() {
		value, err := EncodeMaterialParsing(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(BeNil())
		decodedNil, err := DecodeMaterialParsing(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(decodedNil).To(BeNil())

		parsed := &submission.ParseResult{ParserType: "xlsx", Issue: "warning"}
		encoded, err := EncodeMaterialParsing(parsed)
		Expect(err).NotTo(HaveOccurred())
		decoded, err := DecodeMaterialParsing(encoded)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.ParserType).To(Equal(parsed.ParserType))
		Expect(decoded.Issue).To(Equal(parsed.Issue))
		_, err = DecodeMaterialParsing(new(string))
		Expect(err).To(HaveOccurred())

		outcome := submission.MaterialParse{Result: *parsed}
		data, err := EncodeMaterialParse(outcome)
		Expect(err).NotTo(HaveOccurred())
		decodedOutcome, err := DecodeMaterialParse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(decodedOutcome.Result.ParserType).To(Equal(outcome.Result.ParserType))
		_, err = DecodeMaterialParse("{")
		Expect(err).To(HaveOccurred())
	})
})
