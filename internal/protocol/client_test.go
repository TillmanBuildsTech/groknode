package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeBackend records requests and returns canned mint responses.
type fakeBackend struct {
	mu       sync.Mutex
	requests map[string]http.Header // path -> headers seen
}

func newFakeBackend() (*fakeBackend, *httptest.Server) {
	fb := &fakeBackend{requests: map[string]http.Header{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/sand-box/local-exec-daemon-credential", func(w http.ResponseWriter, r *http.Request) {
		fb.record(r)
		writeJSON(w, map[string]any{"credential": "daemon-cred-xyz", "expiresAtMs": float64(1_761_000_000_000)})
	})
	mux.HandleFunc("/aiserver.v1.GrokBotService/IssueGrokBotUserComputerCredential", func(w http.ResponseWriter, r *http.Request) {
		fb.record(r)
		writeJSON(w, map[string]any{
			"credential":          "computer-cred-abc",
			"expiresAtMs":         float64(1_762_000_000_000),
			"serverAuthoritative": true,
		})
	})
	return fb, httptest.NewServer(mux)
}

func (fb *fakeBackend) record(r *http.Request) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.requests[r.URL.Path] = r.Header.Clone()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestMintDescriptor(t *testing.T) {
	fb, srv := newFakeBackend()
	defer srv.Close()

	c := NewClient(srv.URL, "0.30.0")
	// Pin the clock so the checksum is deterministic and assertable.
	fixed := time.UnixMilli(1_761_480_000_000)
	c.Now = func() time.Time { return fixed }

	creds := Credentials{AccessToken: "header.eyJzdWIiOiJ1c2VyXzEifQ.sig", RefreshToken: "r"}
	const machineID = "node-xyz"

	d, err := c.MintDescriptor(context.Background(), machineID, creds)
	if err != nil {
		t.Fatalf("MintDescriptor: %v", err)
	}

	// Descriptor assembly.
	if d.Credential != "daemon-cred-xyz" {
		t.Errorf("daemon credential = %q", d.Credential)
	}
	if d.BackendURL != srv.URL {
		t.Errorf("backendUrl = %q", d.BackendURL)
	}
	if d.ExpiresAtMs == nil || *d.ExpiresAtMs != 1_761_000_000_000 {
		t.Errorf("daemon expiresAtMs = %v", d.ExpiresAtMs)
	}
	uc := d.UserComputer
	if uc.Credential != "computer-cred-abc" || uc.MachineID != machineID {
		t.Errorf("user-computer cred = %+v", uc)
	}
	if uc.ExpiresAtMs != 1_762_000_000_000 || !uc.ServerAuthoritative {
		t.Errorf("user-computer meta = %+v", uc)
	}
	if len(uc.AccountScope) != 64 {
		t.Errorf("accountScope not a sha256 hex: %q", uc.AccountScope)
	}

	// Header assertions on the daemon-credential request.
	h := fb.requests["/sand-box/local-exec-daemon-credential"]
	if h == nil {
		t.Fatal("daemon-credential endpoint was not called")
	}
	if got := h.Get("authorization"); got != "Bearer "+creds.AccessToken {
		t.Errorf("authorization = %q", got)
	}
	if got := h.Get("x-cursor-client-type"); got != "sand" {
		t.Errorf("x-cursor-client-type = %q", got)
	}
	if got := h.Get("x-sand-box-namespace"); got != "prod" {
		t.Errorf("x-sand-box-namespace = %q", got)
	}
	// Checksum must match the Go transform for the pinned clock + machineID,
	// which equals the Node golden vector for this timestamp.
	wantChecksum := Checksum(machineID, fixed.UnixMilli())
	if got := h.Get("x-cursor-checksum"); got != wantChecksum {
		t.Errorf("x-cursor-checksum = %q, want %q", got, wantChecksum)
	}
	if h.Get("x-request-id") == "" {
		t.Error("missing x-request-id")
	}

	// The RPC request must carry the Connect protocol header.
	rpc := fb.requests["/aiserver.v1.GrokBotService/IssueGrokBotUserComputerCredential"]
	if rpc.Get("connect-protocol-version") != "1" {
		t.Errorf("connect-protocol-version = %q", rpc.Get("connect-protocol-version"))
	}
}
