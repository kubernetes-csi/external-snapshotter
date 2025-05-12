/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"testing"
	"time"
)

func TestWebhookCertReload(t *testing.T) {
	// Initialize test space
	tmpDir := os.TempDir() + "/webhook-cert-tests"
	certFile = tmpDir + "/tls.crt"
	keyFile = tmpDir + "/tls.key"
	port = 30443
	err := os.Mkdir(tmpDir, 0o777)
	if err != nil && err != os.ErrExist {
		t.Errorf("unexpected error occurred while creating tmp dir: %v", err)
	}
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Errorf("unexpected error occurred while deleting certs: %v", err)
		}
	}()
	err = generateTestCertKeyPair(t, certFile, keyFile)
	if err != nil {
		t.Errorf("unexpected error occurred while generating test certs: %v", err)
	}

	// Start test server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cw, err := NewCertWatcher(certFile, keyFile)
	if err != nil {
		t.Errorf("failed to initialize new cert watcher: %v", err)
	}
	tlsConfig := &tls.Config{
		GetCertificate: cw.GetCertificate,
	}
	go func() {
		err := startServer(ctx,
			tlsConfig,
			cw,
		)
		if err != nil {
			panic(err)
		}
	}()
	time.Sleep(250 * time.Millisecond) // Give some time for watcher to start

	// TC: Original cert should not change with no file changes
	originalCert, err := tlsConfig.GetCertificate(nil)
	if err != nil {
		t.Errorf("unexpected error occurred while getting cert: %v", err)
	}
	originalCertStr := string(originalCert.Certificate[0])
	originalKey := originalCert.PrivateKey.(*rsa.PrivateKey)

	newCert, err := tlsConfig.GetCertificate(nil) // get certificate again
	if err != nil {
		t.Errorf("unexpected error occurred while getting  newcert: %v", err)
	}
	if string(newCert.Certificate[0]) != originalCertStr {
		t.Error("new cert was updated when it should not have been")
	}
	newKey := newCert.PrivateKey.(*rsa.PrivateKey)
	if !newKey.Equal(originalKey) {
		t.Error("new key was updated when it should not have been")
	}

	// TC: Certificate should consistently change with a file change
	for i := 0; i < 5; i++ {
		// Generate new key/cert
		err = generateTestCertKeyPair(t, certFile, keyFile)
		if err != nil {
			t.Errorf("unexpected error occurred while generating test certs: %v", err)
		}

		// Wait for certwatcher to update
		time.Sleep(250 * time.Millisecond)
		newCert, err = tlsConfig.GetCertificate(nil)
		if err != nil {
			t.Errorf("unexpected error occurred while getting  newcert: %v", err)
		}
		if string(newCert.Certificate[0]) == originalCertStr {
			t.Errorf("new cert was not updated")
		}

		newKey = newCert.PrivateKey.(*rsa.PrivateKey)
		if newKey.Equal(originalKey) {
			t.Error("new key was not updated")
		}

		originalCertStr = string(newCert.Certificate[0])
		originalKey = newKey
	}
}

// generateTestCertKeyPair generates a new random test key/crt and writes it to tmpDir
// based on https://golang.org/src/crypto/tls/generate_cert.go
func generateTestCertKeyPair(t *testing.T, certPath, keyPath string) error {
	notBefore := time.Now()
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("Failed to generate serial number: %v", err)
	}

	var priv interface{}
	priv, err = rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("Failed to generate key: %v", err)
	}
	keyUsage := x509.KeyUsageDigitalSignature
	if _, isRSA := priv.(*rsa.PrivateKey); isRSA {
		keyUsage |= x509.KeyUsageKeyEncipherment
	}
	randomOrganizationStr := time.Now().String()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{randomOrganizationStr},
		},
		NotBefore: notBefore,
		NotAfter:  time.Now().Add(1 * time.Hour),

		KeyUsage:              keyUsage,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"127.0.0.1"},
	}

	rk := priv.(*rsa.PrivateKey)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &rk.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("Failed to create certificate: %v", err)
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("Failed to open tls.crt for writing: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("Failed to write data to tls.crt: %v", err)
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("Error closing tls.crt: %v", err)
	}
	fmt.Printf("wrote new cert: %s\n", certPath)

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to open tls.key for writing: %v", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("Unable to marshal private key: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return fmt.Errorf("Failed to write data to tls.key: %v", err)
	}
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("Error closing tls.key: %v", err)
	}
	fmt.Printf("wrote new key: %s\n", keyPath)

	return nil
}
