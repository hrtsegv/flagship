package flagship

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type configurationPayload struct {
	Version     *int   `json:"version"`
	Environment string `json:"environment"`
	Flags       []struct {
		Key     string `json:"key"`
		Enabled *bool  `json:"enabled"`
	} `json:"flags"`
}

// FetchSnapshot fetches an environment's current configuration from a
// running Flagship server and builds a Snapshot from it. httpClient may be
// nil, in which case http.DefaultClient is used.
//
// This is the sync protocol's one authoritative source of full state: it's
// used both to bootstrap a Client and, when Client.ApplyEvent reports
// ErrVersionGap, to recover from one.
func FetchSnapshot(ctx context.Context, httpClient *http.Client, baseURL, environmentID string) (*Snapshot, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	endpoint, err := url.JoinPath(baseURL, "api/v1/environments", environmentID, "configuration")
	if err != nil {
		return nil, fmt.Errorf("flagship: building configuration URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("flagship: building configuration request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("flagship: fetching configuration: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flagship: fetching configuration: unexpected status %d", resp.StatusCode)
	}

	var payload configurationPayload
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("flagship: decoding configuration: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("flagship: configuration must contain one JSON value")
	}
	if payload.Version == nil || *payload.Version < 0 || strings.TrimSpace(payload.Environment) == "" || payload.Flags == nil {
		return nil, errors.New("flagship: configuration requires a nonnegative version, an environment, and a flags array")
	}

	flags := make([]Flag, 0, len(payload.Flags))
	keys := make(map[string]struct{}, len(payload.Flags))
	for _, f := range payload.Flags {
		if strings.TrimSpace(f.Key) == "" || f.Enabled == nil {
			return nil, errors.New("flagship: each configuration flag requires a key and a boolean value")
		}
		if _, exists := keys[f.Key]; exists {
			return nil, fmt.Errorf("flagship: configuration contains duplicate flag key %q", f.Key)
		}
		keys[f.Key] = struct{}{}
		flags = append(flags, Flag{Key: f.Key, Enabled: *f.Enabled})
	}

	return NewSnapshot(payload.Environment, *payload.Version, flags), nil
}
