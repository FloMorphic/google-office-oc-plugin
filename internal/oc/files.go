package oc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// This file holds the shared Google Drive plumbing. Listing an account's files
// and mapping a user's name/URL reference to a bare id is a DRIVE operation, and
// every product uses it: the Sheets/Docs pickers list their files through Drive
// (sheets.go, docs.go build on filesOfType / resolveFileID), and the Drive node
// itself works in generic files and folders (the helpers at the bottom). Keeping
// it in one place is why one connected account can back every node.

// actionListFiles is the Google Drive action that lists files. Listing the
// account's Sheets or Docs FILES is a Drive operation (the Sheets/Docs APIs
// cannot); filtering to a mime type gives just that product's files. It backs
// the "Load spreadsheets" / "Load documents" / "Load files" pickers.
const actionListFiles = "googledrive.files.list"

// The Google Drive mime types of the files this plugin lists.
const (
	mimeSpreadsheet = "application/vnd.google-apps.spreadsheet"
	mimeDocument    = "application/vnd.google-apps.document"
	mimeFolder      = "application/vnd.google-apps.folder"
)

// driveURLRe pulls a bare Drive id out of any pasted Drive URL, whatever the
// product: a file (…/file/d/<ID>/view), a folder (…/drive/folders/<ID> or
// …/folders/<ID>), a Workspace doc (…/document/d/<ID>, …/spreadsheets/d/<ID>),
// or an "open" link (…/open?id=<ID> / …?id=<ID>). It is the Drive-node analog of
// the per-product URL regexes: a Drive file reference can be any of these.
var driveURLRe = regexp.MustCompile(`(?:/d/|/folders/|[?&]id=)([A-Za-z0-9_-]+)`)

