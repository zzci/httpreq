package httpdns

import (
	"crypto/rand"
	"math/big"
	"strings"
)

const nanoidAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const nanoidLength = 8

// GenerateSubdomain creates a random nanoid-style subdomain (lowercase alphanumeric).
func GenerateSubdomain() string {
	b := make([]byte, nanoidLength)
	max := big.NewInt(int64(len(nanoidAlphabet)))
	for i := range b {
		n, _ := rand.Int(rand.Reader, max)
		b[i] = nanoidAlphabet[n.Int64()]
	}
	return string(b)
}

// InternalDomain returns the full internal domain for a subdomain under the base domain.
// Example: subdomain "a7x9k2", baseDomain "dns.example.com" -> "a7x9k2.dns.example.com"
func InternalDomain(subdomain, baseDomain string) string {
	baseDomain = strings.TrimSuffix(strings.TrimSpace(baseDomain), ".")
	return subdomain + "." + baseDomain
}

// ExtractDomainFromFQDN strips _acme-challenge. prefix and trailing dot.
// Example: "_acme-challenge.example.com." -> "example.com"
func ExtractDomainFromFQDN(fqdn string) string {
	fqdn = strings.ToLower(strings.TrimSpace(fqdn))
	fqdn = strings.TrimSuffix(fqdn, ".")
	fqdn = strings.TrimPrefix(fqdn, "_acme-challenge.")
	return fqdn
}
