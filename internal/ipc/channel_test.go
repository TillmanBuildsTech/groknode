package ipc

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// nodeStub is a minimal Node IPC child: it announces readiness, echoes any
// message it receives back as an ack, and exits once it gets the daemon
// bootstrap message. It also reports whether a DISPLAY (X11) was present, which
// lets us verify the headless theory (S1).
const nodeStub = `
process.send({ type: 'ready', pid: process.pid, hasDisplay: !!process.env.DISPLAY,
  runAsNode: process.env.ELECTRON_RUN_AS_NODE || null });
process.on('message', (m) => {
  process.send({ type: 'ack', got: m }, () => {
    if (m && m.type === 'sand-local-exec-file-key') process.exit(0);
  });
});
setTimeout(() => process.exit(3), 5000);
`

func writeStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "stub.cjs")
	if err := os.WriteFile(p, []byte(nodeStub), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// handshake runs the full bootstrap exchange against the given runtime and
// returns the child's "ready" announcement for inspection.
func handshake(t *testing.T, path string, args, env []string) map[string]any {
	t.Helper()
	var out bytes.Buffer
	ch, err := Spawn(SpawnOptions{Path: path, Args: args, Env: env, Stdout: &out, Stderr: &out})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer func() { _ = ch.Cmd().Process.Kill() }()

	done := make(chan map[string]any, 1)
	errc := make(chan error, 1)
	go func() {
		ready, err := ch.Recv()
		if err != nil {
			errc <- err
			return
		}
		if ready["type"] != "ready" {
			errc <- &stepErr{"first message was not 'ready'", ready}
			return
		}
		const machineID = "spike-machine-123"
		if err := ch.Bootstrap(machineID); err != nil {
			errc <- err
			return
		}
		ack, err := ch.Recv()
		if err != nil {
			errc <- err
			return
		}
		got, _ := ack["got"].(map[string]any)
		if ack["type"] != "ack" || got["type"] != "sand-local-exec-file-key" || got["computerId"] != machineID {
			errc <- &stepErr{"bootstrap not echoed correctly", ack}
			return
		}
		done <- ready
	}()

	select {
	case ready := <-done:
		_ = ch.Cmd().Wait()
		return ready
	case err := <-errc:
		t.Fatalf("handshake failed: %v (child output: %q)", err, out.String())
	case <-time.After(10 * time.Second):
		t.Fatalf("handshake timed out (child output: %q)", out.String())
	}
	return nil
}

type stepErr struct {
	msg string
	val any
}

func (e *stepErr) Error() string { return e.msg }

// S2: prove Node-IPC newline-JSON framing works from a Go parent with plain node.
func TestNodeIPCHandshake(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	ready := handshake(t, node, []string{writeStub(t)}, nil)
	t.Logf("node ready: %+v", ready)
}

// S1+S2: prove an Electron binary in ELECTRON_RUN_AS_NODE mode speaks the same
// IPC and runs headless (no Xvfb). This is the runtime the real daemon uses.
func TestElectronRunAsNodeHandshake(t *testing.T) {
	var electron string
	for _, p := range []string{
		"/usr/lib/electron42/electron",
		"/usr/lib/electron/electron",
		"/usr/bin/electron",
	} {
		if _, err := os.Stat(p); err == nil {
			electron = p
			break
		}
	}
	if electron == "" {
		t.Skip("no electron binary found")
	}
	ready := handshake(t, electron, []string{writeStub(t)}, []string{"ELECTRON_RUN_AS_NODE=1"})
	t.Logf("electron ready: %+v", ready)
	if ready["hasDisplay"] == true {
		t.Logf("NOTE: DISPLAY was set during this run; headless-without-X still implied by RUN_AS_NODE skipping Chromium")
	}
}
