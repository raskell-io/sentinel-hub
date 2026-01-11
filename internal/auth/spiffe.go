package auth

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// SPIFFEIdentity represents an authenticated SPIFFE identity.
type SPIFFEIdentity struct {
	// SPIFFEID is the full SPIFFE ID (e.g., spiffe://trust-domain/workload).
	SPIFFEID string

	// TrustDomain is the trust domain from the SPIFFE ID.
	TrustDomain string

	// WorkloadPath is the workload path from the SPIFFE ID.
	WorkloadPath string

	// SerialNumber is the certificate serial number.
	SerialNumber string

	// ExpiresAt is the certificate expiry time.
	ExpiresAt time.Time

	// Certificate is the original peer certificate.
	Certificate *x509.Certificate
}

// SPIFFEAuthenticator authenticates gRPC clients using SPIFFE SVIDs.
type SPIFFEAuthenticator struct {
	// Allowlist configuration
	allowedTrustDomains map[string]bool
	allowedSPIFFEIDs    map[string]bool
	allowedPatterns     []*regexp.Regexp

	// Mutex for thread-safe access
	mu sync.RWMutex

	// Whether the authenticator is enabled
	enabled bool
}

// SPIFFEAuthConfig holds configuration for the SPIFFE authenticator.
type SPIFFEAuthConfig struct {
	Enabled             bool
	AllowedTrustDomains []string
	AllowedSPIFFEIDs    []string
	AllowedPatterns     []string
}

// NewSPIFFEAuthenticator creates a new SPIFFE authenticator.
func NewSPIFFEAuthenticator(config SPIFFEAuthConfig) (*SPIFFEAuthenticator, error) {
	auth := &SPIFFEAuthenticator{
		allowedTrustDomains: make(map[string]bool),
		allowedSPIFFEIDs:    make(map[string]bool),
		enabled:             config.Enabled,
	}

	// Build allowlists
	for _, td := range config.AllowedTrustDomains {
		auth.allowedTrustDomains[td] = true
	}
	for _, id := range config.AllowedSPIFFEIDs {
		auth.allowedSPIFFEIDs[id] = true
	}

	// Compile regex patterns
	for _, pattern := range config.AllowedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid SPIFFE ID pattern %q: %w", pattern, err)
		}
		auth.allowedPatterns = append(auth.allowedPatterns, re)
	}

	log.Info().
		Bool("enabled", config.Enabled).
		Int("trust_domains", len(config.AllowedTrustDomains)).
		Int("spiffe_ids", len(config.AllowedSPIFFEIDs)).
		Int("patterns", len(config.AllowedPatterns)).
		Msg("SPIFFE authenticator initialized")

	return auth, nil
}

// AuthenticateFromContext authenticates a gRPC request using peer certificate.
func (a *SPIFFEAuthenticator) AuthenticateFromContext(ctx context.Context) (*SPIFFEIdentity, error) {
	if !a.enabled {
		return nil, status.Error(codes.Unauthenticated, "SPIFFE authentication not enabled")
	}

	// Get peer information
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no peer information in context")
	}

	// Get TLS info
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no TLS info in peer")
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		return nil, status.Error(codes.Unauthenticated, "no client certificate")
	}

	cert := tlsInfo.State.PeerCertificates[0]
	return a.AuthenticateFromCert(cert)
}

// AuthenticateFromCert authenticates using an X.509 certificate.
func (a *SPIFFEAuthenticator) AuthenticateFromCert(cert *x509.Certificate) (*SPIFFEIdentity, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Extract SPIFFE ID from SAN URI
	spiffeID, err := extractSPIFFEID(cert)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "failed to extract SPIFFE ID: %v", err)
	}

	// Parse SPIFFE ID
	trustDomain, workloadPath, err := parseSPIFFEID(spiffeID)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid SPIFFE ID %q: %v", spiffeID, err)
	}

	// Check allowlist
	if !a.isAllowed(spiffeID, trustDomain) {
		log.Warn().
			Str("spiffe_id", spiffeID).
			Str("trust_domain", trustDomain).
			Msg("SPIFFE ID not in allowlist")
		return nil, status.Errorf(codes.PermissionDenied, "SPIFFE ID %q not in allowlist", spiffeID)
	}

	// Check certificate validity
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return nil, status.Error(codes.Unauthenticated, "certificate not yet valid")
	}
	if now.After(cert.NotAfter) {
		return nil, status.Error(codes.Unauthenticated, "certificate has expired")
	}

	identity := &SPIFFEIdentity{
		SPIFFEID:     spiffeID,
		TrustDomain:  trustDomain,
		WorkloadPath: workloadPath,
		SerialNumber: cert.SerialNumber.String(),
		ExpiresAt:    cert.NotAfter,
		Certificate:  cert,
	}

	log.Debug().
		Str("spiffe_id", spiffeID).
		Str("trust_domain", trustDomain).
		Time("expires_at", cert.NotAfter).
		Msg("SPIFFE authentication successful")

	return identity, nil
}

