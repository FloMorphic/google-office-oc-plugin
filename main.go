// Command google-oc is a FloMorphic plugin node for Google Workspace, over
// OpenConnector.
//
// It exposes one node per Google service — Sheets, Drive, Calendar, Docs — but
// holds NO Google token and makes NO Google API calls. A Google account is
// connected once, centrally, in FloMorphic → Connect (via OpenConnector / oomol),
// where OAuth grants the Workspace scopes. Each node is a generic request
// builder: its settings dialog picks which connected account to act as (by
// alias), it lists that service's actions live from the gateway, and every run
// asks the FloMorphic backend, over NATS, to execute the chosen OpenConnector
// action as that account. The backend holds the credential.
//
// Because it reaches FloMorphic's central services (the `flomorphic.svc.oc.*`
// subjects), this plugin must run with an OPEN (multi) runtime credential — a
// strict, plugin-scoped credential cannot publish there. See the README.
package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/FloMorphic/google-office-oc-plugin/internal/actions"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

const version = "v0.1.0"

// sendTimeoutSeconds is the NATS request/reply deadline for the proxy
// round-trip. Above the SDK's 5s default because each action is a NATS hop to
// the backend, which then calls the OpenConnector gateway. Override per
// deployment with REQ_TIMEOUT (seconds) in .env.inflow.
const sendTimeoutSeconds = 30

func main() {
	envFile := os.Getenv("INFLOW_ENV_FILE")
	if envFile == "" {
		envFile = ".env.inflow"
	}

	// The dotenv carries the platform identity only — PLUGIN_ID, INFRA_CRED,
	// INFRA_URL. No Google configuration ever lives here.
	plugin, err := sdkv1.NewPlugin(sdkv1.WithDotEnv(envFile), sdkv1.WithTimeout(sendTimeoutSeconds))
	if err != nil {
		log.Fatalf("google-oc: cannot connect to infra (%s): %v", envFile, err)
	}

	// The registry sends its account/action/catalog requests over the plugin's
	// NATS connection (Plugin.Send: request/reply with retry).
	registry := actions.New(plugin.Send)

	plugin.Intro(sdkv1.PluginIntro{
		Name:     "Google Workspace (OpenConnector)",
		Author:   "mehdi-shokohi",
		Version:  version,
		Settings: registry.SettingsForm(),
	})
	plugin.RequiredParams(registry.Settings())

	all := registry.All()
	plugin.AddAction(all...)
	plugin.AddMeta(registry.Metas()...)

	if err := plugin.Start(); err != nil {
		log.Fatalf("google-oc: start: %v", err)
	}

	methods := make([]string, 0, len(all))
	for _, action := range all {
		methods = append(methods, action.Method)
	}
	log.Printf("google-oc plugin %s ready with %d nodes: %s", version, len(all), strings.Join(methods, ", "))
	log.Printf("google-oc: these nodes act as a Google account connected in FloMorphic → Connect")

	// Start() only wires up subscriptions; the process has to stay alive to
	// serve them.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("google-oc: shutting down")
}
