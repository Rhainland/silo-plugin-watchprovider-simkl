package provider

import (
	"context"
	"net/http"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// probePath is an anonymous lookup Simkl answers according to the client ID
// alone: 412 client_id_failed for an ID it does not know, 401
// user_token_required for an AUTH V2 app, and the result for an AUTH V1 app.
const probePath = "/search/id?imdb=tt0133093"

// configurationChecks runs the plugin's own Check Connection: it asks Simkl
// whether it knows the staged client IDs. Whether an ID can actually sign a
// profile in only shows when a profile connects.
type configurationChecks struct {
	pluginv1.UnimplementedConfigurationChecksServer
	server *Server
}

// ConfigurationChecks returns the ConfigurationChecks service for this
// provider. It shares the server's Simkl client.
func (s *Server) ConfigurationChecks() pluginv1.ConfigurationChecksServer {
	return &configurationChecks{server: s}
}

func (c *configurationChecks) TestConfiguration(ctx context.Context, req *pluginv1.TestConfigurationRequest) (*pluginv1.TestConfigurationResponse, error) {
	var app map[string]any
	for _, entry := range req.GetConfig() {
		if entry.GetKey() == "app" {
			app = entry.GetValue().AsMap()
		}
	}
	clientID := stringField(app, "client_id")
	if clientID == "" {
		return failed("Enter the client ID of your Simkl API app."), nil
	}
	if message := c.checkClientID(ctx, clientID, "client ID", false); message != "" {
		return failed(message), nil
	}
	if v2ClientID := stringField(app, "v2_client_id"); v2ClientID != "" {
		if message := c.checkClientID(ctx, v2ClientID, "AUTH V2 client ID", true); message != "" {
			return failed(message), nil
		}
	}
	return &pluginv1.TestConfigurationResponse{Ok: true}, nil
}

// checkClientID returns why Simkl refused clientID, or "" when Simkl knows it.
// label names the setting in the message. With wantV2, only an AUTH V2 app
// passes: the AUTH V2 sign-in flow refuses an AUTH V1 app's ID.
func (c *configurationChecks) checkClientID(ctx context.Context, clientID, label string, wantV2 bool) string {
	status, fault := c.server.simkl.do(ctx, account{clientID: clientID}, http.MethodGet, probePath, nil, nil)
	switch {
	case status == http.StatusUnauthorized:
		return ""
	case fault == nil && wantV2:
		return "The " + label + " belongs to an AUTH V1 app. Enter the client ID of an AUTH V2 app, or leave it empty."
	case fault == nil:
		return ""
	case status == http.StatusPreconditionFailed:
		return "Simkl does not recognize the " + label + ". Copy it again from simkl.com/settings/developer. If it is correct, Simkl may have blocked it for a while after too many requests."
	default:
		return "Could not check the " + label + " with Simkl: " + fault.GetSafeMessage()
	}
}

func failed(message string) *pluginv1.TestConfigurationResponse {
	return &pluginv1.TestConfigurationResponse{Message: message}
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
