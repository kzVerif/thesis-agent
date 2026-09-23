package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"ws-agent/internal/retry"
	"ws-agent/internal/transportpolicy"
	"ws-agent/model"
)

type registerRequest struct {
	Token      string       `json:"token"`
	AgentID    string       `json:"agent_id"`
	PublicKey  string       `json:"public_key"`
	Hostname   string       `json:"hostname"`
	MACAddress string       `json:"mac_address"`
	OSInfo     model.OSInfo `json:"os_info,omitempty"`
	RoomID     string       `json:"room_id,omitempty"`
}

type registerResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// EnsureRegistered verifies the persisted agent ID and performs first-time
// enrollment only when the API reports that the ID does not exist.
func EnsureRegistered(ctx context.Context, apiBaseURL string) (model.SystemInfo, error) {
	config, err := LoadOrCreateAgentConfig()
	if err != nil {
		return model.SystemInfo{}, err
	}

	return EnsureEnrollment(ctx, apiBaseURL, config, "enrollment_state.json", true)
}

// EnsureEnrollment reuses the existing REST contract. A historical marker never
// bypasses the server existence check, and unattended mode never reads stdin.
func EnsureEnrollment(ctx context.Context, apiBaseURL string, identity AgentConfig, markerPath string, interactive bool) (model.SystemInfo, error) {
	verified, err := ReadEnrollment(markerPath, identity, apiBaseURL)
	if err != nil {
		return model.SystemInfo{}, err
	}
	log.Printf("verifying enrollment; previously_verified=%t", verified)
	backoff := retry.New()
	for {
		if err := ctx.Err(); err != nil {
			return model.SystemInfo{}, err
		}
		status, err := agentExists(ctx, apiBaseURL, identity.AgentID)
		if err != nil {
			var temporary *temporaryAPIError
			if interactive || !errors.As(err, &temporary) {
				return model.SystemInfo{}, err
			}
			delay := backoff.Next()
			log.Printf("API unavailable (%s); enrollment verification pending; retry scheduled in %s", temporary.Error(), delay)
			if err := retry.Wait(ctx, delay); err != nil {
				return model.SystemInfo{}, err
			}
			continue
		}
		info := getSystemInfoForAgent(identity.AgentID)
		if status == http.StatusNotFound {
			if err := WriteEnrollment(markerPath, identity, apiBaseURL, false); err != nil {
				return model.SystemInfo{}, err
			}
			if !interactive {
				return model.SystemInfo{}, fmt.Errorf("agent is not enrolled on configured server; administrative provisioning required")
			}
			stop := context.AfterFunc(ctx, func() { _ = os.Stdin.Close() })
			token, err := readRegistrationToken()
			stop()
			if err != nil {
				return model.SystemInfo{}, err
			}
			if err := registerAgent(ctx, apiBaseURL, token, identity, info); err != nil {
				return model.SystemInfo{}, err
			}
		}
		if err := WriteEnrollment(markerPath, identity, apiBaseURL, true); err != nil {
			return model.SystemInfo{}, err
		}
		log.Printf("enrollment verified")
		return info, nil
	}
}

type temporaryAPIError struct{ cause error }

func (e *temporaryAPIError) Error() string { return transportpolicy.FailureKind(e.cause) }
func (e *temporaryAPIError) Unwrap() error { return e.cause }

func agentExists(ctx context.Context, apiBaseURL, agentID string) (int, error) {
	endpoint := strings.TrimRight(apiBaseURL, "/") + "/api/agents/" + url.PathEscape(agentID) + "/exists"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build agent existence request: %w", err)
	}
	response, err := transportpolicy.NewClient(15 * time.Second).Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, &temporaryAPIError{cause: err}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound {
		return response.StatusCode, nil
	}
	if response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode >= 500 {
		return 0, &temporaryAPIError{}
	}
	return 0, fmt.Errorf("check agent existence: HTTP %d; check service configuration", response.StatusCode)
}

func readRegistrationToken() (string, error) {
	fmt.Fprint(os.Stdout, "Agent registration token: ")
	reader := bufio.NewReader(os.Stdin)
	token, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read registration token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("registration token cannot be empty")
	}
	return token, nil
}

func registerAgent(ctx context.Context, apiBaseURL, token string, config AgentConfig, info model.SystemInfo) error {
	payload := registerRequest{
		Token:      token,
		AgentID:    config.AgentID,
		PublicKey:  config.PublicKey,
		Hostname:   info.Hostname,
		MACAddress: info.MACAddress,
		OSInfo:     info.OSInfo,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode registration request: %w", err)
	}
	endpoint := strings.TrimRight(apiBaseURL, "/") + "/api/agents/register"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build registration request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := transportpolicy.NewClient(20 * time.Second).Do(request)
	if err != nil {
		return fmt.Errorf("registration request failed (%s); verify enrollment with the existing identity before retrying", transportpolicy.FailureKind(err))
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if registerResponseDebugEnabled() {
		body := strings.TrimSpace(string(responseBody))
		if len(body) > 16<<10 {
			body = body[:16<<10] + "...[truncated]"
		}
		log.Printf("DEBUG register response: status=%d content_type=%q body=%q", response.StatusCode, response.Header.Get("Content-Type"), body)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("register agent: HTTP %d; check token validity, quota and existing registration", response.StatusCode)
	}
	var result registerResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("decode registration response: %w", err)
	}
	if result.ID != config.AgentID {
		return errors.New("register agent: response identity does not match; administrator verification required")
	}
	return nil
}

func registerResponseDebugEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_DEBUG_REGISTER_RESPONSE")))
	return value == "1" || value == "true" || value == "yes"
}

func getSystemInfoForAgent(agentID string) model.SystemInfo {
	info := model.SystemInfo{ID: agentID}
	info.OSInfo = getOSInfo()
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}
	info.IPAddress, info.MACAddress = getPrimaryNetwork()
	return info
}
