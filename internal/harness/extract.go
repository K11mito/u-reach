package harness

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

var (
	trailingSpaceRe = regexp.MustCompile(`[ \t]+\n`)
	manyBlankRe     = regexp.MustCompile(`\n{3,}`)
)

// extract returns the plain text of an uploaded playbook file and its kind.
func extract(name string, data []byte) (text, kind string, err error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		kind = "md"
	case ".txt":
		kind = "txt"
	case ".pdf":
		kind = "pdf"
	default:
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return "", "", fmt.Errorf("%w: upload a .md, .txt or .pdf file", ErrInvalid)
		}
		kind = "pdf"
	}
	if kind == "pdf" {
		if text, err = pdfText(data); err != nil {
			return "", "", err
		}
	} else {
		if !utf8.Valid(data) {
			return "", "", fmt.Errorf("%w: %s isn't plain UTF-8 text", ErrInvalid, name)
		}
		text = strings.TrimPrefix(string(data), "\ufeff")
	}
	text = tidy(text)
	if strings.TrimSpace(text) == "" {
		if kind == "pdf" {
			return "", "", fmt.Errorf("%w: no text found in %s. Is it a scanned PDF? Export it with selectable text, or paste it into a .md file", ErrInvalid, name)
		}
		return "", "", fmt.Errorf("%w: %s is empty", ErrInvalid, name)
	}
	return text, kind, nil
}

// pdfText reads the text of every page. The parser can panic on malformed
// files, so that's turned into an ordinary error.
func pdfText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: couldn't read this PDF (%v)", ErrInvalid, r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("%w: couldn't read this PDF (%v)", ErrInvalid, err)
	}
	var b strings.Builder
	fonts := map[string]*pdf.Font{}
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		for _, name := range p.Fonts() {
			if _, ok := fonts[name]; !ok {
				f := p.Font(name)
				fonts[name] = &f
			}
		}
		s, err := p.GetPlainText(fonts)
		if err != nil {
			return "", fmt.Errorf("%w: couldn't read page %d of this PDF (%v)", ErrInvalid, i, err)
		}
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	return b.String(), nil
}

func tidy(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = trailingSpaceRe.ReplaceAllString(s, "\n")
	s = manyBlankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
