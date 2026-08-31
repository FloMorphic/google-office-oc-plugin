// Every form this plugin serves is declared here with the SDK's formkit builder,
// which generates the JSON Schema and the JSON Forms UI schema from one statement
// per field. A malformed form panics at start-up (Build calls Validate) — where
// it is a compile-time-shaped mistake rather than a dialog that will not open.
//
// The forms are package-level values. Each action's input struct lives beside
// the action (sheets.go, …); the field keys here match those JSON tags
// one-for-one, which in turn match the OpenConnector action's inputSchema.
package actions

import (
	"github.com/Inflowenger/go-plugin-sdk/formkit"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// ---------------------------------------------------------------- settings --

// The Google account a settings profile represents. This plugin holds NO Google
// token: a Google account is connected once, centrally, in FloMorphic's Connect
// page (via OpenConnector / oomol), where OAuth grants Sheets/Drive/Calendar/Docs
// scopes. A settings profile just points at one of those connected accounts by
// its alias — so all this dialog does is let the user pick which account these
// nodes act as. One connected account backs every Google node.
//
// The dialog is a PICK-FROM-LIST: press "Load accounts" and the list meta
// rebuilds the "Google account" field into a drop-down of the connected accounts
// (see meta.go's metaAccountList, which returns formkit.Choose). The selected
// value is the account's alias; leave it empty to use the default account.
var settingsForm sdkv1.FormBuilder = formkit.New("Google account (OpenConnector)").
	Describe(
		"These nodes act as a Google account you connected in FloMorphic → Connect. "+
			"Press Load accounts and pick one — nothing to type. No Google token lives here.").
	SubmitTo("google.meta.account.test").
	Add(
		formkit.Text("alias", "Google account").
			Describe("Press ↻ to load the Google accounts connected in FloMorphic → Connect, then pick one. Empty uses the default account.").
			Lookup("google.meta.account.list", "Load accounts").
			Picks("google.meta.account.list"),
		formkit.Text("connection", "Gateway (optional)").
			Describe("Advanced: pin to one Connect connection id when several gateways are configured (hosted oomol vs self-hosted). Empty spans all."),
		formkit.Text("test", "Test account").
			Describe("Press ↻ to confirm the selected account resolves before saving.").
			Lookup("google.meta.account.test", "Test account"),
	).
	Build()

// ------------------------------------------------------------ sheets forms --

// The A1 range every value operation speaks in — shown once so the field
// descriptions stay consistent.
const a1Hint = `A1 notation, e.g. "Sheet1!A1:D50". A sheet name with a space or symbol must be quoted: "'My Sheet'!A1:B2".`

// spreadsheetIDField is the required "Spreadsheet" input every Sheets action
// takes. It is NAME-first: a "Load spreadsheets" button lists the account's
// Sheets files (via Drive) and rebuilds this field into a drop-down of their
// names, so the user never handles an id. The stored value is the file name; the
// plugin resolves it back to an id at run time (Session.ResolveSpreadsheetID).
// ownerMethod is the action whose form the picker rebuilds (Field.Picks). The
// field stays typable, so a name outside the list — or a pasted id / URL /
// {{$.path}} token — still resolves.
func spreadsheetIDField(describe, ownerMethod string) *formkit.Field {
	return formkit.Text("spreadsheetId", "Spreadsheet").
		Required().
		Describe(describe).
		Lookup("googlesheets.meta.spreadsheets.list", "Load spreadsheets").
		Picks(ownerMethod)
}

var sheetsCreateForm = formkit.New("Create spreadsheet").
	Add(
		formkit.Text("title", "Title").
			Describe("Name for the new spreadsheet. Accepts {{$.path}} tokens. Leave empty for an untitled spreadsheet."),
		formkit.Bool("reuseByName", "Reuse existing spreadsheet with this title").
			Default(false).
			Describe("Before creating, look up this exact title in Drive; if a spreadsheet already exists, return it instead of creating a duplicate. Off always creates a new file. (Needs Drive read scope; title match is case-sensitive.)"),
	).
	Build()

var sheetsAddSheetForm = formkit.New("Add sheet").
	Add(
		spreadsheetIDField("The spreadsheet to add a sheet to, by name. Press ↻ Load spreadsheets to pick one, or type its exact name (an id, URL, or {{$.path}} token also works).", "googlesheets.add_sheet"),
		formkit.Text("title", "Sheet title").
			Required().
			Describe("Title for the new sheet (tab). Accepts {{$.path}} tokens."),
		formkit.Bool("forceUnique", "Ensure unique title").
			Default(false).
			Describe("If a sheet with this title already exists, append a suffix to keep the new title unique instead of failing."),
	).
	Build()

var sheetsGetValuesForm = formkit.New("Get values").
	Add(
		spreadsheetIDField("The spreadsheet to read, by name. Press ↻ Load spreadsheets to pick one, or type its exact name (an id, URL, or {{$.path}} token also works).", "googlesheets.get_values"),
		formkit.List("ranges", "Ranges").
			Required().
			Describe("One or more ranges to read. "+a1Hint),
		formkit.Enum("majorDimension", "Major dimension", "", "ROWS", "COLUMNS").
			Default("").
			Describe("Whether each inner array is a row (ROWS) or a column (COLUMNS). Empty uses the API default (ROWS)."),
		formkit.Enum("valueRenderOption", "Value render", "", "FORMATTED_VALUE", "UNFORMATTED_VALUE", "FORMULA").
			Default("").
			Describe("How cell values are rendered. Empty uses the API default (FORMATTED_VALUE)."),
		formkit.Enum("dateTimeRenderOption", "Date/time render", "", "SERIAL_NUMBER", "FORMATTED_STRING").
			Default("").
			Describe("How dates and times are rendered. Empty uses the API default."),
	).
	Build()

var sheetsClearValuesForm = formkit.New("Clear values").
	Add(
		spreadsheetIDField("The spreadsheet to clear from, by name. Press ↻ Load spreadsheets to pick one, or type its exact name (an id, URL, or {{$.path}} token also works).", "googlesheets.clear_values"),
		formkit.Text("range", "Range").
			Required().
			Describe("The range to clear. Only cell values are removed; formatting is kept. "+a1Hint),
	).
	Build()

var sheetsAggregateForm = formkit.New("Aggregate column").
	Add(
		spreadsheetIDField("The spreadsheet to read, by name. Press ↻ Load spreadsheets to pick one, or type its exact name (an id, URL, or {{$.path}} token also works).", "googlesheets.aggregate_column_data"),
		formkit.Text("sheetName", "Sheet name").
			Required().
			Describe("The sheet (tab) to aggregate over. Choose the Spreadsheet first, then press ↻ Load sheets to pick from its tabs.").
			Lookup("googlesheets.meta.sheets.list", "Load sheets").
			Picks("googlesheets.aggregate_column_data"),
		formkit.Text("targetColumn", "Target column").
			Required().
			Describe("The column to aggregate — a header name (with a header row) or a column letter, e.g. \"Amount\" or \"C\"."),
		formkit.Enum("operation", "Operation", "sum", "average", "count", "min", "max", "percentage").
			Required().
			Default("sum").
			Describe("The aggregation to compute over the target column."),
		formkit.Text("searchColumn", "Filter column").
			Describe("Optional. Only aggregate rows whose value in this column matches the filter value below."),
		formkit.Text("searchValue", "Filter value").
			Describe("Optional. The value to match in the filter column. Accepts {{$.path}} tokens."),
		formkit.Bool("caseSensitive", "Case-sensitive filter").
			Default(false).
			Describe("Match the filter value case-sensitively."),
		formkit.Bool("hasHeaderRow", "First row is a header").
			Default(true).
			Describe("Treat the first row as headers, so Target column can be a header name."),
		formkit.Number("percentageTotal", "Percentage total").
			Describe("Only for the percentage operation: the denominator to compute the column's share of."),
	).
	Build()

// -------------------------------------------------------------- docs forms --

// docPickHint is the shared tail for a "Document" field's description.
const docPickHint = "Press ↻ Load documents to pick one, or type its exact name (an id, URL, or {{$.path}} token also works)."

// documentIDField is the required "Document" input the Docs read/update actions
// take. Like spreadsheetIDField it is NAME-first: a "Load documents" button lists
// the account's Docs files (via Drive) and rebuilds this field into a drop-down
// of their names; the plugin resolves the name back to an id at run time
// (Session.ResolveDocumentID). ownerMethod is the action whose form the picker
// rebuilds. The field stays typable, so a name outside the list — or a pasted
// id / URL / {{$.path}} token — still resolves.
func documentIDField(describe, ownerMethod string) *formkit.Field {
	return formkit.Text("documentId", "Document").
		Required().
		Describe(describe).
		Lookup("googledocs.meta.documents.list", "Load documents").
		Picks(ownerMethod)
}

var docsCreateForm = formkit.New("Create document").
	Add(
		formkit.Text("title", "Title").
			Required().
			Describe("Name for the new document. Accepts {{$.path}} tokens."),
		formkit.TextArea("text", "Initial text").
			Describe("Optional text to insert at the beginning of the new document. Accepts {{$.path}} tokens."),
		formkit.Bool("reuseByName", "Reuse existing document with this title").
			Default(false).
			Describe("Before creating, look up this exact title in Drive; if a document already exists, return it instead of creating a duplicate. Off always creates a new file. (Needs Drive read scope; title match is case-sensitive.)"),
	).
	Build()

var docsGetTextForm = formkit.New("Get document text").
	Add(
		documentIDField("The document to read, by name. "+docPickHint, "googledocs.get_document_plaintext"),
		formkit.Bool("includeTables", "Include tables").
			Default(true).
			Describe("Include table content in the plain-text output."),
		formkit.Bool("includeHeaders", "Include headers").
			Default(false).
			Describe("Include header content in the plain-text output."),
		formkit.Bool("includeFooters", "Include footers").
			Default(false).
			Describe("Include footer content in the plain-text output."),
		formkit.Bool("includeFootnotes", "Include footnotes").
			Default(false).
			Describe("Include footnote content in the plain-text output."),
		formkit.Bool("includeTabsContent", "Include all tabs").
			Default(false).
			Describe("Include content from all tabs in the plain-text output."),
	).
	Build()

var docsInsertTextForm = formkit.New("Insert text").
	Add(
		documentIDField("The document to insert text into, by name. "+docPickHint, "googledocs.insert_text_action"),
		formkit.TextArea("text", "Text").
			Required().
			Describe("The text to insert. Accepts {{$.path}} tokens."),
		formkit.Bool("appendToEnd", "Append to end").
			Default(true).
			Describe("Insert the text at the end of the document. Turn off to insert at a specific index instead."),
		formkit.Integer("insertionIndex", "Insertion index").
			Min(0).
			Describe("Used only when Append to end is off: the zero-based index to insert at. The position must be inside an existing paragraph."),
		formkit.Text("segmentId", "Segment id").
			Describe("Optional. The header, footer, or footnote segment to insert into. Empty targets the document body."),
	).
	Build()

var docsCopyForm = formkit.New("Copy document").
	Add(
		documentIDField("The document to copy, by name. "+docPickHint, "googledocs.copy_document"),
		formkit.Text("title", "New title").
			Describe("Title for the copy. Accepts {{$.path}} tokens. Leave empty to let Google name it."),
		formkit.Bool("includeSharedDrives", "Search shared drives").
			Default(false).
			Describe("Also look in shared drives when locating the source document."),
	).
	Build()

var docsExportPDFForm = formkit.New("Export as PDF").
	Add(
		documentIDField("The document to export, by name. "+docPickHint, "googledocs.export_document_as_pdf"),
		formkit.Text("filename", "Filename").
			Describe("Filename for the exported PDF. Accepts {{$.path}} tokens. Leave empty to use the document title."),
	).
	Build()

// ------------------------------------------------------------- drive forms --

// The shared tails for the Drive node's "File" / "Folder" field descriptions.
const (
	driveFilePickHint   = "Press ↻ Load files to pick one, or type its exact name (an id, URL, or {{$.path}} token also works)."
	driveFolderPickHint = "Press ↻ Load folders to pick one, or type its exact name (an id, URL, or {{$.path}} token also works)."
)

// driveFileField is the required "File" input the Drive actions take. Like the
// Sheets/Docs id fields it is NAME-first: a "Load files" button lists the account's
// Drive files (any type) and rebuilds this field into a drop-down of their names;
// the plugin resolves the name back to an id at run time (ResolveDriveFileID).
// ownerMethod is the action whose form the picker rebuilds. The field stays
// typable, so a name outside the list — or a pasted id / URL / {{$.path}} token —
// still resolves.
func driveFileField(describe, ownerMethod string) *formkit.Field {
	return formkit.Text("fileId", "File").
		Required().
		Describe(describe).
		Lookup("googledrive.meta.files.list", "Load files").
		Picks(ownerMethod)
}

// driveFolderField is a "Folder" input (a parent, or a move destination). A "Load
// folders" button lists the account's folders and rebuilds it into a drop-down;
// the plugin resolves the name back to an id at run time (ResolveFolderID). key
// names the field (parentId / destinationFolderId / folderId), required marks it
// mandatory, and ownerMethod is the action whose form the picker rebuilds.
func driveFolderField(key, label, describe, ownerMethod string, required bool) *formkit.Field {
	f := formkit.Text(key, label).
		Describe(describe).
		Lookup("googledrive.meta.folders.list", "Load folders").
		Picks(ownerMethod)
	if required {
		f = f.Required()
	}
	return f
}

var driveListForm = formkit.New("List files").
	Add(
		formkit.Text("query", "Drive query").
			Describe("Advanced: a raw Google Drive query string, e.g. \"mimeType='application/pdf' and trashed=false\". When set, it is used verbatim and the fields below are ignored. Accepts {{$.path}} tokens."),
		formkit.Text("nameContains", "Name contains").
			Describe("Convenience filter: return files whose name contains this text. Ignored when Drive query is set. Accepts {{$.path}} tokens."),
		driveFolderField("folderId", "In folder", "Convenience filter: only files inside this folder. "+driveFolderPickHint+" Ignored when Drive query is set.", "googledrive.files.list", false),
		formkit.Bool("includeTrashed", "Include trashed").
			Default(false).
			Describe("Include files in the trash. Ignored when Drive query is set."),
		formkit.Integer("pageSize", "Max results").
			Min(1).Max(1000).
			Describe("Maximum number of files to return (1–1000). Empty uses the API default."),
		formkit.Text("orderBy", "Order by").
			Describe("Advanced: a comma-separated list of sort keys, e.g. \"modifiedTime desc,name\". Empty sorts by most recently modified."),
	).
	Build()

var driveGetForm = formkit.New("Get file").
	Add(
		driveFileField("The file to read, by name. "+driveFilePickHint, "googledrive.files.get"),
		formkit.Bool("includeSharedDrives", "Search shared drives").
			Default(false).
			Describe("Also look in shared drives when locating the file."),
	).
	Build()

var driveCreateFolderForm = formkit.New("Create folder").
	Add(
		formkit.Text("name", "Name").
			Required().
			Describe("Name for the new folder. Accepts {{$.path}} tokens."),
		driveFolderField("parentId", "Parent folder", "Optional. Create the folder inside this folder. "+driveFolderPickHint+" Leave empty for My Drive (root).", "googledrive.create_folder", false),
		formkit.Bool("reuseByName", "Reuse existing folder with this name").
			Default(false).
			Describe("Before creating, look up this exact name in Drive; if a folder already exists, return it instead of creating a duplicate. Off always creates a new folder. (Needs Drive read scope; name match is case-sensitive.)"),
	).
	Build()

var driveCopyForm = formkit.New("Copy file").
	Add(
		driveFileField("The file to copy, by name. "+driveFilePickHint, "googledrive.files.copy"),
		formkit.Text("name", "New name").
			Describe("Name for the copy. Accepts {{$.path}} tokens. Leave empty to let Google name it (\"Copy of …\")."),
		driveFolderField("parentId", "Into folder", "Optional. Place the copy in this folder. "+driveFolderPickHint+" Leave empty to copy alongside the original.", "googledrive.files.copy", false),
	).
	Build()

var driveMoveForm = formkit.New("Move file").
	Add(
		driveFileField("The file to move, by name. "+driveFilePickHint, "googledrive.move_file"),
		driveFolderField("destinationFolderId", "Destination folder", "The folder to move the file into. "+driveFolderPickHint, "googledrive.move_file", true),
	).
	Build()

var driveRenameForm = formkit.New("Rename file").
	Add(
		driveFileField("The file or folder to rename, by name. "+driveFilePickHint, "googledrive.rename_file"),
		formkit.Text("newName", "New name").
			Required().
			Describe("The new name. Accepts {{$.path}} tokens."),
	).
	Build()

var driveDeleteForm = formkit.New("Delete file").
	Add(
		driveFileField("The file to delete, by name. "+driveFilePickHint, "googledrive.delete_file"),
		formkit.Bool("permanent", "Delete permanently").
			Default(false).
			Describe("Off (default) moves the file to the trash, where it can be restored. On deletes it permanently and irreversibly."),
	).
	Build()

var driveShareForm = formkit.New("Share file").
	Add(
		driveFileField("The file to share, by name. "+driveFilePickHint, "googledrive.share_file"),
		formkit.Enum("role", "Role", "reader", "commenter", "writer", "fileOrganizer", "organizer", "owner").
			Required().
			Default("reader").
			Describe("The access level to grant."),
		formkit.Enum("type", "Grantee type", "user", "group", "domain", "anyone").
			Required().
			Default("user").
			Describe("Who the grant is for. user/group need an email; domain needs a domain; anyone shares with anyone who has the link."),
		formkit.Text("emailAddress", "Email address").
			Describe("The user or group email to share with (required for user/group). Accepts {{$.path}} tokens."),
		formkit.Text("domain", "Domain").
			Describe("The domain to share with, e.g. \"example.com\" (required for a domain grant)."),
		formkit.Bool("sendNotificationEmail", "Send notification email").
			Default(false).
			Describe("Email the grantee that the file was shared. Off is quieter for automation; access is granted either way."),
		formkit.TextArea("message", "Notification message").
			Describe("Optional message to include in the notification email. Only used when Send notification email is on. Accepts {{$.path}} tokens."),
	).
	Build()

var driveExportForm = formkit.New("Export file").
	Add(
		driveFileField("The Google Workspace file to export, by name. "+driveFilePickHint, "googledrive.export_file"),
		formkit.Text("mimeType", "Export as (MIME type)").
			Required().
			Describe("The target format's MIME type, e.g. \"application/pdf\", \"text/plain\", \"text/csv\", or \"application/vnd.openxmlformats-officedocument.wordprocessingml.document\" (.docx). Accepts {{$.path}} tokens."),
	).
	Build()

// ---------------------------------------------------------- calendar forms --

// calendarPickHint is the shared tail for a "Calendar" field's description.
const calendarPickHint = "Press ↻ Load calendars to pick one, or type a calendar id / email. Empty uses your primary calendar."

// timeHint documents the start/end format the event actions accept.
const timeHint = "For a timed event use RFC 3339, e.g. \"2026-09-01T14:00:00-07:00\"; for an all-day event turn on All-day and use a date, e.g. \"2026-09-01\". Accepts {{$.path}} tokens."

// calendarIDField is a "Calendar" input. A "Load calendars" button lists the
// account's calendars and rebuilds the field into a drop-down (value = calendar
// id, label = summary); the id is used directly, so the field is typable for
// "primary" or a pasted id, and empty falls back to the primary calendar.
// ownerMethod is the action whose form the picker rebuilds.
func calendarIDField(key, label, ownerMethod string) *formkit.Field {
	return formkit.Text(key, label).
		Describe("The calendar. "+calendarPickHint).
		Lookup("googlecalendar.meta.calendars.list", "Load calendars").
		Picks(ownerMethod)
}

// sendUpdatesField is the shared "Notify attendees" control (all/externalOnly/
// none), defaulting to none so automation stays quiet unless asked otherwise.
func sendUpdatesField() *formkit.Field {
	return formkit.Enum("sendUpdates", "Notify attendees", "none", "all", "externalOnly").
		Default("none").
		Describe("Who to email about this change: none (quiet, the default), all attendees, or externalOnly (only guests outside your organization).")
}

var calendarListCalendarsForm = formkit.New("List calendars").
	Add(
		formkit.Bool("showHidden", "Include hidden").
			Default(false).
			Describe("Include calendars hidden from the list."),
		formkit.Bool("showDeleted", "Include deleted").
			Default(false).
			Describe("Include deleted calendar-list entries."),
	).
	Build()

var calendarListEventsForm = formkit.New("List events").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.list_events"),
		formkit.Text("q", "Search text").
			Describe("Optional free-text search over event fields (summary, description, location, attendees). Accepts {{$.path}} tokens."),
		formkit.Text("timeMin", "Start of window").
			Describe("Only events ending after this RFC 3339 time, e.g. \"2026-09-01T00:00:00Z\". Accepts {{$.path}} tokens."),
		formkit.Text("timeMax", "End of window").
			Describe("Only events starting before this RFC 3339 time. Accepts {{$.path}} tokens."),
		formkit.Enum("orderBy", "Order by", "", "startTime", "updated").
			Default("startTime").
			Describe("Sort order. \"startTime\" (default) requires single events, which is on."),
		formkit.Integer("maxResults", "Max results").
			Min(1).Max(2500).
			Describe("Maximum number of events to return. Empty uses the API default."),
		formkit.Text("timeZone", "Time zone").
			Describe("IANA time zone for the response, e.g. \"America/Los_Angeles\". Empty uses the calendar's zone."),
		formkit.Bool("showDeleted", "Include cancelled").
			Default(false).
			Describe("Include cancelled events in the results."),
	).
	Build()

