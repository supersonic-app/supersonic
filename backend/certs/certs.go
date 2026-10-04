// Package certs loads client certificates for mutual TLS.
//
// Go's crypto/tls cannot consume OS credential stores directly, so a client
// certificate is provided as a PKCS#12 (.p12/.pfx) file and decrypted
// in-process. For playback the decrypted material is written to short-lived PEM
// files, because mpv only accepts client certificates as file paths.
package certs

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"software.sslmate.com/src/go-pkcs12"
)

// LoadPKCS12 loads a client certificate and private key from a PKCS#12 bundle.
// passphrase may be empty for an unencrypted bundle.
func LoadPKCS12(path, passphrase string) (*tls.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading client certificate %q: %w", path, err)
	}
	privateKey, leaf, chain, err := pkcs12.DecodeChain(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decoding PKCS#12 %q: %w", path, err)
	}
	cert := &tls.Certificate{
		Certificate: [][]byte{leaf.Raw},
		PrivateKey:  privateKey,
		Leaf:        leaf,
	}
	for _, c := range chain {
		cert.Certificate = append(cert.Certificate, c.Raw)
	}
	return cert, nil
}

// WriteTempPEM writes the certificate (with chain) and private key of cert to
// PEM files named <name>.crt/<name>.key in dir (created 0700 if needed). The
// files are written 0600. It is used to hand a client identity to mpv.
func WriteTempPEM(cert *tls.Certificate, dir, name string) (certFile, keyFile string, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	certFile = filepath.Join(dir, name+".crt")
	keyFile = filepath.Join(dir, name+".key")

	var certBuf bytes.Buffer
	for _, der := range cert.Certificate {
		if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
			return "", "", err
		}
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return "", "", fmt.Errorf("marshaling client private key: %w", err)
	}
	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}); err != nil {
		return "", "", err
	}

	if err := os.WriteFile(certFile, certBuf.Bytes(), 0o600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyFile, keyBuf.Bytes(), 0o600); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}
