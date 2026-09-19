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

	info := getSystemInfoForAgent(config.AgentID)
	status, err := agentExists(ctx, apiBaseURL, config.AgentID)
	if err != nil {
		return model.SystemInfo{}, err
	}
	if status == http.StatusNoContent {
		log.Printf("registration: agent_id=%s already exists; skipping enrollment", info.ID)
		return info, nil
	}

	token, err := readRegistrationToken()
	if err != nil {
		return model.SystemInfo{}, err
	}
	if err := registerAgent(ctx, apiBaseURL, token, config, info); err != nil {
		return model.SystemInfo{}, err
	}
	log.Printf("registration: agent_id=%s enrolled successfully", info.ID)
	return info, nil
}

func agentExists(ctx context.Context, apiBaseURL, agentID string) (int, error) {
	endpoint := strings.TrimRight(apiBaseURL, "/") + "/api/agents/" + url.PathEscape(agentID) + "/exists"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build agent existence request: %w", err)
	}
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return 0, fmt.Errorf("check agent existence: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound {
		return response.StatusCode, nil
	}
	return 0, fmt.Errorf("check agent existence: unexpected HTTP status %s", response.Status)
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
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("register agent: %w", err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Never log the token or private key. The server body is retained only to
		// give developers the API error while debugging enrollment failures.
		return fmt.Errorf("register agent: HTTP %s: %s", response.Status, strings.TrimSpace(string(responseBody)))
	}
	var result registerResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("decode registration response: %w", err)
	}
	if result.ID == "" {
		return errors.New("register agent: response did not contain id")
	}
	return nil
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
