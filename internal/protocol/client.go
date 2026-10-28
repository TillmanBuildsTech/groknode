package protocol

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/TillmanBuildsTech/groknode/internal/auth"
)

// Credentials aliases the canonical auth.Credentials type.
type Credentials = auth.Credentials

// Client talks to the Grok Bot / Cursor backend.
type Client struct {
	BaseURL       string
	WebsiteURL    string
	ClientID      string
	ClientVersion string
	HTTP          *http.Client
	Now           func() time.Time // injectable clock for tests
}

// NewClient builds a client with sane defaults.
func NewClient(baseURL, clientVersion string) *Client {
	return &Client{
		BaseURL:       baseURL,
		ClientVersion: clientVersion,
		HTTP:          &http.Client{Timeout: 30 * time.Second},
		Now:           time.Now,
	}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// reqID returns a RFC-4122-ish v4 UUID using crypto/rand (no external dep).
func reqID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// authHeaders builds the exact header set the backend expects.
func (c *Client) authHeaders(machineID string, creds Credentials) http.Header {
	h := http.Header{}
	h.Set("authorization", "Bearer "+creds.AccessToken)
	if creds.SelectedTeamID != nil && creds.SelectedTeamID != "" {
		h.Set("x-cursor-team-id", fmt.Sprintf("%v", creds.SelectedTeamID))
	}
	h.Set("x-cursor-checksum", Checksum(machineID, c.now().UnixMilli()))
	h.Set("x-cursor-client-type", "sand")
	h.Set("x-cursor-client-version", c.ClientVersion)
	h.Set("x-sand-box-namespace", "prod")
	h.Set("x-ghost-mode", "true")
	h.Set("x-request-id", reqID())
	return h
}

func (c *Client) postJSON(ctx context.Context, path string, body any, headers http.Header, action string) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal: %w", action, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%s: request: %w", action, err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("content-type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)

	var parsed map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("%s returned HTTP %d with invalid JSON", action, resp.StatusCode)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := firstString(parsed, "message", "error")
		if msg == "" {
			msg = string(raw)
		}
		return nil, fmt.Errorf("%s returned HTTP %d: %s", action, resp.StatusCode, msg)
	}
	return parsed, nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// ConnectUnary performs a Connect-protocol unary RPC: POST /{service}/{method}.
func (c *Client) ConnectUnary(ctx context.Context, service, method string, request any, creds Credentials, machineID string) (map[string]any, error) {
	h := c.authHeaders(machineID, creds)
	h.Set("connect-protocol-version", "1")
	return c.postJSON(ctx, "/"+service+"/"+method, request, h, method)
}

// Descriptor is what we persist for the daemon to read.
type Descriptor struct {
	Credential   string           `json:"credential"`
	BackendURL   string           `json:"backendUrl"`
	ExpiresAtMs  *int64           `json:"expiresAtMs,omitempty"`
	UserComputer UserComputerCred `json:"userComputer"`
}

// UserComputerCred is the per-machine credential block.
type UserComputerCred struct {
	Credential          string `json:"credential"`
	MachineID           string `json:"machineId"`
	ExpiresAtMs         int64  `json:"expiresAtMs"`
	AccountScope        string `json:"accountScope"`
	ServerAuthoritative bool   `json:"serverAuthoritative"`
}

// MintDescriptor mints the daemon credential and the user-computer credential
// in parallel and assembles the descriptor the daemon consumes.
func (c *Client) MintDescriptor(ctx context.Context, machineID string, creds Credentials) (*Descriptor, error) {
	type res struct {
		body map[string]any
		err  error
	}
	daemonCh := make(chan res, 1)
	computerCh := make(chan res, 1)

	go func() {
		b, err := c.postJSON(ctx, "/sand-box/local-exec-daemon-credential", map[string]any{},
			c.authHeaders(machineID, creds), "Local daemon credential mint")
		daemonCh <- res{b, err}
	}()
	go func() {
		b, err := c.ConnectUnary(ctx, "aiserver.v1.GrokBotService", "IssueGrokBotUserComputerCredential",
			map[string]any{"machineId": machineID}, creds, machineID)
		computerCh <- res{b, err}
	}()

	daemon := <-daemonCh
	computer := <-computerCh
	if daemon.err != nil {
		return nil, daemon.err
	}
	if computer.err != nil {
		return nil, computer.err
	}

	daemonCred := firstString(daemon.body, "credential")
	if daemonCred == "" {
		return nil, fmt.Errorf("daemon credential mint returned no credential")
	}
	computerCred := firstString(computer.body, "credential")
	if computerCred == "" {
		return nil, fmt.Errorf("user-computer credential mint returned no credential")
	}

	d := &Descriptor{
		Credential: daemonCred,
		BackendURL: c.BaseURL,
		UserComputer: UserComputerCred{
			Credential:          computerCred,
			MachineID:           machineID,
			AccountScope:        auth.AccountScope(creds.AccessToken),
			ServerAuthoritative: computer.body["serverAuthoritative"] == true,
		},
	}
	if v, ok := daemon.body["expiresAtMs"].(float64); ok {
		ms := int64(v)
		d.ExpiresAtMs = &ms
	}
	if v, ok := computer.body["expiresAtMs"].(float64); ok {
		d.UserComputer.ExpiresAtMs = int64(v)
	}
	return d, nil
}
