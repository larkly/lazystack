package ssh

import (
	"strings"
	"testing"
)

func TestBuildArgsIncludesIgnoreHostKeyFlags(t *testing.T) {
	args := BuildArgs(Options{
		User:           "ubuntu",
		IP:             "192.0.2.10",
		KeyPath:        "/tmp/id_rsa",
		IgnoreHostKeys: true,
	})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "StrictHostKeyChecking=no") {
		t.Fatalf("args missing StrictHostKeyChecking=no: %v", args)
	}
	if !strings.Contains(joined, "UserKnownHostsFile=/dev/null") {
		t.Fatalf("args missing UserKnownHostsFile=/dev/null: %v", args)
	}
}

func TestBuildCommandStringIncludesIgnoreHostKeyFlags(t *testing.T) {
	cmd := BuildCommandString(Options{
		User:           "ubuntu",
		IP:             "192.0.2.10",
		IgnoreHostKeys: true,
	})

	if !strings.Contains(cmd, "StrictHostKeyChecking=no") {
		t.Fatalf("command missing StrictHostKeyChecking=no: %s", cmd)
	}
	if !strings.Contains(cmd, "UserKnownHostsFile=/dev/null") {
		t.Fatalf("command missing UserKnownHostsFile=/dev/null: %s", cmd)
	}
}

