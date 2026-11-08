package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"slices"
	"testing"
)

const (
	testValidCertFile = "testdata/server-cert.pem"
	testValidKeyFile  = "testdata/server-key.key"
	testValidCAFile   = "testdata/ca-cert.pem"
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
		expectedMinVer uint16
		expectedMaxVer uint16
		expectedCAs    bool
	}{
		{
			name:           "ValidFilesWithCA",
			certFile:       testValidCertFile,
			keyFile:        testValidKeyFile,
			caFile:         testValidCAFile,
			certPool:       x509.NewCertPool(),
			expectedError:  nil,
			expectedMinVer: tls.VersionTLS12,
			expectedMaxVer: tls.VersionTLS13,
			expectedCAs:    true,
		},
		{
			name:           "ValidFilesWithoutCA",
			certFile:       testValidCertFile,
			keyFile:        testValidKeyFile,
			caFile:         "",
			certPool:       x509.NewCertPool(),
			expectedError:  nil,
			expectedMinVer: tls.VersionTLS12,
			expectedMaxVer: tls.VersionTLS13,
			expectedCAs:    false,
		},
		{
			name:           "OptionalMissingCACertSucceeds",
			certFile:       testValidCertFile,
			keyFile:        testValidKeyFile,
			caFile:         "testdata/no-ca-cert.pem",
			certPool:       x509.NewCertPool(),
			expectedError:  nil,
			expectedMinVer: tls.VersionTLS12,
			expectedMaxVer: tls.VersionTLS13,
			expectedCAs:    false,
		},
		{
			name:           "InvalidKeyPair",
			certFile:       "testdata/invalid-cert.pem",
			keyFile:        testValidKeyFile,
			caFile:         testValidCAFile,
			certPool:       &x509.CertPool{},
			expectedError:  ErrTLS,
			expectedErrMsg: "CertError: TLS Error - tls: private key does not match public key (testdata/invalid-cert.pem)",
		},
		{
			name:           "FailedToAppend",
			certFile:       testValidCertFile,
			keyFile:        testValidKeyFile,
			caFile:         testValidCAFile,
			certPool:       &CustomCertPool{certPool: x509.NewCertPool()},
			expectedError:  ErrFailedToAppend,
			expectedErrMsg: "CertError: failed to append Root certificate -  (testdata/server-cert.pem)",
		},
		{
			name:          "UnreadableCAPathError",
			certFile:      testValidCertFile,
			keyFile:       testValidKeyFile,
			caFile:        "testdata",
			certPool:      x509.NewCertPool(),
			expectedError: ErrCACertFile,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tlsConfig, err := setupTLS(tc.certFile, tc.keyFile, tc.caFile, tc.certPool)

			if err != nil {
				if !errors.Is(err, tc.expectedError) {
					t.Fatalf("expected error %v, got %v", tc.expectedError, err)
				}

				if tc.expectedError != nil {
					var certError *CertError
					if !errors.As(err, &certError) {
						t.Fatalf("unexpected error type: got %T, want *CertError", err)
					}

					if !errors.Is(certError.Err, tc.expectedError) {
						t.Errorf("unexpected wrapped error: got %v, want %v", certError.Err, tc.expectedError)
					}

					if tc.expectedErrMsg != "" && err.Error() != tc.expectedErrMsg {
						t.Errorf("unexpected error message: got %s, want %s", err.Error(), tc.expectedErrMsg)
					}
				}
				return
			}

			if tc.expectedError != nil {
				t.Fatalf("expected error %v, got nil", tc.expectedError)
			}

			if tlsConfig.MinVersion != tc.expectedMinVer {
				t.Error("Unexpected MinVersion value")
			}

			if tlsConfig.MaxVersion != tc.expectedMaxVer {
				t.Error("Unexpected MaxVersion value")
			}

			if tc.expectedCAs {
				if tlsConfig.ClientCAs == nil {
					t.Error("Expected ClientCAs to be non-nil")
				} else if tlsConfig.ClientCAs != tc.certPool {
					t.Error("ClientCAs should match the passed certPool")
				}
			} else if tlsConfig.ClientCAs != nil {
				t.Error("Expected ClientCAs to be nil when CA cert is omitted")
			}

			if !slices.Equal(tlsConfig.NextProtos, []string{"h2", "http/1.1"}) {
				t.Errorf("expected NextProtos [h2 http/1.1], got %v", tlsConfig.NextProtos)
			}
		})
	}
}
