package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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

func (m *HostsManager) List() ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read hosts directory: %w", err)
	}

	var domains []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		domains = append(domains, e.Name())
	}
	sort.Strings(domains)
	return domains, nil
}

func (m *HostsManager) Read(domain string) (*HostFile, error) {
	if err := ValidateDomain(domain); err != nil {
		return nil, err
	}

	path := filepath.Join(m.dir, domain)
	data, err := os.ReadFile(path)
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

func (m *HostsManager) Write(domain, content string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	path := filepath.Join(m.dir, domain)

	// Atomic write
	tmp, err := os.CreateTemp(m.dir, ".hosts-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

func (m *HostsManager) Delete(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	path := filepath.Join(m.dir, domain)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("host file does not exist: %s", domain)
	}
	return os.Remove(path)
}

func (m *HostsManager) Exists(domain string) bool {
	if err := ValidateDomain(domain); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(m.dir, domain))
	return err == nil
}

func (m *HostsManager) ReadRaw(domain string) (string, error) {
	if err := ValidateDomain(domain); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(m.dir, domain))
	if err != nil {
		return "", err
	}
	return string(data), nil
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
