package main

import (
	"crypto/tls"
	"errors"
	"fmt"
)

var ErrTLS = errors.New("TLS Error")

type CertError struct {
	Err      error
	CertFile string
	Msg      string
}

func (ce *CertError) Error() string {
	return fmt.Sprintf("CertError: %s - %s (%s)", ce.Err.Error(), ce.Msg, ce.CertFile)
}

func (ce *CertError) Unwrap() error {
	return fmt.Errorf(" %w: %s", ce.Err, ce.Msg)
}

// setupTLS builds the server-side TLS configuration from the given certificate and key files.
func setupTLS(certFile, certKeyFile string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		NextProtos: []string{"h2", "http/1.1"},
	}

	cert, err := tls.LoadX509KeyPair(certFile, certKeyFile)
	if err != nil {
		return nil, &CertError{
			CertFile: certFile,
			Msg:      err.Error(),
			Err:      ErrTLS,
		}
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	return tlsConfig, nil
}
