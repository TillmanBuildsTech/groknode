package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/TillmanBuildsTech/groknode/internal/auth"
)

const (
	tokenRefreshMarginMs = 5 * 60 * 1000
	loginPollAttempts    = 150
	loginPollMaxMs       = 10 * 1000
)

// Refresh renews the access token if it is close to expiry. Returns the
// (possibly unchanged) credentials and whether they were updated.
func (c *Client) Refresh(ctx context.Context, creds Credentials) (Credentials, bool, error) {
	if !auth.ExpiresSoon(creds.AccessToken, c.now().UnixMilli(), tokenRefreshMarginMs) {
		return creds, false, nil
	}
	body, err := c.postJSON(ctx, "/oauth/token", map[string]any{
		"client_id":     c.ClientID,
		"grant_type":    "refresh_token",
		"refresh_token": creds.RefreshToken,
	}, nil, "OAuth refresh")
	if err != nil {
		return creds, false, err
	}
	if body["shouldLogout"] == true {
		return creds, false, fmt.Errorf("OAuth refresh requires a new sign-in")
	}
	at := firstString(body, "accessToken", "access_token")
	rt := firstString(body, "refreshToken", "refresh_token")
	if at != "" {
		creds.AccessToken = at
	}
	if rt != "" {
		creds.RefreshToken = rt
	}
	return creds, true, nil
}

// Login runs the PKCE browser-link flow, printing the sign-in URL to out and
// polling until the backend returns tokens. It does not persist anything.
func (c *Client) Login(ctx context.Context, out io.Writer) (Credentials, error) {
	pkce, err := auth.NewPKCE()
	if err != nil {
		return Credentials{}, err
	}
	uid := auth.UUIDv4()

	lu, err := url.Parse(c.WebsiteURL + "/loginDeepControl")
	if err != nil {
		return Credentials{}, err
	}
	lu.RawQuery = url.Values{
		"challenge":                 {pkce.Challenge},
		"uuid":                      {uid},
		"mode":                      {"login"},
		"redirectTarget":            {"sand"},
		"supportsSelectedTeamLogin": {"true"},
	}.Encode()
	_, _ = fmt.Fprintf(out, "Open this URL in any browser and complete sign-in:\n\n%s\n\nWaiting for authorization...\n", lu.String())

	poll := c.BaseURL + "/auth/poll?" + url.Values{"uuid": {uid}, "verifier": {pkce.Verifier}}.Encode()
	delay := time.Second
	for attempt := 0; attempt < loginPollAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return Credentials{}, ctx.Err()
		default:
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, poll, nil)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return Credentials{}, fmt.Errorf("OAuth poll: %w", err)
		}
		status := resp.StatusCode
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if status != http.StatusNotFound {
			var body map[string]any
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &body); err != nil {
					return Credentials{}, fmt.Errorf("OAuth poll returned HTTP %d with invalid JSON", status)
				}
			}
			if status < 200 || status >= 300 {
				return Credentials{}, fmt.Errorf("OAuth poll returned HTTP %d: %s", status, firstString(body, "message", "error"))
			}
			at := firstString(body, "accessToken", "access_token")
			rt := firstString(body, "refreshToken", "refresh_token")
			if at == "" || rt == "" {
				return Credentials{}, fmt.Errorf("OAuth response did not contain access and refresh tokens")
			}
			creds := Credentials{AccessToken: at, RefreshToken: rt}
			if v, ok := body["selectedTeamId"]; ok && v != nil {
				creds.SelectedTeamID = v
			}
			return creds, nil
		}
		time.Sleep(delay)
		if d := time.Duration(float64(delay) * 1.2); d < loginPollMaxMs*time.Millisecond {
			delay = d
		} else {
			delay = loginPollMaxMs * time.Millisecond
		}
	}
	return Credentials{}, fmt.Errorf("OAuth sign-in timed out")
}

// SyncMachinePolicy registers this machine and sets its local tool permission
// (always|ask|never), returning the backend's machine record.
func (c *Client) SyncMachinePolicy(ctx context.Context, id auth.Identity, value string, creds Credentials) (map[string]any, error) {
	if _, err := c.ConnectUnary(ctx, "aiserver.v1.DashboardService", "RegisterSandMachine",
		map[string]any{"label": id.Label, "localToolPermission": value}, creds, id.MachineID); err != nil {
		return nil, err
	}
	resp, err := c.ConnectUnary(ctx, "aiserver.v1.DashboardService", "UpdateSandMachineLocalToolPermission",
		map[string]any{"machineId": id.MachineID, "localToolPermission": value}, creds, id.MachineID)
	if err != nil {
		return nil, err
	}
	machine, _ := resp["machine"].(map[string]any)
	if machine == nil || machine["machineId"] != id.MachineID {
		return nil, fmt.Errorf("machine permission update returned an invalid machine")
	}
	if got, _ := machine["localToolPermission"].(string); got != value {
		limited := got
		if limited == "" {
			limited = "ask"
		}
		return nil, fmt.Errorf("the backend limited the machine policy to %s", limited)
	}
	return machine, nil
}
