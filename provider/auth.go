package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// deviceAuthServer runs Simkl's PIN sign-in: GET /oauth/pin issues a code the
// user enters at simkl.com/pin, and GET /oauth/pin/{code} answers with the
// access token once the user approves.
type deviceAuthServer struct {
	pluginv1.UnimplementedWatchSyncDeviceAuthorizationServiceServer
	server *Server
}

// pinState is the provider_state the host keeps between polls.
type pinState struct {
	UserCode  string    `json:"user_code"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (d *deviceAuthServer) Start(ctx context.Context, req *pluginv1.WatchSyncDeviceAuthorizationServiceStartRequest) (*pluginv1.WatchSyncDeviceAuthorizationServiceStartResponse, error) {
	clientID, fault := configuredClientID(req.GetCapabilityId(), req.GetProviderConfig())
	if fault != nil {
		return &pluginv1.WatchSyncDeviceAuthorizationServiceStartResponse{Fault: fault}, nil
	}
	ctx, cancel := withRPCDeadline(ctx)
	defer cancel()
	var response pinCodeResponse
	if fault := d.server.simkl.get(ctx, account{clientID: clientID}, "/oauth/pin?client_id="+url.QueryEscape(clientID), &response); fault != nil {
		return &pluginv1.WatchSyncDeviceAuthorizationServiceStartResponse{Fault: fault}, nil
	}
	if response.Result != "OK" || response.UserCode == "" || response.VerificationURL == "" ||
		response.ExpiresIn <= 0 || response.Interval <= 0 {
		return &pluginv1.WatchSyncDeviceAuthorizationServiceStartResponse{
			Fault: permanentFault("Simkl returned an incomplete PIN code"),
		}, nil
	}
	// Simkl keeps device_code only for compatibility; polling uses the user
	// code, as the built-in provider did.
	expiresAt := d.server.now().UTC().Add(time.Duration(response.ExpiresIn) * time.Second)
	state, err := json.Marshal(pinState{UserCode: response.UserCode, ExpiresAt: expiresAt})
	if err != nil {
		return &pluginv1.WatchSyncDeviceAuthorizationServiceStartResponse{Fault: permanentFault("Simkl PIN state could not be encoded")}, nil
	}
	return &pluginv1.WatchSyncDeviceAuthorizationServiceStartResponse{
		UserCode:        response.UserCode,
		VerificationUrl: response.VerificationURL,
		ProviderState:   state,
		PollingInterval: durationpb.New(time.Duration(response.Interval) * time.Second),
		ExpiresAt:       timestamppb.New(expiresAt),
	}, nil
}

func (d *deviceAuthServer) Poll(ctx context.Context, req *pluginv1.WatchSyncDeviceAuthorizationServicePollRequest) (*pluginv1.WatchSyncDeviceAuthorizationServicePollResponse, error) {
	clientID, fault := configuredClientID(req.GetCapabilityId(), req.GetProviderConfig())
	if fault != nil {
		return &pluginv1.WatchSyncDeviceAuthorizationServicePollResponse{Fault: fault}, nil
	}
	var state pinState
	if err := json.Unmarshal(req.GetProviderState(), &state); err != nil || strings.TrimSpace(state.UserCode) == "" {
		return &pluginv1.WatchSyncDeviceAuthorizationServicePollResponse{
			Fault: invalidRequestFault("Simkl PIN state is invalid; start connecting again"),
		}, nil
	}
	if !state.ExpiresAt.IsZero() && !d.server.now().Before(state.ExpiresAt) {
		return &pluginv1.WatchSyncDeviceAuthorizationServicePollResponse{
			Status: pluginv1.WatchSyncDeviceAuthorizationStatus_WATCH_SYNC_DEVICE_AUTHORIZATION_STATUS_EXPIRED,
		}, nil
	}
	ctx, cancel := withRPCDeadline(ctx)
	defer cancel()
	var response pinStatusResponse
	path := "/oauth/pin/" + url.PathEscape(state.UserCode) + "?client_id=" + url.QueryEscape(clientID)
	status, fault := d.server.simkl.do(ctx, account{clientID: clientID}, http.MethodGet, path, nil, &response)
	if status == http.StatusNotFound {
		// Simkl no longer knows the code, so this sign-in cannot complete.
		return &pluginv1.WatchSyncDeviceAuthorizationServicePollResponse{
			Status: pluginv1.WatchSyncDeviceAuthorizationStatus_WATCH_SYNC_DEVICE_AUTHORIZATION_STATUS_EXPIRED,
		}, nil
	}
	if fault != nil {
		return &pluginv1.WatchSyncDeviceAuthorizationServicePollResponse{Fault: fault}, nil
	}
	token := strings.TrimSpace(response.AccessToken)
	if response.Result != "OK" || token == "" {
		// Simkl answers a pending poll with 200 {"result":"KO"}.
		return &pluginv1.WatchSyncDeviceAuthorizationServicePollResponse{
			Status: pluginv1.WatchSyncDeviceAuthorizationStatus_WATCH_SYNC_DEVICE_AUTHORIZATION_STATUS_PENDING,
		}, nil
	}
	// A PIN token never expires and has no refresh token, so the access
	// token alone is the complete credential.
	return &pluginv1.WatchSyncDeviceAuthorizationServicePollResponse{
		Status:      pluginv1.WatchSyncDeviceAuthorizationStatus_WATCH_SYNC_DEVICE_AUTHORIZATION_STATUS_AUTHORIZED,
		Credentials: &pluginv1.WatchSyncCredentials{AccessToken: token},
	}, nil
}

// lookupAccount reads the Simkl account behind the token. The subject is the
// numeric account ID, or the user name when Simkl reports no ID, which is the
// identity the built-in provider recorded for each connection.
func (s *Server) lookupAccount(ctx context.Context, acct account) (*pluginv1.WatchSyncAccount, *pluginv1.WatchSyncFault) {
	var response userSettingsResponse
	if _, fault := s.simkl.post(ctx, acct, "/users/settings", nil, &response); fault != nil {
		return nil, fault
	}
	subject := strconv.Itoa(response.Account.ID)
	if response.Account.ID == 0 {
		subject = response.User.Name
	}
	if strings.TrimSpace(subject) == "" {
		return nil, permanentFault("Simkl did not return an account for this token")
	}
	return &pluginv1.WatchSyncAccount{
		ExternalSubject: subject,
		Username:        response.User.Name,
		DisplayName:     response.User.Name,
	}, nil
}
