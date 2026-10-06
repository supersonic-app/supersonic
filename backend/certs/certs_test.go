package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// testCertificate returns a freshly generated self-signed client certificate
// and its private key.
func testCertificate(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "supersonic-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing certificate: %v", err)
	}
	return key, cert
}

// writePKCS12 encodes key/cert into a PKCS#12 file and returns its path.
func writePKCS12(t *testing.T, key *ecdsa.PrivateKey, cert *x509.Certificate, password string) string {
	t.Helper()
	pfx, err := pkcs12.Modern.Encode(key, cert, nil, password)
	if err != nil {
		t.Fatalf("encoding PKCS#12: %v", err)
	}
	path := filepath.Join(t.TempDir(), "client.p12")
	if err := os.WriteFile(path, pfx, 0o600); err != nil {
		t.Fatalf("writing PKCS#12: %v", err)
	}
	return path
}

func TestLoadPKCS12(t *testing.T) {
	key, cert := testCertificate(t)
	path := writePKCS12(t, key, cert, "secret")

	loaded, err := LoadPKCS12(path, "secret")
	if err != nil {
		t.Fatalf("LoadPKCS12: %v", err)
	}
	if loaded.Leaf == nil {
		t.Fatal("expected leaf certificate to be populated")
	}
	if loaded.Leaf.SerialNumber.Cmp(cert.SerialNumber) != 0 {
		t.Errorf("serial = %v, want %v", loaded.Leaf.SerialNumber, cert.SerialNumber)
	}
	if loaded.PrivateKey == nil {
		t.Error("expected private key to be populated")
	}
	if len(loaded.Certificate) != 1 {
		t.Errorf("chain length = %d, want 1", len(loaded.Certificate))
	}
}

func TestLoadPKCS12WrongPassphrase(t *testing.T) {
	key, cert := testCertificate(t)
	path := writePKCS12(t, key, cert, "right")

	if _, err := LoadPKCS12(path, "wrong"); err == nil {
		t.Fatal("expected an error for the wrong passphrase, got nil")
	}
}

func TestLoadPKCS12MissingFile(t *testing.T) {
	if _, err := LoadPKCS12(filepath.Join(t.TempDir(), "missing.p12"), ""); err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestWriteTempPEM(t *testing.T) {
	key, cert := testCertificate(t)
	loaded := &tls.Certificate{Certificate: [][]byte{cert.Raw}, PrivateKey: key, Leaf: cert}
	dir := t.TempDir()

	certFile, keyFile, err := WriteTempPEM(loaded, dir, "server-id")
	if err != nil {
		t.Fatalf("WriteTempPEM: %v", err)
	}
	if want := filepath.Join(dir, "server-id.crt"); certFile != want {
		t.Errorf("cert file = %q, want %q", certFile, want)
	}
	if want := filepath.Join(dir, "server-id.key"); keyFile != want {
		t.Errorf("key file = %q, want %q", keyFile, want)
	}

	for _, path := range []string{certFile, keyFile} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s mode = %o, want 600", path, perm)
		}
	}

	// The written files must round-trip through crypto/tls.
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("reading cert file: %v", err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("reading key file: %v", err)
	}
	reloaded, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("tls.X509KeyPair on generated files: %v", err)
	}
	if len(reloaded.Certificate) != 1 {
		t.Errorf("reloaded chain length = %d, want 1", len(reloaded.Certificate))
	}
}
