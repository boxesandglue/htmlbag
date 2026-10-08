package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

// A footnote is a Note under PDF/UA-1 and an FENote under PDF/UA-2: PDF 2.0
// SSN has no Note, and veraPDF rejected the namespace of every footnote.
func TestFootnoteRoleFollowsTheFormat(t *testing.T) {
	if role, ns := roleAndNS("Note", document.FormatPDFUA); role != "Note" || ns != "" {
		t.Errorf("PDF/UA-1: %s in %q, want Note in the default namespace", role, ns)
	}
	if role, ns := roleAndNS("Note", document.FormatPDFUA2); role != "FENote" || ns != document.NamespacePDF20SSN {
		t.Errorf("PDF/UA-2: %s in %q, want FENote in PDF 2.0 SSN", role, ns)
	}
}
