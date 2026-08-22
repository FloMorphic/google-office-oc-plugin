package actions

import (
	"fmt"
	"strings"

	"github.com/Inflowenger/go-plugin-sdk/formkit"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// Metas are the settings + form meta RPCs, served outside the job lifecycle so a
// dialog can call them while it is open:
//   - list/test the connected Google account (shared by every node);
//   - list the live action catalog for each service (rebuilds that node's
//     "action" field into a drop-down).
func (r *Registry) Metas() []sdkv1.Meta {
	metas := []sdkv1.Meta{
		{Method: "google.meta.account.list", RequestHandler: r.metaAccountList},
		{Method: "google.meta.account.test", RequestHandler: r.metaAccountTest},
	}
	for _, s := range services {
		metas = append(metas, sdkv1.Meta{
			Method:         s.id + ".meta.actions",
			RequestHandler: r.metaActions(s),
		})
	}
	return metas
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

// metaActions backs a run node's "Load actions" button: it fetches the live
// action catalog for the service and REBUILDS the "action" field into a drop-down
// (value = action name, label = name + operation type). The catalog comes from
// the gateway, so the picker never drifts from what the backend can actually run.
func (r *Registry) metaActions(s service) func(sdkv1.Request) any {
	return func(req sdkv1.Request) any {
		body := decodeMeta(req.Data)
		connection := text(pick(body, "connection"))

		list, err := r.oc.ListActions(s.id, connection)
		if err != nil {
			return formkit.Failure("%s", err.Error()).About("action").Patch(nil)
		}
		if len(list) == 0 {
			return formkit.
				Warning("No %s actions returned by the gateway.", s.id).
				About("action").
				Patch(nil)
		}

		options := make([]formkit.Option, 0, len(list))
		for _, a := range list {
			label := a.Name
			if a.OperationType != "" {
				label += "  (" + a.OperationType + ")"
			}
			options = append(options, formkit.Option{Value: a.Name, Label: label})
		}
		return formkit.Choose(
			r.forms[s.id],
			"action",
			options,
			formkit.FormData(body),
			formkit.Success("%d %s action(s) — pick one:", len(list), s.id).About("action"),
		)
	}
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
