package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"testing"
)

type CustomCertPool struct {
	certPool *x509.CertPool
}

func (c *CustomCertPool) AppendCertsFromPEM(caBytes []byte) bool {
	return false
}

func TestSetupTLS(t *testing.T) {
	testCases := []struct {
		certPool       CertPool
		expectedError  error
		expectedErrMsg string
		name           string
		certFile       string
		keyFile        string
		caFile         string
		expectedSN     string
		expectedMinVer uint16
		expectedCAs    bool
	}{
		{
			name:           "ValidFiles",
			certFile:       "testdata/server-cert.pem",
			keyFile:        "testdata/server-key.key",
			caFile:         "testdata/ca-cert.pem",
			certPool:       x509.NewCertPool(),
			expectedError:  nil,
			expectedErrMsg: "",
			expectedMinVer: tls.VersionTLS13,
			expectedCAs:    true,
			expectedSN:     "0.0.0.0",
		},
		{
			name:           "InvalidCA",
			certFile:       "testdata/invalid-cert.pem",
			keyFile:        "testdata/server-key.key",
			caFile:         "testdata/ca-cert.pem",
			certPool:       &x509.CertPool{},
			expectedError:  ErrTLS,
			expectedErrMsg: "CertError: TLS Error - tls: private key does not match public key (testdata/invalid-cert.pem)",
			expectedMinVer: 0,
			expectedCAs:    false,
			expectedSN:     "",
		},
		{
			name:           "FailedToAppend",
			certFile:       "testdata/server-cert.pem",
			keyFile:        "testdata/server-key.key",
			caFile:         "testdata/ca-cert.pem",
			certPool:       &CustomCertPool{certPool: x509.NewCertPool()},
			expectedError:  ErrFailedToAppend,
			expectedErrMsg: "CertError: failed to append Root certificate -  (testdata/server-cert.pem)",
			expectedMinVer: 0,
			expectedCAs:    false,
			expectedSN:     "",
		},
		{
			name:           "MissingCACert",
			certFile:       "testdata/server-cert.pem",
			keyFile:        "testdata/server-key.key",
			caFile:         "testdata/no-ca-cert.pem",
			certPool:       x509.NewCertPool(),
			expectedError:  ErrCACertFile,
			expectedErrMsg: "CertError: caCertFile Error - open testdata/no-ca-cert.pem: no such file or directory (testdata/server-cert.pem)",
			expectedMinVer: 0,
			expectedCAs:    false,
			expectedSN:     "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tlsConfig, err := setupTLS(tc.certFile, tc.keyFile, tc.caFile, tc.certPool)

			if err != nil {
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("expected error to be %v, got %v", tc.expectedError, err)
				}

				if tc.expectedError != nil {
					certError, ok := err.(*CertError)
					if !ok {
						t.Errorf("unexpected error type: got %T, want *CertError", err)
						return
					}

					if !errors.Is(certError.Err, tc.expectedError) {
						t.Errorf("unexpected error type: got %v, want %v", certError.Err, tc.expectedError)
					}

					if err.Error() != tc.expectedErrMsg {
						t.Errorf("unexpected error message: got %s, want %s", err.Error(), tc.expectedErrMsg)
					}
				}
			} else {
				if tlsConfig.MinVersion != tc.expectedMinVer {
					t.Error("Unexpected MinVersion value")
				}

				if tlsConfig.ClientCAs == nil || tlsConfig.RootCAs == nil {
					if tc.expectedCAs {
						t.Error("Expected ClientCAs and RootCAs to be non-nil")
					}
				} else {
					if tlsConfig.ClientCAs != tc.certPool || tlsConfig.RootCAs != tc.certPool {
						if tc.expectedCAs {
							t.Error("ClientCAs and RootCAs should match the passed certPool")
						}
					}
				}

				if tlsConfig.ServerName != tc.expectedSN {
					t.Error("Unexpected ServerName value")
				}
			}
		})
	}
}
