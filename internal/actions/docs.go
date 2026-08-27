package actions

import (
	"strings"

	"github.com/FloMorphic/google-office-oc-plugin/internal/oc"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// Google Docs actions. Each forwards to a googledocs.* OpenConnector action and
// returns the object the gateway answers with. Unlike Sheets, the Docs actions
// name their document field inconsistently (document_id, documentId, id,
// file_id), so a handler resolves the user's "Document" reference to a bare id
// and builds the gateway payload with that action's exact key — the input struct
// here is the user-facing shape, not the wire shape.
//
// Every action is tagged class="docs" so the frontend groups these ports as one
// product. New Docs operations are added here, one at a time.

// docLink is the canonical edit URL for a document id, returned alongside a
// created/reused document so a downstream node has something clickable.
func docLink(id string) string { return "https://docs.google.com/document/d/" + id + "/edit" }

// docsClass stamps the shared class tag onto a Docs action.
func docsClass() map[string]string { return map[string]string{"class": classDocs} }

// docsFormByMethod lets the documentId picker meta rebuild the right form: a
// "Load documents" button posts its action's method (via Field.Picks), and the
// meta looks the form up here to turn the field into a drop-down. Keep in sync
// with the actions below and their forms in forms.go.
var docsFormByMethod = map[string]sdkv1.FormBuilder{
	"googledocs.get_document_plaintext": docsGetTextForm,
	"googledocs.insert_text_action":     docsInsertTextForm,
	"googledocs.copy_document":          docsCopyForm,
	"googledocs.export_document_as_pdf": docsExportPDFForm,
}

// docsActions is the ordered set of Docs nodes this plugin exposes.
func (r *Registry) docsActions() []sdkv1.Action {
	return []sdkv1.Action{
		r.docsCreateDocument(),
		r.docsGetText(),
		r.docsInsertText(),
		r.docsCopyDocument(),
		r.docsExportPDF(),
	}
}

// ------------------------------------------------------- create document --

type docsCreateInput struct {
	Title       string `json:"title"`
	Text        string `json:"text,omitempty"`
	ReuseByName bool   `json:"reuseByName,omitempty"`
}

func (r *Registry) docsCreateDocument() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledocs.create_document",
		Title:       "Docs: Create document",
		Description: "Create a new Google Docs document, optionally with initial text, and return its id and url (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-file-document-plus"},
		Tags:        docsClass(),
		Form:        docsCreateForm,
		RequestHandler: run(r, "create document", func(job *sdkv1.Job, sess *oc.Session, in docsCreateInput) (map[string]any, error) {
			if err := requireAll(nv("title", in.Title)); err != nil {
				return nil, err
			}
			// The Docs create API always makes a new file (Drive allows duplicate
			// names). When asked to reuse, look the title up in Drive first and
			// return the existing document instead of a duplicate.
			if in.ReuseByName {
				existing, err := sess.FindDocumentByName(in.Title)
				if err != nil {
					return nil, err
				}
				if existing != nil {
					return map[string]any{
						"documentId": existing.ID,
						"title":      existing.Name,
						"url":        docLink(existing.ID),
						"reused":     true,
					}, nil
				}
			}
			// ReuseByName is not a create_document field — forward only title/text.
			payload := map[string]any{"title": in.Title}
			if strings.TrimSpace(in.Text) != "" {
				payload["text"] = in.Text
			}
			raw, err := sess.Do("googledocs.create_document", payload)
			if err != nil {
				return nil, err
			}
			out := object(raw)
			if id, ok := out["documentId"].(string); ok && id != "" {
				out["url"] = docLink(id)
			}
			return out, nil
		}),
	}
}

// ----------------------------------------------------- get document text --

type docsGetTextInput struct {
	DocumentID         string `json:"documentId"`
	IncludeTables      bool   `json:"includeTables,omitempty"`
	IncludeHeaders     bool   `json:"includeHeaders,omitempty"`
	IncludeFooters     bool   `json:"includeFooters,omitempty"`
	IncludeFootnotes   bool   `json:"includeFootnotes,omitempty"`
	IncludeTabsContent bool   `json:"includeTabsContent,omitempty"`
}

