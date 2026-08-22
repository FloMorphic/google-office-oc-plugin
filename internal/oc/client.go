// Package oc is the plugin's bridge to OpenConnector, over FloMorphic's central
// credential.
//
// The FloMorphic backend is a GENERIC proxy: it stores the OpenConnector
// connection (a connected Google account, authorized via OAuth in Connect) and
// forwards a request verbatim over the single NATS subject
// `flomorphic.svc.oc.proxy`, injecting the auth header. It knows nothing about
// Google — ALL of that lives here. This plugin holds no Google token and makes
// no Google API calls; it builds OpenConnector requests and lets the backend
// execute them.
//
// Reaching the proxy needs an OPEN (multi) runtime credential — a strict,
// plugin-scoped credential cannot publish on `flomorphic.>`. See the README.
package oc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
)

// ServicePrefix is the family this plugin targets. A connected Google account
// reports a service id that starts with "google" (googlesheets, googledrive,
// googlecalendar, googledocs, or a single "google" umbrella connection), so one
// authorized account can back every node in this plugin. Filtering on the prefix
// keeps unrelated connections out of the account picker while tolerating whichever
// exact id the gateway assigns.
const ServicePrefix = "google"

// proxySubject is the FloMorphic OpenConnector NATS proxy (see the backend's
// inflow/ocproxy.go). It sits OUTSIDE the inflow node protocol.
const proxySubject = "flomorphic.svc.oc.proxy"

// Sender is a NATS request/reply, satisfied by *sdkv1.Plugin.Send: the plugin's
// connection carries the round-trip (with retry + the configured deadline).
type Sender func(subject string, data []byte) (*nats.Msg, error)

// Client runs every OpenConnector request through the FloMorphic NATS proxy.
type Client struct {
	send Sender
}

func New(send Sender) *Client { return &Client{send: send} }

// Account is one connected Google account, from GET /v1/connections.
type Account struct {
	ID           string `json:"id"`
	Service      string `json:"service"`
	Status       string `json:"status"`
	AccountLabel string `json:"accountLabel"`
	Alias        string `json:"alias"`
	AuthType     string `json:"authType"`
	IsDefault    bool   `json:"isDefault"`
}

// Name is the human label for an account, falling back to the alias then the
// service when the gateway gives no friendly label.
func (a Account) Name() string {
	switch {
	case a.AccountLabel != "":
		return a.AccountLabel
	case a.Alias != "":
		return a.Alias
	default:
		return a.Service
	}
}

// ActionInfo is one entry of a service's action catalog, from
// GET /v1/actions?service=<service>. The plugin lists these to populate the
// action picker; it does not hardcode the catalog.
type ActionInfo struct {
	ID            string `json:"id"`
	Service       string `json:"service"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	OperationType string `json:"operationType"`
}

// proxyRequest mirrors the backend's OCProxyRequest.
type proxyRequest struct {
	Connection string            `json:"connection,omitempty"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      map[string]string `json:"query,omitempty"`
	Body       any               `json:"body,omitempty"`
}

// proxyReply mirrors the backend's OCProxyResponse.
type proxyReply struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// proxy forwards one gateway request and returns its (unwrapped) response body.
func (c *Client) proxy(method, path, connection string, query map[string]string, body any) (json.RawMessage, error) {
	payload, err := json.Marshal(proxyRequest{
		Connection: connection,
		Method:     method,
		Path:       path,
		Query:      query,
		Body:       body,
	})
	if err != nil {
		return nil, fmt.Errorf("oc: encode proxy request: %w", err)
	}

	msg, err := c.send(proxySubject, payload)
	if err != nil {
		return nil, fmt.Errorf("oc: proxy round-trip failed: %w", err)
	}

	var reply proxyReply
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &reply); err != nil {
			return nil, fmt.Errorf("oc: decode proxy reply: %w", err)
		}
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("%s", reply.Error)
	}
	if reply.Status >= 400 {
		return nil, fmt.Errorf("OpenConnector %s returned %d", path, reply.Status)
	}
	return reply.Body, nil
}

// Accounts lists the connected Google accounts (GET /v1/connections, filtered to
// the Google service family).
func (c *Client) Accounts(connection string) ([]Account, error) {
	raw, err := c.proxy("GET", "/v1/connections", connection, nil, nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data []Account `json:"data"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("oc: decode connections: %w", err)
		}
	}
	out := make([]Account, 0, len(env.Data))
	for _, a := range env.Data {
		if strings.HasPrefix(a.Service, ServicePrefix) {
			out = append(out, a)
		}
	}
	return out, nil
}

// ListActions returns the action catalog for one Google service
// (GET /v1/actions?service=<service>), so the action picker is always in sync
// with the gateway instead of a hardcoded list.
func (c *Client) ListActions(service, connection string) ([]ActionInfo, error) {
	raw, err := c.proxy("GET", "/v1/actions", connection, map[string]string{"service": service}, nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data []ActionInfo `json:"data"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("oc: decode actions: %w", err)
		}
	}
	return env.Data, nil
}

// Resolve returns one account by alias (or the default when alias is empty). A
// nil account with a nil error means "nothing connected / no such alias" — a
// caller decides how to phrase that.
func (c *Client) Resolve(alias, connection string) (*Account, error) {
	list, err := c.Accounts(connection)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	wanted := strings.TrimSpace(alias)
	if wanted == "" {
		for i := range list {
			if list[i].IsDefault {
				return &list[i], nil
			}
		}
		return &list[0], nil
	}
	for i := range list {
		if list[i].Alias == wanted || list[i].AccountLabel == wanted {
			return &list[i], nil
		}
	}
	return nil, nil
}

// Bind resolves the account a node is bound to and returns a Session: an
// OpenConnector handle scoped to that account, the analog of a ready API client.
// The error is already phrased for the user.
func (c *Client) Bind(alias, connection string) (*Session, error) {
	acct, err := c.Resolve(alias, connection)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		if strings.TrimSpace(alias) != "" {
			return nil, fmt.Errorf("no connected Google account with alias %q — pick one in the node's settings", alias)
		}
		return nil, fmt.Errorf("no Google account connected in FloMorphic → Connect")
	}
	return &Session{client: c, Account: *acct, connection: connection}, nil
}

// Session is the OpenConnector client bound to one resolved Google account. An
// action calls Do with a fully-qualified OpenConnector action id and its input;
// the backend runs it as this account and returns the gateway's raw `data`
// payload.
type Session struct {
	client     *Client
	Account    Account
	connection string
}

// Do runs one action as this account —
// POST /v1/actions/<service>.<action> {input}. `action` is the fully-qualified
// id, e.g. "googlesheets.create_google_sheet1".
func (s *Session) Do(action string, input any) (json.RawMessage, error) {
	var query map[string]string
	if s.Account.Alias != "" {
		query = map[string]string{"alias": s.Account.Alias}
	}
	raw, err := s.client.proxy("POST", fmt.Sprintf("/v1/actions/%s", action), s.connection, query, map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	// Unwrap the gateway's {data} envelope when present, else return the body.
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &env) == nil && len(env.Data) > 0 {
		return env.Data, nil
	}
	return raw, nil
}
