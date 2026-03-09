package drift

import (
	"fmt"
	"strings"

	"github.com/meganerd/libvirt-inventory/internal/model"
)

// Change represents a single drift item.
type Change struct {
	Hypervisor string `json:"hypervisor"`
	Type       string `json:"type"` // "domain", "volume", "network"
	Name       string `json:"name"`
	Action     string `json:"action"` // "added", "removed", "changed"
	Details    string `json:"details,omitempty"`
}

// Report is the result of a drift comparison.
type Report struct {
	Changes  []Change `json:"changes"`
	HasDrift bool     `json:"has_drift"`
}

// Compare takes a previous and current snapshot and returns a drift report.
func Compare(previous, current *model.Snapshot) *Report {
	r := &Report{}

	prevHVs := indexHypervisors(previous)
	currHVs := indexHypervisors(current)

	// Check all hypervisors in current scan
	for name, currHV := range currHVs {
		prevHV, existed := prevHVs[name]
		if !existed {
			r.add(Change{
				Hypervisor: name,
				Type:       "hypervisor",
				Name:       name,
				Action:     "added",
				Details:    "New hypervisor discovered",
			})
			// Report all resources as new
			for _, d := range currHV.Domains {
				r.add(Change{Hypervisor: name, Type: "domain", Name: d.Name, Action: "added"})
			}
			for _, v := range currHV.Volumes {
				r.add(Change{Hypervisor: name, Type: "volume", Name: v.Pool + "/" + v.Name, Action: "added"})
			}
			for _, n := range currHV.Networks {
				r.add(Change{Hypervisor: name, Type: "network", Name: n.Name, Action: "added"})
			}
			continue
		}

		// Compare domains
		compareDomains(r, name, prevHV.Domains, currHV.Domains)
		compareVolumes(r, name, prevHV.Volumes, currHV.Volumes)
		compareNetworks(r, name, prevHV.Networks, currHV.Networks)
	}

	// Check for hypervisors that were in previous but not current
	for name := range prevHVs {
		if _, exists := currHVs[name]; !exists {
			r.add(Change{
				Hypervisor: name,
				Type:       "hypervisor",
				Name:       name,
				Action:     "removed",
				Details:    "WARNING: Hypervisor no longer reachable or removed from config",
			})
		}
	}

	return r
}

func compareDomains(r *Report, hv string, prev, curr []model.Domain) {
	prevMap := make(map[string]model.Domain)
	for _, d := range prev {
		prevMap[d.UUID] = d
	}
	currMap := make(map[string]model.Domain)
	for _, d := range curr {
		currMap[d.UUID] = d
	}

	for uuid, cd := range currMap {
		pd, existed := prevMap[uuid]
		if !existed {
			r.add(Change{Hypervisor: hv, Type: "domain", Name: cd.Name, Action: "added",
				Details: fmt.Sprintf("UUID: %s, vCPU: %d, Memory: %d MiB", cd.UUID, cd.VCPUs, cd.MemoryMiB)})
			continue
		}
		if diffs := domainDiff(pd, cd); diffs != "" {
			r.add(Change{Hypervisor: hv, Type: "domain", Name: cd.Name, Action: "changed", Details: diffs})
		}
	}

	for uuid, pd := range prevMap {
		if _, exists := currMap[uuid]; !exists {
			r.add(Change{Hypervisor: hv, Type: "domain", Name: pd.Name, Action: "removed",
				Details: fmt.Sprintf("WARNING: Domain %s (UUID: %s) no longer found on hypervisor", pd.Name, pd.UUID)})
		}
	}
}

