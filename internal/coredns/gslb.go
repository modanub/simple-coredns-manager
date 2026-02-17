package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const gslbSuffix = ".yml"

// GSLBConfig represents a complete GSLB YAML configuration file.
type GSLBConfig struct {
	HealthcheckProfiles map[string]HealthcheckProfile `yaml:"healthcheck_profiles,omitempty"`
	Records             map[string]*GSLBRecord        `yaml:"records"`
}

// HealthcheckProfile defines a reusable health check template.
type HealthcheckProfile struct {
	Type   string                 `yaml:"type"`
	Params map[string]interface{} `yaml:"params,omitempty"`
}

// GSLBRecord defines a single GSLB-managed DNS record.
type GSLBRecord struct {
	Mode           string        `yaml:"mode"`
	RecordTTL      int           `yaml:"record_ttl"`
	ScrapeInterval string        `yaml:"scrape_interval"`
	Backends       []GSLBBackend `yaml:"backends"`
}

// GSLBBackend represents a backend server for GSLB routing.
type GSLBBackend struct {
	Address      string                 `yaml:"address"`
	Priority     int                    `yaml:"priority,omitempty"`
	Weight       int                    `yaml:"weight,omitempty"`
	Location     string                 `yaml:"location,omitempty"`
	Disabled     bool                   `yaml:"disabled,omitempty"`
	Healthchecks []interface{}          `yaml:"healthchecks,omitempty"`
	Meta         map[string]interface{} `yaml:"meta,omitempty"`
}

// GSLBManager manages GSLB YAML configuration files.
type GSLBManager struct {
	dir string
}

// GSLBEntry is a summary entry for the list view.
type GSLBEntry struct {
	Domain       string
	RecordCount  int
	BackendCount int
}

func NewGSLBManager(dir string) *GSLBManager {
	return &GSLBManager{dir: dir}
}

// filename returns the path for a GSLB config: db.<domain>.yml
func (m *GSLBManager) filename(domain string) string {
	return filepath.Join(m.dir, zonePrefix+domain+gslbSuffix)
}

// List returns domains that have GSLB YAML configs (db.<domain>.yml files).
func (m *GSLBManager) List() ([]GSLBEntry, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var result []GSLBEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, zonePrefix) || !strings.HasSuffix(name, gslbSuffix) {
			continue
		}
		// Extract domain: db.example.com.yml -> example.com
		domain := strings.TrimPrefix(name, zonePrefix)
		domain = strings.TrimSuffix(domain, gslbSuffix)
		if domain == "" {
			continue
		}

		cfg, err := m.Read(domain)
		entry := GSLBEntry{Domain: domain}
		if err == nil && cfg != nil {
			entry.RecordCount = len(cfg.Records)
			for _, rec := range cfg.Records {
				entry.BackendCount += len(rec.Backends)
			}
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Domain < result[j].Domain
	})
	return result, nil
}

// Read parses a GSLB YAML config file.
func (m *GSLBManager) Read(domain string) (*GSLBConfig, error) {
	if err := ValidateDomain(domain); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(m.filename(domain))
	if err != nil {
		return nil, fmt.Errorf("failed to read GSLB config: %w", err)
	}

	var cfg GSLBConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse GSLB config: %w", err)
	}

	if cfg.Records == nil {
		cfg.Records = make(map[string]*GSLBRecord)
	}

	return &cfg, nil
}

// ReadRaw returns the raw YAML content of a GSLB config file.
func (m *GSLBManager) ReadRaw(domain string) (string, error) {
	if err := ValidateDomain(domain); err != nil {
		return "", err
	}
	data, err := os.ReadFile(m.filename(domain))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Write saves a GSLB YAML config file atomically.
func (m *GSLBManager) Write(domain string, cfg *GSLBConfig) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal GSLB config: %w", err)
	}

	return atomicWrite(m.filename(domain), string(data))
}

// WriteRaw saves raw YAML content, validating it first.
func (m *GSLBManager) WriteRaw(domain, content string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	if err := m.ValidateRaw(content); err != nil {
		return err
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	return atomicWrite(m.filename(domain), content)
}

// Create generates a new GSLB config with a default record.
func (m *GSLBManager) Create(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	if m.Exists(domain) {
		return fmt.Errorf("GSLB config already exists: %s", domain)
	}

	fqdn := domain
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}

	cfg := &GSLBConfig{
		HealthcheckProfiles: map[string]HealthcheckProfile{
			"http_default": {
				Type: "http",
				Params: map[string]interface{}{
					"port":          80,
					"uri":           "/",
					"expected_code": 200,
					"timeout":       "5s",
				},
			},
		},
		Records: map[string]*GSLBRecord{
			"app." + fqdn: {
				Mode:           "failover",
				RecordTTL:      30,
				ScrapeInterval: "10s",
				Backends: []GSLBBackend{
					{
						Address:      "192.168.1.10",
						Priority:     1,
						Healthchecks: []interface{}{"http_default"},
					},
				},
			},
		},
	}

	return m.Write(domain, cfg)
}

