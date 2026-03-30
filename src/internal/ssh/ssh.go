package ssh

import (
	"os"
	"path/filepath"
	"strings"
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
	if keyName == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	sshDir := filepath.Join(home, ".ssh")
	candidates := []string{
		filepath.Join(sshDir, keyName),
		filepath.Join(sshDir, keyName+".pem"),
		filepath.Join(sshDir, "id_"+keyName),
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// ChooseIP selects the best IP for SSH connection.
// Priority: floating IP → IPv6 → IPv4.
func ChooseIP(floatingIPs, ipv6, ipv4 []string) string {
	if len(floatingIPs) > 0 {
		return floatingIPs[0]
	}
	if len(ipv6) > 0 {
		return ipv6[0]
	}
	if len(ipv4) > 0 {
		return ipv4[0]
	}
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
	return args
}

// BuildCommandString returns the full SSH command string for clipboard use.
func BuildCommandString(opts Options) string {
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
	return strings.Join(parts, " ")
}