func compareVolumes(r *Report, hv string, prev, curr []model.Volume) {
	prevMap := make(map[string]model.Volume)
	for _, v := range prev {
		prevMap[v.Path] = v
	}
	currMap := make(map[string]model.Volume)
	for _, v := range curr {
		currMap[v.Path] = v
	}

	for path, cv := range currMap {
		pv, existed := prevMap[path]
		if !existed {
			r.add(Change{Hypervisor: hv, Type: "volume", Name: cv.Pool + "/" + cv.Name, Action: "added",
				Details: fmt.Sprintf("Path: %s, Size: %d bytes", cv.Path, cv.SizeBytes)})
			continue
		}
		if pv.SizeBytes != cv.SizeBytes {
			r.add(Change{Hypervisor: hv, Type: "volume", Name: cv.Pool + "/" + cv.Name, Action: "changed",
				Details: fmt.Sprintf("Size changed: %d → %d bytes", pv.SizeBytes, cv.SizeBytes)})
		}
	}

	for path, pv := range prevMap {
		if _, exists := currMap[path]; !exists {
			r.add(Change{Hypervisor: hv, Type: "volume", Name: pv.Pool + "/" + pv.Name, Action: "removed",
				Details: fmt.Sprintf("WARNING: Volume %s no longer found", pv.Path)})
		}
	}
}

func compareNetworks(r *Report, hv string, prev, curr []model.Network) {
	prevMap := make(map[string]model.Network)
	for _, n := range prev {
		prevMap[n.UUID] = n
	}
	currMap := make(map[string]model.Network)
	for _, n := range curr {
		currMap[n.UUID] = n
	}

	for uuid, cn := range currMap {
		pn, existed := prevMap[uuid]
		if !existed {
			r.add(Change{Hypervisor: hv, Type: "network", Name: cn.Name, Action: "added",
				Details: fmt.Sprintf("UUID: %s, Bridge: %s, Mode: %s", cn.UUID, cn.Bridge, cn.Mode)})
			continue
		}
		if pn.Active != cn.Active {
			r.add(Change{Hypervisor: hv, Type: "network", Name: cn.Name, Action: "changed",
				Details: fmt.Sprintf("Active state changed: %v → %v", pn.Active, cn.Active)})
		}
	}

	for uuid, pn := range prevMap {
		if _, exists := currMap[uuid]; !exists {
			r.add(Change{Hypervisor: hv, Type: "network", Name: pn.Name, Action: "removed",
				Details: fmt.Sprintf("WARNING: Network %s (UUID: %s) no longer found", pn.Name, pn.UUID)})
		}
	}
}

func domainDiff(prev, curr model.Domain) string {
	var diffs []string
	if prev.VCPUs != curr.VCPUs {
		diffs = append(diffs, fmt.Sprintf("vCPU: %d → %d", prev.VCPUs, curr.VCPUs))
	}
	if prev.MemoryMiB != curr.MemoryMiB {
		diffs = append(diffs, fmt.Sprintf("Memory: %d → %d MiB", prev.MemoryMiB, curr.MemoryMiB))
	}
	if prev.State != curr.State {
		diffs = append(diffs, fmt.Sprintf("State: %s → %s", prev.State, curr.State))
	}
	if len(prev.Disks) != len(curr.Disks) {
		diffs = append(diffs, fmt.Sprintf("Disk count: %d → %d", len(prev.Disks), len(curr.Disks)))
	}
	if len(prev.NICs) != len(curr.NICs) {
		diffs = append(diffs, fmt.Sprintf("NIC count: %d → %d", len(prev.NICs), len(curr.NICs)))
	}
	return strings.Join(diffs, "; ")
}

func indexHypervisors(s *model.Snapshot) map[string]model.Hypervisor {
	m := make(map[string]model.Hypervisor, len(s.Hypervisors))
	for _, hv := range s.Hypervisors {
		m[hv.Name] = hv
	}
	return m
}

func (r *Report) add(c Change) {
	r.Changes = append(r.Changes, c)
	r.HasDrift = true
}

// String returns a human-readable drift report.
func (r *Report) String() string {
	if !r.HasDrift {
		return "No drift detected."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Drift detected: %d change(s)\n\n", len(r.Changes))

	for _, c := range r.Changes {
		icon := "?"
		switch c.Action {
		case "added":
			icon = "+"
		case "removed":
			icon = "!"
		case "changed":
			icon = "~"
		}
		fmt.Fprintf(&b, "  [%s] %s/%s %s", icon, c.Hypervisor, c.Type, c.Name)
		if c.Details != "" {
			fmt.Fprintf(&b, "\n      %s", c.Details)
		}
		b.WriteString("\n")
	}

	return b.String()
}