var calendarGetEventForm = formkit.New("Get event").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.get_event"),
		formkit.Text("eventId", "Event id").
			Required().
			Describe("The id of the event to read. Accepts {{$.path}} tokens."),
	).
	Build()

var calendarCreateEventForm = formkit.New("Create event").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.create_event"),
		formkit.Text("summary", "Title").
			Required().
			Describe("The event title. Accepts {{$.path}} tokens."),
		formkit.Text("start", "Start").
			Required().
			Describe("When the event starts. "+timeHint),
		formkit.Text("end", "End").
			Required().
			Describe("When the event ends. "+timeHint),
		formkit.Bool("allDay", "All-day event").
			Default(false).
			Describe("Treat Start/End as dates (YYYY-MM-DD) for an all-day event instead of timestamps."),
		formkit.Text("timeZone", "Time zone").
			Describe("IANA time zone for the start/end times, e.g. \"America/Los_Angeles\". Ignored for all-day events."),
		formkit.Text("location", "Location").
			Describe("Optional location text. Accepts {{$.path}} tokens."),
		formkit.TextArea("description", "Description").
			Describe("Optional event description. Accepts {{$.path}} tokens."),
		formkit.List("attendees", "Attendees").
			Describe("Optional attendee email addresses, one per entry. Each accepts {{$.path}} tokens."),
		sendUpdatesField(),
	).
	Build()

