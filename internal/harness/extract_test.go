package harness

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// pdfDoc builds a minimal PDF from numbered objects (1 is the catalog).
func pdfDoc(objs ...string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func stream(s string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
}

// Each page names its font /F1, but they're different fonts: page 2's maps
// the byte "A" to the glyph "Z". Reading page 2 with page 1's font gets it wrong.
func TestPDFReadsEachPageWithItsOwnFonts(t *testing.T) {
	text := "BT /F1 12 Tf 10 100 Td (A) Tj ET"
	data := pdfDoc(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> >> /Contents 7 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 6 0 R >> >> /Contents 8 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Type /Encoding /Differences [65 /Z] >> >>",
		stream(text),
		stream(text),
	)
	got, kind, err := extract("playbook.pdf", data)
	if err != nil || kind != "pdf" {
		t.Fatal(kind, err)
	}
	if pages := strings.Fields(got); len(pages) != 2 || pages[0] != "A" || pages[1] != "Z" {
		t.Fatalf("want page 1 %q and page 2 %q, got %q", "A", "Z", got)
	}
}
