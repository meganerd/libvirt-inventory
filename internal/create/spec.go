package create

import (
	"crypto/rand"
	"math/big"
)

// VMSpec defines all parameters for creating a VM.
type VMSpec struct {
	Name          string
	Hypervisor    string // hypervisor name from config
	URI           string // resolved qemu+ssh URI
	VCPUs         int
	MemoryMiB     int
	DiskSizeGB    int
	BaseImageURL  string
	Pool          string
	Network       string
	InstallUser   string
	InstallPass   string // generated at runtime
	SSHKeys       []string
	Playbook      string // optional ansible playbook path
}

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"

// GeneratePassword creates a cryptographically random password.
func GeneratePassword(length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordChars))))
		if err != nil {
			return "", err
		}
		result[i] = passwordChars[idx.Int64()]
	}
	return string(result), nil
}

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (s *VMSpec) ApplyDefaults() {
	if s.VCPUs == 0 {
		s.VCPUs = 2
	}
	if s.MemoryMiB == 0 {
		s.MemoryMiB = 2048
	}
	if s.DiskSizeGB == 0 {
		s.DiskSizeGB = 20
	}
	if s.BaseImageURL == "" {
		s.BaseImageURL = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
	}
	if s.Pool == "" {
		s.Pool = "default"
	}
	if s.Network == "" {
		s.Network = "default"
	}
	if s.InstallUser == "" {
		s.InstallUser = "install"
	}
}
