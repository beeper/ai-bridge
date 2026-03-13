package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/beeper/bridge-manager/api/beeperapi"
	"github.com/beeper/bridge-manager/api/hungryapi"
	"maunium.net/go/mautrix"

	"github.com/beeper/agentremote/pkg/shared/bridgeutil"
)

var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var envDomains = map[string]string{
	"prod":    "beeper.com",
	"staging": "beeper-staging.com",
	"dev":     "beeper-dev.com",
	"local":   "beeper.localtest.me",
}

type metadata struct {
	Instance         string    `json:"instance"`
	BridgeType       string    `json:"bridge_type"`
	BeeperBridgeName string    `json:"beeper_bridge_name"`
	ConfigPath       string    `json:"config_path"`
	RegistrationPath string    `json:"registration_path"`
	LogPath          string    `json:"log_path"`
	PIDPath          string    `json:"pid_path"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	initCommands()
	if len(os.Args) < 2 {
		fmt.Print(generateUsage())
		return nil
	}
	name := os.Args[1]
	if name == "-h" || name == "--help" {
		name = "help"
	}
	if name == "--version" || name == "-v" {
		return cmdVersion()
	}
	c := findCommand(name)
	if c == nil {
		return didYouMean(name)
	}
	err := c.Run(os.Args[2:])
	if errors.Is(err, flag.ErrHelp) {
		// Flag parsing hit -h/--help; show our generated help instead of Go's default
		if !c.Hidden {
			fmt.Print(generateCommandHelp(c))
		}
		return nil
	}
	return err
}

// newFlagSet creates a FlagSet that suppresses Go's default -h output,
// so our generated help is shown instead.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// ANSI color helpers — automatically disabled when stdout is not a terminal.
var colorEnabled = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}()

func colorize(code, s string) string {
	if !colorEnabled {
		return s
	}
	return code + s + "\033[0m"
}

func green(s string) string  { return colorize("\033[32m", s) }
func red(s string) string    { return colorize("\033[31m", s) }
func yellow(s string) string { return colorize("\033[33m", s) }
func dim(s string) string    { return colorize("\033[2m", s) }

func colorState(state string) string {
	switch state {
	case "RUNNING", "CONNECTED":
		return green(state)
	case "STARTING", "RECONNECTING":
		return yellow(state)
	case "STOPPED", "ERROR", "BRIDGE_UNREACHABLE", "TRANSIENT_DISCONNECT":
		return red(state)
	default:
		return state
	}
}

func colorLocal(running bool, pid int) string {
	if running {
		return green("running") + fmt.Sprintf(" (pid %d)", pid)
	}
	return red("stopped")
}

func cmdHelp(args []string) error {
	if len(args) == 0 {
		fmt.Print(generateUsage())
		return nil
	}
	if c := findCommand(args[0]); c != nil && !c.Hidden {
		fmt.Print(generateCommandHelp(c))
		return nil
	}
	return didYouMean(args[0])
}

func didYouMean(input string) error {
	best := ""
	bestDist := 4 // only suggest if distance <= 3
	for _, name := range commandNames() {
		d := levenshtein(input, name)
		if d < bestDist {
			bestDist = d
			best = name
		}
	}
	if best != "" {
		return fmt.Errorf("unknown command %q. Did you mean %q?", input, best)
	}
	return fmt.Errorf("unknown command %q, run 'agentremote help' for usage", input)
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// ── Auth commands ──

func cmdLogin(args []string) error {
	fs := newFlagSet("login")
	env := fs.String("env", "prod", "beeper env (prod|staging|dev|local)")
	profile := fs.String("profile", defaultProfile, "profile name")
	email := fs.String("email", "", "email address")
	code := fs.String("code", "", "login code")
	if err := fs.Parse(args); err != nil {
		return err
	}
	domain, ok := envDomains[*env]
	if !ok {
		return fmt.Errorf("invalid env %q", *env)
	}
	if *email == "" {
		v, err := bridgeutil.PromptLine("Email: ")
		if err != nil {
			return err
		}
		*email = v
	}
	if strings.TrimSpace(*email) == "" {
		return fmt.Errorf("email is required")
	}
	start, err := beeperapi.StartLogin(domain)
	if err != nil {
		return err
	}
	if err = beeperapi.SendLoginEmail(domain, start.RequestID, *email); err != nil {
		return err
	}
	if *code == "" {
		v, err := bridgeutil.PromptLine("Code: ")
		if err != nil {
			return err
		}
		*code = v
	}
	if strings.TrimSpace(*code) == "" {
		return fmt.Errorf("code is required")
	}
	resp, err := beeperapi.SendLoginCode(domain, start.RequestID, strings.TrimSpace(*code))
	if err != nil {
		return err
	}
	matrixClient, err := mautrix.NewClient(fmt.Sprintf("https://matrix.%s", domain), "", "")
	if err != nil {
		return fmt.Errorf("failed to create matrix client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	loginResp, err := matrixClient.Login(ctx, &mautrix.ReqLogin{
		Type:                     "org.matrix.login.jwt",
		Token:                    resp.LoginToken,
		InitialDeviceDisplayName: "agentremote",
	})
	if err != nil {
		return fmt.Errorf("matrix login failed: %w", err)
	}
	username := ""
	if resp.Whoami != nil {
		username = resp.Whoami.UserInfo.Username
	}
	if username == "" {
		username = loginResp.UserID.Localpart()
	}
	cfg := authConfig{
		Env:      *env,
		Domain:   domain,
		Username: username,
		Token:    loginResp.AccessToken,
	}
	if err = saveAuthConfig(*profile, cfg); err != nil {
		return err
	}
	fmt.Printf("logged in as @%s:%s (profile: %s)\n", username, domain, *profile)
	return nil
}

func cmdLogout(args []string) error {
	fs := newFlagSet("logout")
	profile := fs.String("profile", defaultProfile, "profile name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := authConfigPath(*profile)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Printf("logged out (profile: %s)\n", *profile)
	return nil
}

func cmdWhoami(args []string) error {
	fs := newFlagSet("whoami")
	profile := fs.String("profile", defaultProfile, "profile name")
	output := fs.String("output", "text", "output format (text|json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := getAuthOrEnv(*profile)
	if err != nil {
		return err
	}
	resp, err := beeperapi.Whoami(cfg.Domain, cfg.Token)
	if err != nil {
		return err
	}
	if cfg.Username == "" || cfg.Username != resp.UserInfo.Username {
		cfg.Username = resp.UserInfo.Username
		if err := saveAuthConfig(*profile, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save auth config: %v\n", err)
		}
	}
	if *output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{
			"user_id": fmt.Sprintf("@%s:%s", resp.UserInfo.Username, cfg.Domain),
			"email":   resp.UserInfo.Email,
			"cluster": resp.UserInfo.BridgeClusterID,
			"profile": *profile,
		})
	}
	fmt.Printf("User ID: @%s:%s\n", resp.UserInfo.Username, cfg.Domain)
	fmt.Printf("Email: %s\n", resp.UserInfo.Email)
	fmt.Printf("Cluster: %s\n", resp.UserInfo.BridgeClusterID)
	fmt.Printf("Profile: %s\n", *profile)
	return nil
}

func cmdProfiles(args []string) error {
	fs := newFlagSet("profiles")
	output := fs.String("output", "text", "output format (text|json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	profiles, err := listProfiles()
	if err != nil {
		return err
	}
	if *output == "json" {
		type profileInfo struct {
			Name     string `json:"name"`
			Username string `json:"username,omitempty"`
			Domain   string `json:"domain,omitempty"`
			Env      string `json:"env,omitempty"`
		}
		var result []profileInfo
		for _, p := range profiles {
			pi := profileInfo{Name: p}
			if cfg, err := loadAuthConfig(p); err == nil {
				pi.Username = cfg.Username
				pi.Domain = cfg.Domain
				pi.Env = cfg.Env
			}
			result = append(result, pi)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	if len(profiles) == 0 {
		fmt.Println("no profiles found")
		return nil
	}
	for _, p := range profiles {
		cfg, err := loadAuthConfig(p)
		if err != nil {
			fmt.Printf("%s: not logged in\n", p)
		} else {
			fmt.Printf("%s: @%s:%s (%s)\n", p, cfg.Username, cfg.Domain, cfg.Env)
		}
	}
	return nil
}

// ── Bridge lifecycle commands ──

func parseBridgeFlags(fs *flag.FlagSet) (*string, *string, *string) {
	profile := fs.String("profile", defaultProfile, "profile name")
	name := fs.String("name", "", "instance name (for running multiple instances of the same bridge)")
	env := fs.String("env", "", "override beeper env for this bridge")
	return profile, name, env
}

func resolveBridgeArgs(fs *flag.FlagSet) (bridgeType string, err error) {
	posArgs := fs.Args()
	if len(posArgs) != 1 {
		return "", fmt.Errorf("expected exactly one bridge type argument (available: ai, codex, opencode, openclaw)")
	}
	bridgeType = posArgs[0]
	if _, ok := bridgeRegistry[bridgeType]; !ok {
		return "", fmt.Errorf("unknown bridge type %q (available: ai, codex, opencode, openclaw)", bridgeType)
	}
	return bridgeType, nil
}

func cmdStart(args []string) error {
	fs := newFlagSet("start")
	profile, name, _ := parseBridgeFlags(fs)
	wait := fs.Bool("wait", false, "block until bridge is connected (timeout 60s)")
	waitTimeout := fs.Duration("wait-timeout", 60*time.Second, "timeout for --wait")
	if err := fs.Parse(args); err != nil {
		return err
	}
	bridgeType, err := resolveBridgeArgs(fs)
	if err != nil {
		return err
	}
	instName := instanceDirName(bridgeType, *name)
	beeperName := beeperBridgeName(bridgeType, *name)

	sp, err := ensureInstanceLayout(*profile, instName)
	if err != nil {
		return err
	}
	meta, err := ensureInitialized(instName, bridgeType, beeperName, sp)
	if err != nil {
		return err
	}
	if err = ensureRegistration(*profile, meta, bridgeType); err != nil {
		return err
	}
	running, pid := bridgeutil.ProcessAliveFromPIDFile(meta.PIDPath)
	if running {
		fmt.Printf("%s already running (pid %d)\n", instName, pid)
		if *wait {
			return waitForBridge(*profile, beeperName, *waitTimeout)
		}
		return nil
	}
	if err = startBridgeProcess(meta, bridgeType); err != nil {
		return err
	}
	fmt.Printf("started %s\n", instName)
	printRuntimePaths(meta)
	if *wait {
		return waitForBridge(*profile, beeperName, *waitTimeout)
	}
	return nil
}

func waitForBridge(profile, beeperName string, timeout time.Duration) error {
	cfg, err := getAuthOrEnv(profile)
	if err != nil {
		return err
	}
	fmt.Printf("waiting for %s to be connected...\n", beeperName)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := beeperapi.Whoami(cfg.Domain, cfg.Token)
		if err == nil {
			if bridge, ok := resp.User.Bridges[beeperName]; ok {
				state := string(bridge.BridgeState.StateEvent)
				if state == "RUNNING" || state == "CONNECTED" {
					fmt.Printf("%s is %s\n", beeperName, state)
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for %s to be connected", beeperName)
}

func cmdRun(args []string) error {
	fs := newFlagSet("run")
	profile, name, _ := parseBridgeFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	bridgeType, err := resolveBridgeArgs(fs)
	if err != nil {
		return err
	}
	instName := instanceDirName(bridgeType, *name)
	beeperName := beeperBridgeName(bridgeType, *name)

	sp, err := ensureInstanceLayout(*profile, instName)
	if err != nil {
		return err
	}
	meta, err := ensureInitialized(instName, bridgeType, beeperName, sp)
	if err != nil {
		return err
	}
	if err = ensureRegistration(*profile, meta, bridgeType); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find own executable: %w", err)
	}
	argv := []string{exe, "__bridge", bridgeType, "-c", meta.ConfigPath}
	fmt.Printf("running %s in foreground\n", instName)
	printRuntimePaths(meta)
	if err = os.Chdir(filepath.Dir(meta.ConfigPath)); err != nil {
		return fmt.Errorf("failed to chdir: %w", err)
	}
	return syscall.Exec(exe, argv, os.Environ())
}

func cmdStop(args []string) error {
	fs := newFlagSet("stop")
	profile := fs.String("profile", defaultProfile, "profile name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	posArgs := fs.Args()
	if len(posArgs) != 1 {
		return fmt.Errorf("expected exactly one instance name argument")
	}
	instName := posArgs[0]

	sp, err := getInstancePaths(*profile, instName)
	if err != nil {
		return err
	}
	meta, err := readMetadata(sp)
	if err != nil {
		// If no metadata, try to stop by PID file directly
		stopped, stopErr := bridgeutil.StopByPIDFile(sp.PIDPath)
		if stopErr != nil {
			return stopErr
		}
		if stopped {
			fmt.Printf("stopped %s\n", instName)
		} else {
			fmt.Printf("%s is not running\n", instName)
		}
		return nil
	}
	stopped, err := bridgeutil.StopByPIDFile(meta.PIDPath)
	if err != nil {
		return err
	}
	if stopped {
		fmt.Printf("stopped %s\n", instName)
	} else {
		fmt.Printf("%s is not running\n", instName)
	}
	return nil
}

func cmdStopAll(args []string) error {
	fs := newFlagSet("stop-all")
	profile := fs.String("profile", defaultProfile, "profile name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	instances, err := listInstancesForProfile(*profile)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		fmt.Println("no instances found")
		return nil
	}
	for _, inst := range instances {
		sp, err := getInstancePaths(*profile, inst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: error: %v\n", inst, err)
			continue
		}
		stopped, err := bridgeutil.StopByPIDFile(sp.PIDPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: error stopping: %v\n", inst, err)
			continue
		}
		if stopped {
			fmt.Printf("stopped %s\n", inst)
		}
	}
	return nil
}

func cmdRestart(args []string) error {
	if err := cmdStop(args); err != nil {
		return err
	}
	return cmdStart(args)
}

type bridgeStatus struct {
	Name       string        `json:"name"`
	State      string        `json:"state,omitempty"`
	SelfHosted bool          `json:"self_hosted,omitempty"`
	Local      *localStatus  `json:"local,omitempty"`
	Logins     []loginStatus `json:"logins,omitempty"`
}

type localStatus struct {
	Running    bool   `json:"running"`
	PID        int    `json:"pid,omitempty"`
	ConfigPath string `json:"config_path"`
}

type loginStatus struct {
	RemoteID   string `json:"remote_id"`
	State      string `json:"state"`
	RemoteName string `json:"remote_name,omitempty"`
}

func cmdStatus(args []string) error {
	fs := newFlagSet("status")
	profile := fs.String("profile", defaultProfile, "profile name")
	noRemote := fs.Bool("no-remote", false, "skip fetching remote bridge state from server")
	output := fs.String("output", "text", "output format (text|json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Fetch remote bridges from server
	var remoteBridges map[string]beeperapi.WhoamiBridge
	if !*noRemote {
		if cfg, err := getAuthOrEnv(*profile); err == nil {
			if resp, err := beeperapi.Whoami(cfg.Domain, cfg.Token); err == nil {
				remoteBridges = resp.User.Bridges
			} else {
				fmt.Fprintf(os.Stderr, "warning: failed to fetch remote state: %v\n", err)
			}
		}
	}

	// Build set of local instances
	filterInstances := fs.Args()
	localInstances, _ := listInstancesForProfile(*profile)
	localSet := make(map[string]bool, len(localInstances))
	for _, inst := range localInstances {
		localSet[inst] = true
	}

	// Determine which bridges to show
	seen := make(map[string]bool)
	var toShow []string

	if len(filterInstances) > 0 {
		toShow = filterInstances
	} else {
		toShow = append(toShow, localInstances...)
		for _, inst := range localInstances {
			seen[inst] = true
			seen["sh-"+inst] = true
		}
		for name := range remoteBridges {
			if !seen[name] {
				toShow = append(toShow, name)
				seen[name] = true
			}
		}
	}

	if len(toShow) == 0 {
		if *output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("no instances found")
		}
		return nil
	}

	var statuses []bridgeStatus
	for _, inst := range toShow {
		remoteName := inst
		localName := inst
		if cut, ok := strings.CutPrefix(inst, "sh-"); ok {
			localName = cut
		} else {
			remoteName = "sh-" + inst
		}

		rb, hasRemote := remoteBridges[remoteName]
		hasLocal := localSet[localName]

		bs := bridgeStatus{Name: remoteName}
		if hasRemote {
			bs.State = string(rb.BridgeState.StateEvent)
			bs.SelfHosted = rb.BridgeState.IsSelfHosted
		}

		if hasLocal {
			sp, err := getInstancePaths(*profile, localName)
			if err == nil {
				running, pid := bridgeutil.ProcessAliveFromPIDFile(sp.PIDPath)
				ls := &localStatus{Running: running, ConfigPath: sp.ConfigPath}
				if running {
					ls.PID = pid
				}
				bs.Local = ls
			}
		}

		if hasRemote {
			for remoteID, rs := range rb.RemoteState {
				ls := loginStatus{
					RemoteID: remoteID,
					State:    string(rs.StateEvent),
				}
				if rs.RemoteName != "" {
					ls.RemoteName = rs.RemoteName
				}
				bs.Logins = append(bs.Logins, ls)
			}
		}

		statuses = append(statuses, bs)
	}

	if *output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(statuses)
	}

	fmt.Printf("Bridges (profile: %s):\n", *profile)
	for _, bs := range statuses {
		if bs.State != "" {
			selfHosted := ""
			if bs.SelfHosted {
				selfHosted = dim(" (self-hosted)")
			}
			fmt.Printf("  %s: %s%s\n", bs.Name, colorState(bs.State), selfHosted)
		} else if bs.Local != nil {
			fmt.Printf("  %s:\n", bs.Name)
		} else {
			fmt.Printf("  %s: %s\n", bs.Name, dim("unknown"))
		}

		if bs.Local != nil {
			fmt.Printf("    local: %s\n", colorLocal(bs.Local.Running, bs.Local.PID))
			fmt.Printf("    config: %s\n", dim(bs.Local.ConfigPath))
		}

		if len(bs.Logins) > 0 {
			fmt.Printf("    logins:\n")
			for _, l := range bs.Logins {
				name := ""
				if l.RemoteName != "" {
					name = dim(fmt.Sprintf(" (%s)", l.RemoteName))
				}
				fmt.Printf("      - %s: %s%s\n", l.RemoteID, colorState(l.State), name)
			}
		}
	}
	return nil
}

func cmdLogs(args []string) error {
	fs := newFlagSet("logs")
	profile := fs.String("profile", defaultProfile, "profile name")
	follow := fs.Bool("follow", false, "follow logs")
	fs.BoolVar(follow, "f", false, "follow logs (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	posArgs := fs.Args()
	if len(posArgs) != 1 {
		return fmt.Errorf("expected exactly one instance name argument")
	}
	instName := posArgs[0]

	sp, err := getInstancePaths(*profile, instName)
	if err != nil {
		return err
	}
	if *follow {
		cmd := exec.Command("tail", "-f", sp.LogPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	f, err := os.Open(sp.LogPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(os.Stdout, f)
	return err
}

func cmdList() error {
	fmt.Println("Available bridge types:")
	for name, def := range bridgeRegistry {
		fmt.Printf("  %-10s %s\n", name, def.Description)
	}
	return nil
}

func cmdDelete(args []string) error {
	fs := newFlagSet("delete")
	profile := fs.String("profile", defaultProfile, "profile name")
	remote := fs.Bool("remote", false, "also delete remote beeper bridge")
	if err := fs.Parse(args); err != nil {
		return err
	}
	posArgs := fs.Args()
	if len(posArgs) != 1 {
		return fmt.Errorf("expected exactly one instance name argument")
	}
	instName := posArgs[0]

	sp, err := getInstancePaths(*profile, instName)
	if err != nil {
		return err
	}
	// Stop if running
	if _, err := bridgeutil.StopByPIDFile(sp.PIDPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to stop: %v\n", err)
	}
	if *remote {
		meta, readErr := readMetadata(sp)
		if readErr == nil {
			if err := deleteRemoteBridge(*profile, meta.BeeperBridgeName); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to delete remote bridge: %v\n", err)
			}
		}
	}
	if err := os.RemoveAll(sp.Root); err != nil {
		return err
	}
	fmt.Printf("deleted %s\n", instName)
	return nil
}

func cmdVersion() error {
	fmt.Printf("agentremote %s\n", Tag)
	fmt.Printf("commit: %s\n", Commit)
	fmt.Printf("built: %s\n", BuildTime)
	return nil
}

func cmdCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: agentremote completion <bash|zsh|fish>")
	}
	switch args[0] {
	case "bash":
		fmt.Print(generateBashCompletion())
	case "zsh":
		fmt.Print(generateZshCompletion())
	case "fish":
		fmt.Print(generateFishCompletion())
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish)", args[0])
	}
	return nil
}

// ── Instance management helpers ──

func ensureInitialized(instName, bridgeType, beeperName string, sp *instancePaths) (*metadata, error) {
	meta, err := readOrSynthesizeMetadata(instName, bridgeType, beeperName, sp)
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(meta.ConfigPath); errors.Is(err, os.ErrNotExist) {
		if err = generateExampleConfig(meta); err != nil {
			return nil, err
		}
	}
	def := bridgeRegistry[bridgeType]
	overrides := map[string]any{
		"appservice.address":  "websocket",
		"appservice.hostname": "127.0.0.1",
		"appservice.port":     def.Port,
		"database.type":       "sqlite3-fk-wal",
		"database.uri":        fmt.Sprintf("file:%s?_txlock=immediate", def.DBName),
		"bridge.permissions": map[string]any{
			"*":          "relay",
			"beeper.com": "admin",
		},
	}
	if err = bridgeutil.ApplyConfigOverrides(meta.ConfigPath, overrides); err != nil {
		return nil, err
	}
	if err = writeMetadata(meta, sp.MetaPath); err != nil {
		return nil, err
	}
	return meta, nil
}

func readOrSynthesizeMetadata(instName, bridgeType, beeperName string, sp *instancePaths) (*metadata, error) {
	if data, err := os.ReadFile(sp.MetaPath); err == nil {
		var m metadata
		if err = json.Unmarshal(data, &m); err == nil {
			m.Instance = instName
			m.BridgeType = bridgeType
			m.BeeperBridgeName = beeperName
			m.ConfigPath = sp.ConfigPath
			m.RegistrationPath = sp.RegistrationPath
			m.LogPath = sp.LogPath
			m.PIDPath = sp.PIDPath
			return &m, nil
		}
	}
	return &metadata{
		Instance:         instName,
		BridgeType:       bridgeType,
		BeeperBridgeName: beeperName,
		ConfigPath:       sp.ConfigPath,
		RegistrationPath: sp.RegistrationPath,
		LogPath:          sp.LogPath,
		PIDPath:          sp.PIDPath,
		UpdatedAt:        time.Now().UTC(),
	}, nil
}

func readMetadata(sp *instancePaths) (*metadata, error) {
	data, err := os.ReadFile(sp.MetaPath)
	if err != nil {
		return nil, err
	}
	var m metadata
	if err = json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func writeMetadata(meta *metadata, path string) error {
	meta.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func generateExampleConfig(meta *metadata) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find own executable: %w", err)
	}
	cmd := exec.Command(exe, "__bridge", meta.BridgeType, "-c", meta.ConfigPath, "-e")
	cmd.Dir = filepath.Dir(meta.ConfigPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureRegistration(profile string, meta *metadata, bridgeType string) error {
	auth, err := getAuthOrEnv(profile)
	if err != nil {
		return err
	}
	who, err := beeperapi.Whoami(auth.Domain, auth.Token)
	if err != nil {
		return fmt.Errorf("whoami failed: %w", err)
	}
	if auth.Username == "" || auth.Username != who.UserInfo.Username {
		auth.Username = who.UserInfo.Username
		if err := saveAuthConfig(profile, auth); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save auth config: %v\n", err)
		}
	}
	hc := hungryapi.NewClient(auth.Domain, auth.Username, auth.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reg, err := hc.GetAppService(ctx, meta.BeeperBridgeName)
	if err != nil {
		reg, err = hc.RegisterAppService(ctx, meta.BeeperBridgeName, hungryapi.ReqRegisterAppService{Push: false, SelfHosted: true})
		if err != nil {
			return fmt.Errorf("register appservice failed: %w", err)
		}
	}
	yml, err := reg.YAML()
	if err != nil {
		return err
	}
	if err = os.WriteFile(meta.RegistrationPath, []byte(yml), 0o600); err != nil {
		return err
	}
	userID := fmt.Sprintf("@%s:%s", auth.Username, auth.Domain)
	if err = bridgeutil.PatchConfigWithRegistration(meta.ConfigPath, &reg, hc.HomeserverURL.String(), meta.BeeperBridgeName, bridgeType, auth.Domain, reg.AppToken, userID, auth.Token, who.User.AsmuxData.LoginToken); err != nil {
		return err
	}

	state := beeperapi.ReqPostBridgeState{
		StateEvent:   "STARTING",
		Reason:       "SELF_HOST_REGISTERED",
		IsSelfHosted: true,
		BridgeType:   bridgeType,
	}
	if err := beeperapi.PostBridgeState(auth.Domain, auth.Username, meta.BeeperBridgeName, reg.AppToken, state); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to post bridge state: %v\n", err)
	}
	return nil
}

func deleteRemoteBridge(profile, beeperName string) error {
	auth, err := getAuthOrEnv(profile)
	if err != nil {
		return err
	}
	if auth.Username == "" {
		who, werr := beeperapi.Whoami(auth.Domain, auth.Token)
		if werr == nil {
			auth.Username = who.UserInfo.Username
			if err := saveAuthConfig(profile, auth); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to save auth config: %v\n", err)
			}
		}
	}
	if auth.Username != "" {
		hc := hungryapi.NewClient(auth.Domain, auth.Username, auth.Token)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := hc.DeleteAppService(ctx, beeperName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to delete appservice: %v\n", err)
		}
		cancel()
	}
	if err = beeperapi.DeleteBridge(auth.Domain, beeperName, auth.Token); err != nil {
		return fmt.Errorf("failed to delete bridge in beeper api: %w", err)
	}
	return nil
}

// ── Process lifecycle ──

func startBridgeProcess(meta *metadata, bridgeType string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find own executable: %w", err)
	}
	return bridgeutil.StartBridgeFromConfig(exe, []string{"__bridge", bridgeType, "-c", meta.ConfigPath}, meta.ConfigPath, meta.LogPath, meta.PIDPath)
}

func printRuntimePaths(meta *metadata) {
	fmt.Printf("paths:\n")
	fmt.Printf("  config: %s\n", meta.ConfigPath)
	fmt.Printf("  registration: %s\n", meta.RegistrationPath)
	fmt.Printf("  log: %s\n", meta.LogPath)
	fmt.Printf("  pid: %s\n", meta.PIDPath)
}