// isAllowed checks if a SPIFFE ID is in the allowlist.
func (a *SPIFFEAuthenticator) isAllowed(spiffeID, trustDomain string) bool {
	// Check exact match
	if a.allowedSPIFFEIDs[spiffeID] {
		return true
	}

	// Check trust domain
	if a.allowedTrustDomains[trustDomain] {
		return true
	}

	// Check patterns
	for _, re := range a.allowedPatterns {
		if re.MatchString(spiffeID) {
			return true
		}
	}

	// If no allowlist is configured, deny by default
	return false
}

// UpdateAllowlist updates the allowlist configuration.
func (a *SPIFFEAuthenticator) UpdateAllowlist(config SPIFFEAuthConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Rebuild allowlists
	newTrustDomains := make(map[string]bool)
	for _, td := range config.AllowedTrustDomains {
		newTrustDomains[td] = true
	}

	newSPIFFEIDs := make(map[string]bool)
	for _, id := range config.AllowedSPIFFEIDs {
		newSPIFFEIDs[id] = true
	}

	var newPatterns []*regexp.Regexp
	for _, pattern := range config.AllowedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid SPIFFE ID pattern %q: %w", pattern, err)
		}
		newPatterns = append(newPatterns, re)
	}

	a.allowedTrustDomains = newTrustDomains
	a.allowedSPIFFEIDs = newSPIFFEIDs
	a.allowedPatterns = newPatterns
	a.enabled = config.Enabled

	log.Info().
		Bool("enabled", config.Enabled).
		Int("trust_domains", len(newTrustDomains)).
		Int("spiffe_ids", len(newSPIFFEIDs)).
		Int("patterns", len(newPatterns)).
		Msg("SPIFFE allowlist updated")

	return nil
}

// IsEnabled returns whether SPIFFE authentication is enabled.
func (a *SPIFFEAuthenticator) IsEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.enabled
}

// extractSPIFFEID extracts SPIFFE ID from certificate SAN URIs.
func extractSPIFFEID(cert *x509.Certificate) (string, error) {
	for _, uri := range cert.URIs {
		if uri.Scheme == "spiffe" {
			return uri.String(), nil
		}
	}
	return "", fmt.Errorf("no SPIFFE ID in certificate SANs")
}

// parseSPIFFEID parses a SPIFFE ID into trust domain and workload path.
func parseSPIFFEID(spiffeID string) (trustDomain, workloadPath string, err error) {
	u, err := url.Parse(spiffeID)
	if err != nil {
		return "", "", fmt.Errorf("invalid URI: %w", err)
	}

	if u.Scheme != "spiffe" {
		return "", "", fmt.Errorf("not a SPIFFE ID: scheme is %q, expected spiffe", u.Scheme)
	}

	trustDomain = u.Host
	if trustDomain == "" {
		return "", "", fmt.Errorf("empty trust domain")
	}

	workloadPath = u.Path
	return trustDomain, workloadPath, nil
}

// Context key for SPIFFE identity
type spiffeIdentityKey struct{}

// SPIFFEIdentityKey is the context key for SPIFFE identity.
var SPIFFEIdentityKey = spiffeIdentityKey{}

// SPIFFEIdentityFromContext extracts the SPIFFE identity from context.
func SPIFFEIdentityFromContext(ctx context.Context) (*SPIFFEIdentity, bool) {
	identity, ok := ctx.Value(SPIFFEIdentityKey).(*SPIFFEIdentity)
	return identity, ok
}

// ContextWithSPIFFEIdentity returns a context with the SPIFFE identity.
func ContextWithSPIFFEIdentity(ctx context.Context, identity *SPIFFEIdentity) context.Context {
	return context.WithValue(ctx, SPIFFEIdentityKey, identity)
}

// ExtractTrustDomain extracts the trust domain from a SPIFFE ID.
func ExtractTrustDomain(spiffeID string) string {
	u, err := url.Parse(spiffeID)
	if err != nil {
		return ""
	}
	return u.Host
}

// ExtractWorkloadPath extracts the workload path from a SPIFFE ID.
func ExtractWorkloadPath(spiffeID string) string {
	u, err := url.Parse(spiffeID)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}
