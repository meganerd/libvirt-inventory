package create

import (
	"fmt"
	"strings"
)

// RenderUserData generates cloud-init user-data YAML.
func RenderUserData(spec *VMSpec) string {
	var keyLines string
	for _, key := range spec.SSHKeys {
		keyLines += fmt.Sprintf("      - %s\n", key)
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
func GenerateCloudInitISO(spec *VMSpec, sshFunc func(cmd string) ([]byte, error)) (string, error) {
	userData := RenderUserData(spec)
	metaData := RenderMetaData(spec)

	tmpDir := fmt.Sprintf("/tmp/lvi-cloudinit-%s", spec.Name)
	isoName := fmt.Sprintf("%s-cloudinit.iso", spec.Name)

	// Escape single quotes in user-data for shell safety
	escapedUserData := strings.ReplaceAll(userData, "'", "'\\''")
	escapedMetaData := strings.ReplaceAll(metaData, "'", "'\\''")

	// Create temp dir, write files, generate ISO, clean up
	script := fmt.Sprintf(`
mkdir -p '%s' && \
printf '%%s' '%s' > '%s/user-data' && \
printf '%%s' '%s' > '%s/meta-data' && \
genisoimage -output '%s/%s' -volid cidata -joliet -rock '%s/user-data' '%s/meta-data' 2>/dev/null && \
echo '%s/%s'
`,
		tmpDir,
		escapedUserData, tmpDir,
		escapedMetaData, tmpDir,
		tmpDir, isoName, tmpDir, tmpDir,
		tmpDir, isoName,
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
