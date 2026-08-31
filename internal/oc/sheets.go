package oc

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Google Sheets session helpers: normalizing a spreadsheet reference to a bare
// id, listing a spreadsheet's tabs for the "Load sheets" picker, and the
// Drive-backed file pickers/resolvers the Sheets forms use. The generic file
// plumbing they build on lives in files.go.

// sheetURLRe pulls the bare spreadsheet id out of a pasted Google Sheets URL
// (…/spreadsheets/d/<ID>/edit…). Ids are URL-safe base64: letters, digits, - _.
var sheetURLRe = regexp.MustCompile(`/spreadsheets/d/([A-Za-z0-9_-]+)`)

// SpreadsheetID normalizes a spreadsheet reference to the bare id the Sheets API
// wants: if a full URL was pasted (a very common mistake that Google answers
// with a bare 404), the id is extracted; otherwise the trimmed input is returned
// unchanged.
func SpreadsheetID(ref string) string {
	ref = strings.TrimSpace(ref)
	if m := sheetURLRe.FindStringSubmatch(ref); m != nil {
		return m[1]
	}
	return ref
}

// actionGetSheetNames is the read action that lists a spreadsheet's sheets (tabs)
// and returns a stable name→sheetId map. It backs the "Load sheets" / "Check"
// dependent-field pickers (GET /v1/actions?service=googlesheets).
const actionGetSheetNames = "googlesheets.get_sheet_names"

// SheetTab is one sheet (tab) inside a spreadsheet: its human title (what
// get_values and aggregate speak in) and its numeric id (what the
// batch/dimension/chart actions want).
type SheetTab struct {
	ID    int
	Title string
}

// Sheets lists a spreadsheet's tabs as this account, so a picker can offer them
// by title (value) while showing the numeric sheetId (label). excludeHidden
// drops hidden tabs. The order follows the gateway's sheetNames; the id comes
// from its sheetIdByName map, and the output is read leniently so a schema tweak
// degrades to "no tabs" rather than an error.
func (s *Session) Sheets(spreadsheetID string, excludeHidden bool) ([]SheetTab, error) {
	spreadsheetID = SpreadsheetID(spreadsheetID)
	input := map[string]any{"spreadsheetId": spreadsheetID}
	if excludeHidden {
		input["excludeHidden"] = true
	}
	raw, err := s.Do(actionGetSheetNames, input)
	if err != nil {
		return nil, err
	}
	var out struct {
		SheetNames    []string       `json:"sheetNames"`
		SheetIDByName map[string]int `json:"sheetIdByName"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	tabs := make([]SheetTab, 0, len(out.SheetNames))
	for _, name := range out.SheetNames {
		tabs = append(tabs, SheetTab{ID: out.SheetIDByName[name], Title: name})
	}
	return tabs, nil
}

// Spreadsheets lists the account's Google Sheets files for the spreadsheetId
// picker. See filesOfType. Needs Drive read scope.
func (s *Session) Spreadsheets() ([]DriveFile, error) { return s.filesOfType(mimeSpreadsheet) }

// FindSpreadsheetByName returns the account's newest non-trashed Sheets file
// whose title is exactly name, or (nil, nil) when none exists. It powers the
// create action's "reuse existing" option: the Sheets create API always makes a
// new file (Drive allows duplicate names), so the dedupe is a Drive name lookup.
func (s *Session) FindSpreadsheetByName(name string) (*DriveFile, error) {
	return firstFile(s.filesNamed(name, mimeSpreadsheet))
}

// ResolveSpreadsheetID resolves a spreadsheet reference (name / URL / id / token)
// to the bare id the Sheets actions need. See resolveFileID.
func (s *Session) ResolveSpreadsheetID(ref string) (string, error) {
	return s.resolveFileID(ref, "spreadsheet", mimeSpreadsheet, sheetURLRe)
}
