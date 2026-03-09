package scanner

import (
	"fmt"

	"github.com/meganerd/libvirt-inventory/internal/hypervisor"
	"github.com/meganerd/libvirt-inventory/internal/model"
	"github.com/meganerd/libvirt-inventory/internal/parser"
)

// ScanHypervisor enumerates all domains, volumes, and networks on a hypervisor.
func ScanHypervisor(name, uri string) (*model.Hypervisor, error) {
	client, err := hypervisor.NewClient(uri)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", name, err)
	}

	hv := &model.Hypervisor{
		Name: name,
		URI:  uri,
	}

	// Enumerate domains
	domainNames, err := client.ListDomains()
	if err != nil {
		return nil, fmt.Errorf("listing domains on %s: %w", name, err)
	}

	for _, dName := range domainNames {
		xmlData, err := client.DomainXML(dName)
		if err != nil {
			fmt.Printf("  [WARN] Failed to get XML for domain %s: %v\n", dName, err)
			continue
		}

		domain, err := parser.ParseDomainXML(xmlData)
		if err != nil {
			fmt.Printf("  [WARN] Failed to parse domain %s: %v\n", dName, err)
			continue
		}

		state, err := client.DomainState(dName)
		if err == nil {
			domain.State = state
		}

		hv.Domains = append(hv.Domains, *domain)
	}

	// Enumerate volumes across all pools
	pools, err := client.ListPools()
	if err != nil {
		fmt.Printf("  [WARN] Failed to list pools on %s: %v\n", name, err)
	} else {
		for _, pool := range pools {
			volNames, err := client.ListVolumes(pool)
			if err != nil {
				fmt.Printf("  [WARN] Failed to list volumes in pool %s: %v\n", pool, err)
				continue
			}

			for _, vName := range volNames {
				xmlData, err := client.VolumeXML(vName, pool)
				if err != nil {
					fmt.Printf("  [WARN] Failed to get XML for volume %s/%s: %v\n", pool, vName, err)
					continue
				}

				vol, err := parser.ParseVolumeXML(xmlData, pool)
				if err != nil {
					fmt.Printf("  [WARN] Failed to parse volume %s/%s: %v\n", pool, vName, err)
					continue
				}

				hv.Volumes = append(hv.Volumes, *vol)
			}
		}
	}

	// Enumerate networks
	netNames, err := client.ListNetworks()
	if err != nil {
		fmt.Printf("  [WARN] Failed to list networks on %s: %v\n", name, err)
	} else {
		for _, nName := range netNames {
			xmlData, err := client.NetworkXML(nName)
			if err != nil {
				fmt.Printf("  [WARN] Failed to get XML for network %s: %v\n", nName, err)
				continue
			}

			net, err := parser.ParseNetworkXML(xmlData)
			if err != nil {
				fmt.Printf("  [WARN] Failed to parse network %s: %v\n", nName, err)
				continue
			}

			active, err := client.NetworkActive(nName)
			if err == nil {
				net.Active = active
			}

			hv.Networks = append(hv.Networks, *net)
		}
	}

	return hv, nil
}
