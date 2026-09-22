package client

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-agent/internal/agentauth"
)

var ErrMigrationRequired = errors.New("legacy private-key protection requires explicit migration before Agent authentication")
var ErrPrivateKeyUnavailable = errors.New("private key unavailable; verify existing identity, protection and runtime ACL")

type PrivateKeyLoader func() (ed25519.PrivateKey, error)
type authenticationError struct{ reason string }

func (e *authenticationError) Error() string { return e.reason }

// ConfigureAuthentication must be called before Run. The runtime supplies the
// authoritative Phase 2 loader; no decrypt, identity creation or fallback lives here.
func (c *Client) ConfigureAuthentication(id string, load PrivateKeyLoader) {
	c.agentID = id
	c.loadPrivateKey = load
}

func (c *Client) authenticate(ctx context.Context, conn *websocket.Conn) error {
	authCtx, cancel := context.WithTimeout(ctx, agentauth.Timeout)
	defer cancel()
	conn.SetReadLimit(agentauth.MessageLimit)
	defer conn.SetReadLimit(32768) // coder/websocket's existing client read limit.
	fail := func() error {
		return &authenticationError{"Agent authentication failed; check server credential and protocol"}
	}
	id, err := agentauth.CanonicalID(c.agentID)
	if err != nil || c.loadPrivateKey == nil {
		return &authenticationError{"Agent authentication identity/loader is not configured"}
	}
	nonce, err := agentauth.Random()
	if err != nil {
		return fail()
	}
	if err := wsjson.Write(authCtx, conn, agentauth.Hello{Type: "auth_hello", Version: agentauth.Version, AgentID: id, ClientNonce: nonce}); err != nil {
		return fail()
	}
	var challenge agentauth.Challenge
	if err := agentauth.Read(authCtx, conn, &challenge); err != nil {
		return fail()
	}
	if challenge.Type != "auth_challenge" || challenge.Version != agentauth.Version {
		return fail()
	}
	transcript, err := agentauth.Transcript(id, challenge.ChallengeID, nonce, challenge.ServerNonce)
	if err != nil {
		return fail()
	}
	key, err := c.loadPrivateKey()
	defer clear(key)
	if err != nil {
		if errors.Is(err, ErrMigrationRequired) {
			return &authenticationError{ErrMigrationRequired.Error()}
		}
		return &authenticationError{ErrPrivateKeyUnavailable.Error()}
	}
	if len(key) != ed25519.PrivateKeySize || authCtx.Err() != nil {
		return &authenticationError{ErrPrivateKeyUnavailable.Error()}
	}
	signature := ed25519.Sign(key, transcript)
	clear(key)
	proof := agentauth.Proof{Type: "auth_proof", Version: agentauth.Version, ChallengeID: challenge.ChallengeID, Signature: base64.StdEncoding.EncodeToString(signature)}
	if err := wsjson.Write(authCtx, conn, proof); err != nil {
		return fail()
	}
	var ok agentauth.OK
	if err := agentauth.Read(authCtx, conn, &ok); err != nil {
		return fail()
	}
	if ok.Type != "auth_ok" || ok.Version != agentauth.Version || authCtx.Err() != nil {
		return fail()
	}
	return nil
}
