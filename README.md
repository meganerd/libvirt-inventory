# lvi — Libvirt Inventory

Go CLI for enumerating libvirt/KVM hypervisors, tracking state over time, and detecting drift.

**lvi never deletes resources.** It creates, observes, records, and warns — but never destroys.

## Features

- **create** — Provision new VMs on remote hypervisors (cloud-init + optional Ansible handoff)
- **scan** — Enumerate all domains (VMs), storage volumes, and networks across multiple hypervisors via SSH
- **drift** — Compare current state against a previous snapshot; warn on added, removed, or changed resources
- **generate** — Produce reference HCL + import blocks for future OpenTofu adoption

## Install

```bash
go install github.com/meganerd/libvirt-inventory/cmd/lvi@latest
```

Or build from source:

```bash
git clone https://github.com/meganerd/libvirt-inventory.git
cd libvirt-inventory
make install
```

## Configuration

```bash
cp config.example.yaml config.yaml
# Edit with your hypervisor URIs
```

```yaml
hypervisors:
  - name: vmh01
    uri: "qemu+ssh://root@vmh01/system"
  - name: vmh02
    uri: "qemu+ssh://root@vmh02/system"

output_dir: "./inventory"

# Default VM creation settings
defaults:
  vcpus: 2
  memory_mib: 2048
  disk_size_gb: 20
  base_image_url: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  install_user: "install"
```

## Usage

```bash
# Create a new VM
lvi create mi-newvm01 --hypervisor vmh01 -c config.yaml

# Create with custom specs + Ansible playbook
lvi create mi-webserver01 --hypervisor vmh02 \
  --vcpu 4 --memory 4096 --disk-size 40 \
  --ssh-key "ssh-ed25519 AAAA..." \
  --playbook ~/src/_meganerd_roles/vm-post-provision.yaml \
  -c config.yaml

# Take inventory snapshot
lvi scan -c config.yaml

# Check for drift
lvi drift -c config.yaml

# Generate reference HCL for tofu adoption
lvi generate -c config.yaml
```

### create

Provisions a new VM on a remote hypervisor. The pipeline:
1. Checks prerequisites (genisoimage, virsh, qemu-img on hypervisor)
2. Downloads/caches base cloud image in the storage pool
3. Creates qcow2 overlay disk (copy-on-write from base)
4. Generates cloud-init ISO with install user, random password, SSH keys
5. Defines and starts the domain via virsh
6. Waits for DHCP IP and SSH readiness
7. Optionally runs an Ansible playbook for post-provision config
8. Updates the inventory snapshot

On completion, prints the VM IP, user, password, and SSH command.

### scan

Connects to each hypervisor via SSH, runs `virsh` commands to enumerate domains, volumes (across all pools), and networks. Saves a JSON snapshot to `output_dir/snapshot-latest.json` plus a timestamped copy.

### drift

Compares current hypervisor state against the latest (or specified) snapshot. Reports:
- `[+]` New resources found on hypervisor
- `[!]` Resources in previous snapshot no longer found (warning only — never deletes)
- `[~]` Resource attributes changed (vCPU, memory, state, disk count, etc.)

Exit code: 0 = no drift, 1 = drift detected (useful for CI/cron).

### generate

Reads the latest snapshot and produces per-hypervisor HCL files:
- `provider.tf` — Provider block with alias
- `domains.tf` — Reference `libvirt_domain` resources (note: provider v2 does not support import)
- `volumes.tf` — `libvirt_volume` resources + `import {}` blocks
- `networks.tf` — `libvirt_network` resources + `import {}` blocks

All generated resources include `lifecycle { prevent_destroy = true }`.

## Prerequisites

- SSH key-based access to hypervisors
- On hypervisors: `virsh`, `qemu-img`, `genisoimage` (for create), `wget` (for base image download)
- On control machine: `sshpass` (for create — initial password-based SSH), `ansible-playbook` (optional)
- Go 1.21+ for building

## License

MIT
