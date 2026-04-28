package create

import (
	"fmt"
	"strings"
)

// RenderUserData generates cloud-init user-data YAML.
func RenderUserData(spec *VMSpec) string {
	var keyLines string
	for _, key := range spec.SSHKeys {
		// Split multiline values (e.g. from curl piped into --ssh-key)
		for _, line := range strings.Split(key, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				keyLines += fmt.Sprintf("      - %s\n", line)
			}
		}
	}

	return fmt.Sprintf(`#cloud-config
hostname: %s
manage_etc_hosts: true

users:
  - name: %s
    groups: sudo
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: false
    ssh_authorized_keys:
%s
chpasswd:
  list: |
    %s:%s
  expire: false

package_update: true
package_upgrade: true
packages:
  - qemu-guest-agent
  - openssh-server
  - python3
  - sudo
  - unzip
  - curl

runcmd:
  - systemctl enable --now qemu-guest-agent
  - systemctl enable --now ssh
`, spec.Name, spec.InstallUser, keyLines, spec.InstallUser, spec.InstallPass)
}

// RenderMetaData generates cloud-init meta-data YAML.
func RenderMetaData(spec *VMSpec) string {
	return fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", spec.Name, spec.Name)
}

// GenerateCloudInitISO creates a cloud-init NoCloud ISO on the remote hypervisor.
// Returns the path to the ISO in the storage pool.
// writeFunc writes a file on the remote host via stdin pipe (no shell escaping).
// sshFunc runs a shell command on the remote host.
func GenerateCloudInitISO(spec *VMSpec, sshFunc func(cmd string) ([]byte, error), writeFunc func(path, content string) error) (string, error) {
	userData := RenderUserData(spec)
	metaData := RenderMetaData(spec)

	tmpDir := fmt.Sprintf("/tmp/lvi-cloudinit-%s", spec.Name)
	isoName := fmt.Sprintf("%s-cloudinit.iso", spec.Name)

	// Create temp dir
	if _, err := sshFunc(fmt.Sprintf("mkdir -p '%s'", tmpDir)); err != nil {
		return "", fmt.Errorf("creating cloud-init temp dir: %w", err)
	}

	// Write files via stdin pipe (avoids shell escaping issues)
	if err := writeFunc(fmt.Sprintf("%s/user-data", tmpDir), userData); err != nil {
		return "", fmt.Errorf("writing user-data: %w", err)
	}
	if err := writeFunc(fmt.Sprintf("%s/meta-data", tmpDir), metaData); err != nil {
		return "", fmt.Errorf("writing meta-data: %w", err)
	}

	// Generate ISO
	script := fmt.Sprintf(
		"genisoimage -output '%s/%s' -volid cidata -joliet -rock '%s/user-data' '%s/meta-data' 2>/dev/null && echo '%s/%s'",
		tmpDir, isoName, tmpDir, tmpDir, tmpDir, isoName,
	)
	out, err := sshFunc(script)
	if err != nil {
		return "", fmt.Errorf("generating cloud-init ISO: %w", err)
	}

	isoPath := strings.TrimSpace(string(out))
	if isoPath == "" {
		return "", fmt.Errorf("cloud-init ISO generation returned empty path")
	}

	return isoPath, nil
}
