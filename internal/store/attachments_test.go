package store

import (
	"errors"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestAttachmentLifecycle(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "with files")

	file := core.ClassifyAttachment("/docs/spec.pdf")
	file.IssueID = iss.ID
	file.Size = 1234
	saved, err := s.CreateAttachment(file)
	if err != nil {
		t.Fatalf("CreateAttachment(file): %v", err)
	}
	if saved.ID == 0 || saved.MIME != "application/pdf" {
		t.Errorf("file attachment not saved right: %+v", saved)
	}

	link := core.ClassifyAttachment("https://example.com/design")
	link.IssueID = iss.ID
	if _, err := s.CreateAttachment(link); err != nil {
		t.Fatalf("CreateAttachment(url): %v", err)
	}

	list, err := s.ListAttachments(iss.ID)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(list))
	}
	// Newest first: the URL was added last.
	if !list[0].IsURL() {
		t.Errorf("expected the URL attachment first, got %+v", list[0])
	}

	if err := s.DeleteAttachment(saved.ID); err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	if list, _ := s.ListAttachments(iss.ID); len(list) != 1 {
		t.Errorf("expected 1 attachment after delete, got %d", len(list))
	}
	if err := s.DeleteAttachment(saved.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting a missing attachment should be ErrNotFound, got %v", err)
	}
}

// Creating and deleting an attachment must publish issue-changed events so the
// detail view refreshes live.
func TestAttachmentEventsPublished(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "watched")

	ch, cancel := s.Subscribe()
	defer cancel()

	a := core.ClassifyAttachment("https://example.com/x")
	a.IssueID = iss.ID
	saved, err := s.CreateAttachment(a)
	if err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}
	if ev := waitEvent(t, ch); ev.Type != core.EventIssueUpdated || ev.IssueID != iss.ID {
		t.Errorf("create should publish issue.updated for %d, got %+v", iss.ID, ev)
	}

	if err := s.DeleteAttachment(saved.ID); err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	if ev := waitEvent(t, ch); ev.Type != core.EventIssueUpdated || ev.IssueID != iss.ID {
		t.Errorf("delete should publish issue.updated for %d, got %+v", iss.ID, ev)
	}
}

func TestCreateAttachmentValidates(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateAttachment(core.Attachment{Path: "/x", Filename: "x"}); err == nil {
		t.Error("an attachment with no issue should be rejected")
	}
}

func TestAttachmentCascadesWithIssue(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "doomed")
	a := core.ClassifyAttachment("/x.png")
	a.IssueID = iss.ID
	s.CreateAttachment(a)

	if err := s.DeleteIssue(iss.ID); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	if list, _ := s.ListAttachments(iss.ID); len(list) != 0 {
		t.Errorf("attachments should cascade-delete with the issue, got %d", len(list))
	}
}
