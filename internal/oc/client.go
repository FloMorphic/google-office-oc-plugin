// Package oc is the plugin's bridge to OpenConnector, over FloMorphic's central
// credential.
//
// The FloMorphic backend is a GENERIC proxy: it stores the OpenConnector
// connection (a connected Google account, authorized via OAuth in Connect) and
// forwards a request verbatim over the single NATS subject
// `flomorphic.svc.oc.proxy`, injecting the auth header. It knows nothing about
// Google — ALL of that lives here. This plugin holds no Google token and makes
// no Google API calls; it builds OpenConnector requests and lets the backend
// execute them.
//
// Reaching the proxy needs an OPEN (multi) runtime credential — a strict,
// plugin-scoped credential cannot publish on `flomorphic.>`. See the README.
package oc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/nats-io/nats.go"
)

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

// ServicePrefix is the family this plugin targets. A connected Google account
// reports a service id that starts with "google" (googlesheets, googledrive,
// googlecalendar, googledocs, or a single "google" umbrella connection), so one
// authorized account can back every node in this plugin. Filtering on the prefix
// keeps unrelated connections out of the account picker while tolerating whichever
// exact id the gateway assigns.
const ServicePrefix = "google"

// proxySubject is the FloMorphic OpenConnector NATS proxy (see the backend's
// inflow/ocproxy.go). It sits OUTSIDE the inflow node protocol.
const proxySubject = "flomorphic.svc.oc.proxy"

// Sender is a NATS request/reply, satisfied by *sdkv1.Plugin.Send: the plugin's
// connection carries the round-trip (with retry + the configured deadline).
type Sender func(subject string, data []byte) (*nats.Msg, error)

// Client runs every OpenConnector request through the FloMorphic NATS proxy.
type Client struct {
	send Sender
}

func New(send Sender) *Client { return &Client{send: send} }

// Account is one connected Google account, from GET /v1/connections.
type Account struct {
	ID           string `json:"id"`
	Service      string `json:"service"`
	Status       string `json:"status"`
	AccountLabel string `json:"accountLabel"`
	Alias        string `json:"alias"`
	AuthType     string `json:"authType"`
	IsDefault    bool   `json:"isDefault"`
}

// Name is the human label for an account, falling back to the alias then the
// service when the gateway gives no friendly label.
func (a Account) Name() string {
	switch {
	case a.AccountLabel != "":
		return a.AccountLabel
	case a.Alias != "":
		return a.Alias
	default:
		return a.Service
	}
}

// proxyRequest mirrors the backend's OCProxyRequest.
type proxyRequest struct {
	Connection string            `json:"connection,omitempty"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      map[string]string `json:"query,omitempty"`
	Body       any               `json:"body,omitempty"`
}

// proxyReply mirrors the backend's OCProxyResponse.
type proxyReply struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// proxy forwards one gateway request and returns its (unwrapped) response body.
func (c *Client) proxy(method, path, connection string, query map[string]string, body any) (json.RawMessage, error) {
	payload, err := json.Marshal(proxyRequest{
		Connection: connection,
		Method:     method,
		Path:       path,
		Query:      query,
		Body:       body,
	})
	if err != nil {
		return nil, fmt.Errorf("oc: encode proxy request: %w", err)
	}

	msg, err := c.send(proxySubject, payload)
	if err != nil {
		return nil, fmt.Errorf("oc: proxy round-trip failed: %w", err)
	}

	var reply proxyReply
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &reply); err != nil {
			return nil, fmt.Errorf("oc: decode proxy reply: %w", err)
		}
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("%s", reply.Error)
	}
	if reply.Status >= 400 {
		return nil, fmt.Errorf("%s failed: %s", strings.TrimPrefix(path, "/v1/actions/"), gatewayError(reply.Body, reply.Status))
	}
	return reply.Body, nil
}

// gatewayError renders a 4xx/5xx into the most specific message the gateway
// gave — Google's own "Unable to parse range" rather than a bare status code —
// by digging the description out of the gateway's error envelope, and falling
// back to the raw body then the status when it has no message.
func gatewayError(body json.RawMessage, status int) string {
	if len(body) > 0 {
		var env struct {
			Message     string `json:"message"`
			Error       string `json:"error"`
			Description string `json:"description"`
			Data        struct {
				Message     string `json:"message"`
				Description string `json:"description"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &env) == nil {
			for _, m := range []string{env.Description, env.Message, env.Error, env.Data.Description, env.Data.Message} {
				if strings.TrimSpace(m) != "" {
					return fmt.Sprintf("%s (HTTP %d)", strings.TrimSpace(m), status)
				}
			}
		}
		if s := strings.TrimSpace(string(body)); s != "" && len(s) <= 300 {
			return fmt.Sprintf("HTTP %d: %s", status, s)
		}
	}
	return fmt.Sprintf("HTTP %d", status)
}

