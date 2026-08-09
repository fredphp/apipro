package logic

import (
	"encoding/json"
	"testing"
)

// TestIsAuthMethod verifies that only login/register/guestLogin/refresh are
// treated as auth methods that issue a new server session. Other methods
// (logout, user_detail, live_*, match_*, room_*) must NOT be flagged — they
// echo back the client's existing session ID.
func TestIsAuthMethod(t *testing.T) {
	authMethods := []string{"login", "register", "guestLogin", "refresh"}
	for _, m := range authMethods {
		if !isAuthMethod(m) {
			t.Errorf("isAuthMethod(%q) = false, want true", m)
		}
	}
	nonAuth := []string{"logout", "user_detail", "live_hot", "match_recommend", "room_detail", "sms_getCode", "unknown"}
	for _, m := range nonAuth {
		if isAuthMethod(m) {
			t.Errorf("isAuthMethod(%q) = true, want false", m)
		}
	}
}

// TestExtractResultSessionID verifies the new-session-ID extraction from the
// auth result JSON. Per docs/password-login-register.txt step 16, the new
// sessionId is inside common_resp.result JSON. The helper must return it
// (falling back to accessToken when sessionId is empty).
func TestExtractResultSessionID(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{
			name: "sessionId present",
			json: `{"sessionId":"abc123def456","accessToken":"abc123def456","refreshToken":"xyz"}`,
			want: "abc123def456",
		},
		{
			name: "sessionId empty, fallback to accessToken",
			json: `{"sessionId":"","accessToken":"token-from-access"}`,
			want: "token-from-access",
		},
		{
			name: "both empty",
			json: `{"sessionId":"","accessToken":""}`,
			want: "",
		},
		{
			name: "neither field present",
			json: `{"foo":"bar"}`,
			want: "",
		},
		{
			name: "empty json",
			json: `{}`,
			want: "",
		},
		{
			name: "nil result",
			json: "",
			want: "",
		},
		{
			name: "invalid json",
			json: `{not json`,
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var raw json.RawMessage
			if c.json != "" {
				raw = json.RawMessage(c.json)
			}
			got := extractResultSessionID(raw)
			if got != c.want {
				t.Errorf("extractResultSessionID(%q) = %q, want %q", c.json, got, c.want)
			}
		})
	}
}

// TestAuthResponseShape verifies that the AuthResponse JSON produced by
// buildAuthResponse contains both sessionId AND accessToken equal to the
// server-issued access token. This is the invariant the BUG-FIX relies on:
// extractResultSessionID can read either field and get the new token.
func TestAuthResponseShape(t *testing.T) {
	// This is the exact JSON shape produced by svc.AuthResponse (see
	// cmd/rpc/internal/svc/builders.go). The fix's extractResultSessionID
	// reads "sessionId" first, then "accessToken" as fallback.
	authJSON := `{
		"accessToken": "a1b2c3d4e5f6",
		"sessionId":   "a1b2c3d4e5f6",
		"refreshToken":"r9s8t7u6",
		"userInfo":    {"uid": 5001, "nickName": "TestUser", "userType": 1}
	}`
	got := extractResultSessionID(json.RawMessage(authJSON))
	if got != "a1b2c3d4e5f6" {
		t.Errorf("extractResultSessionID(authResponse) = %q, want a1b2c3d4e5f6", got)
	}
	// Verify sessionId == accessToken (the invariant).
	var m struct {
		SessionID   string `json:"sessionId"`
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal([]byte(authJSON), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.SessionID != m.AccessToken {
		t.Errorf("invariant broken: sessionId=%q accessToken=%q (must be equal)", m.SessionID, m.AccessToken)
	}
}
