package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type CertPool interface {
	AppendCertsFromPEM(caBytes []byte) bool
}

var (
	ErrFailedToAppend = errors.New("failed to append Root certificate")
	ErrTLS            = errors.New("TLS Error")
	ErrCertPool       = errors.New("certPool Error")
	ErrCACertFile     = errors.New("caCertFile Error")
)

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

func setupTLS(certFile, certKeyFile, caFile string, certPool CertPool) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		NextProtos: []string{"h2", "http/1.1"},
	}
	var err error

	tlsConfig.Certificates = make([]tls.Certificate, 1)
	tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(
		certFile,
		certKeyFile,
	)

	if err != nil {
		cErr := &CertError{
			CertFile: certFile,
			Msg:      err.Error(),
			Err:      ErrTLS,
		}
		return nil, cErr
	}

	if caFile == "" {
		return tlsConfig, nil
	}

	caBytes, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tlsConfig, nil
		}
		cErr := &CertError{
			CertFile: certFile,
			Msg:      err.Error(),
			Err:      ErrCACertFile,
		}
		return nil, cErr
	}

	ok := certPool.AppendCertsFromPEM(caBytes)
	if !ok {
		cErr := &CertError{
			CertFile: certFile,
			Err:      ErrFailedToAppend,
		}
		return nil, cErr
	}

	certPoolAsX509CertPool, ok := certPool.(*x509.CertPool)
	if !ok {
		cErr := &CertError{
			CertFile: certFile,
			Err:      ErrCertPool,
		}
		return nil, cErr
	}

	tlsConfig.ClientCAs = certPoolAsX509CertPool

	return tlsConfig, nil
}