// Accounts lists the connected Google accounts (GET /v1/connections, filtered to
// the Google service family).
func (c *Client) Accounts(connection string) ([]Account, error) {
	raw, err := c.proxy("GET", "/v1/connections", connection, nil, nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data []Account `json:"data"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("oc: decode connections: %w", err)
		}
	}
	out := make([]Account, 0, len(env.Data))
	for _, a := range env.Data {
		if strings.HasPrefix(a.Service, ServicePrefix) {
			out = append(out, a)
		}
	}
	return out, nil
}

// Resolve returns one account by alias (or the default when alias is empty). A
// nil account with a nil error means "nothing connected / no such alias" — a
// caller decides how to phrase that.
func (c *Client) Resolve(alias, connection string) (*Account, error) {
	list, err := c.Accounts(connection)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	wanted := strings.TrimSpace(alias)
	if wanted == "" {
		for i := range list {
			if list[i].IsDefault {
				return &list[i], nil
			}
		}
		return &list[0], nil
	}
	for i := range list {
		if list[i].Alias == wanted || list[i].AccountLabel == wanted {
			return &list[i], nil
		}
	}
	return nil, nil
}

// Bind resolves the account a node is bound to and returns a Session: an
// OpenConnector handle scoped to that account, the analog of a ready API client.
// The error is already phrased for the user.
func (c *Client) Bind(alias, connection string) (*Session, error) {
	acct, err := c.Resolve(alias, connection)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		if strings.TrimSpace(alias) != "" {
			return nil, fmt.Errorf("no connected Google account with alias %q — pick one in the node's settings", alias)
		}
		return nil, fmt.Errorf("no Google account connected in FloMorphic → Connect")
	}
	return &Session{client: c, Account: *acct, connection: connection}, nil
}

// Session is the OpenConnector client bound to one resolved Google account. An
// action calls Do with a fully-qualified OpenConnector action id and its input;
// the backend runs it as this account and returns the gateway's raw `data`
// payload.
type Session struct {
	client     *Client
	Account    Account
	connection string
}

// Do runs one action as this account —
// POST /v1/actions/<service>.<action> {input}. `action` is the fully-qualified
// id, e.g. "googlesheets.create_google_sheet1".
func (s *Session) Do(action string, input any) (json.RawMessage, error) {
	var query map[string]string
	if s.Account.Alias != "" {
		query = map[string]string{"alias": s.Account.Alias}
	}
	raw, err := s.client.proxy("POST", fmt.Sprintf("/v1/actions/%s", action), s.connection, query, map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	// Unwrap the gateway's {data} envelope when present, else return the body.
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &env) == nil && len(env.Data) > 0 {
		return env.Data, nil
	}
	return raw, nil
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

// actionListFiles is the Google Drive action that lists files. Listing the
// account's spreadsheet FILES is a Drive operation (the Sheets API cannot);
// filtering to the Sheets mime type gives just the spreadsheets. It backs the
// "Load spreadsheets" picker on the spreadsheetId field.
const actionListFiles = "googledrive.files.list"

// mimeSpreadsheet is the Google Drive mime type of a Sheets file.
const mimeSpreadsheet = "application/vnd.google-apps.spreadsheet"

// DriveFile is one Drive file the account can see: its id (what a Sheets action
// wants as spreadsheetId) and its name.
type DriveFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Spreadsheets lists the account's Google Sheets files (most-recently-modified
// first) as {id, name}, for the spreadsheetId picker. It filters Drive's file
// list to the Sheets mime type with the `q` param (verified against the live
// googledrive.files.list action), and reads the { files:[{id,name}] } output.
// The account must have been connected with Drive read scope, or the gateway
// answers 403.
func (s *Session) Spreadsheets() ([]DriveFile, error) {
	input := map[string]any{
		"q":        fmt.Sprintf("mimeType='%s' and trashed=false", mimeSpreadsheet),
		"orderBy":  "modifiedTime desc",
		"pageSize": 100,
	}
	raw, err := s.Do(actionListFiles, input)
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

// driveNameQuoteRe escapes a Drive `q` string literal: a name may contain single
// quotes or backslashes, both of which must be backslash-escaped or the query is
// a 400 (or, worse, injects extra clauses).
var driveNameQuoteRe = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

// looksLikeID matches a bare spreadsheet id: 40+ URL-safe base64 chars, which no
// human-typed spreadsheet NAME realistically is. A reference that looks like an
// id skips the Drive name lookup, so a pasted id or an upstream
// {{$.spreadsheetId}} token still resolves for an account that has only Sheets
// scope (no Drive read access).
var looksLikeID = regexp.MustCompile(`^[A-Za-z0-9_-]{40,}$`)

// spreadsheetsNamed lists the account's non-trashed Sheets files whose title is
// exactly name — case-sensitive, as Drive stores it — newest first. An empty
// name yields nil. name is quote-escaped for the `q` literal. Needs Drive read
// scope, or the gateway answers 403.
func (s *Session) spreadsheetsNamed(name string) ([]DriveFile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	input := map[string]any{
		"q":        fmt.Sprintf("name = '%s' and mimeType='%s' and trashed=false", driveNameQuoteRe.Replace(name), mimeSpreadsheet),
		"orderBy":  "modifiedTime desc",
		"pageSize": 10,
	}
	raw, err := s.Do(actionListFiles, input)
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

// FindSpreadsheetByName returns the account's newest non-trashed Sheets file
// whose title is exactly name, or (nil, nil) when none exists. It powers the
// create action's "reuse existing" option: the Sheets create API always makes a
// new file (Drive allows duplicate names), so the dedupe is a Drive name lookup.
func (s *Session) FindSpreadsheetByName(name string) (*DriveFile, error) {
	files, err := s.spreadsheetsNamed(name)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	return &files[0], nil
}

// ResolveSpreadsheetID turns whatever a user gave for a spreadsheet — a name
// (picked from the list or typed), a pasted URL, or a bare id / {{$.path}} token
// that resolved to one — into the bare id the Sheets actions need. This is where
// the plugin, not the user, owns the name→id mapping:
//
//   - a pasted Sheets URL yields its id directly;
//   - a reference that already looks like an id is used as-is (no Drive call, so
//     an id or token works even without Drive scope);
//   - otherwise the reference is looked up as a file NAME in Drive: one match
//     yields its id; several same-named files are an ambiguity error; no match
//     falls back to using the reference as an id (a clear 404 follows if it is
//     neither a name nor an id).
//
// The name lookup needs Drive read scope. An empty ref is returned unchanged so
// the caller's required-input check owns that message.
func (s *Session) ResolveSpreadsheetID(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	if m := sheetURLRe.FindStringSubmatch(ref); m != nil {
		return m[1], nil
	}
	if looksLikeID.MatchString(ref) {
		return ref, nil
	}
	files, err := s.spreadsheetsNamed(ref)
	if err != nil {
		return "", err
	}
	switch len(files) {
	case 0:
		return ref, nil // not a known name — assume it is already an id
	case 1:
		return files[0].ID, nil
	default:
		return "", fmt.Errorf("%d spreadsheets are named %q — rename one so the name is unique, or paste its id / URL instead", len(files), ref)
	}
}
