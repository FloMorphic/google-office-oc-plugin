// Package actions wires the OpenConnector bridge onto the plugin's node actions:
// one Action per concrete Google operation, plus the meta RPCs the settings form
// uses to list and test the connected Google accounts.
//
// This plugin is a pure request builder: it holds no Google token and makes no
// Google API calls. Each action builds a typed input payload and asks the
// FloMorphic backend to run the matching OpenConnector action as the chosen
// connected account; the backend holds the credential (see ../oc).
//
// One binary hosts several Google products (Sheets, Docs, Calendar, Drive). Each
// action is tagged with Tags["class"] = its service, so the frontend bundles and
// isolates each product's ports together. Operations are added one at a time in
// the per-service files (sheets.go, …).
package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FloMorphic/google-office-oc-plugin/internal/oc"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// class labels group an action under its Google product on the canvas via
// Tags["class"]; the frontend renders each class as its own bundle of ports.
const (
	classSheets   = "sheets"
	classDocs     = "docs"
	classCalendar = "calendar"
	classDrive    = "drive"
)

// Registry owns what the actions share: the OpenConnector client that turns each
// call's bound settings profile into a session. The plugin holds no Google
// configuration of its own.
type Registry struct {
	oc *oc.Client
}

// New builds the registry over a NATS sender (the plugin's Send).
func New(send oc.Sender) *Registry { return &Registry{oc: oc.New(send)} }

// All returns every action this plugin exposes, in the order the canvas shows
// them — grouped by service so a class's ports sit together.
func (r *Registry) All() []sdkv1.Action {
	var all []sdkv1.Action
	all = append(all, r.sheetsActions()...)
	all = append(all, r.docsActions()...)
	all = append(all, r.driveActions()...)
	all = append(all, r.calendarActions()...)
	return all
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

// handler is what each action implements: pure work over a ready session (the
// OpenConnector handle bound to the chosen account), with finishing the job left
// to run. Returning an error fails the node.
type handler[T any] func(job *sdkv1.Job, sess *oc.Session, in T) (map[string]any, error)

// run adapts a handler into an SDK job handler: decode the typed body and the
// settings profile that came with it, bind the account, resolve {{$...}} tokens,
// report progress, and terminate the job exactly once on every path — success,
// bad input, a missing/unresolved account, or a failed gateway call. The plugin
// builds and vets the request; FloMorphic only proxies it.
func run[T any](r *Registry, title string, fn handler[T]) sdkv1.JobHandler {
	return func(job sdkv1.Job) {
		req, err := sdkv1.CastRequestTo[T](job.Req.Data)
		if err != nil {
			job.DoneWithError("invalid request body: " + err.Error())
			return
		}
		alias, connection := decodeSettings(job.Req.Data)

		sess, err := r.oc.Bind(alias, connection)
		if err != nil {
			job.DoneWithError(err.Error())
			return
		}

		// Resolve {{$...}} tokens in every string input against the flow scope.
		resolveInputVars(&job, &req.Body)

		job.Progress(20, sdkv1.Frame{Title: title, Content: "as " + sess.Account.Name()})

		out, err := fn(&job, sess, req.Body)
		if err != nil {
			job.DoneWithError(err.Error())
			return
		}
		if out == nil {
			out = map[string]any{}
		}

		job.Progress(90, sdkv1.Frame{Title: title, Content: "done"})
		job.Done(out)
	}
}

// decodeSettings pulls the bound profile's alias/connection out of the request
// body, reading the same bytes the action input came from so the settings
// envelope stays out of every action's input struct.
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

type namedValue struct{ name, value string }

func nv(name, value string) namedValue { return namedValue{name: name, value: value} }

// requireAll reports the required inputs left blank, so an action fails with a
// clear message instead of a raw gateway 400.
func requireAll(values ...namedValue) error {
	var missing []string
	for _, v := range values {
		if strings.TrimSpace(v.value) == "" {
			missing = append(missing, v.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required input: %s", strings.Join(missing, ", "))
	}
	return nil
}

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
