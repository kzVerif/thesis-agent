// Package transportpolicy defines endpoint and redirect policy without changing
// Go's platform certificate-chain, validity or hostname verification.
package transportpolicy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func Mode(value string, service bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if service {
			return "production", nil
		}
		return "development", nil
	}
	if value != "production" && value != "development" {
		return "", errors.New("TRANSPORT_MODE must be production or development")
	}
	return value, nil
}

func Loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ValidateEndpoint(value, name string, socket, production bool) error {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("%s must be an absolute endpoint without credentials, query or fragment", name)
	}
	plain, secure := "http", "https"
	if socket {
		plain, secure = "ws", "wss"
	}
	if u.Scheme == secure {
		return nil
	}
	if production {
		return fmt.Errorf("production %s must use %s", name, secure)
	}
	if u.Scheme != plain || !Loopback(u.Hostname()) {
		return fmt.Errorf("development %s allows plaintext only on localhost/loopback; otherwise use %s", name, secure)
	}
	return nil
}

var ErrRedirect = errors.New("unsafe or excessive redirect blocked; configure the final trusted endpoint")

// Only same-host/port redirects, or a same-host HTTP:80 -> HTTPS:443 upgrade,
// are allowed. This prevents replay of enrollment bodies and download tokens to
// another authority. No callback or custom roots bypass TLS verification.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 || len(via) == 0 {
		return ErrRedirect
	}
	target := req.URL
	previous := via[len(via)-1].URL
	if target.User != nil || target.Hostname() == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return ErrRedirect
	}
	if previous.Scheme == "https" && target.Scheme != "https" {
		return ErrRedirect
	}
	if !strings.EqualFold(previous.Hostname(), target.Hostname()) {
		return ErrRedirect
	}
	if port(previous) != port(target) && !(previous.Scheme == "http" && target.Scheme == "https" && port(previous) == "80" && port(target) == "443") {
		return ErrRedirect
	}
	return nil
}

func port(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: CheckRedirect}
}

// FailureKind returns only fixed operator-facing text, never URL/query/body or
// remote close-reason text. Wrapped x509 errors remain available to callers.
func FailureKind(err error) string {
	var hostname x509.HostnameError
	var authority x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var verification *tls.CertificateVerificationError
	switch {
	case errors.As(err, &hostname):
		return "TLS certificate hostname mismatch"
	case errors.As(err, &authority):
		return "TLS certificate authority untrusted"
	case errors.As(err, &invalid):
		if invalid.Reason == x509.Expired {
			return "TLS certificate expired or not yet valid"
		}
		return "TLS certificate invalid"
	case errors.As(err, &verification):
		return "TLS certificate verification failed"
	case errors.Is(err, ErrRedirect):
		return "unsafe redirect blocked"
	default:
		return "connection unavailable or closed"
	}
}
