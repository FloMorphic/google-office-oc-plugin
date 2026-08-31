package oc

import "regexp"

// Google Docs session helpers: the Drive-backed file picker/resolver the Docs
// forms use. The generic file plumbing they build on lives in files.go.

// docURLRe pulls the bare document id out of a pasted Google Docs URL
// (…/document/d/<ID>/edit…), the Docs analog of sheetURLRe.
var docURLRe = regexp.MustCompile(`/document/d/([A-Za-z0-9_-]+)`)

// Documents lists the account's Google Docs files for the documentId picker.
// See filesOfType. Needs Drive read scope.
func (s *Session) Documents() ([]DriveFile, error) { return s.filesOfType(mimeDocument) }

// FindDocumentByName returns the account's newest non-trashed Docs file whose
// title is exactly name, or (nil, nil) when none exists. It powers the create
// action's "reuse existing" option, the Docs analog of FindSpreadsheetByName.
func (s *Session) FindDocumentByName(name string) (*DriveFile, error) {
	return firstFile(s.filesNamed(name, mimeDocument))
}

// ResolveDocumentID resolves a document reference (name / URL / id / token) to
// the bare id the Docs actions need. See resolveFileID.
func (s *Session) ResolveDocumentID(ref string) (string, error) {
	return s.resolveFileID(ref, "document", mimeDocument, docURLRe)
}
