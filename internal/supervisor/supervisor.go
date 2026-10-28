// Package supervisor runs the credential + daemon lifecycle: mint credentials,
// launch the daemon under a stock Electron runtime, heartbeat, renew, and
// restart on crash with backoff.
package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/TillmanBuildsTech/groknode/internal/auth"
	"github.com/TillmanBuildsTech/groknode/internal/compat"
	"github.com/TillmanBuildsTech/groknode/internal/config"
	"github.com/TillmanBuildsTech/groknode/internal/daemon"
	"github.com/TillmanBuildsTech/groknode/internal/protocol"
	"github.com/TillmanBuildsTech/groknode/internal/state"
)

const (
	heartbeatInterval  = 20 * time.Second
	descriptorInterval = 60 * time.Second
	stopGrace          = 5 * time.Second
	restartMin         = 1 * time.Second
	restartMax         = 60 * time.Second
	stableAfter        = 60 * time.Second
)

// Supervisor holds resolved runtime state.
type Supervisor struct {
	cfg           *config.Config
	client        *protocol.Client
	log           *slog.Logger
	electron      string
	clientVersion string
	identity      auth.Identity
}

// New resolves the Electron runtime, client version, identity, and HTTP client.
func New(cfg *config.Config, log *slog.Logger) (*Supervisor, error) {
	version := os.Getenv("SAND_CLIENT_APP_VERSION")
	if version == "" {
		v, err := compat.InstalledVersion(cfg.AsarPath)
		if err != nil {
			return nil, fmt.Errorf("read installed version: %w", err)
		}
		version = v
	}

	wantMajor, err := compat.RequiredElectronMajor(cfg.AppBinary)
	if err != nil {
		log.Warn("could not determine required Electron major; proceeding with any found runtime", "err", err)
	}
	electron, err := compat.FindElectronRuntime(wantMajor, cfg.ElectronOverride, cfg.RuntimeCacheDir)
	if err != nil {
		// In headless environments (containers, VMs without graphics), Electron may exist but
		// fail the version check due to missing libs or sandboxing issues. If an Electron binary
		// exists, proceed with a warning; only fail if truly missing.
		if existing := compat.FirstExistingElectron(wantMajor, cfg.ElectronOverride, cfg.RuntimeCacheDir); existing != "" {
			log.Warn("Electron version check failed; using available binary", "path", existing, "err", err)
			electron = existing
		} else {
			return nil, err
		}
	}

	id, err := auth.LoadOrCreateMachine(cfg.MachinePath())
	if err != nil {
		return nil, err
	}

	client := protocol.NewClient(cfg.APIURL, version)
	client.WebsiteURL = cfg.WebsiteURL
	client.ClientID = cfg.AuthClientID

	return &Supervisor{
		cfg: cfg, client: client, log: log,
		electron: electron, clientVersion: version, identity: id,
	}, nil
}

// Electron returns the resolved runtime path.
func (s *Supervisor) Electron() string { return s.electron }

// ClientVersion returns the resolved vendor client version.
func (s *Supervisor) ClientVersion() string { return s.clientVersion }

// RenewDescriptor loads+refreshes credentials and mints a fresh daemon
// descriptor, persisting both.
func (s *Supervisor) RenewDescriptor(ctx context.Context) error {
	creds, err := auth.LoadCredentials(s.cfg.CredentialsPath())
	if err != nil {
		return err
	}
	refreshed, changed, err := s.client.Refresh(ctx, creds)
	if err != nil {
		return err
	}
	if changed {
		if refreshed, err = auth.SaveCredentials(s.cfg.CredentialsPath(), refreshed); err != nil {
			return err
		}
	}
	desc, err := s.client.MintDescriptor(ctx, s.identity.MachineID, refreshed)
	if err != nil {
		return err
	}
	return state.WriteJSON(s.cfg.DescriptorPath(), desc)
}

func (s *Supervisor) heartbeat() error {
	return state.WriteJSON(s.cfg.HeartbeatPath(), map[string]any{
		"pid": os.Getpid(), "at": time.Now().UnixMilli(),
	})
}

// Run is the main supervision loop. It returns when ctx is cancelled.
func (s *Supervisor) Run(ctx context.Context) error {
	if err := os.MkdirAll(s.cfg.DataRoot, 0o700); err != nil {
		return err
	}
	if err := s.RenewDescriptor(ctx); err != nil {
		return fmt.Errorf("initial credential mint: %w", err)
	}
	if err := s.heartbeat(); err != nil {
		s.log.Warn("heartbeat write failed", "err", err)
	}
	s.log.Info("node started", "label", s.identity.Label, "machineId", s.identity.MachineID,
		"electron", s.electron, "clientVersion", s.clientVersion)

	hb := time.NewTicker(heartbeatInterval)
	defer hb.Stop()
	desc := time.NewTicker(descriptorInterval)
	defer desc.Stop()

	backoff := restartMin
	for {
		startedAt := time.Now()
		d, err := daemon.Start(daemon.Options{
			Electron: s.electron, DaemonScript: s.cfg.DaemonScript,
			DataRoot: s.cfg.DataRoot, ClientVersion: s.clientVersion,
			MachineID: s.identity.MachineID,
			Stdout:    &lineLogger{log: s.log, level: slog.LevelInfo, tag: "daemon"},
			Stderr:    &lineLogger{log: s.log, level: slog.LevelWarn, tag: "daemon"},
		})
		if err != nil {
			s.log.Error("failed to start daemon", "err", err)
		} else {
			go d.Messages(func(m map[string]any) { s.log.Debug("daemon ipc", "msg", m) })
			exit := make(chan error, 1)
			go func() { exit <- d.Wait() }()

		wait:
			for {
				select {
				case <-ctx.Done():
					s.log.Info("stopping; terminating daemon")
					d.Terminate(stopGrace)
					_ = state.Remove(s.cfg.HeartbeatPath())
					return nil
				case err := <-exit:
					s.log.Warn("daemon exited", "err", err, "uptime", time.Since(startedAt).Round(time.Second))
					break wait
				case <-hb.C:
					if err := s.heartbeat(); err != nil {
						s.log.Warn("heartbeat failed", "err", err)
					}
				case <-desc.C:
					if err := s.RenewDescriptor(ctx); err != nil {
						s.log.Warn("credential renewal failed", "err", err)
					}
				}
			}
			if time.Since(startedAt) >= stableAfter {
				backoff = restartMin // ran long enough; reset backoff
			}
		}

		// Restart with backoff unless cancelled.
		select {
		case <-ctx.Done():
			_ = state.Remove(s.cfg.HeartbeatPath())
			return nil
		case <-time.After(backoff):
		}
		s.log.Info("restarting daemon", "backoff", backoff)
		backoff *= 2
		if backoff > restartMax {
			backoff = restartMax
		}
	}
}

// lineLogger turns child stdout/stderr into structured log lines.
type lineLogger struct {
	log   *slog.Logger
	level slog.Level
	tag   string
	buf   []byte
}

func (w *lineLogger) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimRight(w.buf[:i], "\r"))
		if line != "" {
			w.log.Log(context.Background(), w.level, line, "src", w.tag)
		}
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}
