package main

import (
	"crypto/tls"
	"errors"
	"slices"
	"testing"
)

const (
	testValidCertFile = "testdata/server-cert.pem"
	testValidKeyFile  = "testdata/server-key.key"
)

func TestSetupTLS(t *testing.T) {
	testCases := []struct {
		expectedError  error
		name           string
		certFile       string
		keyFile        string
		expectedErrMsg string
	}{
		{
			name:     "ValidFiles",
			certFile: testValidCertFile,
			keyFile:  testValidKeyFile,
		},
		{
			name:           "InvalidKeyPair",
			certFile:       "testdata/invalid-cert.pem",
			keyFile:        testValidKeyFile,
			expectedError:  ErrTLS,
			expectedErrMsg: "CertError: TLS Error - tls: private key does not match public key (testdata/invalid-cert.pem)",
		},
		{
			name:          "MissingFiles",
			certFile:      "testdata/does-not-exist.pem",
			keyFile:       "testdata/does-not-exist.key",
			expectedError: ErrTLS,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tlsConfig, err := setupTLS(tc.certFile, tc.keyFile)

			if tc.expectedError != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.expectedError)
				}
				if !errors.Is(err, tc.expectedError) {
					t.Fatalf("expected error %v, got %v", tc.expectedError, err)
				}
				var certError *CertError
				if !errors.As(err, &certError) {
					t.Fatalf("unexpected error type: got %T, want *CertError", err)
				}
				if tc.expectedErrMsg != "" && err.Error() != tc.expectedErrMsg {
					t.Errorf("unexpected error message: got %s, want %s", err.Error(), tc.expectedErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tlsConfig.MinVersion != tls.VersionTLS12 {
				t.Error("Unexpected MinVersion value")
			}
			if tlsConfig.MaxVersion != tls.VersionTLS13 {
				t.Error("Unexpected MaxVersion value")
			}
			if len(tlsConfig.Certificates) != 1 {
				t.Errorf("expected 1 certificate, got %d", len(tlsConfig.Certificates))
			}
			if tlsConfig.ClientCAs != nil || tlsConfig.ClientAuth != tls.NoClientCert {
				t.Error("client certificate verification must not be configured")
			}
			if !slices.Equal(tlsConfig.NextProtos, []string{"h2", "http/1.1"}) {
				t.Errorf("expected NextProtos [h2 http/1.1], got %v", tlsConfig.NextProtos)
			}
		})
	}
}
