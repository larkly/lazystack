package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ActionType categorizes user actions.
type ActionType string

const (
	ActionCreate       ActionType = "create"
	ActionDelete       ActionType = "delete"
	ActionReboot       ActionType = "reboot"
	ActionResize       ActionType = "resize"
	ActionRebuild      ActionType = "rebuild"
	ActionRename       ActionType = "rename"
	ActionSnapshot     ActionType = "snapshot"
	ActionRescue       ActionType = "rescue"
	ActionUnrescue     ActionType = "unrescue"
	ActionPause        ActionType = "pause"
	ActionUnpause      ActionType = "unpause"
	ActionSuspend      ActionType = "suspend"
	ActionResume       ActionType = "resume"
	ActionShelve       ActionType = "shelve"
	ActionUnshelve     ActionType = "unshelve"
	ActionStop         ActionType = "stop"
	ActionStart        ActionType = "start"
	ActionLock         ActionType = "lock"
	ActionUnlock       ActionType = "unlock"
	ActionMigrate      ActionType = "migrate"
	ActionEvacuate     ActionType = "evacuate"
	ActionForceDelete  ActionType = "force_delete"
	ActionResetState   ActionType = "reset_state"
	ActionAttachVolume ActionType = "attach_volume"
	ActionDetachVolume ActionType = "detach_volume"
	ActionAttachFIP    ActionType = "attach_fip"
	ActionDetachFIP    ActionType = "detach_fip"
	ActionAttachSG     ActionType = "attach_sg"
	ActionDetachSG     ActionType = "detach_sg"
	ActionCreateKey    ActionType = "create_key"
	ActionDeleteKey    ActionType = "delete_key"
	ActionCreateNet    ActionType = "create_network"
	ActionDeleteNet    ActionType = "delete_network"
	ActionCreateRouter ActionType = "create_router"
	ActionDeleteRouter ActionType = "delete_router"
	ActionCreatePort   ActionType = "create_port"
	ActionDeletePort   ActionType = "delete_port"
	ActionCreateSubnet ActionType = "create_subnet"
	ActionDeleteSubnet ActionType = "delete_subnet"
	ActionCreateLB     ActionType = "create_lb"
	ActionDeleteLB     ActionType = "delete_lb"
	ActionCreateImage  ActionType = "create_image"
	ActionDeleteImage  ActionType = "delete_image"
	ActionDownload     ActionType = "download"
	ActionUpload       ActionType = "upload"
	ActionCreateZone   ActionType = "create_zone"
	ActionDeleteZone   ActionType = "delete_zone"
	ActionCreateRecord ActionType = "create_record"
	ActionDeleteRecord ActionType = "delete_record"
	ActionClone        ActionType = "clone"
	ActionConsole      ActionType = "console"
	ActionSSH          ActionType = "ssh"
	ActionCopy         ActionType = "copy"
	ActionConfig       ActionType = "config"
	ActionUnknown      ActionType = "unknown"
)

// Entry is a single audit log record.
type Entry struct {
	Timestamp   time.Time       `json:"timestamp"`
	Cloud       string          `json:"cloud"`
	Project     string          `json:"project,omitempty"`
	Action      ActionType      `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID  string          `json:"resource_id,omitempty"`
	ResourceName string        `json:"resource_name,omitempty"`
	Result      string          `json:"result"` // success, error, cancelled
	Error       string          `json:"error,omitempty"`
	Details     json.RawMessage `json:"details,omitempty"`
}

// Logger writes structured audit entries to a rotating JSON log file.
type Logger struct {
	mu        sync.Mutex
	path      string
	maxSize   int64
	maxFiles  int
	enabled   bool
}

// NewLogger creates an audit logger.
func NewLogger(path string, enabled bool) *Logger {
	return &Logger{
		path:     path,
		maxSize:  10 * 1024 * 1024, // 10 MB
		maxFiles: 5,
		enabled:  enabled,
	}
}

// DefaultPath returns ~/.local/share/lazystack/audit.log.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "lazystack", "audit.log")
}

// SetEnabled toggles logging.
func (l *Logger) SetEnabled(v bool) {
	l.mu.Lock()
	l.enabled = v
	l.mu.Unlock()
}

// IsEnabled returns whether logging is active.
func (l *Logger) IsEnabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enabled
}

// Log writes an audit entry.
func (l *Logger) Log(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.enabled {
		return nil
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	if err := l.rotateIfNeeded(); err != nil {
		return fmt.Errorf("audit rotate: %w", err)
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit open: %w", err)
	}
	defer f.Close()

	b, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit marshal: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("audit write: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("audit newline: %w", err)
	}
	return nil
}

// rotateIfNeeded renames existing log files when the current exceeds maxSize.
func (l *Logger) rotateIfNeeded() error {
	info, err := os.Stat(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() < l.maxSize {
		return nil
	}

	// Remove oldest if it exists
	oldest := fmt.Sprintf("%s.%d", l.path, l.maxFiles)
	_ = os.Remove(oldest)

	// Shift existing backups
	for i := l.maxFiles - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.path, i)
		newPath := fmt.Sprintf("%s.%d", l.path, i+1)
		_ = os.Rename(oldPath, newPath)
	}

	return os.Rename(l.path, l.path+".1")
}

// ReadEntries reads up to limit entries in reverse chronological order (newest first).
func ReadEntries(path string, limit int) ([]Entry, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read all lines; this is fine for 10 MB files.
	var lines []string
	var buf [4096]byte
	var leftover []byte
	for {
		n, err := f.Read(buf[:])
		if n > 0 {
			data := append(leftover, buf[:n]...)
			for {
				idx := 0
				for i, b := range data {
					if b == '\n' {
						lines = append(lines, string(data[idx:i]))
						idx = i + 1
					}
				}
				leftover = data[idx:]
				break
			}
		}
		if err != nil {
			break
		}
	}
	if len(leftover) > 0 {
		lines = append(lines, string(leftover))
	}

	var entries []Entry
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	return entries, nil
}
