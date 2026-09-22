package client

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"ws-agent/internal/agentauth"
)

const clientAuthTestID = "11111111-1111-4111-8111-111111111111"

func clientAuthTestKey() ed25519.PrivateKey { return ed25519.NewKeyFromSeed(make([]byte, 32)) }
func configureTestAuth(c *Client) {
	c.ConfigureAuthentication(clientAuthTestID, func() (ed25519.PrivateKey, error) { return clientAuthTestKey(), nil })
}
func testChallenge(ctx context.Context, conn *websocket.Conn) (agentauth.Hello, agentauth.Challenge, error) {
	var hello agentauth.Hello
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		return hello, agentauth.Challenge{}, err
	}
	id, _ := agentauth.Random()
	nonce, _ := agentauth.Random()
	challenge := agentauth.Challenge{Type: "auth_challenge", Version: agentauth.Version, ChallengeID: id, ServerNonce: nonce}
	return hello, challenge, nil
}
func testVerifyProof(ctx context.Context, conn *websocket.Conn, hello agentauth.Hello, challenge agentauth.Challenge) error {
	if err := wsjson.Write(ctx, conn, challenge); err != nil {
		return err
	}
	var proof agentauth.Proof
	if err := wsjson.Read(ctx, conn, &proof); err != nil {
		return err
	}
	transcript, err := agentauth.Transcript(hello.AgentID, challenge.ChallengeID, hello.ClientNonce, challenge.ServerNonce)
	if err != nil {
		return err
	}
	signature, err := agentauth.Decode(proof.Signature, ed25519.SignatureSize)
	if err != nil {
		return err
	}
	if proof.ChallengeID != challenge.ChallengeID || proof.Version != agentauth.Version || proof.Type != "auth_proof" || !ed25519.Verify(clientAuthTestKey().Public().(ed25519.PublicKey), transcript, signature) {
		return errors.New("test signature verification failed")
	}
	return nil
}
func acceptTestAuth(t *testing.T, ctx context.Context, conn *websocket.Conn) bool {
	t.Helper()
	hello, challenge, err := testChallenge(ctx, conn)
	if err != nil {
		return false
	}
	if err := testVerifyProof(ctx, conn, hello, challenge); err != nil {
		t.Error(err)
		return false
	}
	if err := wsjson.Write(ctx, conn, agentauth.OK{Type: "auth_ok", Version: agentauth.Version}); err != nil {
		return false
	}
	return true
}

func TestAuthenticationWaitsForOKAndClearsKey(t *testing.T) {
	passed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		hello, challenge, err := testChallenge(r.Context(), conn)
		if err != nil {
			return
		}
		if err := testVerifyProof(r.Context(), conn, hello, challenge); err != nil {
			t.Error(err)
			return
		}
		normal := make(chan map[string]string, 1)
		go func() {
			var msg map[string]string
			if wsjson.Read(r.Context(), conn, &msg) == nil {
				normal <- msg
			}
		}()
		select {
		case <-normal:
			t.Error("normal message before auth_ok")
			return
		case <-time.After(50 * time.Millisecond):
		}
		_ = wsjson.Write(r.Context(), conn, agentauth.OK{Type: "auth_ok", Version: agentauth.Version})
		select {
		case msg := <-normal:
			if msg["id"] != clientAuthTestID {
				t.Error("initial metadata changed")
			}
			passed <- struct{}{}
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	c := New("ws"+strings.TrimPrefix(server.URL, "http"), time.Hour, time.Hour, time.Second)
	defer c.Close()
	key := clientAuthTestKey()
	c.ConfigureAuthentication(clientAuthTestID, func() (ed25519.PrivateKey, error) { return key, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = c.runConnection(ctx, map[string]string{"id": clientAuthTestID}, nil, nil, nil, nil)
	select {
	case <-passed:
	case <-ctx.Done():
		t.Fatal("authenticated initial message missing")
	}
	for _, b := range key {
		if b != 0 {
			t.Fatal("private key buffer not cleared")
		}
	}
}

func TestAuthenticationPrivateKeyAndChallengeFailures(t *testing.T) {
	for _, scenario := range []string{"load failure", "migration required", "short key", "bad version", "bad nonce", "wrong sequence", "missing auth_ok", "wrong auth_ok"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				hello, challenge, err := testChallenge(r.Context(), conn)
				if err != nil {
					return
				}
				switch scenario {
				case "bad version":
					challenge.Version = "V0"
				case "bad nonce":
					challenge.ServerNonce = base64.StdEncoding.EncodeToString([]byte("short"))
				case "wrong sequence":
					challenge.Type = "auth_ok"
				}
				_ = wsjson.Write(r.Context(), conn, challenge)
				var proof agentauth.Proof
				if wsjson.Read(r.Context(), conn, &proof) != nil {
					return
				}
				if scenario == "wrong auth_ok" {
					_ = wsjson.Write(r.Context(), conn, agentauth.OK{Type: "auth_ok", Version: "V0"})
				}
				_ = hello
			}))
			defer server.Close()
			c := New("ws"+strings.TrimPrefix(server.URL, "http"), time.Hour, time.Hour, time.Second)
			defer c.Close()
			c.ConfigureAuthentication(clientAuthTestID, func() (ed25519.PrivateKey, error) {
				switch scenario {
				case "load failure":
					return nil, errors.New("SECRET_LOADER_DETAIL")
				case "migration required":
					return nil, ErrMigrationRequired
				case "short key":
					return make([]byte, 32), nil
				}
				return clientAuthTestKey(), nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := c.runConnection(ctx, map[string]string{"id": clientAuthTestID}, nil, nil, nil, nil)
			if err == nil || c.connection != nil {
				t.Fatal("authentication failure entered normal session")
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("private loader detail leaked")
			}
			if scenario == "migration required" && !strings.Contains(err.Error(), "migration") {
				t.Fatal("migration diagnostic lost")
			}
		})
	}
}

func TestAuthHelloNonceFreshPerConnection(t *testing.T) {
	nonces := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		var hello agentauth.Hello
		if wsjson.Read(r.Context(), conn, &hello) == nil {
			nonces <- hello.ClientNonce
		}
	}))
	defer server.Close()
	c := New("ws"+strings.TrimPrefix(server.URL, "http"), time.Hour, time.Hour, time.Second)
	defer c.Close()
	configureTestAuth(c)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for range 2 {
		_ = c.runConnection(ctx, nil, nil, nil, nil, nil)
	}
	first, second := <-nonces, <-nonces
	if first == second {
		t.Fatal("client nonce reused")
	}
	if _, err := agentauth.Decode(first, 32); err != nil {
		t.Fatal(err)
	}
}