var calendarQuickAddForm = formkit.New("Quick add event").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.quick_add_event"),
		formkit.Text("text", "Phrase").
			Required().
			Describe("A natural-language description of the event, e.g. \"Lunch with Sam tomorrow 1pm\". Accepts {{$.path}} tokens."),
		sendUpdatesField(),
	).
	Build()

var calendarUpdateEventForm = formkit.New("Update event").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.patch_event"),
		formkit.Text("eventId", "Event id").
			Required().
			Describe("The id of the event to update. Accepts {{$.path}} tokens."),
		formkit.Text("summary", "Title").
			Describe("New title. Leave empty to keep the current one. Accepts {{$.path}} tokens."),
		formkit.Text("start", "Start").
			Describe("New start. Leave empty to keep it. "+timeHint),
		formkit.Text("end", "End").
			Describe("New end. Leave empty to keep it. "+timeHint),
		formkit.Bool("allDay", "All-day event").
			Default(false).
			Describe("When changing Start/End, treat them as all-day dates rather than timestamps."),
		formkit.Text("timeZone", "Time zone").
			Describe("IANA time zone for a changed start/end. Ignored for all-day events."),
		formkit.Text("location", "Location").
			Describe("New location. Leave empty to keep it. Accepts {{$.path}} tokens."),
		formkit.TextArea("description", "Description").
			Describe("New description. Leave empty to keep it. Accepts {{$.path}} tokens."),
		formkit.List("attendees", "Attendees").
			Describe("Replace the attendee list with these email addresses. Leave empty to keep the current attendees."),
		sendUpdatesField(),
	).
	Build()

