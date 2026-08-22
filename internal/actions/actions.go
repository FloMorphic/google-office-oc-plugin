// Package actions wires the OpenConnector bridge onto the plugin's node actions.
//
// This plugin is a pure request builder: it holds no Google token and makes no
// Google API calls. It exposes ONE node per Google service (Sheets, Drive,
// Calendar, Docs) — a generic "run action" node. Each node picks an
// OpenConnector action by name (populated live from the gateway) and forwards a
// JSON input payload to be run as the chosen connected account; the backend
// holds the credential (see ../oc).
//
// Why generic rather than one node per operation: the four services expose well
// over a hundred actions between them, the SDK cannot populate a form's options
// at runtime for every one, and the catalog moves. A generic runner stays in
// sync with the gateway and keeps the codebase small; the action picker is filled
// live via a meta (see meta.go).
package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
	"github.com/mehdi-shokohi/google-oc-plugin/internal/oc"
)

// Registry owns what the actions share: the OpenConnector client that turns each
// call's bound settings profile into a session, and the per-service forms the
// action pickers rebuild. The plugin holds no Google configuration of its own.
type Registry struct {
	oc    *oc.Client
	forms map[string]sdkv1.FormBuilder // service id -> its run form
}

// New builds the registry over a NATS sender (the plugin's Send).
func New(send oc.Sender) *Registry {
	r := &Registry{oc: oc.New(send), forms: make(map[string]sdkv1.FormBuilder)}
	for _, s := range services {
		r.forms[s.id] = runForm(s)
	}
	return r
}

// service describes one Google service this plugin fronts as a node.
type service struct {
	id    string // OpenConnector service id, e.g. "googlesheets"
	title string // node title on the canvas
	icon  string // mdi icon
	desc  string // node description
	class string // Action.Tags["class"]: the sub-product ports group under
}

// services is the fixed set of Google services this plugin exposes, in canvas
// order. Adding a Google service is a one-line change here — the runner, the
// form, and the action picker are all generic.
var services = []service{
	{id: "googlesheets", title: "Google Sheets (OpenConnector)", icon: "mdi-google-spreadsheet", desc: "Run any Google Sheets action as a connected Google account (via OpenConnector).", class: "sheets"},
	{id: "googledrive", title: "Google Drive (OpenConnector)", icon: "mdi-google-drive", desc: "Run any Google Drive action as a connected Google account (via OpenConnector).", class: "drive"},
	{id: "googlecalendar", title: "Google Calendar (OpenConnector)", icon: "mdi-calendar", desc: "Run any Google Calendar action as a connected Google account (via OpenConnector).", class: "calendar"},
	{id: "googledocs", title: "Google Docs (OpenConnector)", icon: "mdi-file-document", desc: "Run any Google Docs action as a connected Google account (via OpenConnector).", class: "docs"},
}

// All returns every action this plugin exposes, in the order the canvas shows them.
func (r *Registry) All() []sdkv1.Action {
	out := make([]sdkv1.Action, 0, len(services))
	for _, s := range services {
		out = append(out, r.runAction(s))
	}
	return out
}

// settingsEnvelope is the platform-managed half of every request body. The
// runtime folds the settings profile bound to the node into the call as
// `body.settings`; the account this node acts as travels here, never as action
// input. Actions declare only their own fields — this envelope is decoded
// alongside them.
type settingsEnvelope struct {
	Settings struct {
		Alias      string `json:"alias"`
		Connection string `json:"connection"`
	} `json:"settings"`
}

// runInput is the body of a run node: which OpenConnector action to invoke, and
// the JSON input to pass to it.
type runInput struct {
	Action string `json:"action"`
	Input  string `json:"input"`
}

// runAction builds the generic "run action" node for one service.
func (r *Registry) runAction(s service) sdkv1.Action {
	return sdkv1.Action{
		Method:      s.id + ".run",
		Title:       s.title,
		Description: s.desc,
		Icon:        sdkv1.Icon{Icon: s.icon},
		Tags:        map[string]string{"class": s.class},
		Form:        r.forms[s.id],
		RequestHandler: func(job sdkv1.Job) {
			req, err := sdkv1.CastRequestTo[runInput](job.Req.Data)
			if err != nil {
				job.DoneWithError("invalid request body: " + err.Error())
				return
			}
			alias, connection := decodeSettings(job.Req.Data)

			action := strings.TrimSpace(req.Body.Action)
			if action == "" {
				job.DoneWithError("missing required input: action — press Load actions and pick one")
				return
			}

			bot, err := r.oc.Bind(alias, connection)
			if err != nil {
				job.DoneWithError(err.Error())
				return
			}

			input, err := parseInput(req.Body.Input)
			if err != nil {
				job.DoneWithError(err.Error())
				return
			}
			// Resolve {{$...}} tokens in every string leaf of the input against
			// the flow scope, so any field can reference upstream data.
			input = resolveInputVars(&job, input)

			fq := qualify(s.id, action)
			job.Progress(20, sdkv1.Frame{Title: s.title, Content: fq + " as " + bot.Account.Name()})

			raw, err := bot.Do(fq, input)
			if err != nil {
				job.DoneWithError(err.Error())
				return
			}

			job.Progress(90, sdkv1.Frame{Title: s.title, Content: "done"})
			job.Done(object(raw))
		},
	}
}

// qualify turns a picked action name into the fully-qualified id the gateway
// wants, tolerating a value the user typed with or without the service prefix.
func qualify(serviceID, action string) string {
	if strings.HasPrefix(action, serviceID+".") {
		return action
	}
	return serviceID + "." + action
}

// parseInput decodes the JSON object the user supplied as the action's input.
// Blank means "no input". A non-object (array, scalar) is rejected — every
// gateway action takes a named-field object.
func parseInput(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var in map[string]any
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, fmt.Errorf("input is not a valid JSON object: %v", err)
	}
	if in == nil {
		return map[string]any{}, nil
	}
	return in, nil
}

// decodeSettings pulls the bound profile's alias/connection out of the request
// body, reading the same bytes the action input came from so the settings
// envelope stays out of the action's input struct.
func decodeSettings(data []byte) (alias, connection string) {
	env, err := sdkv1.CastRequestTo[settingsEnvelope](data)
	if err != nil || env == nil {
		return "", ""
	}
	return strings.TrimSpace(env.Body.Settings.Alias), strings.TrimSpace(env.Body.Settings.Connection)
}

// ---------------------------------------------------------------- results --

// object decodes the gateway's response into the map the SDK commits onto the
// node's scope. A JSON object is used as-is; anything else is wrapped under
// "result" so it still lands somewhere addressable downstream.
func object(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{"ok": true}
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil && obj != nil {
		return obj
	}
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		return map[string]any{"result": v}
	}
	return map[string]any{"result": string(raw)}
}

// ---------------------------------------------------------------- helpers --

// decodeMeta reads a meta RPC's arguments. Meta calls come from the form
// renderer rather than the job pipeline, so the payload may or may not be
// wrapped in the `{_registry, body}` envelope — try both, and treat anything
// unreadable as "no arguments".
func decodeMeta(data []byte) map[string]any {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}
	}
	var envelope struct {
		Body json.RawMessage `json:"body"`
	}
	if json.Unmarshal(data, &envelope) == nil && len(envelope.Body) > 0 {
		var out map[string]any
		if json.Unmarshal(envelope.Body, &out) == nil && out != nil {
			return out
		}
	}
	var out map[string]any
	if json.Unmarshal(data, &out) == nil && out != nil {
		return out
	}
	return map[string]any{}
}
