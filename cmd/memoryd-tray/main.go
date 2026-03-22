package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/kindling-sh/memoryd/internal/config"
)

func main() {
	systray.Run(onReady, onExit)
}

var (
	daemonCmd *exec.Cmd
	daemonMu  sync.Mutex
)

func onReady() {
	systray.SetTitle("M")
	systray.SetTooltip("memoryd – memory layer for coding agents")

	mStatus := systray.AddMenuItem("Status: checking...", "Daemon status")
	mStatus.Disable()

	systray.AddSeparator()

	// --- Mode submenu ---
	mMode := systray.AddMenuItem("Mode", "How memoryd integrates with your agent")
	mModeProxy := mMode.AddSubMenuItem("Proxy – auto read & write", "Proxy intercepts API calls, captures everything automatically")
	mModeMCP := mMode.AddSubMenuItem("MCP – read & write", "Agent uses MCP tools to search and store explicitly")
	mModeMCPRO := mMode.AddSubMenuItem("MCP – read only", "Agent can search memories but cannot store new ones")

	systray.AddSeparator()

	mToggle := systray.AddMenuItem("Start", "Start or stop the daemon")
	mDash := systray.AddMenuItem("Open Dashboard", "Open web dashboard in browser")
	mAddSource := systray.AddMenuItem("Add Source...", "Crawl a URL into the knowledge base")
	mCopy := systray.AddMenuItem("Copy export command", "Copy ANTHROPIC_BASE_URL export")
	mConfig := systray.AddMenuItem("Open config", "Open config file in editor")
	mLogs := systray.AddMenuItem("Open logs directory", "Open ~/.memoryd in Finder")

	systray.AddSeparator()

	mUninstall := systray.AddMenuItem("Uninstall memoryd...", "Remove memoryd and all its data")

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Quit", "Quit memoryd tray")

	cfg, _ := config.Load()
	port := cfg.Port

	// Apply initial mode checkmarks.
	setModeChecks(cfg.Mode, mModeProxy, mModeMCP, mModeMCPRO)

	binaryPath := findBinary()

	// Auto-start daemon on launch if not already running.
	if !checkHealth(port) {
		startDaemon(binaryPath)
	}

	// Poll daemon health every 3 seconds.
	running := false
	go func() {
		for {
			ok := checkHealth(port)
			if ok != running {
				running = ok
				if running {
					mStatus.SetTitle("Status: ● running on port " + fmt.Sprintf("%d", port))
					mToggle.SetTitle("Stop")
					systray.SetTitle("M●")
				} else {
					mStatus.SetTitle("Status: ○ stopped")
					mToggle.SetTitle("Start")
					systray.SetTitle("M○")
				}
			}
			time.Sleep(3 * time.Second)
		}
	}()

	for {
		select {
		case <-mModeProxy.ClickedCh:
			switchMode(config.ModeProxy, mModeProxy, mModeMCP, mModeMCPRO, binaryPath, &running)

		case <-mModeMCP.ClickedCh:
			switchMode(config.ModeMCP, mModeProxy, mModeMCP, mModeMCPRO, binaryPath, &running)

		case <-mModeMCPRO.ClickedCh:
			switchMode(config.ModeMCPReadOnly, mModeProxy, mModeMCP, mModeMCPRO, binaryPath, &running)

		case <-mToggle.ClickedCh:
			if running {
				stopDaemon()
				running = false
				mToggle.SetTitle("Start")
				mStatus.SetTitle("Status: ○ stopped")
				systray.SetTitle("M○")
			} else {
				startDaemon(binaryPath)
				running = true
				mToggle.SetTitle("Stop")
				mStatus.SetTitle("Status: ● running on port " + fmt.Sprintf("%d", port))
				systray.SetTitle("M●")
			}

		case <-mAddSource.ClickedCh:
			go addSourceDialog(port)

		case <-mCopy.ClickedCh:
			text := fmt.Sprintf("export ANTHROPIC_BASE_URL=http://127.0.0.1:%d", port)
			copyToClipboard(text)

		case <-mDash.ClickedCh:
			exec.Command("open", fmt.Sprintf("http://127.0.0.1:%d", port)).Start()

		case <-mConfig.ClickedCh:
			exec.Command("open", "-t", config.Path()).Start()

		case <-mLogs.ClickedCh:
			exec.Command("open", config.Dir()).Start()

		case <-mUninstall.ClickedCh:
			go uninstallDialog(binaryPath)

		case <-mQuit.ClickedCh:
			stopDaemon()
			systray.Quit()
		}
	}
}

