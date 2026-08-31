package actions

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Inflowenger/go-plugin-sdk/formkit"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// Metas are the meta RPCs served outside the job lifecycle so a dialog can call
// them while it is open:
//   - list/test the connected Google account (settings form);
//   - resolve the dependent id fields an action form cannot ask a user to type:
//     a spreadsheet's sheets (tabs) and a check that a spreadsheet id resolves.
func (r *Registry) Metas() []sdkv1.Meta {
	return []sdkv1.Meta{
		{Method: "google.meta.account.list", RequestHandler: r.metaAccountList},
		{Method: "google.meta.account.test", RequestHandler: r.metaAccountTest},
		{Method: "googlesheets.meta.spreadsheets.list", RequestHandler: r.metaSpreadsheetsList},
		{Method: "googlesheets.meta.sheets.list", RequestHandler: r.metaSheetsList},
		{Method: "googledocs.meta.documents.list", RequestHandler: r.metaDocumentsList},
		{Method: "googledrive.meta.files.list", RequestHandler: r.metaFilesList},
		{Method: "googledrive.meta.folders.list", RequestHandler: r.metaFoldersList},
		{Method: "googlecalendar.meta.calendars.list", RequestHandler: r.metaCalendarsList},
	}
}

// metaSpreadsheetsList backs a "Load spreadsheets" button on a spreadsheetId
// field: it lists the bound account's Google Sheets files (via Drive) and
// REBUILDS the field into a drop-down of them (value = file id, label = name).
// The field stays typable, so a pasted id or a {{$.path}} token still works when
// the account lacks Drive scope (the button then reports the gateway's error).
func (r *Registry) metaSpreadsheetsList(req sdkv1.Request) any {
	body := decodeMeta(req.Data)
	target := text(body["targetField"])
	if target == "" {
		target = "spreadsheetId"
	}

	form, ok := sheetsFormByMethod[text(body["form"])]
	if !ok {
		return formkit.Failure("internal: no form to rebuild for this field").About(target).Patch(nil)
	}

	alias, connection := settingsFrom(body)
	sess, err := r.oc.Bind(alias, connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).About(target).Patch(nil)
	}
	files, err := sess.Spreadsheets()
	if err != nil {
		return formkit.Failure("%s%s  [as account %s]", err.Error(), driveScopeHint(err), sess.Account.Name()).About(target).Patch(nil)
	}
	if len(files) == 0 {
		return formkit.Warning("No spreadsheets found for account %s.", sess.Account.Name()).About(target).Patch(nil)
	}

	// The value written is the file NAME, not its id: the plugin resolves the
	// name back to an id at run time (Session.ResolveSpreadsheetID), so the user
	// never handles an id. The field stays typable, so a name not in this list
	// (or a pasted id / URL / {{$.path}} token) still works.
	options := make([]formkit.Option, 0, len(files))
	for _, f := range files {
		options = append(options, formkit.Option{Value: f.Name, Label: f.Name})
	}
	return formkit.Choose(
		form,
		target,
		options,
		formkit.FormData(body),
		formkit.Success("%d spreadsheet(s) for %s — pick one:", len(files), sess.Account.Name()).About(target),
	)
}

// driveScopeHint nudges toward the usual cause when the Drive list is refused:
// the connected account was not granted Drive scope.
func driveScopeHint(err error) string {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "403") || strings.Contains(msg, "permission") || strings.Contains(msg, "insufficient") || strings.Contains(msg, "scope") {
		return " — this needs Google Drive read access; reconnect the account in FloMorphic → Connect with Drive scope, or paste the file id directly."
	}
	return ""
}

