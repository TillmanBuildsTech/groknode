// Package ipc reproduces Node.js child_process fork() IPC from a non-Node
// parent process. The proprietary local-exec daemon is normally fork()ed by the
// Node controller and expects:
//
//   - an inherited connected socket whose fd number is given in NODE_CHANNEL_FD
//   - newline-delimited JSON framing (NODE_CHANNEL_SERIALIZATION=json, the
//     default when 'advanced'/v8 is not requested)
//
// The parent sends exactly one bootstrap message after spawn:
//
//	{"type":"sand-local-exec-file-key","key":null,"computerId":"<machineId>"}
//
// This package is runtime-agnostic: it drives plain `node`, an Electron binary
// in ELECTRON_RUN_AS_NODE mode, or the real daemon identically.
package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// Channel is a live Node-IPC connection to a spawned child.
type Channel struct {
	cmd    *exec.Cmd
	conn   *os.File
	reader *bufio.Reader
}

// SpawnOptions configures a Node-IPC child launch.
type SpawnOptions struct {
	Path   string    // runtime binary: node, or the Electron binary
	Args   []string  // argv after Path (e.g. the daemon script path)
	Env    []string  // extra env (merged onto os.Environ)
	Stdout io.Writer // child stdout (nil -> discarded)
	Stderr io.Writer // child stderr (nil -> discarded)
}

// Spawn launches the child with an IPC socket wired to fd 3.
func Spawn(opts SpawnOptions) (*Channel, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("socketpair: %w", err)
	}
	parent := os.NewFile(uintptr(fds[0]), "ipc-parent")
	child := os.NewFile(uintptr(fds[1]), "ipc-child")

	cmd := exec.Command(opts.Path, opts.Args...) // #nosec G204 -- launching the configured runtime is this function's purpose; Path/Args come from trusted config, not external input
	cmd.Env = append(os.Environ(), opts.Env...)
	// fd 3 (first ExtraFiles entry) is the IPC channel in the child.
	cmd.Env = append(cmd.Env,
		"NODE_CHANNEL_FD=3",
		"NODE_CHANNEL_SERIALIZATION=json",
	)
	cmd.ExtraFiles = []*os.File{child}
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	if err := cmd.Start(); err != nil {
		_ = parent.Close()
		_ = child.Close()
		return nil, fmt.Errorf("start %s: %w", opts.Path, err)
	}
	// The child owns its end now; the parent must close its copy so EOF
	// propagates correctly when the child exits.
	_ = child.Close()

	return &Channel{
		cmd:    cmd,
		conn:   parent,
		reader: bufio.NewReader(parent),
	}, nil
}

// Send writes one newline-framed JSON message to the child.
func (c *Channel) Send(msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal ipc message: %w", err)
	}
	b = append(b, '\n')
	if _, err := c.conn.Write(b); err != nil {
		return fmt.Errorf("write ipc message: %w", err)
	}
	return nil
}

// Recv reads one newline-framed JSON message from the child. It returns
// io.EOF when the channel closes.
func (c *Channel) Recv() (map[string]any, error) {
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		return nil, fmt.Errorf("decode ipc message %q: %w", line, err)
	}
	return m, nil
}

// Bootstrap sends the daemon's required startup message.
func (c *Channel) Bootstrap(machineID string) error {
	return c.Send(map[string]any{
		"type":       "sand-local-exec-file-key",
		"key":        nil,
		"computerId": machineID,
	})
}

// Cmd exposes the underlying process handle for lifecycle control.
func (c *Channel) Cmd() *exec.Cmd { return c.cmd }

// Close tears down the IPC socket (does not kill the child).
func (c *Channel) Close() error { return c.conn.Close() }
