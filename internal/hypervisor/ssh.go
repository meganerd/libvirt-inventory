package hypervisor

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Client executes virsh commands on a remote hypervisor via SSH.
type Client struct {
	Host string // SSH host (extracted from qemu+ssh URI)
	User string // SSH user
}

// NewClient creates a Client from a qemu+ssh URI.
// URI format: qemu+ssh://user@host/system
func NewClient(uri string) (*Client, error) {
	// Parse qemu+ssh://user@host/system
	trimmed := strings.TrimPrefix(uri, "qemu+ssh://")
	trimmed = strings.TrimPrefix(trimmed, "qemu://") // local
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("invalid libvirt URI: %s", uri)
	}

	userHost := parts[0]
	user := ""
	host := userHost
	if at := strings.Index(userHost, "@"); at >= 0 {
		user = userHost[:at]
		host = userHost[at+1:]
	}

	return &Client{Host: host, User: user}, nil
}

// Virsh runs a virsh command on the remote hypervisor and returns stdout.
func (c *Client) Virsh(args ...string) ([]byte, error) {
	sshTarget := c.Host
	if c.User != "" {
		sshTarget = c.User + "@" + c.Host
	}

	sshArgs := []string{
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		sshTarget,
		"virsh",
	}
	sshArgs = append(sshArgs, args...)

	cmd := exec.Command("ssh", sshArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("virsh %s on %s: %w\nstderr: %s",
			strings.Join(args, " "), c.Host, err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// ListDomains returns the names of all domains (running and stopped).
func (c *Client) ListDomains() ([]string, error) {
	out, err := c.Virsh("list", "--all", "--name")
	if err != nil {
		return nil, err
	}
	return filterEmpty(strings.Split(string(out), "\n")), nil
}

// DomainXML returns the XML definition of a domain.
func (c *Client) DomainXML(name string) ([]byte, error) {
	return c.Virsh("dumpxml", name)
}

// DomainState returns the state of a domain (running, shut off, etc).
func (c *Client) DomainState(name string) (string, error) {
	out, err := c.Virsh("domstate", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ListPools returns the names of all storage pools.
func (c *Client) ListPools() ([]string, error) {
	out, err := c.Virsh("pool-list", "--all", "--name")
	if err != nil {
		return nil, err
	}
	return filterEmpty(strings.Split(string(out), "\n")), nil
}

// ListVolumes returns the names of all volumes in a pool.
func (c *Client) ListVolumes(pool string) ([]string, error) {
	out, err := c.Virsh("vol-list", "--pool", pool)
	if err != nil {
		return nil, err
	}
	// vol-list output has header rows; parse name column
	lines := strings.Split(string(out), "\n")
	var names []string
	for i, line := range lines {
		if i < 2 { // skip header + separator
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

// VolumeXML returns the XML definition of a volume.
func (c *Client) VolumeXML(name, pool string) ([]byte, error) {
	return c.Virsh("vol-dumpxml", "--pool", pool, name)
}

// ListNetworks returns the names of all networks.
func (c *Client) ListNetworks() ([]string, error) {
	out, err := c.Virsh("net-list", "--all", "--name")
	if err != nil {
		return nil, err
	}
	return filterEmpty(strings.Split(string(out), "\n")), nil
}

// NetworkXML returns the XML definition of a network.
func (c *Client) NetworkXML(name string) ([]byte, error) {
	return c.Virsh("net-dumpxml", name)
}

// NetworkActive returns whether a network is active.
func (c *Client) NetworkActive(name string) (bool, error) {
	out, err := c.Virsh("net-info", name)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), "Active:         yes"), nil
}

func filterEmpty(ss []string) []string {
	var result []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}
