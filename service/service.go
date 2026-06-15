package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const Label = "com.skorten.granary"

func PlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

func LogDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "granary")
}

func currentUID() string {
	out, err := exec.Command("id", "-u").Output()
	if err != nil {
		return "501"
	}
	return strings.TrimSpace(string(out))
}

func generatePlist(binaryPath string) string {
	logDir := LogDir()
	stdoutLog := filepath.Join(logDir, "stdout.log")
	stderrLog := filepath.Join(logDir, "stderr.log")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>run</string>
    </array>
    <key>StartInterval</key>
    <integer>7200</integer>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>`, Label, binaryPath, stdoutLog, stderrLog)
}

func Install(force bool) error {
	plist := PlistPath()

	if _, err := os.Stat(plist); err == nil && !force {
		return fmt.Errorf("LaunchAgent already installed at %s\nUse --force to overwrite", plist)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine binary path: %w", err)
	}

	// Unload existing agent if overwriting
	if _, err := os.Stat(plist); err == nil {
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%s/%s", currentUID(), Label)).Run()
	}

	// Create log directory
	if err := os.MkdirAll(LogDir(), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Write plist
	content := generatePlist(binaryPath)
	if err := os.WriteFile(plist, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write plist to %s: %w", plist, err)
	}

	// Load agent
	out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%s", currentUID()), plist).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap failed: %s", strings.TrimSpace(string(out)))
	}

	fmt.Println("Done. Granary will now back up your Granola transcripts automatically")
	fmt.Println("every 2 hours, in the background, while your Mac is on.")
	fmt.Println()
	fmt.Println("  To check it's set up:  granary status")
	fmt.Println("  To turn it off:        granary uninstall")
	fmt.Printf("  Logs are saved in:     %s\n", LogDir())
	return nil
}

func Uninstall() error {
	// Unload (ignore errors if not loaded)
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%s/%s", currentUID(), Label)).Run()

	plist := PlistPath()
	if _, err := os.Stat(plist); err == nil {
		if err := os.Remove(plist); err != nil {
			return fmt.Errorf("failed to remove %s: %w", plist, err)
		}
		fmt.Println("LaunchAgent uninstalled.")
	} else {
		fmt.Println("LaunchAgent was not installed.")
	}

	return nil
}

func Status() (installed bool, running bool, err error) {
	err = exec.Command("launchctl", "list", Label).Run()
	running = err == nil
	err = nil

	_, statErr := os.Stat(PlistPath())
	installed = statErr == nil

	return installed, running, nil
}
