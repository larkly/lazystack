package ssh

import (
	"os"
	"path/filepath"
	"strings"
)

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

// BuildArgs returns the argument slice for exec.Command("ssh", args...).
func BuildArgs(user, ip, keyPath string) []string {
	var args []string
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, user+"@"+ip)
	return args
}

// BuildCommandString returns the full SSH command string for clipboard use.
func BuildCommandString(user, ip, keyPath string) string {
	var parts []string
	parts = append(parts, "ssh")
	if keyPath != "" {
		parts = append(parts, "-i", keyPath)
	}
	parts = append(parts, user+"@"+ip)
	return strings.Join(parts, " ")
}
