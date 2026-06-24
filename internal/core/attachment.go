package core

import (
	"mime"
	"net/url"
	"path/filepath"
	"strings"
)

// AttachmentURIMIME marks an attachment that points at a URL rather than a local
// file. Jeera never stores binary contents — only the reference — so a "link" and
// an "uploaded file" are the same kind of record, distinguished by this MIME.
const AttachmentURIMIME = "text/uri-list"

// IsURL reports whether the attachment is a link rather than a local file.
func (a Attachment) IsURL() bool { return a.MIME == AttachmentURIMIME }

// ClassifyAttachment turns a user- or agent-supplied reference — a URL or a file
// path — into an Attachment. It fills Filename, Path and MIME; the caller sets
// IssueID and (for files) Size. A reference with an http/https scheme is recorded
// as a URL; anything else is treated as a file path, with the MIME guessed from
// its extension.
func ClassifyAttachment(ref string) Attachment {
	ref = strings.TrimSpace(ref)
	if u, err := url.Parse(ref); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		name := strings.TrimSuffix(u.Host+u.Path, "/")
		return Attachment{Filename: name, Path: ref, MIME: AttachmentURIMIME}
	}
	mimeType := mime.TypeByExtension(filepath.Ext(ref))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = mimeType[:i] // drop "; charset=utf-8" and the like
	}
	return Attachment{Filename: filepath.Base(ref), Path: ref, MIME: mimeType}
}

// Validate checks an attachment.
func (a Attachment) Validate() error {
	if a.IssueID == 0 {
		return invalidf("attachment must belong to an issue")
	}
	if strings.TrimSpace(a.Path) == "" {
		return invalidf("attachment needs a path or URL")
	}
	if strings.TrimSpace(a.Filename) == "" {
		return invalidf("attachment needs a filename")
	}
	return nil
}