var calendarDeleteEventForm = formkit.New("Delete event").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.delete_event"),
		formkit.Text("eventId", "Event id").
			Required().
			Describe("The id of the event to delete. Accepts {{$.path}} tokens."),
		sendUpdatesField(),
	).
	Build()

var calendarMoveEventForm = formkit.New("Move event").
	Add(
		calendarIDField("calendarId", "Source calendar", "googlecalendar.move_event"),
		formkit.Text("eventId", "Event id").
			Required().
			Describe("The id of the event to move. Accepts {{$.path}} tokens."),
		calendarIDField("destinationCalendarId", "Destination calendar", "googlecalendar.move_event").
			Required(),
		sendUpdatesField(),
	).
	Build()

var calendarFindFreeForm = formkit.New("Find free/busy").
	Add(
		formkit.List("calendarIds", "Calendars").
			Describe("Calendar ids / emails to check, one per entry. Empty checks your primary calendar. Each accepts {{$.path}} tokens."),
		formkit.Text("timeMin", "Start of window").
			Required().
			Describe("Start of the range to check, RFC 3339, e.g. \"2026-09-01T00:00:00Z\". Accepts {{$.path}} tokens."),
		formkit.Text("timeMax", "End of window").
			Required().
			Describe("End of the range to check, RFC 3339. Accepts {{$.path}} tokens."),
		formkit.Text("timeZone", "Time zone").
			Describe("IANA time zone for the query, e.g. \"America/Los_Angeles\"."),
	).
	Build()

var calendarAddAttendeeForm = formkit.New("Add attendee").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.add_attendee"),
		formkit.Text("eventId", "Event id").
			Required().
			Describe("The id of the event to add the attendee to. Accepts {{$.path}} tokens."),
		formkit.Text("attendeeEmail", "Attendee email").
			Required().
			Describe("The email address to add as an attendee. Accepts {{$.path}} tokens."),
		formkit.Text("displayName", "Display name").
			Describe("Optional display name for the attendee. Accepts {{$.path}} tokens."),
		formkit.Bool("optional", "Optional attendee").
			Default(false).
			Describe("Mark the attendee as optional rather than required."),
		sendUpdatesField(),
	).
	Build()

var calendarRemoveAttendeeForm = formkit.New("Remove attendee").
	Add(
		calendarIDField("calendarId", "Calendar", "googlecalendar.remove_attendee"),
		formkit.Text("eventId", "Event id").
			Required().
			Describe("The id of the event to remove the attendee from. Accepts {{$.path}} tokens."),
		formkit.Text("attendeeEmail", "Attendee email").
			Required().
			Describe("The email address of the attendee to remove. Accepts {{$.path}} tokens."),
		sendUpdatesField(),
	).
	Build()
