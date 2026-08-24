package flagship

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SyncConfig configures Sync.
type SyncConfig struct {
	// HTTPClient is used for both the stream connection and any snapshot
	// fetches Sync performs. Defaults to http.DefaultClient if nil.
	HTTPClient *http.Client

	BaseURL       string
	EnvironmentID string

	// OnError, if set, is called with every transient error Sync recovers
	// from (a failed connection, a dropped stream, a failed gap-recovery
	// fetch) before it retries. It is never called for the final error
	// Sync returns. A nil OnError is a no-op: Sync still retries, it just
	// doesn't report why.
	OnError func(error)

	// MinBackoff and MaxBackoff bound the randomized exponential backoff
	// between reconnect attempts. They default to 500ms and 30s.
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

func (cfg SyncConfig) validate() error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return errors.New("flagship: SyncConfig.BaseURL is required")
	}
	if strings.TrimSpace(cfg.EnvironmentID) == "" {
		return errors.New("flagship: SyncConfig.EnvironmentID is required")
	}
	return nil
}

// Sync is the SDK's side of the versioned push transport: it connects to
// the control plane's SSE stream, applies each ConfigEvent it receives to
// client via ApplyEvent, and recovers from any version gap by fetching a
// full snapshot, all without the caller managing any of that by hand.
// Most application code only needs Bool; Sync is what keeps the Snapshot
// behind it current.
//
// Sync blocks until ctx is canceled (or a permanent, non-retryable error
// occurs) and is meant to be run in its own goroutine:
//
//	go flagship.Sync(ctx, client, flagship.SyncConfig{
//	    BaseURL:       "http://localhost:8080",
//	    EnvironmentID: environmentID,
//	})
//
// Sync owns no goroutines of its own beyond the one it's called from --
// canceling ctx is what stops it, and Sync returns ctx.Err() when it
// does. A dropped connection, a failed fetch, or any other transient
// error is retried with jittered exponential backoff rather than ending
// Sync; the one exception is ErrWrongEnvironment, which signals a caller
// configuration mistake (client and stream pointed at different
// environments) that retrying cannot fix.
func Sync(ctx context.Context, client *Client, cfg SyncConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	minBackoff := cfg.MinBackoff
	if minBackoff <= 0 {
		minBackoff = 500 * time.Millisecond
	}
	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}

	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if client.snapshot.Load() == nil {
			snapshot, err := FetchSnapshot(ctx, httpClient, cfg.BaseURL, cfg.EnvironmentID)
			if err != nil {
				if err := ctx.Err(); err != nil {
					return err
				}
				notifySyncError(cfg.OnError, fmt.Errorf("flagship: bootstrap snapshot: %w", err))
				if !waitBackoff(ctx, backoffDuration(attempt, minBackoff, maxBackoff)) {
					return ctx.Err()
				}
				attempt++
				continue
			}
			client.UpdateSnapshot(snapshot)
		}

		connected := false
		err := connectAndStream(ctx, client, httpClient, cfg, func() { connected = true })
		if connected {
			attempt = 0
		}
		if errors.Is(err, ErrWrongEnvironment) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		notifySyncError(cfg.OnError, err)
		if !waitBackoff(ctx, backoffDuration(attempt, minBackoff, maxBackoff)) {
			return ctx.Err()
		}
		attempt++
	}
}

func notifySyncError(onError func(error), err error) {
	if onError != nil && err != nil {
		onError(err)
	}
}

// connectAndStream opens one SSE connection, resuming from the client's
// current version via Last-Event-ID, and applies events until the stream
// ends or ctx is canceled. onConnected is called once the server has
// accepted the connection, so the caller can reset its backoff -- a
// stream that fails after being open for a while shouldn't be penalized
// the same as one that never connects at all.
func connectAndStream(ctx context.Context, client *Client, httpClient *http.Client, cfg SyncConfig, onConnected func()) error {
	sinceVersion := 0
	if current := client.snapshot.Load(); current != nil {
		sinceVersion = current.version
	}

	endpoint, err := url.JoinPath(cfg.BaseURL, "api/v1/environments", cfg.EnvironmentID, "stream")
	if err != nil {
		return fmt.Errorf("flagship: building stream URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("flagship: building stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", strconv.Itoa(sinceVersion))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("flagship: connecting to stream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("flagship: connecting to stream: unexpected status %d", resp.StatusCode)
	}
	onConnected()

	reader := bufio.NewReader(resp.Body)
	for {
		frame, err := readSSEFrame(reader)
		if err != nil {
			return fmt.Errorf("flagship: reading stream: %w", err)
		}
		if frame.event != "configuration.updated" || frame.data == "" {
			continue // heartbeat, comment, or an event type this SDK doesn't know about
		}

		var payload configEventPayload
		if err := json.Unmarshal([]byte(frame.data), &payload); err != nil {
			return fmt.Errorf("flagship: decoding stream event: %w", err)
		}
		event := ConfigEvent{
			Environment: payload.Environment,
			Version:     payload.Version,
			Type:        ConfigEventType(payload.Type),
			FlagKey:     payload.FlagKey,
			Enabled:     payload.Enabled,
		}

		if _, err := client.ApplyEvent(event); err != nil {
			if !errors.Is(err, ErrVersionGap) {
				return err // includes ErrWrongEnvironment
			}
			recovered, ferr := FetchSnapshot(ctx, httpClient, cfg.BaseURL, cfg.EnvironmentID)
			if ferr != nil {
				return fmt.Errorf("flagship: recovering from version gap: %w", ferr)
			}
			client.UpdateSnapshot(recovered)
			// Whatever this same connection delivers next reconciles on
			// its own: events at or below the recovered version are now
			// stale and ApplyEvent silently ignores them, and once the
			// stream catches up to exactly current+1, sequential
			// application resumes -- no reconnect required.
		}
	}
}

// configEventPayload mirrors the server's internal/httpapi.configEventPayload.
// The two aren't the same Go type on purpose: this is the wire contract as
// the SDK understands it, independent of the control plane's implementation.
type configEventPayload struct {
	Environment string `json:"environment"`
	Version     int    `json:"version"`
	Type        string `json:"type"`
	FlagKey     string `json:"flag_key"`
	Enabled     bool   `json:"enabled"`
}
