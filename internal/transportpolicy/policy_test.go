package transportpolicy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestEndpointPolicy(t *testing.T) {
	for _, tc := range []struct {
		url             string
		ws, prod, valid bool
	}{
		{"https://api.example.com", false, true, true}, {"wss://socket.example.com/ws", true, true, true},
		{"http://localhost", false, true, false}, {"ws://localhost/ws", true, true, false},
		{"http://127.0.0.1:8080", false, false, true}, {"ws://[::1]:8081/ws", true, false, true},
		{"http://remote.example.com", false, false, false}, {"https://user:secret@api.example.com", false, true, false},
		{"https://api.example.com?token=secret", false, true, false}, {"https://api.example.com#x", false, true, false},
		{"wss:///ws", true, true, false},
	} {
		if err := ValidateEndpoint(tc.url, "endpoint", tc.ws, tc.prod); (err == nil) != tc.valid {
			t.Errorf("policy mismatch for test endpoint: %v", err)
		}
	}
	for _, service := range []bool{false, true} {
		got, err := Mode("", service)
		if err != nil || (got == "production") != service {
			t.Fatal("default mode mismatch")
		}
	}
	if _, err := Mode("typo", true); err == nil {
		t.Fatal("unknown mode accepted")
	}
	if got, err := Mode("development", true); err != nil || got != "development" {
		t.Fatal("explicit Service development rejected")
	}
}

// Test-only CA: never installed into Windows or any machine/user trust store.
func tlsFixture(t *testing.T, wrongHost, expired bool, handler http.Handler) (*httptest.Server, *http.Transport) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ephemeral test CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if wrongHost {
		leaf.IPAddresses = nil
		leaf.DNSNames = []string{"other.example.com"}
	}
	if expired {
		leaf.NotAfter = now.Add(-time.Minute)
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der, caDER}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(root)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	t.Cleanup(transport.CloseIdleConnections)
	return server, transport
}

func TestHTTPSAndWSSCertificateValidation(t *testing.T) {
	for _, tc := range []struct {
		name, kind                string
		wrong, expired, untrusted bool
	}{
		{"trusted", "", false, false, false},
		{"wrong hostname", "TLS certificate hostname mismatch", true, false, false},
		{"expired", "TLS certificate expired or not yet valid", false, true, false},
		{"untrusted", "TLS certificate authority untrusted", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, tr := tlsFixture(t, tc.wrong, tc.expired, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ws" {
					conn, err := websocket.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer conn.CloseNow()
					_, _, _ = conn.Read(r.Context())
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			if tc.untrusted {
				tr.TLSClientConfig.RootCAs = x509.NewCertPool()
			}
			client := NewClient(3 * time.Second)
			client.Transport = tr
			resp, err := client.Get(server.URL)
			if resp != nil {
				resp.Body.Close()
			}
			if tc.kind == "" && err != nil {
				t.Fatal(err)
			}
			if tc.kind != "" && (err == nil || FailureKind(err) != tc.kind) {
				t.Fatalf("HTTPS failure classification: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(server.URL, "https")+"/ws", &websocket.DialOptions{HTTPClient: client})
			if conn != nil {
				conn.CloseNow()
			}
			if tc.kind == "" && err != nil {
				t.Fatal(err)
			}
			if tc.kind != "" && (err == nil || FailureKind(err) != tc.kind) {
				t.Fatalf("WSS failure classification: %v", err)
			}
		})
	}
}

func TestRedirectsDoNotLeakSensitiveBody(t *testing.T) {
	var sinkHits atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkHits.Add(1) }))
	defer sink.Close()
	var sameHits atomic.Int32
	server, tr := tlsFixture(t, false, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/downgrade":
			http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
		case "/cross-authority":
			http.Redirect(w, r, strings.Replace(sink.URL, "http:", "https:", 1), http.StatusTemporaryRedirect)
		case "/same":
			http.Redirect(w, r, "/ok", http.StatusTemporaryRedirect)
		case "/ok":
			b, _ := io.ReadAll(r.Body)
			if r.Method != "POST" || string(b) != "test-only-token" {
				t.Error("same-origin body changed")
			}
			sameHits.Add(1)
			w.WriteHeader(204)
		}
	}))
	client := NewClient(3 * time.Second)
	client.Transport = tr
	for _, path := range []string{"/downgrade", "/cross-authority", "/same"} {
		resp, err := client.Post(server.URL+path, "application/json", strings.NewReader("test-only-token"))
		if resp != nil {
			resp.Body.Close()
		}
		if path == "/same" {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, ErrRedirect) {
			t.Fatalf("redirect not rejected: %v", err)
		}
	}
	if sinkHits.Load() != 0 || sameHits.Load() != 1 {
		t.Fatal("redirect destination behavior incorrect")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(server.URL, "https")+"/downgrade", &websocket.DialOptions{HTTPClient: client})
	if conn != nil {
		conn.CloseNow()
	}
	if !errors.Is(err, ErrRedirect) || sinkHits.Load() != 0 {
		t.Fatal("WSS downgrade not blocked")
	}
}

func TestRedirectLimitsAndUpgrade(t *testing.T) {
	original, _ := http.NewRequest("GET", "http://example.com/start", nil)
	upgrade, _ := http.NewRequest("GET", "https://example.com/next", nil)
	if err := CheckRedirect(upgrade, []*http.Request{original}); err != nil {
		t.Fatal(err)
	}
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = original
	}
	if !errors.Is(CheckRedirect(upgrade, via), ErrRedirect) {
		t.Fatal("redirect loop allowed")
	}
	secretErr := errors.New("https://user:secret@example.com/?token=secret")
	if strings.Contains(FailureKind(secretErr), "secret") {
		t.Fatal("unsafe diagnostic")
	}
}
