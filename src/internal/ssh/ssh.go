package ssh

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
)

// Options holds SSH connection parameters.
type Options struct {
	User           string
	IP             string
	KeyPath        string
	Debug          bool
	IgnoreHostKeys bool
}

// FindKeyPath looks for a private key matching the given key pair name
// in ~/.ssh/. Returns the path if found, empty string otherwise.
func FindKeyPath(keyName string) string {
	shared.Debugf("[ssh] FindKeyPath: start keyName=%q", keyName)
	if keyName == "" {
		shared.Debugf("[ssh] FindKeyPath: empty keyName, returning empty")
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		shared.Debugf("[ssh] FindKeyPath: error getting home dir: %v", err)
		return ""
	}
	sshDir := filepath.Join(home, ".ssh")
	candidates := []string{
		filepath.Join(sshDir, keyName),
		filepath.Join(sshDir, keyName+".pem"),
		filepath.Join(sshDir, "id_"+keyName),
	}
	shared.Debugf("[ssh] FindKeyPath: checking candidates %v", candidates)
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			shared.Debugf("[ssh] FindKeyPath: found key at %s", p)
			return p
		}
	}
	shared.Debugf("[ssh] FindKeyPath: no key found for %q", keyName)
	return ""
}

// ChooseIP selects the best IP for SSH connection.
// Priority: floating IP → IPv6 → IPv4.
func ChooseIP(floatingIPs, ipv6, ipv4 []string) string {
	shared.Debugf("[ssh] ChooseIP: start floatingIPs=%v ipv6=%v ipv4=%v", floatingIPs, ipv6, ipv4)
	if len(floatingIPs) > 0 {
		shared.Debugf("[ssh] ChooseIP: selected floating IP %s", floatingIPs[0])
		return floatingIPs[0]
	}
	if len(ipv6) > 0 {
		shared.Debugf("[ssh] ChooseIP: selected IPv6 %s", ipv6[0])
		return ipv6[0]
	}
	if len(ipv4) > 0 {
		shared.Debugf("[ssh] ChooseIP: selected IPv4 %s", ipv4[0])
		return ipv4[0]
	}
	shared.Debugf("[ssh] ChooseIP: no IP available")
	return ""
}

func baseArgs() []string {
	return []string{
		"-t",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	}
}

// BuildArgs returns the argument slice for exec.Command("ssh", args...).
func BuildArgs(opts Options) []string {
	shared.Debugf("[ssh] BuildArgs: start user=%s ip=%s keyPath=%s", opts.User, opts.IP, opts.KeyPath)
	args := baseArgs()
	if opts.Debug {
		args = append(args, "-v")
	}
	if opts.IgnoreHostKeys {
		args = append(args,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
		)
	}
	if opts.KeyPath != "" {
		args = append(args, "-i", opts.KeyPath)
	}
	args = append(args, opts.User+"@"+opts.IP)
	shared.Debugf("[ssh] BuildArgs: final args %v", args)
	return args
}

// BuildCommandString returns the full SSH command string for clipboard use.
func BuildCommandString(opts Options) string {
	shared.Debugf("[ssh] BuildCommandString: start user=%s ip=%s", opts.User, opts.IP)
	var parts []string
	parts = append(parts, "ssh")
	parts = append(parts, baseArgs()...)
	if opts.IgnoreHostKeys {
		parts = append(parts,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
		)
	}
	if opts.KeyPath != "" {
		parts = append(parts, "-i", opts.KeyPath)
	}
	parts = append(parts, opts.User+"@"+opts.IP)
	cmd := strings.Join(parts, " ")
	shared.Debugf("[ssh] BuildCommandString: result %s", cmd)
	return cmd
}
