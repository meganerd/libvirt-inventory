package create

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/meganerd/libvirt-inventory/internal/hypervisor"
)

// Result holds the outcome of a VM creation.
type Result struct {
	Name     string
	IP       string
	User     string
	Password string
}

// CreateVM orchestrates the full VM creation pipeline.
func CreateVM(spec *VMSpec) (*Result, error) {
	spec.ApplyDefaults()

	// Generate password
	pass, err := GeneratePassword(24)
	if err != nil {
		return nil, fmt.Errorf("generating password: %w", err)
	}
	spec.InstallPass = pass

	client, err := hypervisor.NewClient(spec.URI)
	if err != nil {
		return nil, fmt.Errorf("connecting to hypervisor: %w", err)
	}

	// Helper for running shell commands on hypervisor
	sshFunc := func(cmd string) ([]byte, error) {
		return client.RunCommand(cmd)
	}

	fmt.Printf("[1/7] Checking prerequisites on %s...\n", spec.Hypervisor)
	if err := checkPrereqs(sshFunc); err != nil {
		return nil, err
	}

	fmt.Printf("[2/7] Ensuring base image is available in pool '%s'...\n", spec.Pool)
	basePath, err := ensureBaseImage(spec, client)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[3/7] Creating disk overlay (%dG)...\n", spec.DiskSizeGB)
	diskPath, err := createDiskOverlay(spec, client, basePath)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[4/7] Generating cloud-init ISO...\n")
	isoPath, err := GenerateCloudInitISO(spec, sshFunc)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[5/7] Defining and starting domain...\n")
	if err := defineDomain(spec, client, diskPath, isoPath); err != nil {
		return nil, err
	}

	fmt.Printf("[6/7] Waiting for IP address...\n")
	ip, err := waitForIP(spec, client)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[7/7] Waiting for SSH on %s...\n", ip)
	if err := waitForSSH(ip, spec.InstallUser, spec.InstallPass); err != nil {
		return nil, fmt.Errorf("SSH wait failed: %w", err)
	}

	// Run Ansible if playbook specified
	if spec.Playbook != "" {
		fmt.Printf("[+] Running Ansible playbook: %s\n", spec.Playbook)
		if err := runAnsible(spec, ip); err != nil {
			fmt.Printf("[WARN] Ansible failed: %v\n", err)
			fmt.Println("  VM is running and accessible — you can run Ansible manually.")
		}
	}

	return &Result{
		Name:     spec.Name,
		IP:       ip,
		User:     spec.InstallUser,
		Password: spec.InstallPass,
	}, nil
}

func checkPrereqs(sshFunc func(string) ([]byte, error)) error {
	out, err := sshFunc("which genisoimage virsh qemu-img 2>&1")
	if err != nil {
		return fmt.Errorf("prerequisite check failed: %w\nInstall: genisoimage, libvirt-clients, qemu-utils", err)
	}
	output := string(out)
	for _, tool := range []string{"genisoimage", "virsh", "qemu-img"} {
		if !strings.Contains(output, tool) {
			return fmt.Errorf("missing prerequisite on hypervisor: %s", tool)
		}
	}
	return nil
}