func onExit() {
	stopDaemon()
}

func checkHealth(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	if json.Unmarshal(body, &result) != nil {
		return false
	}
	return result["status"] == "ok"
}

func startDaemon(binary string) {
	daemonMu.Lock()
	defer daemonMu.Unlock()

	if daemonCmd != nil && daemonCmd.Process != nil {
		return // already running
	}

	cmd := exec.Command(binary, "start")
	// Send logs to a file
	logDir := config.Dir()
	os.MkdirAll(logDir, 0700)
	logFile, err := os.OpenFile(filepath.Join(logDir, "daemon.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return
	}
	daemonCmd = cmd

	// Wait in background so we can clean up
	go func() {
		cmd.Wait()
		daemonMu.Lock()
		if daemonCmd == cmd {
			daemonCmd = nil
		}
		daemonMu.Unlock()
		if logFile != nil {
			logFile.Close()
		}
	}()
}

func stopDaemon() {
	daemonMu.Lock()
	defer daemonMu.Unlock()

	if daemonCmd == nil || daemonCmd.Process == nil {
		return
	}
	daemonCmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		daemonCmd.Process.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		daemonCmd.Process.Kill()
	}
	daemonCmd = nil
}

func findBinary() string {
	// 1. Same directory as the tray binary
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	candidate := filepath.Join(dir, "memoryd")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// 2. PATH lookup
	if p, err := exec.LookPath("memoryd"); err == nil {
		return p
	}

	// 3. Common install location
	if runtime.GOOS == "darwin" {
		for _, p := range []string{"/opt/homebrew/bin/memoryd", "/usr/local/bin/memoryd"} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return "memoryd" // hope PATH has it
}

func addSourceDialog(port int) {
	// Prompt for source name.
	nameOut, err := exec.Command("osascript", "-e",
		`display dialog "Source name (e.g. Company Docs):" default answer "" buttons {"Cancel", "Next"} default button "Next" with title "memoryd - Add Source"`, "-e",
		`text returned of result`).Output()
	if err != nil {
		return // user cancelled
	}
	name := strings.TrimSpace(string(nameOut))
	if name == "" {
		return
	}

	// Prompt for base URL.
	urlOut, err := exec.Command("osascript", "-e",
		fmt.Sprintf(`display dialog "Base URL to crawl:" default answer "https://" buttons {"Cancel", "Add"} default button "Add" with title "memoryd - Add Source: %s"`, name), "-e",
		`text returned of result`).Output()
	if err != nil {
		return // user cancelled
	}
	baseURL := strings.TrimSpace(string(urlOut))
	if baseURL == "" || baseURL == "https://" {
		return
	}

	// POST to the daemon.
	payload := fmt.Sprintf(`{"name":%q,"base_url":%q}`, name, baseURL)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://127.0.0.1:%d/api/sources", port),
		"application/json",
		strings.NewReader(payload),
	)
	if err != nil {
		exec.Command("osascript", "-e",
			fmt.Sprintf(`display dialog "Failed to add source: %s" buttons {"OK"} with icon stop with title "memoryd"`, err.Error())).Run()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification "Crawl started for %s" with title "memoryd"`, name)).Run()
	} else {
		body, _ := io.ReadAll(resp.Body)
		exec.Command("osascript", "-e",
			fmt.Sprintf(`display dialog "Failed: %s" buttons {"OK"} with icon stop with title "memoryd"`, string(body))).Run()
	}
}

func copyToClipboard(text string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	cmd.Run()
}

// setModeChecks updates submenu check marks to reflect the active mode.
func setModeChecks(mode string, proxy, mcp, mcpRO *systray.MenuItem) {
	proxy.Uncheck()
	mcp.Uncheck()
	mcpRO.Uncheck()

	switch mode {
	case config.ModeMCP:
		mcp.Check()
	case config.ModeMCPReadOnly:
		mcpRO.Check()
	default: // proxy is the default
		proxy.Check()
	}
}

// switchMode persists the new mode, updates submenu checks, and restarts the daemon.
func switchMode(mode string, proxy, mcp, mcpRO *systray.MenuItem, binaryPath string, running *bool) {
	if err := config.SetMode(mode); err != nil {
		exec.Command("osascript", "-e",
			fmt.Sprintf(`display dialog "Failed to set mode: %s" buttons {"OK"} with icon stop with title "memoryd"`, err.Error())).Run()
		return
	}

	setModeChecks(mode, proxy, mcp, mcpRO)

	// Restart daemon so it picks up the new mode.
	if *running {
		stopDaemon()
		time.Sleep(500 * time.Millisecond)
		startDaemon(binaryPath)
	}

	label := map[string]string{
		config.ModeProxy:       "Proxy – auto read & write",
		config.ModeMCP:         "MCP – read & write",
		config.ModeMCPReadOnly: "MCP – read only",
	}[mode]
	exec.Command("osascript", "-e",
		fmt.Sprintf(`display notification "Mode set to: %s" with title "memoryd"`, label)).Run()
}

// uninstallDialog confirms with the user and removes memoryd components.
func uninstallDialog(binaryPath string) {
	// Confirm.
	_, err := exec.Command("osascript", "-e",
		`display dialog "This will stop the daemon and remove:\n\n• memoryd binary\n• ~/.memoryd (config, models, logs)\n• Docker container (memoryd-mongo)\n• Claude Desktop MCP entry\n\nThis cannot be undone." buttons {"Cancel", "Uninstall"} default button "Cancel" with icon caution with title "Uninstall memoryd"`, "-e",
		`button returned of result`).Output()
	if err != nil {
		return // user cancelled
	}

	// 1. Stop daemon.
	stopDaemon()

	var removed []string
	var failed []string

	// 2. Remove Claude Desktop MCP config entry.
	if cleanClaudeDesktopConfig() {
		removed = append(removed, "Claude Desktop MCP config")
	}

	// 3. Stop and remove Docker container.
	if containerExists("memoryd-mongo") {
		exec.Command("docker", "stop", "memoryd-mongo").Run()
		if exec.Command("docker", "rm", "memoryd-mongo").Run() == nil {
			removed = append(removed, "Docker container")
		} else {
			failed = append(failed, "Docker container (remove manually: docker rm memoryd-mongo)")
		}
	}

	// 4. Remove ~/.memoryd.
	memorydDir := config.Dir()
	if _, err := os.Stat(memorydDir); err == nil {
		if os.RemoveAll(memorydDir) == nil {
			removed = append(removed, "~/.memoryd")
		} else {
			failed = append(failed, "~/.memoryd (permission denied)")
		}
	}

	// 5. Remove binary.
	if binaryPath != "" && binaryPath != "memoryd" {
		if os.Remove(binaryPath) == nil {
			removed = append(removed, "memoryd binary")
		} else {
			// May need sudo — try.
			if exec.Command("sudo", "rm", "-f", binaryPath).Run() == nil {
				removed = append(removed, "memoryd binary")
			} else {
				failed = append(failed, fmt.Sprintf("binary at %s (remove manually)", binaryPath))
			}
		}
	}

	// 6. Show result.
	msg := "memoryd has been uninstalled."
	if len(removed) > 0 {
		msg += "\n\nRemoved:\n• " + strings.Join(removed, "\n• ")
	}
	if len(failed) > 0 {
		msg += "\n\nCould not remove:\n• " + strings.Join(failed, "\n• ")
	}
	msg += "\n\nThe tray app will now quit."

	exec.Command("osascript", "-e",
		fmt.Sprintf(`display dialog "%s" buttons {"OK"} with title "memoryd"`, msg)).Run()

	systray.Quit()
}

// cleanClaudeDesktopConfig removes the memoryd entry from Claude Desktop's config.
func cleanClaudeDesktopConfig() bool {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		return false
	}

	if _, exists := servers["memoryd"]; !exists {
		return false
	}

	delete(servers, "memoryd")
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false
	}

	return os.WriteFile(configPath, out, 0600) == nil
}

// containerExists checks if a Docker container exists (running or stopped).
func containerExists(name string) bool {
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}