// metaDocumentsList backs a "Load documents" button on a documentId field: it
// lists the bound account's Google Docs files (via Drive) and REBUILDS the field
// into a drop-down of them. Like the spreadsheets picker, the value written is
// the file NAME (resolved back to an id at run time by ResolveDocumentID), and
// the field stays typable so a pasted id / URL / {{$.path}} token still works
// when the account lacks Drive scope.
func (r *Registry) metaDocumentsList(req sdkv1.Request) any {
	body := decodeMeta(req.Data)
	target := text(body["targetField"])
	if target == "" {
		target = "documentId"
	}

	form, ok := docsFormByMethod[text(body["form"])]
	if !ok {
		return formkit.Failure("internal: no form to rebuild for this field").About(target).Patch(nil)
	}

	alias, connection := settingsFrom(body)
	sess, err := r.oc.Bind(alias, connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).About(target).Patch(nil)
	}
	files, err := sess.Documents()
	if err != nil {
		return formkit.Failure("%s%s  [as account %s]", err.Error(), driveScopeHint(err), sess.Account.Name()).About(target).Patch(nil)
	}
	if len(files) == 0 {
		return formkit.Warning("No documents found for account %s.", sess.Account.Name()).About(target).Patch(nil)
	}

	options := make([]formkit.Option, 0, len(files))
	for _, f := range files {
		options = append(options, formkit.Option{Value: f.Name, Label: f.Name})
	}
	return formkit.Choose(
		form,
		target,
		options,
		formkit.FormData(body),
		formkit.Success("%d document(s) for %s — pick one:", len(files), sess.Account.Name()).About(target),
	)
}

// metaFilesList backs a "Load files" button on a Drive fileId field: it lists the
// bound account's Drive files (any type, folders excluded) and REBUILDS the field
// into a drop-down of them. Like the other file pickers, the value written is the
// file NAME (resolved back to an id at run time by ResolveDriveFileID), and the
// field stays typable so a pasted id / URL / {{$.path}} token still works when the
// account lacks Drive scope.
func (r *Registry) metaFilesList(req sdkv1.Request) any {
	body := decodeMeta(req.Data)
	target := text(body["targetField"])
	if target == "" {
		target = "fileId"
	}

	form, ok := driveFormByMethod[text(body["form"])]
	if !ok {
		return formkit.Failure("internal: no form to rebuild for this field").About(target).Patch(nil)
	}

	alias, connection := settingsFrom(body)
	sess, err := r.oc.Bind(alias, connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).About(target).Patch(nil)
	}
	files, err := sess.DriveFiles()
	if err != nil {
		return formkit.Failure("%s%s  [as account %s]", err.Error(), driveScopeHint(err), sess.Account.Name()).About(target).Patch(nil)
	}
	if len(files) == 0 {
		return formkit.Warning("No files found for account %s.", sess.Account.Name()).About(target).Patch(nil)
	}

	options := make([]formkit.Option, 0, len(files))
	for _, f := range files {
		options = append(options, formkit.Option{Value: f.Name, Label: f.Name})
	}
	return formkit.Choose(
		form,
		target,
		options,
		formkit.FormData(body),
		formkit.Success("%d file(s) for %s — pick one:", len(files), sess.Account.Name()).About(target),
	)
}

// metaFoldersList backs a "Load folders" button on a Drive folder field (a parent
// or a move destination): it lists the bound account's folders and REBUILDS the
// target field into a drop-down of them. The value written is the folder NAME
// (resolved back to an id at run time by ResolveFolderID); the field stays typable
// so a pasted id / URL / {{$.path}} token still works.
func (r *Registry) metaFoldersList(req sdkv1.Request) any {
	body := decodeMeta(req.Data)
	target := text(body["targetField"])
	if target == "" {
		target = "folderId"
	}

	form, ok := driveFormByMethod[text(body["form"])]
	if !ok {
		return formkit.Failure("internal: no form to rebuild for this field").About(target).Patch(nil)
	}

	alias, connection := settingsFrom(body)
	sess, err := r.oc.Bind(alias, connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).About(target).Patch(nil)
	}
	folders, err := sess.Folders()
	if err != nil {
		return formkit.Failure("%s%s  [as account %s]", err.Error(), driveScopeHint(err), sess.Account.Name()).About(target).Patch(nil)
	}
	if len(folders) == 0 {
		return formkit.Warning("No folders found for account %s.", sess.Account.Name()).About(target).Patch(nil)
	}

	options := make([]formkit.Option, 0, len(folders))
	for _, f := range folders {
		options = append(options, formkit.Option{Value: f.Name, Label: f.Name})
	}
	return formkit.Choose(
		form,
		target,
		options,
		formkit.FormData(body),
		formkit.Success("%d folder(s) for %s — pick one:", len(folders), sess.Account.Name()).About(target),
	)
}

