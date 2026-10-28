package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func appEntry(t *testing.T, fields map[string]any) []*pluginv1.ConfigEntry {
	t.Helper()
	value, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatal(err)
	}
	return []*pluginv1.ConfigEntry{{Key: "app", Value: value}}
}

// TestConfigurationCheck answers each client ID the way api.simkl.com does
// without a user token: 200 for an app that allows anonymous reads, 401
// user_token_required for an AUTH V2 app, and 412 client_id_failed for an ID
// Simkl does not know.
func TestConfigurationCheck(t *testing.T) {
	answers := map[string]struct {
		status int
		body   string
	}{
		"v1-app":  {http.StatusOK, `[]`},
		"v2-app":  {http.StatusUnauthorized, `{"error":"user_token_required","code":401}`},
		"unknown": {http.StatusPreconditionFailed, `{"error":"client_id_failed","code":412}`},
		"down":    {http.StatusBadGateway, `{}`},
	}
	for _, tt := range []struct {
		name        string
		app         map[string]any
		wantOK      bool
		wantMessage string
		wantProbes  int
	}{
		{name: "AUTH V1 app", app: map[string]any{"client_id": "v1-app"}, wantOK: true, wantProbes: 1},
		{name: "AUTH V2 app", app: map[string]any{"client_id": "v2-app"}, wantOK: true, wantProbes: 1},
		{name: "AUTH V1 app with AUTH V2 app", app: map[string]any{"client_id": "v1-app", "v2_client_id": "v2-app"}, wantOK: true, wantProbes: 2},
		{name: "unknown client ID", app: map[string]any{"client_id": "unknown"}, wantMessage: "Simkl does not recognize the client ID", wantProbes: 1},
		{name: "unknown AUTH V2 client ID", app: map[string]any{"client_id": "v1-app", "v2_client_id": "unknown"}, wantMessage: "Simkl does not recognize the AUTH V2 client ID", wantProbes: 2},
		{name: "Simkl unavailable", app: map[string]any{"client_id": "down"}, wantMessage: "Could not check the client ID with Simkl", wantProbes: 1},
		{name: "blank client ID", app: map[string]any{"client_id": "  "}, wantMessage: "Enter the client ID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			probes := 0
			server, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				probes++
				if r.Method != http.MethodGet || r.URL.Path != "/search/id" {
					t.Errorf("request = %s %s, want GET /search/id", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("check sent an Authorization header")
				}
				answer := answers[r.Header.Get("simkl-api-key")]
				w.WriteHeader(answer.status)
				writeJSON(t, w, answer.body)
			}))

			resp, err := server.ConfigurationChecks().TestConfiguration(context.Background(),
				&pluginv1.TestConfigurationRequest{Config: appEntry(t, tt.app)})
			if err != nil {
				t.Fatalf("TestConfiguration error = %v", err)
			}
			if resp.GetOk() != tt.wantOK || !strings.Contains(resp.GetMessage(), tt.wantMessage) {
				t.Fatalf("TestConfiguration = ok %v, message %q; want ok %v, message containing %q",
					resp.GetOk(), resp.GetMessage(), tt.wantOK, tt.wantMessage)
			}
			if probes != tt.wantProbes {
				t.Fatalf("Simkl requests = %d, want %d", probes, tt.wantProbes)
			}
		})
	}
}
