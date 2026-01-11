// Package config provides configuration types for Sentinel Hub.
package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfig holds TLS configuration for the gRPC server.
type TLSConfig struct {
	// Enabled enables TLS for gRPC connections.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// CertFile is the path to the server certificate file.
	CertFile string `json:"cert_file" yaml:"cert_file"`

	// KeyFile is the path to the server private key file.
	KeyFile string `json:"key_file" yaml:"key_file"`

	// CAFile is the path to the CA certificate for client verification.
	CAFile string `json:"ca_file" yaml:"ca_file"`

	// RequireClientCert enables mTLS (require client certificate).
	RequireClientCert bool `json:"require_client_cert" yaml:"require_client_cert"`

	// MinVersion is the minimum TLS version (1.2 or 1.3).
	MinVersion string `json:"min_version" yaml:"min_version"`

	// SPIFFE holds SPIFFE-specific configuration.
	SPIFFE SPIFFEConfig `json:"spiffe" yaml:"spiffe"`
}

// SPIFFEConfig holds SPIFFE-specific configuration.
type SPIFFEConfig struct {
	// Enabled enables SPIFFE-based authentication.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// AgentSocket is the path to the SPIRE agent socket.
	AgentSocket string `json:"agent_socket" yaml:"agent_socket"`

	// AllowedTrustDomains are the trust domains that are allowed to connect.
	AllowedTrustDomains []string `json:"allowed_trust_domains" yaml:"allowed_trust_domains"`

	// AllowedSPIFFEIDs are specific SPIFFE IDs that are allowed (exact match).
	AllowedSPIFFEIDs []string `json:"allowed_spiffe_ids" yaml:"allowed_spiffe_ids"`

	// AllowedPatterns are regex patterns for matching SPIFFE IDs.
	AllowedPatterns []string `json:"allowed_patterns" yaml:"allowed_patterns"`
}

// DefaultTLSConfig returns a TLS configuration with sensible defaults.
func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		Enabled:           false,
		MinVersion:        "1.2",
		RequireClientCert: false,
		SPIFFE: SPIFFEConfig{
			Enabled:     false,
			AgentSocket: "/run/spire/sockets/agent.sock",
		},
	}
}

// Validate checks if the TLS configuration is valid.
func (c *TLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.CertFile == "" {
		return fmt.Errorf("TLS enabled but cert_file not specified")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("TLS enabled but key_file not specified")
	}

	// Check that certificate files exist
	if _, err := os.Stat(c.CertFile); os.IsNotExist(err) {
		return fmt.Errorf("certificate file not found: %s", c.CertFile)
	}
	if _, err := os.Stat(c.KeyFile); os.IsNotExist(err) {
		return fmt.Errorf("key file not found: %s", c.KeyFile)
	}

	// Check CA file if mTLS is enabled
	if c.RequireClientCert && c.CAFile != "" {
		if _, err := os.Stat(c.CAFile); os.IsNotExist(err) {
			return fmt.Errorf("CA file not found: %s", c.CAFile)
		}
	}

	// Validate SPIFFE config if enabled
	if c.SPIFFE.Enabled {
		if c.SPIFFE.AgentSocket == "" {
			return fmt.Errorf("SPIFFE enabled but agent_socket not specified")
		}
		if len(c.SPIFFE.AllowedTrustDomains) == 0 &&
			len(c.SPIFFE.AllowedSPIFFEIDs) == 0 &&
			len(c.SPIFFE.AllowedPatterns) == 0 {
			return fmt.Errorf("SPIFFE enabled but no allowlist configured")
		}
	}

	return nil
}

// LoadTLSConfig loads the TLS configuration and returns a tls.Config.
func (c *TLSConfig) LoadTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	// Load server certificate
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Parse min version
	switch c.MinVersion {
	case "1.3", "TLS1.3":
		tlsConfig.MinVersion = tls.VersionTLS13
	case "1.2", "TLS1.2":
		tlsConfig.MinVersion = tls.VersionTLS12
	default:
		tlsConfig.MinVersion = tls.VersionTLS12
	}

	// Load CA for client verification
	if c.CAFile != "" || c.RequireClientCert {
		caPool := x509.NewCertPool()
		if c.CAFile != "" {
			caData, err := os.ReadFile(c.CAFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA file: %w", err)
			}
			if !caPool.AppendCertsFromPEM(caData) {
				return nil, fmt.Errorf("failed to parse CA certificates")
			}
		}
		tlsConfig.ClientCAs = caPool

		if c.RequireClientCert {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		}
	}

	return tlsConfig, nil
}

// GRPCConfig holds configuration for the gRPC server.
type GRPCConfig struct {
	// Port is the gRPC server port.
	Port int `json:"port" yaml:"port"`

	// TLS holds TLS configuration.
	TLS TLSConfig `json:"tls" yaml:"tls"`

	// MaxRecvMsgSize is the maximum message size in bytes.
	MaxRecvMsgSize int `json:"max_recv_msg_size" yaml:"max_recv_msg_size"`

	// MaxSendMsgSize is the maximum send message size in bytes.
	MaxSendMsgSize int `json:"max_send_msg_size" yaml:"max_send_msg_size"`
}

// DefaultGRPCConfig returns a gRPC configuration with sensible defaults.
func DefaultGRPCConfig() *GRPCConfig {
	return &GRPCConfig{
		Port:           9090,
		TLS:            *DefaultTLSConfig(),
		MaxRecvMsgSize: 4 * 1024 * 1024,  // 4MB
		MaxSendMsgSize: 10 * 1024 * 1024, // 10MB
	}
}
