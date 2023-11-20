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

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM([]byte(caBytes))

	if !ok {
		return nil, fmt.Errorf(
			"failed to parse root certificate: %q", certFile,
		)
	}

	tlsConfig.ClientCAs = certPool
	tlsConfig.RootCAs = certPool
	tlsConfig.ServerName = "0.0.0.0"

	return tlsConfig, nil
}
