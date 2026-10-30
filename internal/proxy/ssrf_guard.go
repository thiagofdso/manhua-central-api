package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidScheme   = errors.New("invalid scheme: only http and https are allowed")
	ErrForbiddenDomain = errors.New("forbidden domain: not on proxy allowlist")
	ErrSSRFBlocked     = errors.New("ssrf blocked: resolved address is private or reserved")
)

// DefaultAllowedDomains contains the official domain allowlist for manhua image extraction and proxies
var DefaultAllowedDomains = []string{
	"asurascans.com",
	"*.asurascans.com",
	"cdn.asurascans.com",
	"ravenscans.org",
	"*.ravenscans.org",
	"cdn*.ravenscans.org",
	"mangafire.to",
	"*.mangafire.to",
	"*.mfcdn.net",
	"*mfcdn*.*",
	"*.mfcdn*.xyz",
	"*.mfcdn.nl",
	"mangak.io",
	"*.mangak.io",
	"*.qvzr*.org",
	"*qvzr*.org",
	"resmk.org",
	"*.resmk.org",
	"*resmk*.org",
	"manhuaus.com",
	"*.manhuaus.com",
	"comix.to",
	"*.comix.to",
	"*.site",
	"arenascan.com",
	"*.arenascan.com",
	"arenascans.com",
	"*.arenascans.com",
	"cdn.arenascan.com",
	"manhuatop.org",
	"*.manhuatop.org",
	"*.wp.com",
	"mgeko.cc",
	"*.mgeko.cc",
	"www.mgeko.cc",
	"*imgsrv*.com",
	"*.imgsrv*.com",
	"levelingwithgods.com",
	"*.levelingwithgods.com",
	"pickmeupgacha.com",
	"*.pickmeupgacha.com",
	"abyssrift.com",
	"*.abyssrift.com",
	"magicemperor.online",
	"*.magicemperor.online",
	"black-clover.org",
	"*.black-clover.org",
	"googleusercontent.com",
	"*.googleusercontent.com",
}

type SSRFGuard struct {
	allowedDomains  []string
	privateIPBlocks []*net.IPNet
}

func NewSSRFGuard(allowedDomains []string) *SSRFGuard {
	if len(allowedDomains) == 0 {
		allowedDomains = DefaultAllowedDomains
	}

	cidrBlocks := []string{
		"0.0.0.0/8",          // Current network
		"10.0.0.0/8",          // RFC 1918 Private
		"100.64.0.0/10",       // Carrier-grade NAT (RFC 6598)
		"127.0.0.0/8",         // Loopback
		"169.254.0.0/16",      // Link-local & Cloud Metadata (169.254.169.254)
		"172.16.0.0/12",       // RFC 1918 Private
		"192.0.0.0/24",        // IETF Protocol Assignments
		"192.0.2.0/24",        // TEST-NET-1
		"192.168.0.0/16",      // RFC 1918 Private
		"198.18.0.0/15",       // Network benchmark tests
		"198.51.100.0/24",     // TEST-NET-2
		"203.0.113.0/24",      // TEST-NET-3
		"224.0.0.0/4",         // Multicast
		"240.0.0.0/4",         // Reserved for future use
		"255.255.255.255/32",  // Broadcast
		"::/128",              // IPv6 Unspecified
		"::1/128",             // IPv6 Loopback
		"fc00::/7",            // IPv6 Unique Local Address (ULA)
		"fe80::/10",           // IPv6 Link-Local
	}

	var parsedBlocks []*net.IPNet
	for _, cidr := range cidrBlocks {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			parsedBlocks = append(parsedBlocks, block)
		}
	}

	return &SSRFGuard{
		allowedDomains:  allowedDomains,
		privateIPBlocks: parsedBlocks,
	}
}

func (g *SSRFGuard) IsPrivateOrReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Standard Go net check
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}

	// Check against all explicit CIDR blocks
	for _, block := range g.privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

func (g *SSRFGuard) IsDomainAllowed(hostname string) bool {
	host := strings.ToLower(strings.TrimSpace(hostname))
	if host == "" {
		return false
	}

	// Strip port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Direct IP addresses are never allowed in the domain allowlist
	if parsed := net.ParseIP(host); parsed != nil {
		return false
	}

	for _, pattern := range g.allowedDomains {
		p := strings.ToLower(strings.TrimSpace(pattern))
		if p == host {
			return true
		}

		if strings.HasPrefix(p, "*.") {
			baseDomain := strings.TrimPrefix(p, "*.")
			if host == baseDomain || strings.HasSuffix(host, "."+baseDomain) {
				return true
			}
		}

		// Handle wildcards using filepath.Match (e.g. *.qvzr*.org, *imgsrv*.com)
		if strings.Contains(p, "*") {
			if matched, _ := filepath.Match(p, host); matched {
				return true
			}
			if strings.HasPrefix(p, "*.") {
				baseDomain := strings.TrimPrefix(p, "*.")
				if matched, _ := filepath.Match(baseDomain, host); matched {
					return true
				}
			}
		}
	}

	return false
}

func (g *SSRFGuard) ValidateURL(ctx context.Context, targetURL string) (*url.URL, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("malformed url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrInvalidScheme
	}

	hostname := u.Hostname()
	if !g.IsDomainAllowed(hostname) {
		return nil, ErrForbiddenDomain
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("dns resolution failed: %w", err)
	}

	if len(ips) == 0 {
		return nil, errors.New("no ip addresses resolved")
	}

	for _, ipAddr := range ips {
		if g.IsPrivateOrReservedIP(ipAddr.IP) {
			return nil, ErrSSRFBlocked
		}
	}

	return u, nil
}
