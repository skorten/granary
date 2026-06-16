package service

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func generatePlist(binaryPath string, hour, minute int) string {
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
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key>
        <integer>%d</integer>
        <key>Minute</key>
        <integer>%d</integer>
    </dict>
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
</plist>`, Label, binaryPath, hour, minute, stdoutLog, stderrLog)
}

// pickRandomTime returns a per-user random daily run time in [00:00, 03:00).
// Randomizing across installs avoids a synchronized spike against Granola's API.
func pickRandomTime() (int, int) {
	return rand.IntN(3), rand.IntN(60)
}

// parseAtTime parses an "HH:MM" 24-hour time.
func parseAtTime(at string) (int, int, error) {
	parts := strings.Split(at, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q: use HH:MM (24-hour), e.g. 02:30", at)
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid time %q: use HH:MM (24-hour), e.g. 02:30", at)
	}
	return hour, minute, nil
}

func Install(force bool, at string) error {
	plist := PlistPath()

	if _, err := os.Stat(plist); err == nil && !force {
		return fmt.Errorf("LaunchAgent already installed at %s\nUse --force to overwrite", plist)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine binary path: %w", err)
	}

	var hour, minute int
	if at == "" {
		hour, minute = pickRandomTime()
	} else {
		var err error
		hour, minute, err = parseAtTime(at)
		if err != nil {
			return err
		}
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
	content := generatePlist(binaryPath, hour, minute)
	if err := os.WriteFile(plist, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write plist to %s: %w", plist, err)
	}

	// Load agent
	out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%s", currentUID()), plist).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap failed: %s", strings.TrimSpace(string(out)))
	}

	fmt.Printf("Done. Granary will back up your Granola transcripts automatically\n")
	fmt.Printf("once a day at %02d:%02d, in the background, while your Mac is on.\n", hour, minute)
	fmt.Println("(If your Mac is asleep at that time, it runs at the next wake.)")
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
