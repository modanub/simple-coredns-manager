package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const hostsPrefix = "hosts."

var validDomainRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$`)

type HostEntry struct {
	IP       string
	Hostname string
}

type HostFile struct {
	Domain  string
	Entries []HostEntry
	Raw     string
}

type HostsManager struct {
	dir string
}

func NewHostsManager(dir string) *HostsManager {
	return &HostsManager{dir: dir}
}

// ValidateDomain validates the domain part (without hosts. prefix).
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if strings.ContainsAny(domain, "/\\") {
		return fmt.Errorf("domain contains invalid path characters")
	}
	if strings.Contains(domain, "..") {
		return fmt.Errorf("domain contains path traversal sequence")
	}
	if !validDomainRe.MatchString(domain) {
		return fmt.Errorf("domain contains invalid characters (allowed: a-z, 0-9, ., -)")
	}
	return nil
}

func (m *HostsManager) filename(domain string) string {
	return filepath.Join(m.dir, hostsPrefix+domain)
}

// List returns domain names (without hosts. prefix) of all host files.
func (m *HostsManager) List() ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var domains []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), hostsPrefix) {
			continue
		}
		domain := strings.TrimPrefix(e.Name(), hostsPrefix)
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	sort.Strings(domains)
	return domains, nil
}

func (m *HostsManager) Read(domain string) (*HostFile, error) {
	if err := ValidateDomain(domain); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(m.filename(domain))
	if err != nil {
		return nil, fmt.Errorf("failed to read host file: %w", err)
	}

	raw := string(data)
	entries := ParseHostEntries(raw)

	return &HostFile{
		Domain:  domain,
		Entries: entries,
		Raw:     raw,
	}, nil
}

func (m *HostsManager) ReadRaw(domain string) (string, error) {
	if err := ValidateDomain(domain); err != nil {
		return "", err
	}
	data, err := os.ReadFile(m.filename(domain))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *HostsManager) Write(domain, content string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	return atomicWrite(m.filename(domain), content)
}

func (m *HostsManager) Delete(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	path := m.filename(domain)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("host file does not exist: %s", domain)
	}
	return os.Remove(path)
}

func (m *HostsManager) Exists(domain string) bool {
	if err := ValidateDomain(domain); err != nil {
		return false
	}
	_, err := os.Stat(m.filename(domain))
	return err == nil
}

// AddEntry appends an IP+hostname line to the host file.
func (m *HostsManager) AddEntry(domain, ip, hostname string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	path := m.filename(domain)
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	content := string(raw)
	line := fmt.Sprintf("%s\t%s", ip, hostname)

	if content == "" {
		content = line + "\n"
	} else {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += line + "\n"
	}

	return atomicWrite(path, content)
}

// RemoveEntry removes the first line matching ip+hostname from the host file.
func (m *HostsManager) RemoveEntry(domain, ip, hostname string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	path := m.filename(domain)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(raw), "\n")
	var result []string
	removed := false
	for _, line := range lines {
		if !removed {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "#") && trimmed != "" {
				fields := strings.Fields(trimmed)
				if len(fields) >= 2 && fields[0] == ip && fields[1] == hostname {
					removed = true
					continue
				}
			}
		}
		result = append(result, line)
	}

	content := strings.Join(result, "\n")
	return atomicWrite(path, content)
}

func atomicWrite(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hosts-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Preserve permissions if file exists
	if info, err := os.Stat(path); err == nil {
		os.Chmod(tmpPath, info.Mode())
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

func ParseHostEntries(content string) []HostEntry {
	var entries []HostEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			entries = append(entries, HostEntry{
				IP:       fields[0],
				Hostname: fields[1],
			})
		}
	}
	return entries
}