// metaCalendarsList backs a "Load calendars" button on a Calendar field: it lists
// the bound account's calendars and REBUILDS the target field into a drop-down of
// them. Unlike the file pickers, the value written is the calendar ID itself
// (label = summary), since that is what the event actions speak in; the field
// stays typable so "primary" or a pasted id still works.
func (r *Registry) metaCalendarsList(req sdkv1.Request) any {
	body := decodeMeta(req.Data)
	target := text(body["targetField"])
	if target == "" {
		target = "calendarId"
	}

	form, ok := calendarFormByMethod[text(body["form"])]
	if !ok {
		return formkit.Failure("internal: no form to rebuild for this field").About(target).Patch(nil)
	}

	alias, connection := settingsFrom(body)
	sess, err := r.oc.Bind(alias, connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).About(target).Patch(nil)
	}
	calendars, err := sess.Calendars()
	if err != nil {
		return formkit.Failure("%s  [as account %s]", err.Error(), sess.Account.Name()).About(target).Patch(nil)
	}
	if len(calendars) == 0 {
		return formkit.Warning("No calendars found for account %s.", sess.Account.Name()).About(target).Patch(nil)
	}

	options := make([]formkit.Option, 0, len(calendars))
	for _, c := range calendars {
		label := c.Summary
		if c.Primary {
			label += "  (primary)"
		}
		options = append(options, formkit.Option{Value: c.ID, Label: label})
	}
	return formkit.Choose(
		form,
		target,
		options,
		formkit.FormData(body),
		formkit.Success("%d calendar(s) for %s — pick one:", len(calendars), sess.Account.Name()).About(target),
	)
}

// metaSheetsList backs a "Load sheets" button on a sheetName / sheetId field: it
// reads the spreadsheetId the user already entered, lists that spreadsheet's tabs
// as the bound account, and REBUILDS the target field into a drop-down of them.
// The value written is the tab title (for sheetName fields) or the numeric
// sheetId (for sheetId fields); the label shows both.
func (r *Registry) metaSheetsList(req sdkv1.Request) any {
	body := decodeMeta(req.Data)
	target := text(body["targetField"])
	if target == "" {
		target = "sheetName"
	}

	form, ok := sheetsFormByMethod[text(body["form"])]
	if !ok {
		return formkit.Failure("internal: no form to rebuild for this field").About(target).Patch(nil)
	}

	spreadsheetRef := text(pick(body, "spreadsheetId"))
	if spreadsheetRef == "" {
		return formkit.Warning("Choose or type the Spreadsheet first, then press Load sheets.").About(target).Patch(nil)
	}

	alias, connection := settingsFrom(body)
	sess, err := r.oc.Bind(alias, connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).About(target).Patch(nil)
	}
	id, err := sess.ResolveSpreadsheetID(spreadsheetRef)
	if err != nil {
		return formkit.Failure("%s%s  [for spreadsheet %q as account %s]", err.Error(), driveScopeHint(err), spreadsheetRef, sess.Account.Name()).About(target).Patch(nil)
	}
	tabs, err := sess.Sheets(id, false)
	if err != nil {
		return formkit.Failure("%s%s  [tried %q → id %q as account %s]", err.Error(), notFoundHint(err), spreadsheetRef, id, sess.Account.Name()).About(target).Patch(nil)
	}
	if len(tabs) == 0 {
		return formkit.Warning("No sheets found in that spreadsheet.").About(target).Patch(nil)
	}

	wantID := target == "sheetId"
	options := make([]formkit.Option, 0, len(tabs))
	for _, t := range tabs {
		value := t.Title
		if wantID {
			value = strconv.Itoa(t.ID)
		}
		options = append(options, formkit.Option{Value: value, Label: fmt.Sprintf("%s  (id %d)", t.Title, t.ID)})
	}
	return formkit.Choose(
		form,
		target,
		options,
		formkit.FormData(body),
		formkit.Success("%d sheet(s) — pick one:", len(tabs)).About(target),
	)
}

// notFoundHint appends actionable guidance when the gateway reports the
// spreadsheet was not found — nearly always a wrong id (a pasted URL is handled
// automatically) or the connected account not having access to it.
func notFoundHint(err error) string {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not_found") || strings.Contains(msg, "not found") || strings.Contains(msg, "404") {
		return " — check the spreadsheet name (or id) is correct and that the connected Google account can open this spreadsheet (it must be the owner or shared with it)."
	}
	return ""
}