// DriveFile is one Drive file the account can see: its id (what a Sheets/Docs/
// Drive action wants as the file id) and its name.
type DriveFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// listFiles runs one googledrive.files.list query and reads its
// { files:[{id,name}] } output leniently, so a schema tweak degrades to "no
// files" rather than an error. The `q` is caller-built; results are
// most-recently-modified first. Needs Drive read scope, or the gateway answers
// 403.
func (s *Session) listFiles(q string, pageSize int) ([]DriveFile, error) {
	raw, err := s.Do(actionListFiles, map[string]any{
		"q":        q,
		"orderBy":  "modifiedTime desc",
		"pageSize": pageSize,
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Files []DriveFile `json:"files"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out.Files, nil
}

// filesOfType lists up to 100 of the account's non-trashed files of one mime
// type, newest first — for the "Load …" file pickers.
func (s *Session) filesOfType(mime string) ([]DriveFile, error) {
	return s.listFiles(fmt.Sprintf("mimeType='%s' and trashed=false", mime), 100)
}

// driveNameQuoteRe escapes a Drive `q` string literal: a name may contain single
// quotes or backslashes, both of which must be backslash-escaped or the query is
// a 400 (or, worse, injects extra clauses).
var driveNameQuoteRe = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

// looksLikeID matches a bare Drive file id: 40+ URL-safe base64 chars, which no
// human-typed file NAME realistically is. A reference that looks like an id
// skips the Drive name lookup, so a pasted id or an upstream token still
// resolves for an account that has only Sheets/Docs scope (no Drive read
// access).
var looksLikeID = regexp.MustCompile(`^[A-Za-z0-9_-]{40,}$`)

// filesNamed lists the account's non-trashed files whose title is exactly name —
// case-sensitive, as Drive stores it — newest first. An empty mime spans every
// file type (the Drive node's generic file reference), a non-empty one narrows
// to that product (Sheets/Docs/folders). An empty name yields nil. name is
// quote-escaped for the `q` literal.
func (s *Session) filesNamed(name, mime string) ([]DriveFile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	q := fmt.Sprintf("name = '%s' and trashed=false", driveNameQuoteRe.Replace(name))
	if mime != "" {
		q = fmt.Sprintf("name = '%s' and mimeType='%s' and trashed=false", driveNameQuoteRe.Replace(name), mime)
	}
	return s.listFiles(q, 10)
}

// resolveFileID turns whatever a user gave for a file — a name (picked from the
// list or typed), a pasted URL, or a bare id / {{$.path}} token that resolved to
// one — into the bare id the Sheets/Docs/Drive actions need. This is where the
// plugin, not the user, owns the name→id mapping:
//
//   - a pasted URL yields its id directly (via urlRe);
//   - a reference that already looks like an id is used as-is (no Drive call, so
//     an id or token works even without Drive scope);
//   - otherwise the reference is looked up as a file NAME in Drive: one match
//     yields its id; several same-named files are an ambiguity error; no match
//     falls back to using the reference as an id (a clear 404 follows if it is
//     neither a name nor an id).
//
// kind ("spreadsheet"/"document"/"file"/"folder") only shapes the ambiguity
// message. An empty mime spans every file type. The name lookup needs Drive read
// scope. An empty ref is returned unchanged so the caller's required-input check
// owns that message.
func (s *Session) resolveFileID(ref, kind, mime string, urlRe *regexp.Regexp) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	if m := urlRe.FindStringSubmatch(ref); m != nil {
		return m[1], nil
	}
	if looksLikeID.MatchString(ref) {
		return ref, nil
	}
	files, err := s.filesNamed(ref, mime)
	if err != nil {
		return "", err
	}
	switch len(files) {
	case 0:
		return ref, nil // not a known name — assume it is already an id
	case 1:
		return files[0].ID, nil
	default:
		return "", fmt.Errorf("%d %ss are named %q — rename one so the name is unique, or paste its id / URL instead", len(files), kind, ref)
	}
}

// firstFile adapts a name lookup to the "reuse existing" callers: the newest
// match, or (nil, nil) when the list is empty (or the lookup errored).
func firstFile(files []DriveFile, err error) (*DriveFile, error) {
	if err != nil || len(files) == 0 {
		return nil, err
	}
	return &files[0], nil
}

// ------------------------------------------------------------- drive node --
// The Drive node speaks in generic files (any type) and folders, rather than one
// product's mime type. These reuse the same list/name-lookup plumbing as the
// Sheets/Docs pickers, just without (or with the folder) mime filter.

// DriveFiles lists up to 100 of the account's non-trashed files for the fileId
// picker, folders excluded (a folder is picked with Folders, and Drive cannot
// copy/export one). Newest first. Needs Drive read scope.
func (s *Session) DriveFiles() ([]DriveFile, error) {
	return s.listFiles(fmt.Sprintf("trashed=false and mimeType != '%s'", mimeFolder), 100)
}

// Folders lists up to 100 of the account's non-trashed folders for a folder
// (parent/destination) picker. See filesOfType. Needs Drive read scope.
func (s *Session) Folders() ([]DriveFile, error) { return s.filesOfType(mimeFolder) }

// FindFolderByName returns the account's newest non-trashed folder whose title
// is exactly name, or (nil, nil) when none exists. It powers the create-folder
// action's "reuse existing" option (Drive allows duplicate folder names).
func (s *Session) FindFolderByName(name string) (*DriveFile, error) {
	return firstFile(s.filesNamed(name, mimeFolder))
}

// ResolveDriveFileID resolves a Drive file reference (name / URL / id / token) to
// the bare id the Drive actions need, across any file type. See resolveFileID.
func (s *Session) ResolveDriveFileID(ref string) (string, error) {
	return s.resolveFileID(ref, "file", "", driveURLRe)
}

// ResolveFolderID resolves a folder reference (name / URL / id / token) to the
// bare id the Drive actions need, narrowed to folders. See resolveFileID.
func (s *Session) ResolveFolderID(ref string) (string, error) {
	return s.resolveFileID(ref, "folder", mimeFolder, driveURLRe)
}

// FileParents returns the ids of a file's current parent folders — what a move
// (files.update) must pass as removeParents to detach the file from where it is
// before addParents attaches it to the destination. Read leniently, so a missing
// parents field degrades to "no parents" rather than an error.
func (s *Session) FileParents(fileID string) ([]string, error) {
	raw, err := s.Do("googledrive.files.get", map[string]any{
		"fileId":              fileID,
		"includeSharedDrives": true,
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Parents []string `json:"parents"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out.Parents, nil
}
