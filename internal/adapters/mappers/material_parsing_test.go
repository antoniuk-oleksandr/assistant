package mappers

import (
	"testing"

	"github.com/alex/assistent/internal/domain/submission"
)

func TestMaterialParsingRoundTripsAndHandlesNil(t *testing.T) {
	if value, err := EncodeMaterialParsing(nil); err != nil || value != nil {
		t.Fatalf("EncodeMaterialParsing(nil) = %v, %v", value, err)
	}
	if value, err := DecodeMaterialParsing(nil); err != nil || value != nil {
		t.Fatalf("DecodeMaterialParsing(nil) = %v, %v", value, err)
	}

	parsed := &submission.ParseResult{ParserType: "xlsx", Issue: "warning"}
	encoded, err := EncodeMaterialParsing(parsed)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMaterialParsing(encoded)
	if err != nil || decoded.ParserType != parsed.ParserType || decoded.Issue != parsed.Issue {
		t.Fatalf("round trip = %#v, %v", decoded, err)
	}
	if _, err := DecodeMaterialParsing(new(string)); err == nil {
		t.Fatal("invalid JSON unexpectedly decoded")
	}

	outcome := submission.MaterialParse{Result: *parsed}
	data, err := EncodeMaterialParse(outcome)
	if err != nil {
		t.Fatal(err)
	}
	decodedOutcome, err := DecodeMaterialParse(data)
	if err != nil || decodedOutcome.Result.ParserType != outcome.Result.ParserType {
		t.Fatalf("outcome round trip = %#v, %v", decodedOutcome, err)
	}
	if _, err := DecodeMaterialParse("{"); err == nil {
		t.Fatal("invalid outcome JSON unexpectedly decoded")
	}
}
