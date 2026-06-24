package core

import "testing"

func TestClassifyAttachmentURL(t *testing.T) {
	a := ClassifyAttachment("https://example.com/docs/spec.pdf")
	if !a.IsURL() {
		t.Errorf("https ref should be a URL, MIME=%q", a.MIME)
	}
	if a.Path != "https://example.com/docs/spec.pdf" {
		t.Errorf("URL path = %q", a.Path)
	}
	if a.Filename != "example.com/docs/spec.pdf" {
		t.Errorf("URL filename = %q", a.Filename)
	}
}

func TestClassifyAttachmentFile(t *testing.T) {
	a := ClassifyAttachment("/home/me/diagram.png")
	if a.IsURL() {
		t.Error("a file path should not be a URL")
	}
	if a.Filename != "diagram.png" {
		t.Errorf("filename = %q, want diagram.png", a.Filename)
	}
	if a.MIME != "image/png" {
		t.Errorf("mime = %q, want image/png", a.MIME)
	}
}

// A type that carries a charset parameter (text/html) must have it stripped, so
// the stored MIME is the bare media type.
func TestClassifyAttachmentStripsMIMECharset(t *testing.T) {
	a := ClassifyAttachment("/notes/readme.html")
	if a.MIME != "text/html" {
		t.Errorf("mime = %q, want bare text/html (charset stripped)", a.MIME)
	}
}

func TestClassifyAttachmentUnknownExtension(t *testing.T) {
	a := ClassifyAttachment("notes")
	if a.IsURL() || a.Filename != "notes" {
		t.Errorf("bare name classified wrong: %+v", a)
	}
	// Unknown/extensionless: MIME is empty, which is fine.
	if a.MIME != "" {
		t.Errorf("expected empty mime for an extensionless file, got %q", a.MIME)
	}
}

func TestAttachmentValidate(t *testing.T) {
	ok := Attachment{IssueID: 1, Path: "/x/y.png", Filename: "y.png"}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid attachment rejected: %v", err)
	}
	for _, bad := range []Attachment{
		{Path: "/x", Filename: "x"},               // no issue
		{IssueID: 1, Filename: "x"},               // no path
		{IssueID: 1, Path: "/x"},                  // no filename
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("expected validation error for %+v", bad)
		}
	}
}
