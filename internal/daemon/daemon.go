// Package daemon launches and controls the vendor local-exec daemon under an
// ABI-matched stock Electron runtime (because the vendor binary has run_as_node
// fused off and cannot be used as a Node runtime on current builds).
package daemon

import (
	"fmt"
	"io"
	"syscall"
	"time"

	"github.com/TillmanBuildsTech/grokbot-node/internal/ipc"
)

// Options configures a daemon launch.
type Options struct {
	Electron      string // stock Electron binary (run_as_node capable)
	DaemonScript  string // path to dist/local-exec-daemon/main.cjs
	DataRoot      string // SAND_DATA_ROOT
	ClientVersion string // SAND_CLIENT_APP_VERSION
	MachineID     string // sent in the bootstrap file-key message
	Stdout        io.Writer
	Stderr        io.Writer
}

// Daemon is a running daemon process plus its IPC channel.
type Daemon struct {
	ch   *ipc.Channel
	opts Options
}

// Start launches the daemon and sends the bootstrap file-key message.
func Start(opts Options) (*Daemon, error) {
	ch, err := ipc.Spawn(ipc.SpawnOptions{
		Path: opts.Electron,
		Args: []string{opts.DaemonScript},
		Env: []string{
			"ELECTRON_RUN_AS_NODE=1",
			"SAND_PACKAGED=1",
			"SAND_DATA_ROOT=" + opts.DataRoot,
			"SAND_CLIENT_APP_VERSION=" + opts.ClientVersion,
		},
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("spawn daemon: %w", err)
	}
	if err := ch.Bootstrap(opts.MachineID); err != nil {
		_ = ch.Cmd().Process.Kill()
		return nil, fmt.Errorf("daemon bootstrap: %w", err)
	}
	return &Daemon{ch: ch, opts: opts}, nil
}

// Wait blocks until the daemon process exits.
func (d *Daemon) Wait() error { return d.ch.Cmd().Wait() }

// Pid returns the daemon process id.
func (d *Daemon) Pid() int { return d.ch.Cmd().Process.Pid }

// Messages drains IPC messages from the daemon, invoking fn for each, until the
// channel closes. Intended to run in a goroutine.
func (d *Daemon) Messages(fn func(map[string]any)) {
	for {
		msg, err := d.ch.Recv()
		if err != nil {
			return
		}
		if fn != nil {
			fn(msg)
		}
	}
}

// Terminate disconnects IPC, sends SIGTERM, and escalates to SIGKILL after
// grace. Safe to call once.
func (d *Daemon) Terminate(grace time.Duration) {
	proc := d.ch.Cmd().Process
	if proc == nil {
		return
	}
	_ = d.ch.Close()
	_ = proc.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() { _ = d.ch.Cmd().Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(grace):
		_ = proc.Signal(syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}
