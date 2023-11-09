package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
)

func setupTLS(certFile, certKeyFile, caFile string) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	var err error

	tlsConfig.Certificates = make([]tls.Certificate, 1)
	tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(
		certFile,
		certKeyFile,
	)

	log.Printf("Certificates: %s - %s, tlsConfig: %v", certFile, certKeyFile, tlsConfig.Certificates)

	if err != nil {
		return nil, err
	}

	caBytes, err := ioutil.ReadFile(caFile)

	ca := x509.NewCertPool()
	ok := ca.AppendCertsFromPEM([]byte(caBytes))

	if !ok {
		return nil, fmt.Errorf(
			"failed to parse root certificate: %q", certFile,
		)
	}

	tlsConfig.RootCAs = ca
	tlsConfig.ServerName = "0.0.0.0:8585"

	return tlsConfig, nil
}
