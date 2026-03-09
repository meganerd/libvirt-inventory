package parser

import (
	"encoding/xml"
	"fmt"

	"github.com/meganerd/libvirt-inventory/internal/model"
)

// Libvirt domain XML structures
type xmlDomain struct {
	XMLName xml.Name        `xml:"domain"`
	Name    string          `xml:"name"`
	UUID    string          `xml:"uuid"`
	VCPU    int             `xml:"vcpu"`
	Memory  xmlMemory       `xml:"memory"`
	Devices xmlDevices      `xml:"devices"`
}

type xmlMemory struct {
	Value int    `xml:",chardata"`
	Unit  string `xml:"unit,attr"`
}

type xmlDevices struct {
	Disks      []xmlDisk      `xml:"disk"`
	Interfaces []xmlInterface `xml:"interface"`
}

type xmlDisk struct {
	Device string        `xml:"device,attr"`
	Driver xmlDiskDriver `xml:"driver"`
	Source xmlDiskSource `xml:"source"`
	Target xmlDiskTarget `xml:"target"`
}

type xmlDiskDriver struct {
	Type string `xml:"type,attr"`
}

type xmlDiskSource struct {
	File string `xml:"file,attr"`
	Dev  string `xml:"dev,attr"`
}

type xmlDiskTarget struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

type xmlInterface struct {
	Type   string         `xml:"type,attr"`
	Source xmlIfaceSource `xml:"source"`
	MAC    xmlMAC         `xml:"mac"`
	Model  xmlIfaceModel  `xml:"model"`
}

type xmlIfaceSource struct {
	Network string `xml:"network,attr"`
	Bridge  string `xml:"bridge,attr"`
}

type xmlMAC struct {
	Address string `xml:"address,attr"`
}

type xmlIfaceModel struct {
	Type string `xml:"type,attr"`
}

// Libvirt volume XML structures
type xmlVolume struct {
	XMLName  xml.Name        `xml:"volume"`
	Name     string          `xml:"name"`
	Key      string          `xml:"key"`
	Capacity xmlCapacity     `xml:"capacity"`
	Target   xmlVolumeTarget `xml:"target"`
}

type xmlCapacity struct {
	Value int64  `xml:",chardata"`
	Unit  string `xml:"unit,attr"`
}

type xmlVolumeTarget struct {
	Format xmlVolumeFormat `xml:"format"`
}

type xmlVolumeFormat struct {
	Type string `xml:"type,attr"`
}

// Libvirt network XML structures
type xmlNetwork struct {
	XMLName xml.Name      `xml:"network"`
	Name    string        `xml:"name"`
	UUID    string        `xml:"uuid"`
	Bridge  xmlBridge     `xml:"bridge"`
	Forward xmlForward    `xml:"forward"`
}

type xmlBridge struct {
	Name string `xml:"name,attr"`
}

type xmlForward struct {
	Mode string `xml:"mode,attr"`
}

// ParseDomainXML parses virsh dumpxml output for a domain.
func ParseDomainXML(xmlData []byte) (*model.Domain, error) {
	var d xmlDomain
	if err := xml.Unmarshal(xmlData, &d); err != nil {
		return nil, fmt.Errorf("parsing domain XML: %w", err)
	}

	memMiB := d.Memory.Value
	switch d.Memory.Unit {
	case "KiB":
		memMiB = d.Memory.Value / 1024
	case "GiB":
		memMiB = d.Memory.Value * 1024
	case "bytes":
		memMiB = d.Memory.Value / (1024 * 1024)
	}

	domain := &model.Domain{
		Name:      d.Name,
		UUID:      d.UUID,
		VCPUs:     d.VCPU,
		MemoryMiB: memMiB,
	}

	for _, disk := range d.Devices.Disks {
		source := disk.Source.File
		if source == "" {
			source = disk.Source.Dev
		}
		domain.Disks = append(domain.Disks, model.DiskInfo{
			Device: disk.Device,
			Source: source,
			Format: disk.Driver.Type,
			Bus:    disk.Target.Bus,
		})
	}

	for _, iface := range d.Devices.Interfaces {
		source := iface.Source.Network
		if source == "" {
			source = iface.Source.Bridge
		}
		domain.NICs = append(domain.NICs, model.NICInfo{
			Type:   iface.Type,
			Source: source,
			MAC:    iface.MAC.Address,
			Model:  iface.Model.Type,
		})
	}

	return domain, nil
}

// ParseVolumeXML parses virsh vol-dumpxml output.
func ParseVolumeXML(xmlData []byte, pool string) (*model.Volume, error) {
	var v xmlVolume
	if err := xml.Unmarshal(xmlData, &v); err != nil {
		return nil, fmt.Errorf("parsing volume XML: %w", err)
	}

	sizeBytes := v.Capacity.Value
	switch v.Capacity.Unit {
	case "KiB", "kB":
		sizeBytes = v.Capacity.Value * 1024
	case "MiB", "MB":
		sizeBytes = v.Capacity.Value * 1024 * 1024
	case "GiB", "GB":
		sizeBytes = v.Capacity.Value * 1024 * 1024 * 1024
	}

	return &model.Volume{
		Name:      v.Name,
		Pool:      pool,
		Path:      v.Key,
		SizeBytes: sizeBytes,
		Format:    v.Target.Format.Type,
	}, nil
}

// ParseNetworkXML parses virsh net-dumpxml output.
func ParseNetworkXML(xmlData []byte) (*model.Network, error) {
	var n xmlNetwork
	if err := xml.Unmarshal(xmlData, &n); err != nil {
		return nil, fmt.Errorf("parsing network XML: %w", err)
	}

	return &model.Network{
		Name:   n.Name,
		UUID:   n.UUID,
		Bridge: n.Bridge.Name,
		Mode:   n.Forward.Mode,
	}, nil
}
