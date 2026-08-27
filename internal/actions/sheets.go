package actions

import (
	"errors"
	"strings"

	"github.com/FloMorphic/google-office-oc-plugin/internal/oc"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// errNoRanges fails a get-values run that supplied no A1 range to read.
var errNoRanges = errors.New("missing required input: ranges — add at least one A1 range, e.g. Sheet1!A1:D50")

// Google Sheets actions. Each forwards to a googlesheets.* OpenConnector action
// and returns the object the gateway answers with. The input struct's JSON tags
// match that action's inputSchema one-for-one (see GET /v1/actions?service=
// googlesheets), so the struct passes straight through as the action's `input`.
//
// Every action is tagged class="sheets" so the frontend groups these ports as
// one product. New Sheets operations are added here, one at a time.

// sheetsClass stamps the shared class tag onto a Sheets action.
func sheetsClass() map[string]string { return map[string]string{"class": classSheets} }

// sheetsFormByMethod lets a dependent-field picker meta rebuild the right form:
// a "Load sheets" button posts its action's method (via Field.Picks), and the
// meta looks the form up here to turn the target field into a drop-down. Keep in
// sync with the actions below and their forms in forms.go.
var sheetsFormByMethod = map[string]sdkv1.FormBuilder{
	"googlesheets.add_sheet":             sheetsAddSheetForm,
	"googlesheets.get_values":            sheetsGetValuesForm,
	"googlesheets.clear_values":          sheetsClearValuesForm,
	"googlesheets.aggregate_column_data": sheetsAggregateForm,
}

// sheetsActions is the ordered set of Sheets nodes this plugin exposes.
func (r *Registry) sheetsActions() []sdkv1.Action {
	return []sdkv1.Action{
		r.sheetsCreateSpreadsheet(),
		r.sheetsAddSheet(),
		r.sheetsGetValues(),
		r.sheetsClearValues(),
		r.sheetsAggregateColumn(),
	}
}

// ---------------------------------------------------------- create sheet --

type sheetsCreateInput struct {
	Title       string `json:"title,omitempty"`
	ReuseByName bool   `json:"reuseByName,omitempty"`
}

func (r *Registry) sheetsCreateSpreadsheet() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlesheets.create_spreadsheet",
		Title:       "Sheets: Create spreadsheet",
		Description: "Create a new Google Sheets spreadsheet and return its id, url and metadata (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-google-spreadsheet"},
		Tags:        sheetsClass(),
		Form:        sheetsCreateForm,
		RequestHandler: run(r, "create spreadsheet", func(job *sdkv1.Job, sess *oc.Session, in sheetsCreateInput) (map[string]any, error) {
			// The Sheets create API always makes a new file (Drive allows
			// duplicate names). When asked to reuse, look the title up in Drive
			// first and return the existing spreadsheet instead of a duplicate.
			if in.ReuseByName && strings.TrimSpace(in.Title) != "" {
				existing, err := sess.FindSpreadsheetByName(in.Title)
				if err != nil {
					return nil, err
				}
				if existing != nil {
					return map[string]any{
						"spreadsheetId": existing.ID,
						"title":         existing.Name,
						"url":           "https://docs.google.com/spreadsheets/d/" + existing.ID + "/edit",
						"reused":        true,
					}, nil
				}
			}
			// ReuseByName is not a create_google_sheet1 field — strip it before
			// forwarding so the gateway sees only the title it expects.
			raw, err := sess.Do("googlesheets.create_google_sheet1", sheetsCreateInput{Title: in.Title})
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// -------------------------------------------------------------- add sheet --

type sheetsAddSheetInput struct {
	SpreadsheetID string `json:"spreadsheetId"`
	Title         string `json:"title"`
	ForceUnique   bool   `json:"forceUnique,omitempty"`
}

func (r *Registry) sheetsAddSheet() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlesheets.add_sheet",
		Title:       "Sheets: Add sheet",
		Description: "Add a new sheet (tab) to an existing spreadsheet (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-tab-plus"},
		Tags:        sheetsClass(),
		Form:        sheetsAddSheetForm,
		RequestHandler: run(r, "add sheet", func(job *sdkv1.Job, sess *oc.Session, in sheetsAddSheetInput) (map[string]any, error) {
			if err := requireAll(nv("spreadsheetId", in.SpreadsheetID), nv("title", in.Title)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveSpreadsheetID(in.SpreadsheetID)
			if err != nil {
				return nil, err
			}
			in.SpreadsheetID = id
			raw, err := sess.Do("googlesheets.add_sheet", in)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ------------------------------------------------------------- get values --

// sheetsGetValuesInput matches googlesheets.batch_get: read one or more A1
// ranges at once.
type sheetsGetValuesInput struct {
	SpreadsheetID        string   `json:"spreadsheetId"`
	Ranges               []string `json:"ranges"`
	MajorDimension       string   `json:"majorDimension,omitempty"`
	ValueRenderOption    string   `json:"valueRenderOption,omitempty"`
	DateTimeRenderOption string   `json:"dateTimeRenderOption,omitempty"`
}

func (r *Registry) sheetsGetValues() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlesheets.get_values",
		Title:       "Sheets: Get values",
		Description: "Read one or more A1 ranges from a spreadsheet and return their cell values (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-table-eye"},
		Tags:        sheetsClass(),
		Form:        sheetsGetValuesForm,
		RequestHandler: run(r, "get values", func(job *sdkv1.Job, sess *oc.Session, in sheetsGetValuesInput) (map[string]any, error) {
			if err := requireAll(nv("spreadsheetId", in.SpreadsheetID)); err != nil {
				return nil, err
			}
			if len(in.Ranges) == 0 {
				return nil, errNoRanges
			}
			id, err := sess.ResolveSpreadsheetID(in.SpreadsheetID)
			if err != nil {
				return nil, err
			}
			in.SpreadsheetID = id
			raw, err := sess.Do("googlesheets.batch_get", in)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ----------------------------------------------------------- clear values --

type sheetsClearValuesInput struct {
	SpreadsheetID string `json:"spreadsheetId"`
	Range         string `json:"range"`
}

func (r *Registry) sheetsClearValues() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlesheets.clear_values",
		Title:       "Sheets: Clear values",
		Description: "Clear the cell values in a single A1 range (formatting is kept) (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-eraser"},
		Tags:        sheetsClass(),
		Form:        sheetsClearValuesForm,
		RequestHandler: run(r, "clear values", func(job *sdkv1.Job, sess *oc.Session, in sheetsClearValuesInput) (map[string]any, error) {
			if err := requireAll(nv("spreadsheetId", in.SpreadsheetID), nv("range", in.Range)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveSpreadsheetID(in.SpreadsheetID)
			if err != nil {
				return nil, err
			}
			in.SpreadsheetID = id
			raw, err := sess.Do("googlesheets.clear_values", in)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ------------------------------------------------------- aggregate column --

type sheetsAggregateInput struct {
	SpreadsheetID   string  `json:"spreadsheetId"`
	SheetName       string  `json:"sheetName"`
	TargetColumn    string  `json:"targetColumn"`
	Operation       string  `json:"operation"`
	SearchColumn    string  `json:"searchColumn,omitempty"`
	SearchValue     string  `json:"searchValue,omitempty"`
	CaseSensitive   bool    `json:"caseSensitive,omitempty"`
	HasHeaderRow    bool    `json:"hasHeaderRow,omitempty"`
	PercentageTotal float64 `json:"percentageTotal,omitempty"`
}

func (r *Registry) sheetsAggregateColumn() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlesheets.aggregate_column_data",
		Title:       "Sheets: Aggregate column",
		Description: "Sum, average, count, min, max or percentage a numeric column, optionally filtered by another column (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-sigma"},
		Tags:        sheetsClass(),
		Form:        sheetsAggregateForm,
		RequestHandler: run(r, "aggregate column", func(job *sdkv1.Job, sess *oc.Session, in sheetsAggregateInput) (map[string]any, error) {
			if err := requireAll(
				nv("spreadsheetId", in.SpreadsheetID),
				nv("sheetName", in.SheetName),
				nv("targetColumn", in.TargetColumn),
				nv("operation", in.Operation),
			); err != nil {
				return nil, err
			}
			id, err := sess.ResolveSpreadsheetID(in.SpreadsheetID)
			if err != nil {
				return nil, err
			}
			in.SpreadsheetID = id
			raw, err := sess.Do("googlesheets.aggregate_column_data", in)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}
