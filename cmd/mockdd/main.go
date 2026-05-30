// Command mockdd is a minimal fake Dell PowerProtect DD appliance for end-to-end
// demos. It serves the same REST surface the exporter calls (token auth on
// /api/v1/auth plus the per-resource GET endpoints) over self-signed TLS on :3009,
// returning canned JSON from embedded fixtures. It is NOT a faithful DD emulator —
// it exists so the Compose stacks light up a Grafana dashboard without real hardware.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"io"
	"log"
	"math/big"
	"net/http"
	"time"
)

//go:embed fixtures/*.json
var fixtures embed.FS

// routes maps an exporter request path to its embedded fixture file.
var routes = map[string]string{
	"/api/v1/dd-systems/0/file-system":        "fixtures/file-system.json",
	"/api/v1/dd-systems/0/mtrees":             "fixtures/mtrees.json",
	"/api/v1/dd-systems/0/replications":       "fixtures/replications.json",
	"/api/v1/dd-systems/0/hardware/disks":     "fixtures/disks.json",
	"/api/v1/dd-systems/0/alerts":             "fixtures/alerts.json",
	"/api/v1/dd-systems/0/stats/system-stats": "fixtures/system-stats.json",
}

const mockToken = "mockdd-session-token"

func main() {
	mux := http.NewServeMux()

	// Auth: POST returns a session token header; DELETE logs out. Credentials are
	// not checked — this is a demo appliance.
	mux.HandleFunc("/api/v1/auth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("X-DD-AUTH-TOKEN", mockToken)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	for path, file := range routes {
		file := file
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-DD-AUTH-TOKEN") != mockToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			b, err := fixtures.ReadFile(file)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			writeBytes(w, b)
		})
	}

	srv := &http.Server{
		Addr:              ":3009",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{mustSelfSignedCert()},
		},
	}
	log.Println("mockdd: serving fake DD API on https://0.0.0.0:3009")
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

// writeBytes writes b to w. It takes io.Writer (not http.ResponseWriter) so the raw
// write is isolated to one helper, the same pattern the tests use.
func writeBytes(w io.Writer, b []byte) { _, _ = w.Write(b) }

// mustSelfSignedCert generates an in-memory self-signed certificate at startup.
// Clients connect with insecureSkipVerify, so the cert only needs to be valid TLS.
func mustSelfSignedCert() tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("mockdd: generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mockdd"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * 365 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"mockdd", "localhost"},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		log.Fatalf("mockdd: create cert: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
