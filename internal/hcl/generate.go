package hcl

import (
	"fmt"
	"strings"

	"github.com/meganerd/libvirt-inventory/internal/model"
)

// sanitize converts a name to a valid HCL identifier.
func sanitize(name string) string {
	r := strings.NewReplacer("-", "_", ".", "_", " ", "_")
	return r.Replace(name)
}

// GenerateProviderHCL returns a provider block with alias for a hypervisor.
func GenerateProviderHCL(name, uri string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    libvirt = {
      source  = "dmacvicar/libvirt"
      version = "~> 0.8"
    }
  }
}

# Hypervisor: %s
provider "libvirt" {
  alias = "%s"
  uri   = "%s"
}
`, name, sanitize(name), uri)
}

// GenerateDomainHCL returns HCL for a libvirt_domain resource.
// Note: domains cannot be imported in provider v2 — this is reference HCL only.
func GenerateDomainHCL(d *model.Domain, hypervisorAlias string) string {
	resName := sanitize(d.Name)
	alias := sanitize(hypervisorAlias)

	var disks string
	for _, disk := range d.Disks {
		if disk.Source == "" {
			continue
		}
		disks += fmt.Sprintf(`
  disk {
    # source: %s (format: %s)
    volume_id = "TODO" # manual: look up volume resource ID
  }
`, disk.Source, disk.Format)
	}

	var nics string
	for _, nic := range d.NICs {
		nics += fmt.Sprintf(`
  network_interface {
    network_name = "%s"
    # MAC: %s
  }
`, nic.Source, nic.MAC)
	}

	return fmt.Sprintf(`# Domain: %s (UUID: %s)
# NOTE: libvirt_domain does not support import in provider v2.
# This HCL is reference-only. To manage this domain with tofu,
# you would need to recreate it or wait for upstream import support.
resource "libvirt_domain" "%s" {
  provider = libvirt.%s
  name     = "%s"
  memory   = %d
  vcpu     = %d
%s%s
  lifecycle {
    prevent_destroy = true
  }
}
`, d.Name, d.UUID, resName, alias, d.Name, d.MemoryMiB, d.VCPUs, disks, nics)
}

// GenerateVolumeHCL returns HCL for a libvirt_volume resource + import block.
func GenerateVolumeHCL(v *model.Volume, hypervisorAlias string) string {
	resName := sanitize(v.Pool + "_" + v.Name)
	alias := sanitize(hypervisorAlias)

	return fmt.Sprintf(`# Volume: %s (pool: %s, path: %s)
import {
  to = libvirt_volume.%s
  id = "%s"
}

resource "libvirt_volume" "%s" {
  provider = libvirt.%s
  name     = "%s"
  pool     = "%s"
  # size: %d bytes, format: %s

  lifecycle {
    prevent_destroy = true
  }
}
`, v.Name, v.Pool, v.Path, resName, v.Path, resName, alias, v.Name, v.Pool, v.SizeBytes, v.Format)
}

// GenerateNetworkHCL returns HCL for a libvirt_network resource + import block.
func GenerateNetworkHCL(n *model.Network, hypervisorAlias string) string {
	resName := sanitize(n.Name)
	alias := sanitize(hypervisorAlias)

	mode := n.Mode
	if mode == "" {
		mode = "none"
	}

	var forwardBlock string
	if n.Mode != "" {
		forwardBlock = fmt.Sprintf(`
  mode = "%s"
`, n.Mode)
	}

	return fmt.Sprintf(`# Network: %s (UUID: %s, bridge: %s)
import {
  to = libvirt_network.%s
  id = "%s"
}

resource "libvirt_network" "%s" {
  provider = libvirt.%s
  name     = "%s"%s

  lifecycle {
    prevent_destroy = true
  }
}
`, n.Name, n.UUID, n.Bridge, resName, n.UUID, resName, alias, n.Name, forwardBlock)
}