// metaAccountList backs the settings form's "Load accounts" button: it asks the
// backend for the Google accounts connected in FloMorphic → Connect and REBUILDS
// the "alias" field into a drop-down of them (value = alias, label = the account).
func (r *Registry) metaAccountList(req sdkv1.Request) any {
	body := decodeMeta(req.Data)
	connection := text(pick(body, "connection"))

	accounts, err := r.oc.Accounts(connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).About("alias").Patch(nil)
	}
	if len(accounts) == 0 {
		return formkit.
			Warning("No Google account connected. Connect one in FloMorphic → Connect, then retry.").
			About("alias").
			Patch(nil)
	}

	options := make([]formkit.Option, 0, len(accounts))
	for _, a := range accounts {
		label := a.Name()
		if a.IsDefault {
			label += "  (default)"
		}
		options = append(options, formkit.Option{Value: a.Alias, Label: label})
	}
	return formkit.Choose(
		settingsForm,
		"alias",
		options,
		formkit.FormData(body),
		formkit.Success("%d connected Google account(s) — pick one:", len(accounts)).About("alias"),
	)
}

// metaAccountTest backs "Test account" and the submit validation: it resolves
// the alias (or the default account) so a wrong alias is caught in the dialog.
func (r *Registry) metaAccountTest(req sdkv1.Request) any {
	conn := connFrom(decodeMeta(req.Data))
	alias := text(conn["alias"])
	connection := text(conn["connection"])

	acc, err := r.oc.Resolve(alias, connection)
	if err != nil {
		return formkit.Failure("%s", err.Error()).Patch(nil)
	}
	if acc == nil {
		if alias != "" {
			return formkit.Failure("No connected Google account with alias %q.", alias).Patch(nil)
		}
		return formkit.Failure("No Google account connected.").Patch(nil)
	}
	suffix := ""
	if acc.IsDefault {
		suffix = " — default"
	}
	return formkit.Success("Resolved %s (alias %s)%s.", acc.Name(), acc.Alias, suffix).Patch(nil)
}

// Settings declares the profile: the form plus the submit handler that validates
// it on save (the chosen account must resolve). The platform stores the profile
// and ships it back with every call as body.settings — the plugin never keeps it.
func (r *Registry) Settings() *sdkv1.Settings {
	return &sdkv1.Settings{
		FormBuilder: settingsForm,
		SubmitHandler: func(req sdkv1.Request) sdkv1.Response {
			conn := connFrom(decodeMeta(req.Data))
			alias := text(conn["alias"])
			connection := text(conn["connection"])

			acc, err := r.oc.Resolve(alias, connection)
			if err != nil {
				return sdkv1.Response{Error: err.Error()}
			}
			if acc == nil {
				if alias != "" {
					return sdkv1.Response{Error: fmt.Sprintf("No connected Google account with alias %q. Press Load accounts.", alias)}
				}
				return sdkv1.Response{Error: "No Google account connected in FloMorphic → Connect."}
			}
			return sdkv1.Response{Data: map[string]any{
				"ok":      true,
				"account": acc.AccountLabel,
				"alias":   acc.Alias,
			}}
		},
	}
}

// SettingsForm exposes the same form for PluginIntro.Settings — the plugin set-up
// dialog reads it.
func (r *Registry) SettingsForm() *sdkv1.FormBuilder {
	form := settingsForm
	return &form
}

// ------------------------------------------------------------- decoding --
// (decodeMeta lives in actions.go — it is shared with the job pipeline.)

// connFrom pulls the profile values out of a meta/submit call. On an action's
// drawer the bound profile is under "settings"; the set-up dialog has no bound
// profile and sends the edited values at the top level, alongside host keys.
func connFrom(body map[string]any) map[string]any {
	if nested, ok := body["settings"].(map[string]any); ok && len(nested) > 0 {
		return nested
	}
	conn := make(map[string]any, len(body))
	for k, v := range body {
		conn[k] = v
	}
	for _, hostKey := range []string{"settings", "value", "targetField", "form"} {
		delete(conn, hostKey)
	}
	return conn
}

// pick reads a key from the call, preferring a nested "settings" object.
func pick(body map[string]any, key string) any {
	if nested, ok := body["settings"].(map[string]any); ok {
		if v, ok := nested[key]; ok {
			return v
		}
	}
	return body[key]
}

func text(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// settingsFrom reads the bound profile a meta call carries under "settings" —
// the account the picker must resolve as. Empty when the call has no bound
// profile (e.g. the set-up dialog), which the pickers report as "no account".
func settingsFrom(body map[string]any) (alias, connection string) {
	nested, ok := body["settings"].(map[string]any)
	if !ok {
		return "", ""
	}
	return text(nested["alias"]), text(nested["connection"])
}
