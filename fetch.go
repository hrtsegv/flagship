package flagship

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type configurationPayload struct {
	Version     int    `json:"version"`
	Environment string `json:"environment"`
	Flags       []struct {
		Key     string `json:"key"`
		Enabled bool   `json:"enabled"`
	} `json:"flags"`
}

// FetchSnapshot fetches an environment's current configuration from a
// running Flagship server and builds a Snapshot from it. httpClient may be
// nil, in which case http.DefaultClient is used.
//
// This is the sync protocol's one authoritative source of full state (see
// ADR-0004): it's used both to bootstrap a Client and, when
// Client.ApplyEvent reports ErrVersionGap, to recover from one.
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flagship: fetching configuration: unexpected status %d", resp.StatusCode)
	}

	var payload configurationPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("flagship: decoding configuration: %w", err)
	}

	flags := make([]Flag, 0, len(payload.Flags))
	for _, f := range payload.Flags {
		flags = append(flags, Flag{Key: f.Key, Enabled: f.Enabled})
	}

	return NewSnapshot(payload.Environment, payload.Version, flags), nil
}
