package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

// SentinelManager handles interaction with the Sentinel proxy process.
type SentinelManager struct {
	configPath    string
	pidFile       string
	backupDir     string
	currentConfig string
}

// NewSentinelManager creates a new SentinelManager.
func NewSentinelManager(configPath string) *SentinelManager {
	return &SentinelManager{
		configPath: configPath,
		pidFile:    "/var/run/sentinel.pid",
		backupDir:  filepath.Dir(configPath),
	}
}

// SetPIDFile sets a custom PID file path.
func (s *SentinelManager) SetPIDFile(path string) {
	s.pidFile = path
}

// SetBackupDir sets a custom backup directory.
func (s *SentinelManager) SetBackupDir(path string) {
	s.backupDir = path
}

// ReadCurrentConfig reads the current config from disk.
func (s *SentinelManager) ReadCurrentConfig() (string, error) {
	content, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read config: %w", err)
	}
	s.currentConfig = string(content)
	return s.currentConfig, nil
}

// WriteConfig writes a new config to disk with backup.
func (s *SentinelManager) WriteConfig(content string) error {
	// Create backup of current config if it exists
	if _, err := os.Stat(s.configPath); err == nil {
		backupPath := filepath.Join(s.backupDir, fmt.Sprintf("config.kdl.bak"))
		if err := s.copyFile(s.configPath, backupPath); err != nil {
			log.Warn().Err(err).Str("backup", backupPath).Msg("Failed to create config backup")
		} else {
			log.Debug().Str("backup", backupPath).Msg("Created config backup")
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write new config atomically (write to temp file, then rename)
	tempPath := s.configPath + ".tmp"
	if err := os.WriteFile(tempPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	if err := os.Rename(tempPath, s.configPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename config: %w", err)
	}

	s.currentConfig = content
	log.Info().Str("path", s.configPath).Int("size", len(content)).Msg("Config written successfully")
	return nil
}

// Reload sends SIGHUP to the Sentinel process to trigger a config reload.
func (s *SentinelManager) Reload() error {
	pid, err := s.getSentinelPID()
	if err != nil {
		return fmt.Errorf("failed to get Sentinel PID: %w", err)
	}

	log.Info().Int("pid", pid).Msg("Sending SIGHUP to Sentinel...")

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send SIGHUP: %w", err)
	}

	log.Info().Int("pid", pid).Msg("SIGHUP sent successfully")
	return nil
}

// Rollback restores the backup config.
func (s *SentinelManager) Rollback() error {
	backupPath := filepath.Join(s.backupDir, "config.kdl.bak")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup config found")
	}

	content, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	if err := os.WriteFile(s.configPath, content, 0644); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	s.currentConfig = string(content)
	log.Info().Str("backup", backupPath).Msg("Config rolled back successfully")
	return nil
}

// getSentinelPID finds the PID of the running Sentinel process.
func (s *SentinelManager) getSentinelPID() (int, error) {
	// Try PID file first
	if s.pidFile != "" {
		if content, err := os.ReadFile(s.pidFile); err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
			if err == nil && pid > 0 {
				// Verify process exists
				if process, err := os.FindProcess(pid); err == nil {
					if err := process.Signal(syscall.Signal(0)); err == nil {
						return pid, nil
					}
				}
			}
		}
	}

	// Fall back to pgrep
	cmd := exec.Command("pgrep", "-x", "sentinel")
	output, err := cmd.Output()
	if err != nil {
		// Try alternative names
		cmd = exec.Command("pgrep", "-f", "sentinel-proxy")
		output, err = cmd.Output()
		if err != nil {
			return 0, fmt.Errorf("sentinel process not found")
		}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0, fmt.Errorf("sentinel process not found")
	}

	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0, fmt.Errorf("invalid PID: %s", lines[0])
	}

	return pid, nil
}

// copyFile copies a file from src to dst.
func (s *SentinelManager) copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, content, 0644)
}

// CheckHealth checks if Sentinel is running and healthy.
func (s *SentinelManager) CheckHealth() error {
	pid, err := s.getSentinelPID()
	if err != nil {
		return err
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}

	// Check if process is alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process not responding: %w", err)
	}

	return nil
}

// IsRunning returns true if Sentinel is running.
func (s *SentinelManager) IsRunning() bool {
	return s.CheckHealth() == nil
}

// GetConfigPath returns the config file path.
func (s *SentinelManager) GetConfigPath() string {
	return s.configPath
}

// GetCurrentConfig returns the current config content.
func (s *SentinelManager) GetCurrentConfig() string {
	return s.currentConfig
}
