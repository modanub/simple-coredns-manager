package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const zonePrefix = "db."

var validDomainRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$`)

type RecordType string

const (
	TypeA     RecordType = "A"
	TypeAAAA  RecordType = "AAAA"
	TypeCNAME RecordType = "CNAME"
	TypeMX    RecordType = "MX"
	TypeTXT   RecordType = "TXT"
	TypeNS    RecordType = "NS"
)

type Record struct {
	Name     string     // relative to zone (e.g., "app", "@")
	Type     RecordType // A, AAAA, CNAME, MX, TXT, NS
	TTL      uint32
	Value    string
	Priority uint16 // MX only
}

type SOAData struct {
	MName   string
	RName   string
	Serial  uint32
	Refresh uint32
	Retry   uint32
	Expire  uint32
	MinTTL  uint32
}

type ZoneFile struct {
	Domain  string
	Records []Record
	SOA     *SOAData
	Raw     string
}

type ZoneManager struct {
	dir string
}

func NewZoneManager(dir string) *ZoneManager {
	return &ZoneManager{dir: dir}
}

// ValidateDomain validates the domain part (without db. prefix).
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

func (m *ZoneManager) filename(domain string) string {
	return filepath.Join(m.dir, zonePrefix+domain)
}

// List returns domain names (without db. prefix) of all zone files.
func (m *ZoneManager) List() ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var domains []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), zonePrefix) {
			continue
		}
		domain := strings.TrimPrefix(e.Name(), zonePrefix)
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	sort.Strings(domains)
	return domains, nil
}

// Read parses a zone file and returns structured data.
func (m *ZoneManager) Read(domain string) (*ZoneFile, error) {
	if err := ValidateDomain(domain); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(m.filename(domain))
	if err != nil {
		return nil, fmt.Errorf("failed to read zone file: %w", err)
	}

	raw := string(data)
	origin := dns.Fqdn(domain)
	records, soa := parseZoneFile(raw, origin)

	return &ZoneFile{
		Domain:  domain,
		Records: records,
		SOA:     soa,
		Raw:     raw,
	}, nil
}

// ReadRaw returns the raw content of a zone file.
func (m *ZoneManager) ReadRaw(domain string) (string, error) {
	if err := ValidateDomain(domain); err != nil {
		return "", err
	}
	data, err := os.ReadFile(m.filename(domain))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Write saves zone file content, auto-incrementing the SOA serial.
func (m *ZoneManager) Write(domain, content string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	content = incrementSOASerial(content)

	return atomicWrite(m.filename(domain), content)
}

// Create generates a new zone file with default SOA and NS records.
func (m *ZoneManager) Create(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	if m.Exists(domain) {
		return fmt.Errorf("zone file already exists: %s", domain)
	}

	serial := time.Now().Format("20060102") + "01"
	origin := dns.Fqdn(domain)

	content := fmt.Sprintf(`$ORIGIN %s
$TTL 3600

@ IN SOA ns1.%s admin.%s (
    %s ; serial
    3600       ; refresh
    900        ; retry
    604800     ; expire
    300        ; minimum TTL
)

