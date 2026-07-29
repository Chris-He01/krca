package sandbox

import (
	"testing"
)

func TestGuardCheckCommand(t *testing.T) {
	guard, err := NewGuard(nil, true)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}

	denied := []string{
		"rm -rf /",
		"rm -f important.txt",
		"sudo shutdown now",
		"reboot",
		"poweroff",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda",
	}
	for _, cmd := range denied {
		if err := guard.CheckCommand(cmd); err == nil {
			t.Errorf("expected deny for %q, got nil", cmd)
		}
	}

	allowed := []string{
		"ls -la",
		"cat /etc/hostname",
		"ps aux",
		"df -h",
		"top -bn1",
		"echo hello",
		"grep -r pattern .",
	}
	for _, cmd := range allowed {
		if err := guard.CheckCommand(cmd); err != nil {
			t.Errorf("expected allow for %q, got %v", cmd, err)
		}
	}
}

func TestGuardCheckPath(t *testing.T) {
	guard, err := NewGuard(nil, true)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}

	denied := []string{
		"../etc/passwd",
		"foo/../../etc/shadow",
		"/etc/passwd",
		"/root/.ssh/id_rsa",
	}
	for _, p := range denied {
		if err := guard.CheckPath(p); err == nil {
			t.Errorf("expected deny for path %q, got nil", p)
		}
	}

	allowed := []string{
		"file.txt",
		"subdir/file.txt",
		"data/output.json",
	}
	for _, p := range allowed {
		if err := guard.CheckPath(p); err != nil {
			t.Errorf("expected allow for path %q, got %v", p, err)
		}
	}

	// Without workspace restriction
	guardNoRestrict, _ := NewGuard(nil, false)
	if err := guardNoRestrict.CheckPath("../etc/passwd"); err != nil {
		t.Errorf("expected allow with no restriction, got %v", err)
	}
}