// Delete removes a GSLB config file.
func (m *GSLBManager) Delete(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	path := m.filename(domain)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("GSLB config does not exist: %s", domain)
	}
	return os.Remove(path)
}

// Exists checks if a GSLB config file exists.
func (m *GSLBManager) Exists(domain string) bool {
	if err := ValidateDomain(domain); err != nil {
		return false
	}
	_, err := os.Stat(m.filename(domain))
	return err == nil
}

// ValidateRaw validates raw YAML content as a valid GSLB config.
func (m *GSLBManager) ValidateRaw(content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("GSLB config cannot be empty")
	}

	var cfg GSLBConfig
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	if len(cfg.Records) == 0 {
		return fmt.Errorf("GSLB config must contain at least one record")
	}

	validModes := map[string]bool{
		"failover":   true,
		"roundrobin": true,
		"random":     true,
		"weighted":   true,
		"geoip":      true,
	}

	for name, rec := range cfg.Records {
		if rec == nil {
			return fmt.Errorf("record %q is nil", name)
		}
		if !validModes[rec.Mode] {
			return fmt.Errorf("record %q has invalid mode %q (valid: failover, roundrobin, random, weighted, geoip)", name, rec.Mode)
		}
		if len(rec.Backends) == 0 {
			return fmt.Errorf("record %q must have at least one backend", name)
		}
		for i, b := range rec.Backends {
			if b.Address == "" {
				return fmt.Errorf("record %q backend %d has no address", name, i+1)
			}
		}
	}

	return nil
}

// AddBackend adds a backend to a specific GSLB record.
func (m *GSLBManager) AddBackend(domain, recordName string, backend GSLBBackend) error {
	cfg, err := m.Read(domain)
	if err != nil {
		return err
	}

	rec, ok := cfg.Records[recordName]
	if !ok {
		return fmt.Errorf("record %q not found", recordName)
	}

	rec.Backends = append(rec.Backends, backend)
	return m.Write(domain, cfg)
}

// RemoveBackend removes a backend by index from a GSLB record.
func (m *GSLBManager) RemoveBackend(domain, recordName string, index int) error {
	cfg, err := m.Read(domain)
	if err != nil {
		return err
	}

	rec, ok := cfg.Records[recordName]
	if !ok {
		return fmt.Errorf("record %q not found", recordName)
	}

	if index < 0 || index >= len(rec.Backends) {
		return fmt.Errorf("backend index %d out of range", index)
	}

	rec.Backends = append(rec.Backends[:index], rec.Backends[index+1:]...)
	return m.Write(domain, cfg)
}

// AddRecord adds a new GSLB record entry.
func (m *GSLBManager) AddRecord(domain, recordName, mode string, ttl int, scrapeInterval string) error {
	cfg, err := m.Read(domain)
	if err != nil {
		return err
	}

	if _, exists := cfg.Records[recordName]; exists {
		return fmt.Errorf("record %q already exists", recordName)
	}

	cfg.Records[recordName] = &GSLBRecord{
		Mode:           mode,
		RecordTTL:      ttl,
		ScrapeInterval: scrapeInterval,
		Backends:       []GSLBBackend{},
	}

	return m.Write(domain, cfg)
}

// RemoveRecord removes a GSLB record entry.
func (m *GSLBManager) RemoveRecord(domain, recordName string) error {
	cfg, err := m.Read(domain)
	if err != nil {
		return err
	}

	if _, exists := cfg.Records[recordName]; !exists {
		return fmt.Errorf("record %q not found", recordName)
	}

	delete(cfg.Records, recordName)
	return m.Write(domain, cfg)
}

// UpdateRecord updates settings for an existing GSLB record.
func (m *GSLBManager) UpdateRecord(domain, recordName, mode string, ttl int, scrapeInterval string) error {
	cfg, err := m.Read(domain)
	if err != nil {
		return err
	}

	rec, ok := cfg.Records[recordName]
	if !ok {
		return fmt.Errorf("record %q not found", recordName)
	}

	rec.Mode = mode
	rec.RecordTTL = ttl
	rec.ScrapeInterval = scrapeInterval

	return m.Write(domain, cfg)
}

// SortedRecordNames returns record names sorted alphabetically.
func SortedRecordNames(records map[string]*GSLBRecord) []string {
	names := make([]string, 0, len(records))
	for name := range records {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
