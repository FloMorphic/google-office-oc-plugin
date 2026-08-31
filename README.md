# Google Workspace (OpenConnector) — `google-oc`

A FloMorphic **plugin node** set for Google Workspace that authenticates through
FloMorphic's central **Connect** feature (OpenConnector / oomol), built on the Go
[`go-plugin-sdk`](https://github.com/Inflowenger/go-plugin-sdk) (`sdkv1`).

It appears on the workflow canvas as **four nodes** — Google Sheets, Google
Drive, Google Calendar, and Google Docs. It holds **no Google token** and makes
**no Google API calls** — it is a *request builder*. A Google account is
connected **once, centrally**, in **FloMorphic → Connect** (OAuth grants the
Workspace scopes); these nodes just pick which connected account to act as, and
ask the FloMorphic backend to run each action for them.

> Why the name? This plugin depends on FloMorphic's central auth. A future
> `google` plugin could instead talk to the Google APIs directly with its own
> OAuth client — the `-oc` suffix keeps that door open.

## How it works

The FloMorphic backend is a **generic proxy**: it stores the OpenConnector token
and forwards any request over one NATS subject, `flomorphic.svc.oc.proxy`,
injecting the auth header. It knows nothing about Google. **All** Google
knowledge — which accounts exist, which actions a service has, which endpoint to
call — is read **live from the gateway** by this plugin.

```
 google-oc node                    FloMorphic backend            OpenConnector gateway
 (builds + vets the request)       (injects auth, forwards)      (oomol cloud / self-host)
   │  flomorphic.svc.oc.proxy                │                            │
   │  {method,path,body,connection?}         │  <method> <path>           │
   ├──────────── NATS req ──────────────────▶│  + Authorization header ──▶│
   │◀──────────── {status,body} ─────────────┤◀───────────────────────────┤
```

Each node uses that one proxy to:
1. `GET /v1/connections` → the connected Google accounts (filtered to the
   `google*` service family);
2. `GET /v1/actions?service=<service>` → the live action catalog, to fill the
   node's action picker;
3. `POST /v1/actions/<service>.<action>?alias=…` → run the chosen action as that
   account.

- The plugin never sees the Google token. The backend holds the OpenConnector
  credential and performs the call.
- One connected account can be shared by every node and every service;
  re-authorizing it happens in the Connect page, no plugin redeploy.

## Design — a generic runner, not one node per operation

Between Sheets, Drive, Calendar and Docs there are **well over a hundred**
OpenConnector actions, and the catalog changes over time. The SDK also **cannot**
populate a form's drop-down options at runtime for a compiled-in field. So rather
than hand-code a node per operation, each service is **one generic "run action"
node**:

- **Action** — a text field with a **Load actions** button. Pressing it fetches
  the service's live catalog and rebuilds the field into a drop-down of exactly
  what the gateway can run (value = action name, label shows the operation type).
- **Input (JSON)** — the action's input as a JSON object, forwarded verbatim as
  the OpenConnector `input`. String values accept `{{$.path}}` tokens resolved
  against the flow scope, so ids and text can reference upstream data.

This keeps the plugin small and always in sync with the gateway. The trade-off is
less per-field hand-holding than a bespoke form — you supply inputs as JSON. The
shape of each action's input is described by its `inputSchema` in
`GET /v1/actions?service=<service>`.

### Example

Node: **Google Sheets (OpenConnector)**

```
action:  create_google_sheet1        [Load actions ↻]
input:   { "title": "Q3 report" }
```

→ `POST /v1/actions/googlesheets.create_google_sheet1  { "input": { "title": "Q3 report" } }`

Reading a range with an upstream id:

```
action:  batch_get
input:   { "spreadsheetId": "{{$.trigger.sheetId}}", "ranges": ["Sheet1!A1:B10"] }
```

## Nodes

| Method | Title | OpenConnector service | `tags.class` |
|--------|-------|--------|--------|
| `googlesheets.run`   | Google Sheets (OpenConnector)   | `googlesheets`   | `sheets` |
| `googledrive.run`    | Google Drive (OpenConnector)    | `googledrive`    | `drive` |
| `googlecalendar.run` | Google Calendar (OpenConnector) | `googlecalendar` | `calendar` |
| `googledocs.run`     | Google Docs (OpenConnector)     | `googledocs`     | `docs` |

Each node carries `Action.Tags["class"]` (the SDK's reserved grouping key), so
the canvas palette groups the four sub-products even though they ship in one
plugin binary. Adding another Google service is a one-line entry in `services` in
[actions.go](internal/actions/actions.go) — the runner, form, action picker, and
class tag are all derived from it.

## Set-up (pick from a list — nothing to type)

1. **Connect a Google account** once in FloMorphic → **Connect** (OAuth; it shows
   as connected there with the Workspace scopes granted).
2. On any node, open **settings**, press **Load accounts**, and **pick** your
   account from the drop-down. (Leave it on the default to use the default
   account.) Press **Test account**, then **Save** — the platform stores this as
   a reusable **settings profile**.
3. Bind that profile on each Google node, then press **Load actions** on the node
   and pick the action to run.

There is **no** token to paste or alias to copy — the account list and the action
catalog are populated live from the gateway, and you just choose. The optional
**Gateway** field pins to one Connect connection when several are configured
(hosted oomol vs self-hosted); leave it empty to span all.

## Capability check

The gateway remains the authoritative gate: each action declares its own
`requiredScopes`, and OpenConnector enforces them against the connected account's
OAuth grant. This plugin does not re-implement that check — if the connected
account lacks a scope, the gateway rejects the call and the node surfaces the
error. Connect (re-)requests the needed scopes when you authorize the account.

## Install

Install it like any other plugin: add it from its GitHub repo on the FloMorphic
extension **Add plugin** page. Its runtime credential reaches the
`flomorphic.svc.*` subjects out of the box **only** when it is an OPEN (multi)
credential (the NATS token must allow `flomorphic.svc.>`). A strict,
plugin-scoped credential **cannot** publish there — see below.

The one prerequisite is a Google connection: complete it once in FloMorphic →
**Connect**. After that the nodes' account drop-down and action pickers populate
live and the plugin runs.

## Credential (important)

Because this plugin reaches FloMorphic's central services on
`flomorphic.svc.oc.*`, `INFRA_CRED` must be an **OPEN (multi)** runtime
credential. Mint one from FloMorphic → Settings → MultiPlugin Credential (or the
installer's multi option). A strict, plugin-scoped credential cannot publish on
`flomorphic.>` and every action will fail with a NATS no-responders/timeout.

## Request timeout

Every action is a NATS round-trip to the FloMorphic backend, which then calls
OpenConnector. The node sets **30s** in code (`WithTimeout` in [main.go](main.go)),
above the SDK's 5s default.

Override it per deployment — without touching code — with the **`REQ_TIMEOUT`**
env var (in **seconds**) in your `.env.inflow`:

```env
# .env.inflow
REQ_TIMEOUT=50
```

Precedence: `REQ_TIMEOUT` (env) → `WithTimeout(30)` (code) → 5s (SDK default). If
a call still exceeds the deadline, the node reports a bare `TIMEOUT`; raise
`REQ_TIMEOUT`.

## Develop

```bash
go mod tidy
go build ./...       # compile
go run .             # run (reads .env.inflow)
```

The SDK logs each subscribed subject on startup. Verify by adding a node to a
flow and running it.

> **SDK dependency.** Pinned to the published
> `github.com/Inflowenger/go-plugin-sdk v0.2.0`, which carries both the
> `WithTimeout` / `REQ_TIMEOUT` support and the `Action.Tags` grouping key this
> plugin uses. No local `replace` is needed.

## Layout

```
main.go                        entry point: identity, intro, register nodes/metas, block
internal/oc/client.go          the generic OpenConnector proxy client (connections, account resolution, run)
internal/oc/files.go           shared Drive file plumbing (list/name→id) + the Drive-node helpers
internal/oc/sheets.go          Sheets session helpers (tabs, spreadsheet resolve)
internal/oc/docs.go            Docs session helpers (document resolve)
internal/oc/calendar.go        Calendar session helpers (calendar list)
internal/actions/actions.go    node wiring + the shared job handler + result decoding
internal/actions/sheets.go     Sheets actions
internal/actions/docs.go       Docs actions
internal/actions/drive.go      Drive actions
internal/actions/calendar.go   Calendar actions
internal/actions/forms.go      formkit forms: settings + each action's form
internal/actions/meta.go       account list/test + the dependent-field (file/folder/tab) pickers
internal/actions/vars.go       {{$.path}} token resolution over the action input
```
