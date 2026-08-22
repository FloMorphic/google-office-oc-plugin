// Every form this plugin serves is declared here with the SDK's formkit builder,
// which generates the JSON Schema and the JSON Forms UI schema from one statement
// per field. The forms are package-level or built once at start-up, so a
// malformed one panics at start-up (Build calls Validate) — where it is a
// compile-time-shaped mistake rather than a dialog that will not open.
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

// ------------------------------------------------------------------ actions --

// runForm builds the generic "run action" form for one service. The `action`
// field carries a "Load actions" button whose meta rebuilds it into a drop-down
// of the service's live catalog (see meta.go's metaActions / formkit.Choose).
func runForm(s service) sdkv1.FormBuilder {
	return formkit.New(s.title).
		Describe("Pick an action, then supply its input as a JSON object. Press Load actions to fetch the live catalog for this service.").
		Add(
			formkit.Text("action", "Action").
				Required().
				Describe("The "+s.id+" action to run, e.g. its name from the catalog. Press ↻ to load and pick one.").
				Lookup(s.id+".meta.actions", "Load actions").
				Picks(s.id+".meta.actions"),
			formkit.TextArea("input", "Input (JSON)").
				Describe(`The action's input as a JSON object, e.g. {"spreadsheetId":"…","range":"Sheet1!A1:B2"}. String values accept {{$.path}} tokens resolved against the flow scope. Leave empty for actions that take no input.`),
		).
		Build()
}
