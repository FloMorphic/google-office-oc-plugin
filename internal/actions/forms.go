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