func ensureBaseImage(spec *VMSpec, client *hypervisor.Client) (string, error) {
	baseName := fmt.Sprintf("lvi-base-%s.qcow2", sanitizeImageName(spec.BaseImageURL))

	// Check if base image already exists in pool
	out, err := client.Virsh("vol-path", "--pool", spec.Pool, baseName)
	if err == nil {
		path := strings.TrimSpace(string(out))
		if path != "" {
			fmt.Printf("  Base image already exists: %s\n", path)
			return path, nil
		}
	}

	// Download base image to a temp location and upload to pool
	fmt.Printf("  Downloading base image (this may take a while)...\n")
	script := fmt.Sprintf(`
tmpfile=$(mktemp /tmp/lvi-base-XXXXXX.img) && \
wget -q -O "$tmpfile" '%s' && \
echo "$tmpfile"
`, spec.BaseImageURL)

	out, err = client.RunCommand(script)
	if err != nil {
		return "", fmt.Errorf("downloading base image: %w", err)
	}
	tmpFile := strings.TrimSpace(string(out))

	// Create volume in pool from downloaded image
	_, err = client.Virsh("vol-create-as", "--pool", spec.Pool,
		"--name", baseName, "--capacity", "0", "--format", "qcow2")
	if err != nil {
		return "", fmt.Errorf("creating base volume: %w", err)
	}

	_, err = client.Virsh("vol-upload", "--pool", spec.Pool, baseName, tmpFile)
	if err != nil {
		return "", fmt.Errorf("uploading base image to pool: %w", err)
	}

	// Clean up temp file
	client.RunCommand(fmt.Sprintf("rm -f '%s'", tmpFile))

	// Get the path of the newly created volume
	out, err = client.Virsh("vol-path", "--pool", spec.Pool, baseName)
	if err != nil {
		return "", fmt.Errorf("getting base volume path: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

func createDiskOverlay(spec *VMSpec, client *hypervisor.Client, basePath string) (string, error) {
	diskName := fmt.Sprintf("%s-disk.qcow2", spec.Name)

	_, err := client.Virsh("vol-create-as",
		"--pool", spec.Pool,
		"--name", diskName,
		"--capacity", fmt.Sprintf("%dG", spec.DiskSizeGB),
		"--format", "qcow2",
		"--backing-vol", basePath,
		"--backing-vol-format", "qcow2",
	)
	if err != nil {
		return "", fmt.Errorf("creating disk overlay: %w", err)
	}

	out, err := client.Virsh("vol-path", "--pool", spec.Pool, diskName)
	if err != nil {
		return "", fmt.Errorf("getting disk path: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

func defineDomain(spec *VMSpec, client *hypervisor.Client, diskPath, isoPath string) error {
	domainXML := GenerateDomainXML(spec, diskPath, isoPath)

	// Write XML to temp file on hypervisor, define, then start
	tmpXML := fmt.Sprintf("/tmp/lvi-domain-%s.xml", spec.Name)
	if err := client.WriteFileViaSSH(tmpXML, domainXML); err != nil {
		return fmt.Errorf("writing domain XML: %w", err)
	}

	_, err := client.Virsh("define", tmpXML)
	if err != nil {
		return fmt.Errorf("defining domain: %w", err)
	}

	_, err = client.Virsh("start", spec.Name)
	if err != nil {
		return fmt.Errorf("starting domain: %w", err)
	}

	// Clean up temp XML
	client.RunCommand(fmt.Sprintf("rm -f '%s'", tmpXML))

	return nil
}

func waitForIP(spec *VMSpec, client *hypervisor.Client) (string, error) {
	timeout := 120 * time.Second
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ip, err := client.DomainIPAddress(spec.Name)
		if err == nil && ip != "" {
			return ip, nil
		}
		time.Sleep(interval)
	}

	return "", fmt.Errorf("timed out waiting for IP address after %s", timeout)
}

func waitForSSH(ip, user, pass string) error {
	timeout := 120 * time.Second
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.Command("sshpass", "-p", pass,
			"ssh",
			"-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=no",
			"-o", "BatchMode=no",
			fmt.Sprintf("%s@%s", user, ip),
			"true",
		)
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("SSH not reachable on %s after %s", ip, timeout)
}

func runAnsible(spec *VMSpec, ip string) error {
	cmd := exec.Command("ansible-playbook",
		"-i", ip+",",
		spec.Playbook,
		"--extra-vars", fmt.Sprintf("ansible_user=%s ansible_ssh_pass=%s",
			spec.InstallUser, spec.InstallPass),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "ANSIBLE_HOST_KEY_CHECKING=False")

	return cmd.Run()
}

func sanitizeImageName(url string) string {
	// Extract filename-ish identifier from URL
	parts := strings.Split(url, "/")
	name := parts[len(parts)-1]
	name = strings.ReplaceAll(name, ".img", "")
	name = strings.ReplaceAll(name, ".qcow2", "")
	// Keep it short
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}