@ IN NS ns1.%s
`, origin, origin, origin, serial, origin)

	return atomicWrite(m.filename(domain), content)
}

// Delete removes a zone file.
func (m *ZoneManager) Delete(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	path := m.filename(domain)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("zone file does not exist: %s", domain)
	}
	return os.Remove(path)
}

// Exists checks if a zone file exists.
func (m *ZoneManager) Exists(domain string) bool {
	if err := ValidateDomain(domain); err != nil {
		return false
	}
	_, err := os.Stat(m.filename(domain))
	return err == nil
}

// AddRecord appends a DNS record line to the zone file.
func (m *ZoneManager) AddRecord(domain string, rec Record) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	path := m.filename(domain)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(raw)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	line := formatRecord(rec)
	content += line + "\n"
	content = incrementSOASerial(content)

	return atomicWrite(path, content)
}

// RemoveRecord removes the first matching record line from the zone file.
func (m *ZoneManager) RemoveRecord(domain string, name string, rtype RecordType, value string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	path := m.filename(domain)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	origin := dns.Fqdn(domain)
	lines := strings.Split(string(raw), "\n")
	var result []string
	removed := false

	for _, line := range lines {
		if !removed && matchesRecord(line, name, rtype, value, origin) {
			removed = true
			continue
		}
		result = append(result, line)
	}

	if !removed {
		return fmt.Errorf("record not found")
	}

	content := strings.Join(result, "\n")
	content = incrementSOASerial(content)
	return atomicWrite(path, content)
}

// Validate checks that the content is a valid zone file with an SOA record.
func (m *ZoneManager) Validate(domain, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("zone file content cannot be empty")
	}

	origin := dns.Fqdn(domain)
	parser := dns.NewZoneParser(strings.NewReader(content), origin, "")

	hasSOA := false
	for rr, ok := parser.Next(); ok; rr, ok = parser.Next() {
		if _, isSOA := rr.(*dns.SOA); isSOA {
			hasSOA = true
		}
	}

	if err := parser.Err(); err != nil {
		return fmt.Errorf("zone parse error: %w", err)
	}

	if !hasSOA {
		return fmt.Errorf("zone file must contain an SOA record")
	}

	return nil
}

// parseZoneFile parses a zone file and returns records and SOA data.
func parseZoneFile(content, origin string) ([]Record, *SOAData) {
	parser := dns.NewZoneParser(strings.NewReader(content), origin, "")

	var records []Record
	var soa *SOAData

	for rr, ok := parser.Next(); ok; rr, ok = parser.Next() {
		name := relativeName(rr.Header().Name, origin)
		ttl := rr.Header().Ttl

		switch v := rr.(type) {
		case *dns.SOA:
			soa = &SOAData{
				MName:   v.Ns,
				RName:   v.Mbox,
				Serial:  v.Serial,
				Refresh: v.Refresh,
				Retry:   v.Retry,
				Expire:  v.Expire,
				MinTTL:  v.Minttl,
			}
		case *dns.NS:
			// Skip apex NS records (required, not user-editable)
			if name == "@" {
				continue
			}
			records = append(records, Record{
				Name:  name,
				Type:  TypeNS,
				TTL:   ttl,
				Value: v.Ns,
			})
		case *dns.A:
			records = append(records, Record{
				Name:  name,
				Type:  TypeA,
				TTL:   ttl,
				Value: v.A.String(),
			})
		case *dns.AAAA:
			records = append(records, Record{
				Name:  name,
				Type:  TypeAAAA,
				TTL:   ttl,
				Value: v.AAAA.String(),
			})
		case *dns.CNAME:
			records = append(records, Record{
				Name:  name,
				Type:  TypeCNAME,
				TTL:   ttl,
				Value: v.Target,
			})
		case *dns.MX:
			records = append(records, Record{
				Name:     name,
				Type:     TypeMX,
				TTL:      ttl,
				Value:    v.Mx,
				Priority: v.Preference,
			})
		case *dns.TXT:
			records = append(records, Record{
				Name:  name,
				Type:  TypeTXT,
				TTL:   ttl,
				Value: strings.Join(v.Txt, " "),
			})
		}
	}

	return records, soa
}

// relativeName converts an FQDN to a name relative to the origin.
// e.g., "app.example.com." with origin "example.com." returns "app"
// "example.com." with origin "example.com." returns "@"
func relativeName(fqdn, origin string) string {
	fqdn = dns.Fqdn(fqdn)
	origin = dns.Fqdn(origin)

	if fqdn == origin {
		return "@"
	}

	if strings.HasSuffix(fqdn, "."+origin) {
		rel := strings.TrimSuffix(fqdn, "."+origin)
		return rel
	}

	return fqdn
}

// formatRecord formats a Record as a zone file line.
func formatRecord(rec Record) string {
	ttlStr := ""
	if rec.TTL > 0 {
		ttlStr = fmt.Sprintf("%d ", rec.TTL)
	}

	switch rec.Type {
	case TypeMX:
		return fmt.Sprintf("%s %sIN MX %d %s", rec.Name, ttlStr, rec.Priority, rec.Value)
	case TypeTXT:
		// Ensure TXT values are quoted
		val := rec.Value
		if !strings.HasPrefix(val, `"`) {
			val = `"` + val + `"`
		}
		return fmt.Sprintf("%s %sIN TXT %s", rec.Name, ttlStr, val)
	default:
		return fmt.Sprintf("%s %sIN %s %s", rec.Name, ttlStr, rec.Type, rec.Value)
	}
}

// matchesRecord checks if a zone file line matches the given record parameters.
func matchesRecord(line, name string, rtype RecordType, value, origin string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "$") {
		return false
	}

	// Parse this single line as a zone record
	parser := dns.NewZoneParser(strings.NewReader(trimmed+"\n"), origin, "")
	rr, ok := parser.Next()
	if !ok {
		return false
	}

	recName := relativeName(rr.Header().Name, origin)
	if recName != name {
		return false
	}

	switch v := rr.(type) {
	case *dns.A:
		return rtype == TypeA && v.A.String() == value
	case *dns.AAAA:
		return rtype == TypeAAAA && v.AAAA.String() == value
	case *dns.CNAME:
		return rtype == TypeCNAME && (v.Target == value || v.Target == dns.Fqdn(value))
	case *dns.MX:
		return rtype == TypeMX && (v.Mx == value || v.Mx == dns.Fqdn(value))
	case *dns.TXT:
		return rtype == TypeTXT && strings.Join(v.Txt, " ") == value
	case *dns.NS:
		return rtype == TypeNS && (v.Ns == value || v.Ns == dns.Fqdn(value))
	}

	return false
}

// incrementSOASerial finds the SOA serial in the content and increments it.
// Uses YYYYMMDDNN format. If today's date matches, increments NN. Otherwise resets to today+01.
func incrementSOASerial(content string) string {
	// Match serial line in SOA record: digits followed by optional whitespace and ; serial comment
	re := regexp.MustCompile(`(\s+)(\d{10})(\s*;\s*serial)`)
	match := re.FindStringSubmatch(content)
	if match == nil {
		// Try without comment
		re = regexp.MustCompile(`(\s+)(\d{10})(\s)`)
		match = re.FindStringSubmatch(content)
		if match == nil {
			return content
		}
	}

	oldSerial := match[2]
	today := time.Now().Format("20060102")

	var newSerial string
	if strings.HasPrefix(oldSerial, today) {
		// Same day, increment the sequence number
		nn, _ := strconv.Atoi(oldSerial[8:])
		nn++
		newSerial = fmt.Sprintf("%s%02d", today, nn)
	} else {
		newSerial = today + "01"
	}

	return strings.Replace(content, match[0], match[1]+newSerial+match[3], 1)
}

func atomicWrite(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".zone-*.tmp")
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
