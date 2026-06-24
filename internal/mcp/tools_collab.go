package mcp

import (
	"context"

	"github.com/03-CiprianoG/jeera/internal/core"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- add_comment -------------------------------------------------------------

type AddCommentArgs struct {
	Key    string `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
	Body   string `json:"body" jsonschema:"the Markdown comment body"`
	Author string `json:"author,omitempty" jsonschema:"defaults to human; agent runs pass run:<id>"`
}

type CommentResult struct {
	Comment CommentDTO `json:"comment"`
}

func (svc *Service) addComment(_ context.Context, _ *mcpsdk.CallToolRequest, args AddCommentArgs) (*mcpsdk.CallToolResult, CommentResult, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, CommentResult{}, err
	}
	c, err := svc.store.AddComment(core.Comment{IssueID: iss.ID, Author: args.Author, Body: args.Body})
	if err != nil {
		return nil, CommentResult{}, err
	}
	return nil, CommentResult{Comment: CommentDTO{Author: c.Author, Body: c.Body, CreatedAt: formatTime(c.CreatedAt)}}, nil
}

// --- add_attachment ----------------------------------------------------------

type AddAttachmentArgs struct {
	Key string `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
	Ref string `json:"ref" jsonschema:"a URL or an absolute file path to attach"`
}

type AttachmentDTO struct {
	Filename string `json:"filename"`
	Path     string `json:"path"`
	MIME     string `json:"mime,omitempty"`
	IsURL    bool   `json:"is_url"`
}

type AttachmentResult struct {
	Attachment AttachmentDTO `json:"attachment"`
}

func (svc *Service) addAttachment(_ context.Context, _ *mcpsdk.CallToolRequest, args AddAttachmentArgs) (*mcpsdk.CallToolResult, AttachmentResult, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, AttachmentResult{}, err
	}
	a := core.ClassifyAttachment(args.Ref)
	a.IssueID = iss.ID
	saved, err := svc.store.CreateAttachment(a)
	if err != nil {
		return nil, AttachmentResult{}, err
	}
	return nil, AttachmentResult{Attachment: AttachmentDTO{
		Filename: saved.Filename, Path: saved.Path, MIME: saved.MIME, IsURL: saved.IsURL(),
	}}, nil
}

// --- link_issues -------------------------------------------------------------

type LinkIssuesArgs struct {
	Source string `json:"source" jsonschema:"the source issue key, e.g. JEE-12"`
	Target string `json:"target" jsonschema:"the target issue key, e.g. JEE-34"`
	Type   string `json:"type" jsonschema:"the relationship: blocks, blocked_by, relates, or duplicates"`
}

type LinkResult struct {
	OK     bool   `json:"ok"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

func (svc *Service) linkIssues(_ context.Context, _ *mcpsdk.CallToolRequest, args LinkIssuesArgs) (*mcpsdk.CallToolResult, LinkResult, error) {
	source, err := svc.resolveIssue(args.Source)
	if err != nil {
		return nil, LinkResult{}, err
	}
	target, err := svc.resolveIssue(args.Target)
	if err != nil {
		return nil, LinkResult{}, err
	}
	if _, err := svc.store.CreateLink(core.IssueLink{
		SourceID: source.ID,
		TargetID: target.ID,
		Type:     core.LinkType(args.Type),
	}); err != nil {
		return nil, LinkResult{}, err
	}
	return nil, LinkResult{OK: true, Source: source.Key, Target: target.Key, Type: args.Type}, nil
}
