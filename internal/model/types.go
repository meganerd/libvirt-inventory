package model

import "time"

// Domain represents a libvirt domain (VM).
type Domain struct {
	Name      string     `json:"name"`
	UUID      string     `json:"uuid"`
	State     string     `json:"state"`
	VCPUs     int        `json:"vcpus"`
	MemoryMiB int        `json:"memory_mib"`
	Disks     []DiskInfo `json:"disks,omitempty"`
	NICs      []NICInfo  `json:"nics,omitempty"`
}

// DiskInfo represents a domain's disk device.
type DiskInfo struct {
	Device string `json:"device"`
	Source string `json:"source"`
	Format string `json:"format"`
	Bus    string `json:"bus"`
}

// NICInfo represents a domain's network interface.
type NICInfo struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	MAC    string `json:"mac"`
	Model  string `json:"model"`
}

// Volume represents a libvirt storage volume.
type Volume struct {
	Name      string `json:"name"`
	Pool      string `json:"pool"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Format    string `json:"format"`
}

// Network represents a libvirt network.
type Network struct {
	Name   string `json:"name"`
	UUID   string `json:"uuid"`
	Bridge string `json:"bridge"`
	Mode   string `json:"mode"`
	Active bool   `json:"active"`
}

// Hypervisor represents a single hypervisor's complete state.
type Hypervisor struct {
	Name     string    `json:"name"`
	URI      string    `json:"uri"`
	Domains  []Domain  `json:"domains"`
	Volumes  []Volume  `json:"volumes"`
	Networks []Network `json:"networks"`
}

// Snapshot is a point-in-time capture of all hypervisor state.
type Snapshot struct {
	Timestamp   time.Time    `json:"timestamp"`
	Hypervisors []Hypervisor `json:"hypervisors"`
}