func (r *Registry) docsGetText() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledocs.get_document_plaintext",
		Title:       "Docs: Get document text",
		Description: "Read a document and return its content as best-effort plain text (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-file-document-outline"},
		Tags:        docsClass(),
		Form:        docsGetTextForm,
		RequestHandler: run(r, "get document text", func(job *sdkv1.Job, sess *oc.Session, in docsGetTextInput) (map[string]any, error) {
			if err := requireAll(nv("documentId", in.DocumentID)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDocumentID(in.DocumentID)
			if err != nil {
				return nil, err
			}
			raw, err := sess.Do("googledocs.get_document_plaintext", map[string]any{
				"document_id":          id,
				"include_tables":       in.IncludeTables,
				"include_headers":      in.IncludeHeaders,
				"include_footers":      in.IncludeFooters,
				"include_footnotes":    in.IncludeFootnotes,
				"include_tabs_content": in.IncludeTabsContent,
			})
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ---------------------------------------------------------- insert text --

type docsInsertTextInput struct {
	DocumentID     string `json:"documentId"`
	Text           string `json:"text"`
	AppendToEnd    bool   `json:"appendToEnd,omitempty"`
	InsertionIndex *int   `json:"insertionIndex,omitempty"`
	SegmentID      string `json:"segmentId,omitempty"`
}

func (r *Registry) docsInsertText() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledocs.insert_text_action",
		Title:       "Docs: Insert text",
		Description: "Append text to a document, or insert it at a specific index (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-form-textbox"},
		Tags:        docsClass(),
		Form:        docsInsertTextForm,
		RequestHandler: run(r, "insert text", func(job *sdkv1.Job, sess *oc.Session, in docsInsertTextInput) (map[string]any, error) {
			if err := requireAll(nv("documentId", in.DocumentID), nv("text", in.Text)); err != nil {
				return nil, err
			}
			// The gateway needs either append_to_end or an insertion_index. Default
			// to appending when the user gave neither, so the common case just works.
			if !in.AppendToEnd && in.InsertionIndex == nil {
				in.AppendToEnd = true
			}
			id, err := sess.ResolveDocumentID(in.DocumentID)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{
				"document_id":    id,
				"text_to_insert": in.Text,
			}
			if in.AppendToEnd {
				payload["append_to_end"] = true
			} else {
				payload["insertion_index"] = *in.InsertionIndex
			}
			if strings.TrimSpace(in.SegmentID) != "" {
				payload["segment_id"] = in.SegmentID
			}
			raw, err := sess.Do("googledocs.insert_text_action", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// --------------------------------------------------------- copy document --

type docsCopyInput struct {
	DocumentID          string `json:"documentId"`
	Title               string `json:"title,omitempty"`
	IncludeSharedDrives bool   `json:"includeSharedDrives,omitempty"`
}

func (r *Registry) docsCopyDocument() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledocs.copy_document",
		Title:       "Docs: Copy document",
		Description: "Duplicate an existing document (e.g. a template) through Drive, optionally under a new title (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-content-copy"},
		Tags:        docsClass(),
		Form:        docsCopyForm,
		RequestHandler: run(r, "copy document", func(job *sdkv1.Job, sess *oc.Session, in docsCopyInput) (map[string]any, error) {
			if err := requireAll(nv("documentId", in.DocumentID)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDocumentID(in.DocumentID)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{"document_id": id}
			if strings.TrimSpace(in.Title) != "" {
				payload["title"] = in.Title
			}
			if in.IncludeSharedDrives {
				payload["include_shared_drives"] = true
			}
			raw, err := sess.Do("googledocs.copy_document", payload)
			if err != nil {
				return nil, err
			}
			out := object(raw)
			if newID, ok := out["id"].(string); ok && newID != "" {
				out["url"] = docLink(newID)
			}
			return out, nil
		}),
	}
}

// -------------------------------------------------------- export as PDF --

type docsExportPDFInput struct {
	DocumentID string `json:"documentId"`
	Filename   string `json:"filename,omitempty"`
}

func (r *Registry) docsExportPDF() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledocs.export_document_as_pdf",
		Title:       "Docs: Export as PDF",
		Description: "Export a document as a PDF and return its Base64-encoded bytes (via OpenConnector; ~10 MB Drive limit).",
		Icon:        sdkv1.Icon{Icon: "mdi-file-pdf-box"},
		Tags:        docsClass(),
		Form:        docsExportPDFForm,
		RequestHandler: run(r, "export as PDF", func(job *sdkv1.Job, sess *oc.Session, in docsExportPDFInput) (map[string]any, error) {
			if err := requireAll(nv("documentId", in.DocumentID)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDocumentID(in.DocumentID)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{"file_id": id}
			if strings.TrimSpace(in.Filename) != "" {
				payload["filename"] = in.Filename
			}
			raw, err := sess.Do("googledocs.export_document_as_pdf", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}
