// Command groknode is the headless Grok Bot local-computer node controller.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TillmanBuildsTech/grokbot-node/internal/auth"
	"github.com/TillmanBuildsTech/grokbot-node/internal/compat"
	"github.com/TillmanBuildsTech/grokbot-node/internal/config"
	"github.com/TillmanBuildsTech/grokbot-node/internal/ipc"
	"github.com/TillmanBuildsTech/grokbot-node/internal/protocol"
	"github.com/TillmanBuildsTech/grokbot-node/internal/state"
	"github.com/TillmanBuildsTech/grokbot-node/internal/supervisor"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "login":
		err = cmdLogin()
	case "run":
		err = cmdRun()
	case "status":
		err = cmdStatus()
	case "check":
		err = cmdCheck(os.Args[2:])
	case "policy":
		err = cmdPolicy(os.Args[2:])
	case "logout":
		err = cmdLogout()
	case "fetch-runtime":
		err = cmdFetchRuntime()
	case "checksum":
		cmdChecksum(os.Args[2:])
	case "ipc-probe":
		cmdIPCProbe(os.Args[2:])
	case "daemon-probe":
		cmdDaemonProbe(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "groknode: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `groknode <command>

  login              Browser-link sign-in; stores account credentials.
  run                Supervise the local-exec daemon (service entrypoint).
  status             Print node/daemon status as JSON.
  check [--local]    Compatibility check; --local skips cloud auth.
  policy <value>     Set local tool permission: always|ask|never.
  logout             Remove credentials and local state.

  (diagnostics)
  checksum <machineId>
  ipc-probe <runtime> <script> [--run-as-node]
  daemon-probe <runtime> <daemon.cjs> <clientVersion> <seconds>
`)
}

func loadCfg() (*config.Config, error) { return config.Load() }

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("GROKBOT_DEBUG") == "1" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func newClient(cfg *config.Config) *protocol.Client {
	version := os.Getenv("SAND_CLIENT_APP_VERSION")
	if version == "" {
		if v, e := compat.InstalledVersion(cfg.AsarPath); e == nil {
			version = v
		}
	}
	client := protocol.NewClient(cfg.APIURL, version)
	client.WebsiteURL = cfg.WebsiteURL
	client.ClientID = cfg.AuthClientID
	return client
}

func cmdLogin() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	client := newClient(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	creds, err := client.Login(ctx, os.Stdout)
	if err != nil {
		return err
	}
	if _, err := auth.SaveCredentials(cfg.CredentialsPath(), creds); err != nil {
		return err
	}
	host, _ := os.Hostname()
	fmt.Printf("Account authorized for %s.\n", host)
	return nil
}

func cmdRun() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	log := newLogger()
	sup, err := supervisor.New(cfg, log)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return sup.Run(ctx)
}

func cmdStatus() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	var identity auth.Identity
	var discovery, descriptor, settings map[string]any
	_, _ = state.ReadJSON(cfg.MachinePath(), &identity)
	_, _ = state.ReadJSON(cfg.DiscoveryPath(), &discovery)
	_, _ = state.ReadJSON(cfg.DescriptorPath(), &descriptor)
	_, _ = state.ReadJSON(cfg.SettingsPath(), &settings)

	daemonPid := anyInt(discovery["pid"])
	out := map[string]any{
		"label":               orHostname(identity.Label),
		"machineId":           nilIfEmpty(identity.MachineID),
		"daemonPid":           daemonPid,
		"daemonAlive":         daemonPid > 0 && processAlive(daemonPid),
		"localToolPermission": orStr(settings["localToolPermission"], "ask"),
		"dataRoot":            cfg.DataRoot,
	}
	if uc, ok := descriptor["userComputer"].(map[string]any); ok {
		out["credentialExpiresAtMs"] = uc["expiresAtMs"]
	}
	return printJSON(out)
}

func cmdCheck(args []string) error {
	local := false
	if len(args) > 0 {
		if args[0] != "--local" {
			return fmt.Errorf("usage: groknode check [--local]")
		}
		local = true
	}
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	for label, path := range map[string]string{"application binary": cfg.AppBinary, "daemon script asar": cfg.AsarPath} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("cannot access Grok Bot %s: %s", label, path)
		}
	}
	version, err := compat.InstalledVersion(cfg.AsarPath)
	if err != nil {
		return fmt.Errorf("read installed version: %w", err)
	}
	if _, err := compat.ReadAsarFile(cfg.AsarPath, "dist/local-exec-daemon/main.cjs"); err != nil {
		return fmt.Errorf("daemon entry not found in asar: %w", err)
	}
	wantMajor, _ := compat.RequiredElectronMajor(cfg.AppBinary)
	electron, electronErr := compat.FindElectronRuntime(wantMajor, cfg.ElectronOverride, cfg.RuntimeCacheDir)

	report := map[string]any{
		"installedVersion":      version,
		"requiredElectronMajor": wantMajor,
		"daemonEntry":           "verified",
	}
	if electronErr != nil {
		report["electronRuntime"] = "MISSING: " + electronErr.Error()
	} else {
		report["electronRuntime"] = electron
	}

	// Shared-library preflight. The Electron binary links GTK/X11/NSS at load
	// (even in run_as_node mode), so a slim server/LXC may be missing them.
	diagBin := electron
	if diagBin == "" {
		diagBin = compat.FirstExistingElectron(wantMajor, cfg.ElectronOverride, cfg.RuntimeCacheDir)
	}
	if diagBin != "" {
		if missing := compat.MissingSharedLibs(diagBin); len(missing) > 0 {
			report["missingSharedLibs"] = missing
			apt, pac := compat.PackageHints(missing)
			report["installHint"] = map[string]any{
				"debian/ubuntu": "sudo apt-get install -y " + strings.Join(apt, " "),
				"arch":          "sudo pacman -S --needed " + strings.Join(pac, " "),
			}
		}
	}

	if local {
		report["authentication"] = "skipped"
		if electronErr != nil {
			_ = printJSON(report)
			return electronErr
		}
		return printJSON(report)
	}

	if electronErr != nil {
		return electronErr
	}
	sup, err := supervisor.New(cfg, newLogger())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sup.RenewDescriptor(ctx); err != nil {
		return err
	}
	report["authentication"] = "verified"
	return printJSON(report)
}

func cmdPolicy(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: groknode policy <always|ask|never>")
	}
	value := args[0]
	switch value {
	case "always", "ask", "never":
	default:
		return fmt.Errorf("policy must be one of: always, ask, never")
	}
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	id, err := auth.LoadOrCreateMachine(cfg.MachinePath())
	if err != nil {
		return err
	}
	creds, err := auth.LoadCredentials(cfg.CredentialsPath())
	if err != nil {
		return err
	}
	client := newClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	refreshed, changed, err := client.Refresh(ctx, creds)
	if err != nil {
		return err
	}
	if changed {
		if refreshed, err = auth.SaveCredentials(cfg.CredentialsPath(), refreshed); err != nil {
			return err
		}
	}
	if _, err := client.SyncMachinePolicy(ctx, id, value, refreshed); err != nil {
		return err
	}
	var settings map[string]any
	_, _ = state.ReadJSON(cfg.SettingsPath(), &settings)
	if settings == nil {
		settings = map[string]any{}
	}
	settings["version"] = 1
	settings["localToolPermission"] = value
	if err := state.WriteJSON(cfg.SettingsPath(), settings); err != nil {
		return err
	}
	fmt.Printf("Local and backend execution policy set to %s. Restart the service to apply it locally.\n", value)
	return nil
}

func cmdLogout() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	var discovery map[string]any
	_, _ = state.ReadJSON(cfg.DiscoveryPath(), &discovery)
	if pid := anyInt(discovery["pid"]); pid > 0 && processAlive(pid) {
		return fmt.Errorf("the local execution daemon is still active (pid %d); stop the service before logout", pid)
	}
	for _, p := range []string{cfg.CredentialsPath(), cfg.DescriptorPath(), cfg.DiscoveryPath(), cfg.HeartbeatPath()} {
		if err := state.Remove(p); err != nil {
			return err
		}
	}
	fmt.Println("Credentials and local state removed.")
	return nil
}

// cmdFetchRuntime detects the vendor's Electron version and downloads the
// matching stock Electron into the runtime cache. Idempotent; run on install
// and after any Grok Bot update.
func cmdFetchRuntime() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfg.AppBinary); err != nil {
		return fmt.Errorf("vendor binary not found at %s (install Grok Bot first)", cfg.AppBinary)
	}
	version, err := compat.RequiredElectronVersion(cfg.AppBinary)
	if err != nil {
		return err
	}
	major, err := compat.RequiredElectronMajor(cfg.AppBinary)
	if err != nil {
		return err
	}
	// If a compatible runtime is already resolvable, do nothing.
	if bin, err := compat.FindElectronRuntime(major, cfg.ElectronOverride, cfg.RuntimeCacheDir); err == nil {
		fmt.Printf("Compatible Electron %d already available: %s\n", major, bin)
		return nil
	}
	fmt.Printf("Fetching stock Electron v%s into %s ...\n", version, cfg.RuntimeCacheDir)
	fmt.Printf("  (%s)\n", compat.ElectronReleaseURL(version))
	bin, err := compat.FetchElectron(version, major, cfg.RuntimeCacheDir)
	if err != nil {
		return err
	}
	fmt.Printf("Electron runtime ready: %s\n", bin)
	return nil
}

func cmdChecksum(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: groknode checksum <machineId>")
		os.Exit(2)
	}
	fmt.Println(protocol.ChecksumNow(args[0]))
}

// --- helpers ---

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func anyInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func orHostname(s string) string {
	if s != "" {
		return s
	}
	h, _ := os.Hostname()
	return h
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func orStr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

// --- diagnostics (probes) ---

// cmdDaemonProbe launches the REAL vendor daemon under a given runtime, sends
// the file-key bootstrap immediately, and logs every IPC message plus child
// stdout/stderr for a bounded window. Runs in a throwaway SAND_DATA_ROOT with no
// credentials, so the daemon cannot execute anything or disturb a logged-in
// desktop session.
func cmdDaemonProbe(args []string) {
	if len(args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: groknode daemon-probe <runtime-binary> <daemon.cjs> <clientVersion> <seconds>")
		os.Exit(2)
	}
	runtime, script, version := args[0], args[1], args[2]
	secs, err := strconv.Atoi(args[3])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad seconds: %v\n", err)
		os.Exit(2)
	}
	dataRoot, err := os.MkdirTemp("", "groknode-probe-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdtemp: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(dataRoot) }()
	fmt.Fprintf(os.Stderr, "[probe] runtime=%s\n[probe] script=%s\n[probe] dataRoot=%s\n[probe] window=%ds (no credentials provided)\n\n", runtime, script, dataRoot, secs)

	ch, err := ipc.Spawn(ipc.SpawnOptions{
		Path: runtime, Args: []string{script},
		Env: []string{
			"ELECTRON_RUN_AS_NODE=1", "SAND_PACKAGED=1",
			"SAND_DATA_ROOT=" + dataRoot, "SAND_CLIENT_APP_VERSION=" + version,
		},
		Stdout: prefixWriter("[daemon stdout] "),
		Stderr: prefixWriter("[daemon stderr] "),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ch.Cmd().Process.Kill() }()

	const machineID = "probe-machine-0001"
	if err := ch.Bootstrap(machineID); err != nil {
		fmt.Fprintf(os.Stderr, "[probe] bootstrap send failed: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "[probe] sent bootstrap sand-local-exec-file-key (computerId=%s)\n", machineID)
	}
	go func() {
		for {
			msg, err := ch.Recv()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[probe] IPC recv ended: %v\n", err)
				return
			}
			b, _ := json.Marshal(msg)
			fmt.Fprintf(os.Stderr, "[daemon IPC->us] %s\n", string(b))
		}
	}()
	exited := make(chan error, 1)
	go func() { exited <- ch.Cmd().Wait() }()
	select {
	case err := <-exited:
		fmt.Fprintf(os.Stderr, "\n[probe] daemon exited early: %v\n", err)
	case <-time.After(time.Duration(secs) * time.Second):
		fmt.Fprintf(os.Stderr, "\n[probe] window elapsed; daemon still running -> killing\n")
	}
}

func prefixWriter(prefix string) *linePrefixer { return &linePrefixer{prefix: prefix} }

type linePrefixer struct {
	prefix string
	buf    []byte
}

func (w *linePrefixer) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := -1
		for i, c := range w.buf {
			if c == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		fmt.Fprintf(os.Stderr, "%s%s\n", w.prefix, string(w.buf[:idx]))
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

func cmdIPCProbe(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: groknode ipc-probe <runtime-binary> <script> [--run-as-node]")
		os.Exit(2)
	}
	runtime, script := args[0], args[1]
	var env []string
	for _, a := range args[2:] {
		if a == "--run-as-node" {
			env = append(env, "ELECTRON_RUN_AS_NODE=1")
		}
	}
	ch, err := ipc.Spawn(ipc.SpawnOptions{Path: runtime, Args: []string{script}, Env: env, Stdout: os.Stderr, Stderr: os.Stderr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ch.Cmd().Process.Kill() }()

	type result struct {
		Ready map[string]any `json:"ready"`
		Ack   map[string]any `json:"ack"`
	}
	out := make(chan result, 1)
	go func() {
		ready, err := ch.Recv()
		if err != nil {
			fmt.Fprintf(os.Stderr, "recv ready: %v\n", err)
			os.Exit(1)
		}
		const machineID = "probe-machine"
		if err := ch.Bootstrap(machineID); err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
			os.Exit(1)
		}
		ack, err := ch.Recv()
		if err != nil {
			fmt.Fprintf(os.Stderr, "recv ack: %v\n", err)
			os.Exit(1)
		}
		out <- result{ready, ack}
	}()
	select {
	case r := <-out:
		b, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(b))
	case <-time.After(10 * time.Second):
		fmt.Fprintln(os.Stderr, "timed out waiting for handshake")
		os.Exit(1)
	}
}
