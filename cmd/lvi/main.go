package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/meganerd/libvirt-inventory/internal/config"
	"github.com/meganerd/libvirt-inventory/internal/create"
	"github.com/meganerd/libvirt-inventory/internal/drift"
	"github.com/meganerd/libvirt-inventory/internal/hcl"
	"github.com/meganerd/libvirt-inventory/internal/model"
	"github.com/meganerd/libvirt-inventory/internal/scanner"
	"github.com/spf13/cobra"
)

var cfgFile string

func main() {
	root := &cobra.Command{
		Use:   "lvi",
		Short: "Libvirt Inventory — enumerate, track, and detect drift on libvirt hypervisors",
	}

	root.PersistentFlags().StringVarP(&cfgFile, "config", "c", "config.yaml", "path to config file")

	root.AddCommand(scanCmd())
	root.AddCommand(driftCmd())
	root.AddCommand(generateCmd())
	root.AddCommand(createCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func loadConfig() *config.Config {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func scanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Enumerate all domains, volumes, and networks across hypervisors",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadConfig()

			snap := &model.Snapshot{
				Timestamp: time.Now().UTC(),
			}

			for _, hv := range cfg.Hypervisors {
				fmt.Printf("Scanning %s (%s)...\n", hv.Name, hv.URI)
				result, err := scanner.ScanHypervisor(hv.Name, hv.URI)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  [ERROR] %s: %v\n", hv.Name, err)
					continue
				}
				fmt.Printf("  Found: %d domains, %d volumes, %d networks\n",
					len(result.Domains), len(result.Volumes), len(result.Networks))
				snap.Hypervisors = append(snap.Hypervisors, *result)
			}

			// Write snapshot
			outDir := cfg.OutputDir
			if err := os.MkdirAll(outDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
				os.Exit(1)
			}

			// Write latest snapshot
			latestPath := filepath.Join(outDir, "snapshot-latest.json")
			data, err := json.MarshalIndent(snap, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error marshaling snapshot: %v\n", err)
				os.Exit(1)
			}

			if err := os.WriteFile(latestPath, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing snapshot: %v\n", err)
				os.Exit(1)
			}

			// Also write timestamped copy for history
			tsPath := filepath.Join(outDir, fmt.Sprintf("snapshot-%s.json",
				snap.Timestamp.Format("20060102-150405")))
			if err := os.WriteFile(tsPath, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing timestamped snapshot: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("\nSnapshot written to:\n  %s\n  %s\n", latestPath, tsPath)
		},
	}
}

func driftCmd() *cobra.Command {
	var prevFile string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Compare current state against a previous snapshot (warn-only, never deletes)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadConfig()

			// Load previous snapshot
			if prevFile == "" {
				prevFile = filepath.Join(cfg.OutputDir, "snapshot-latest.json")
			}
			prevData, err := os.ReadFile(prevFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading previous snapshot %s: %v\n", prevFile, err)
				fmt.Fprintln(os.Stderr, "Run 'lvi scan' first to create an initial snapshot.")
				os.Exit(1)
			}

			var previous model.Snapshot
			if err := json.Unmarshal(prevData, &previous); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing previous snapshot: %v\n", err)
				os.Exit(1)
			}

			// Scan current state
			current := &model.Snapshot{
				Timestamp: time.Now().UTC(),
			}
			for _, hv := range cfg.Hypervisors {
				fmt.Printf("Scanning %s...\n", hv.Name)
				result, err := scanner.ScanHypervisor(hv.Name, hv.URI)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  [ERROR] %s: %v\n", hv.Name, err)
					continue
				}
				current.Hypervisors = append(current.Hypervisors, *result)
			}

			// Compare
			report := drift.Compare(&previous, current)
			fmt.Println()
			fmt.Print(report.String())

			if report.HasDrift {
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVar(&prevFile, "previous", "", "path to previous snapshot (default: snapshot-latest.json)")
	return cmd
}

func generateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate reference HCL + import blocks from the latest snapshot",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadConfig()

			// Load latest snapshot
			snapPath := filepath.Join(cfg.OutputDir, "snapshot-latest.json")
			data, err := os.ReadFile(snapPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading snapshot %s: %v\n", snapPath, err)
				fmt.Fprintln(os.Stderr, "Run 'lvi scan' first.")
				os.Exit(1)
			}

			var snap model.Snapshot
			if err := json.Unmarshal(data, &snap); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing snapshot: %v\n", err)
				os.Exit(1)
			}

			for _, hv := range snap.Hypervisors {
				hvDir := filepath.Join(cfg.OutputDir, "hcl", hv.Name)
				if err := os.MkdirAll(hvDir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "Error creating dir %s: %v\n", hvDir, err)
					continue
				}

				// Provider
				providerHCL := hcl.GenerateProviderHCL(hv.Name, hv.URI)
				writeHCL(filepath.Join(hvDir, "provider.tf"), providerHCL)

				// Domains
				var domHCL string
				for _, d := range hv.Domains {
					domHCL += hcl.GenerateDomainHCL(&d, hv.Name) + "\n"
				}
				if domHCL != "" {
					writeHCL(filepath.Join(hvDir, "domains.tf"), domHCL)
				}

				// Volumes
				var volHCL string
				for _, v := range hv.Volumes {
					volHCL += hcl.GenerateVolumeHCL(&v, hv.Name) + "\n"
				}
				if volHCL != "" {
					writeHCL(filepath.Join(hvDir, "volumes.tf"), volHCL)
				}

				// Networks
				var netHCL string
				for _, n := range hv.Networks {
					netHCL += hcl.GenerateNetworkHCL(&n, hv.Name) + "\n"
				}
				if netHCL != "" {
					writeHCL(filepath.Join(hvDir, "networks.tf"), netHCL)
				}

				fmt.Printf("Generated HCL for %s in %s/\n", hv.Name, hvDir)
			}
		},
	}
}

