package actions

import (
	"fmt"
	"strings"

	"github.com/FloMorphic/google-office-oc-plugin/internal/oc"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// Google Drive actions. Each forwards to a googledrive.* OpenConnector action and
// returns the object the gateway answers with. Unlike Sheets/Docs, the Drive node
// speaks in GENERIC files (any type) and folders: the user's "File"/"Folder"
// reference is resolved to a bare id (Session.ResolveDriveFileID /
// ResolveFolderID) before the gateway payload is built, so a name, a pasted URL,
// a bare id, or a {{$.path}} token all work.
//
// Several nodes here (Move, Rename, Delete-to-trash) are the same underlying
// googledrive.files.update action shaped for one job; the node's Method is its
// canvas identity (and the Picks key its picker rebuilds), not the OC action id —
// the handler names the real action it calls. Every action is tagged
// class="drive" so the frontend groups these ports as one product.

// mimeFolder is the Drive mime type of a folder, used to create one.
const mimeFolder = "application/vnd.google-apps.folder"

// driveQuoteRe escapes a value going into a Drive `q` string literal: a name or
// id may contain a single quote or backslash, both of which must be
// backslash-escaped or the query is a 400 (or injects extra clauses).
var driveQuoteRe = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

// driveQuote escapes s for use inside a single-quoted Drive `q` literal.
func driveQuote(s string) string { return driveQuoteRe.Replace(s) }

// driveFileLink is a best-effort clickable URL for a Drive file id, returned
// alongside actions whose gateway response carries no webViewLink of its own.
func driveFileLink(id string) string { return "https://drive.google.com/file/d/" + id + "/view" }

// driveClass stamps the shared class tag onto a Drive action.
func driveClass() map[string]string { return map[string]string{"class": classDrive} }

// driveFormByMethod lets the file/folder picker metas rebuild the right form: a
// "Load files"/"Load folders" button posts its action's method (via Field.Picks),
// and the meta looks the form up here to turn the target field into a drop-down.
// Keep in sync with the actions below and their forms in forms.go.
var driveFormByMethod = map[string]sdkv1.FormBuilder{
	"googledrive.files.list":    driveListForm,
	"googledrive.files.get":     driveGetForm,
	"googledrive.create_folder": driveCreateFolderForm,
	"googledrive.files.copy":    driveCopyForm,
	"googledrive.move_file":     driveMoveForm,
	"googledrive.rename_file":   driveRenameForm,
	"googledrive.delete_file":   driveDeleteForm,
	"googledrive.share_file":    driveShareForm,
	"googledrive.export_file":   driveExportForm,
}

// driveActions is the ordered set of Drive nodes this plugin exposes.
func (r *Registry) driveActions() []sdkv1.Action {
	return []sdkv1.Action{
		r.driveListFiles(),
		r.driveGetFile(),
		r.driveCreateFolder(),
		r.driveCopyFile(),
		r.driveMoveFile(),
		r.driveRenameFile(),
		r.driveDeleteFile(),
		r.driveShareFile(),
		r.driveExportFile(),
	}
}

// -------------------------------------------------------------- list files --

type driveListInput struct {
	Query          string `json:"query,omitempty"`
	NameContains   string `json:"nameContains,omitempty"`
	FolderID       string `json:"folderId,omitempty"`
	IncludeTrashed bool   `json:"includeTrashed,omitempty"`
	PageSize       int    `json:"pageSize,omitempty"`
	OrderBy        string `json:"orderBy,omitempty"`
}

func (r *Registry) driveListFiles() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.files.list",
		Title:       "Drive: List files",
		Description: "List or search Drive files — by name, inside a folder, or with a raw Drive query (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-folder-search"},
		Tags:        driveClass(),
		Form:        driveListForm,
		RequestHandler: run(r, "list files", func(job *sdkv1.Job, sess *oc.Session, in driveListInput) (map[string]any, error) {
			// A raw query wins outright; otherwise build one from the convenience
			// fields so the common cases (name / folder) need no Drive query syntax.
			q := strings.TrimSpace(in.Query)
			if q == "" {
				var clauses []string
				if !in.IncludeTrashed {
					clauses = append(clauses, "trashed=false")
				}
				if name := strings.TrimSpace(in.NameContains); name != "" {
					clauses = append(clauses, fmt.Sprintf("name contains '%s'", driveQuote(name)))
				}
				if ref := strings.TrimSpace(in.FolderID); ref != "" {
					folderID, err := sess.ResolveFolderID(ref)
					if err != nil {
						return nil, err
					}
					clauses = append(clauses, fmt.Sprintf("'%s' in parents", driveQuote(folderID)))
				}
				q = strings.Join(clauses, " and ")
			}
			payload := map[string]any{}
			if q != "" {
				payload["q"] = q
			}
			payload["orderBy"] = "modifiedTime desc"
			if strings.TrimSpace(in.OrderBy) != "" {
				payload["orderBy"] = in.OrderBy
			}
			if in.PageSize > 0 {
				payload["pageSize"] = in.PageSize
			}
			raw, err := sess.Do("googledrive.files.list", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// --------------------------------------------------------------- get file --

type driveGetInput struct {
	FileID              string `json:"fileId"`
	IncludeSharedDrives bool   `json:"includeSharedDrives,omitempty"`
}

func (r *Registry) driveGetFile() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.files.get",
		Title:       "Drive: Get file",
		Description: "Read a file's Drive metadata (name, type, size, parents, owners, links) (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-file-search"},
		Tags:        driveClass(),
		Form:        driveGetForm,
		RequestHandler: run(r, "get file", func(job *sdkv1.Job, sess *oc.Session, in driveGetInput) (map[string]any, error) {
			if err := requireAll(nv("fileId", in.FileID)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDriveFileID(in.FileID)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{"fileId": id}
			if in.IncludeSharedDrives {
				payload["includeSharedDrives"] = true
			}
			raw, err := sess.Do("googledrive.files.get", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ---------------------------------------------------------- create folder --

type driveCreateFolderInput struct {
	Name        string `json:"name"`
	ParentID    string `json:"parentId,omitempty"`
	ReuseByName bool   `json:"reuseByName,omitempty"`
}

func (r *Registry) driveCreateFolder() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.create_folder",
		Title:       "Drive: Create folder",
		Description: "Create a new folder, optionally inside a parent folder, and return its id and url (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-folder-plus"},
		Tags:        driveClass(),
		Form:        driveCreateFolderForm,
		RequestHandler: run(r, "create folder", func(job *sdkv1.Job, sess *oc.Session, in driveCreateFolderInput) (map[string]any, error) {
			if err := requireAll(nv("name", in.Name)); err != nil {
				return nil, err
			}
			// files.create always makes a new folder (Drive allows duplicate names).
			// When asked to reuse, look the name up first and return the existing one.
			if in.ReuseByName {
				existing, err := sess.FindFolderByName(in.Name)
				if err != nil {
					return nil, err
				}
				if existing != nil {
					return map[string]any{
						"id":     existing.ID,
						"name":   existing.Name,
						"url":    driveFileLink(existing.ID),
						"reused": true,
					}, nil
				}
			}
			payload := map[string]any{
				"name":     in.Name,
				"mimeType": mimeFolder,
			}
			if ref := strings.TrimSpace(in.ParentID); ref != "" {
				parentID, err := sess.ResolveFolderID(ref)
				if err != nil {
					return nil, err
				}
				payload["parents"] = []string{parentID}
			}
			raw, err := sess.Do("googledrive.files.create", payload)
			if err != nil {
				return nil, err
			}
			out := object(raw)
			if id, ok := out["id"].(string); ok && id != "" {
				if _, hasLink := out["webViewLink"]; !hasLink {
					out["url"] = driveFileLink(id)
				}
			}
			return out, nil
		}),
	}
}

// ---------------------------------------------------------------- copy file --

type driveCopyInput struct {
	FileID   string `json:"fileId"`
	Name     string `json:"name,omitempty"`
	ParentID string `json:"parentId,omitempty"`
}

func (r *Registry) driveCopyFile() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.files.copy",
		Title:       "Drive: Copy file",
		Description: "Duplicate a file, optionally under a new name and into a target folder (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-content-copy"},
		Tags:        driveClass(),
		Form:        driveCopyForm,
		RequestHandler: run(r, "copy file", func(job *sdkv1.Job, sess *oc.Session, in driveCopyInput) (map[string]any, error) {
			if err := requireAll(nv("fileId", in.FileID)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDriveFileID(in.FileID)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{"fileId": id}
			if strings.TrimSpace(in.Name) != "" {
				payload["name"] = in.Name
			}
			if ref := strings.TrimSpace(in.ParentID); ref != "" {
				parentID, err := sess.ResolveFolderID(ref)
				if err != nil {
					return nil, err
				}
				payload["parents"] = []string{parentID}
			}
			raw, err := sess.Do("googledrive.files.copy", payload)
			if err != nil {
				return nil, err
			}
			out := object(raw)
			if newID, ok := out["id"].(string); ok && newID != "" {
				if _, hasLink := out["webViewLink"]; !hasLink {
					out["url"] = driveFileLink(newID)
				}
			}
			return out, nil
		}),
	}
}

// ---------------------------------------------------------------- move file --

type driveMoveInput struct {
	FileID              string `json:"fileId"`
	DestinationFolderID string `json:"destinationFolderId"`
}

func (r *Registry) driveMoveFile() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.move_file",
		Title:       "Drive: Move file",
		Description: "Move a file into another folder (detaches it from its current folders) (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-file-move"},
		Tags:        driveClass(),
		Form:        driveMoveForm,
		RequestHandler: run(r, "move file", func(job *sdkv1.Job, sess *oc.Session, in driveMoveInput) (map[string]any, error) {
			if err := requireAll(nv("fileId", in.FileID), nv("destinationFolderId", in.DestinationFolderID)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDriveFileID(in.FileID)
			if err != nil {
				return nil, err
			}
			dest, err := sess.ResolveFolderID(in.DestinationFolderID)
			if err != nil {
				return nil, err
			}
			// Drive moves by re-parenting: add the destination and remove the file's
			// current parents. Fetch the current parents so the file ends up in the
			// destination only, not in both places.
			parents, err := sess.FileParents(id)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{
				"fileId":     id,
				"addParents": dest,
			}
			if len(parents) > 0 {
				payload["removeParents"] = strings.Join(parents, ",")
			}
			raw, err := sess.Do("googledrive.files.update", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// -------------------------------------------------------------- rename file --

type driveRenameInput struct {
	FileID  string `json:"fileId"`
	NewName string `json:"newName"`
}

func (r *Registry) driveRenameFile() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.rename_file",
		Title:       "Drive: Rename file",
		Description: "Rename a file or folder (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-rename-box"},
		Tags:        driveClass(),
		Form:        driveRenameForm,
		RequestHandler: run(r, "rename file", func(job *sdkv1.Job, sess *oc.Session, in driveRenameInput) (map[string]any, error) {
			if err := requireAll(nv("fileId", in.FileID), nv("newName", in.NewName)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDriveFileID(in.FileID)
			if err != nil {
				return nil, err
			}
			raw, err := sess.Do("googledrive.files.update", map[string]any{
				"fileId": id,
				"name":   in.NewName,
			})
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// -------------------------------------------------------------- delete file --

type driveDeleteInput struct {
	FileID    string `json:"fileId"`
	Permanent bool   `json:"permanent,omitempty"`
}

func (r *Registry) driveDeleteFile() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.delete_file",
		Title:       "Drive: Delete file",
		Description: "Move a file to the trash, or delete it permanently (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-trash-can"},
		Tags:        driveClass(),
		Form:        driveDeleteForm,
		RequestHandler: run(r, "delete file", func(job *sdkv1.Job, sess *oc.Session, in driveDeleteInput) (map[string]any, error) {
			if err := requireAll(nv("fileId", in.FileID)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDriveFileID(in.FileID)
			if err != nil {
				return nil, err
			}
			// Permanent delete is unrecoverable; trashing (the default) is not, so
			// the common, reversible path is the default.
			if in.Permanent {
				raw, err := sess.Do("googledrive.files.delete", map[string]any{"fileId": id})
				if err != nil {
					return nil, err
				}
				out := object(raw)
				out["fileId"] = id
				out["deleted"] = true
				return out, nil
			}
			raw, err := sess.Do("googledrive.files.update", map[string]any{
				"fileId":  id,
				"trashed": true,
			})
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// --------------------------------------------------------------- share file --

type driveShareInput struct {
	FileID                string `json:"fileId"`
	Role                  string `json:"role"`
	Type                  string `json:"type"`
	EmailAddress          string `json:"emailAddress,omitempty"`
	Domain                string `json:"domain,omitempty"`
	Message               string `json:"message,omitempty"`
	SendNotificationEmail bool   `json:"sendNotificationEmail,omitempty"`
}

func (r *Registry) driveShareFile() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.share_file",
		Title:       "Drive: Share file",
		Description: "Grant a user, group, domain, or anyone a role (reader/commenter/writer) on a file (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-account-plus"},
		Tags:        driveClass(),
		Form:        driveShareForm,
		RequestHandler: run(r, "share file", func(job *sdkv1.Job, sess *oc.Session, in driveShareInput) (map[string]any, error) {
			if err := requireAll(nv("fileId", in.FileID), nv("role", in.Role), nv("type", in.Type)); err != nil {
				return nil, err
			}
			// user/group grants target an email; a domain grant targets a domain.
			switch in.Type {
			case "user", "group":
				if err := requireAll(nv("emailAddress", in.EmailAddress)); err != nil {
					return nil, err
				}
			case "domain":
				if err := requireAll(nv("domain", in.Domain)); err != nil {
					return nil, err
				}
			}
			id, err := sess.ResolveDriveFileID(in.FileID)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{
				"fileId": id,
				"role":   in.Role,
				"type":   in.Type,
			}
			if strings.TrimSpace(in.EmailAddress) != "" {
				payload["emailAddress"] = in.EmailAddress
			}
			if strings.TrimSpace(in.Domain) != "" {
				payload["domain"] = in.Domain
			}
			if strings.TrimSpace(in.Message) != "" {
				payload["email_message"] = in.Message
			}
			payload["send_notification_email"] = in.SendNotificationEmail
			raw, err := sess.Do("googledrive.permissions.create", payload)
			if err != nil {
				return nil, err
			}
			out := object(raw)
			out["fileId"] = id
			if _, hasLink := out["webViewLink"]; !hasLink {
				out["url"] = driveFileLink(id)
			}
			return out, nil
		}),
	}
}

// -------------------------------------------------------------- export file --

type driveExportInput struct {
	FileID   string `json:"fileId"`
	MimeType string `json:"mimeType"`
}

func (r *Registry) driveExportFile() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googledrive.export_file",
		Title:       "Drive: Export file",
		Description: "Export a Google Workspace file (Docs/Sheets/Slides) to another format and return a transit URL (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-file-export"},
		Tags:        driveClass(),
		Form:        driveExportForm,
		RequestHandler: run(r, "export file", func(job *sdkv1.Job, sess *oc.Session, in driveExportInput) (map[string]any, error) {
			if err := requireAll(nv("fileId", in.FileID), nv("mimeType", in.MimeType)); err != nil {
				return nil, err
			}
			id, err := sess.ResolveDriveFileID(in.FileID)
			if err != nil {
				return nil, err
			}
			raw, err := sess.Do("googledrive.files.export", map[string]any{
				"fileId":   id,
				"mimeType": in.MimeType,
			})
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}
