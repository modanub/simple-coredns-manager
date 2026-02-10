package handlers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type DigData struct {
	Query   string
	Type    string
	Server  string
	Results []DigResult
	Error   string
}

type DigResult struct {
	Name  string
	Type  string
	Value string
	TTL   string
}

func (h *Handler) DigPage(c echo.Context) error {
	// Default DNS server is the CoreDNS container
	server := h.Config.CoreDNSContainerName + ":53"
	pd := h.page(c, "DNS Lookup", "dig", DigData{Server: server})
	return c.Render(http.StatusOK, "dig", pd)
}

func (h *Handler) DigQuery(c echo.Context) error {
	query := strings.TrimSpace(c.FormValue("query"))
	qtype := strings.TrimSpace(c.FormValue("type"))
	server := strings.TrimSpace(c.FormValue("server"))

	if query == "" {
		return c.HTML(http.StatusOK, `<div class="alert alert-warning">Enter a hostname to look up</div>`)
	}
	if qtype == "" {
		qtype = "A"
	}
	if server == "" {
		server = h.Config.CoreDNSContainerName + ":53"
	}
	if !strings.Contains(server, ":") {
		server = server + ":53"
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", server)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data := DigData{
		Query:  query,
		Type:   qtype,
		Server: server,
	}

	switch strings.ToUpper(qtype) {
	case "A", "AAAA":
		ips, err := resolver.LookupHost(ctx, query)
		if err != nil {
			data.Error = err.Error()
		} else {
			for _, ip := range ips {
				rtype := "A"
				if strings.Contains(ip, ":") {
					rtype = "AAAA"
				}
				if strings.ToUpper(qtype) != "A" && strings.ToUpper(qtype) != rtype {
					continue
				}
				data.Results = append(data.Results, DigResult{
					Name:  query,
					Type:  rtype,
					Value: ip,
				})
			}
			if len(data.Results) == 0 {
				data.Error = fmt.Sprintf("No %s records found", qtype)
			}
		}
	case "CNAME":
		cname, err := resolver.LookupCNAME(ctx, query)
		if err != nil {
			data.Error = err.Error()
		} else {
			data.Results = append(data.Results, DigResult{
				Name:  query,
				Type:  "CNAME",
				Value: cname,
			})
		}
	case "MX":
		mxs, err := resolver.LookupMX(ctx, query)
		if err != nil {
			data.Error = err.Error()
		} else {
			for _, mx := range mxs {
				data.Results = append(data.Results, DigResult{
					Name:  query,
					Type:  "MX",
					Value: fmt.Sprintf("%d %s", mx.Pref, mx.Host),
				})
			}
		}
	case "TXT":
		txts, err := resolver.LookupTXT(ctx, query)
		if err != nil {
			data.Error = err.Error()
		} else {
			for _, txt := range txts {
				data.Results = append(data.Results, DigResult{
					Name:  query,
					Type:  "TXT",
					Value: txt,
				})
			}
		}
	case "NS":
		nss, err := resolver.LookupNS(ctx, query)
		if err != nil {
			data.Error = err.Error()
		} else {
			for _, ns := range nss {
				data.Results = append(data.Results, DigResult{
					Name:  query,
					Type:  "NS",
					Value: ns.Host,
				})
			}
		}
	default:
		data.Error = "Unsupported record type: " + qtype
	}

	return c.Render(http.StatusOK, "dig_result", data)
}