func createCmd() *cobra.Command {
	var (
		hvName    string
		vcpu      int
		memory    int
		diskSize  int
		imageURL  string
		pool      string
		network   string
		user      string
		sshKeys   []string
		playbook  string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new VM on a hypervisor (never deletes — only creates)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadConfig()

			if hvName == "" {
				fmt.Fprintln(os.Stderr, "Error: --hypervisor is required")
				os.Exit(1)
			}

			// Resolve VM name from args
			if len(args) == 0 {
				fmt.Fprintln(os.Stderr, "Error: VM name is required as argument")
				fmt.Fprintln(os.Stderr, "Usage: lvi create <vm-name> --hypervisor <name>")
				os.Exit(1)
			}
			vmName := args[0]

			hv, err := cfg.FindHypervisor(hvName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			// Build spec from flags + config defaults
			spec := &create.VMSpec{
				Name:         vmName,
				Hypervisor:   hv.Name,
				URI:          hv.URI,
				VCPUs:        firstNonZero(vcpu, cfg.Defaults.VCPUs),
				MemoryMiB:    firstNonZero(memory, cfg.Defaults.MemoryMiB),
				DiskSizeGB:   firstNonZero(diskSize, cfg.Defaults.DiskSizeGB),
				BaseImageURL: firstNonEmpty(imageURL, cfg.Defaults.BaseImageURL),
				Pool:         firstNonEmpty(pool, cfg.Defaults.Pool),
				Network:      firstNonEmpty(network, cfg.Defaults.Network),
				InstallUser:  firstNonEmpty(user, cfg.Defaults.InstallUser),
				SSHKeys:      sshKeys,
				Playbook:     playbook,
			}

			fmt.Printf("Creating VM %q on %s...\n\n", vmName, hv.Name)

			result, err := create.CreateVM(spec)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("\nVM created successfully!\n")
			fmt.Printf("  Name:     %s\n", result.Name)
			fmt.Printf("  IP:       %s\n", result.IP)
			fmt.Printf("  User:     %s\n", result.User)
			fmt.Printf("  Password: %s\n", result.Password)
			fmt.Printf("  SSH:      ssh %s@%s\n", result.User, result.IP)

			// Update inventory
			fmt.Printf("\nUpdating inventory snapshot...\n")
			hvResult, err := scanner.ScanHypervisor(hv.Name, hv.URI)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[WARN] Failed to update inventory: %v\n", err)
				return
			}

			// Load existing snapshot or create new
			snapPath := filepath.Join(cfg.OutputDir, "snapshot-latest.json")
			var snap model.Snapshot
			if data, err := os.ReadFile(snapPath); err == nil {
				json.Unmarshal(data, &snap)
			}
			snap.Timestamp = time.Now().UTC()

			// Replace or add hypervisor data
			found := false
			for i, h := range snap.Hypervisors {
				if h.Name == hv.Name {
					snap.Hypervisors[i] = *hvResult
					found = true
					break
				}
			}
			if !found {
				snap.Hypervisors = append(snap.Hypervisors, *hvResult)
			}

			if err := os.MkdirAll(cfg.OutputDir, 0755); err == nil {
				if data, err := json.MarshalIndent(snap, "", "  "); err == nil {
					os.WriteFile(snapPath, data, 0644)
					fmt.Printf("Inventory updated: %s\n", snapPath)
				}
			}
		},
	}

	cmd.Flags().StringVar(&hvName, "hypervisor", "", "target hypervisor name (required)")
	cmd.Flags().IntVar(&vcpu, "vcpu", 0, "number of vCPUs (default: from config or 2)")
	cmd.Flags().IntVar(&memory, "memory", 0, "memory in MiB (default: from config or 2048)")
	cmd.Flags().IntVar(&diskSize, "disk-size", 0, "disk size in GB (default: from config or 20)")
	cmd.Flags().StringVar(&imageURL, "image", "", "base cloud image URL")
	cmd.Flags().StringVar(&pool, "pool", "", "storage pool name (default: from config or 'default')")
	cmd.Flags().StringVar(&network, "network", "", "network name (default: from config or 'default')")
	cmd.Flags().StringVar(&user, "user", "", "install user name (default: from config or 'install')")
	cmd.Flags().StringArrayVar(&sshKeys, "ssh-key", nil, "SSH public key to inject (repeatable)")
	cmd.Flags().StringVar(&playbook, "playbook", "", "Ansible playbook to run after VM creation")

	return cmd
}

func firstNonZero(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func writeHCL(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
	}
}
