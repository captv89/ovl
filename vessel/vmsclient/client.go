// SPDX-License-Identifier: AGPL-3.0-only

// Package vmsclient talks to a vessel's configured VMS (voyage
// management system) reference-data REST service — the second contract
// of the sensor+VMS stub expansion (docs/superpowers/specs/2026-08-01-
// sensor-and-vms-stub-expansion-design.md). Vessel-initiated only, same
// communication philosophy as vessel/sensorclient (CLAUDE.md; office
// sync is the other example) — the external system only needs to expose
// its current voyage plan and cargo manifest, never OVD/schema
// knowledge, and never pushes anything to vessel unsolicited.
//
// Contract: GET {baseURL}/voyage-data?at=<RFC3339>, bearer API key,
// returns {"voyageData": {"<VMS field name>": <number|string|boolean>, ...}}.
// Unlike sensorclient's [from, to] window, this is a snapshot at one
// instant — voyage-plan and cargo-manifest data isn't a rate, it's
// "what's currently true" at the report's own EventTime. Field names
// are the VMS's own vocabulary; vessel/httpapi's vmsFieldMapFor
// translates those into curated OVD schema field names — this package
// knows nothing about schemas.
package vmsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client fetches VMS reference data for a configured base URL/API key.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// New returns a Client with a bounded-timeout http.Client — a stalled
// VMS must not hang the report form indefinitely. Trims whitespace off
// both baseURL and apiKey, same rationale as sensorclient.New.
func New(baseURL, apiKey string) *Client {
	return &Client{BaseURL: strings.TrimSpace(baseURL), APIKey: strings.TrimSpace(apiKey), HTTPClient: &http.Client{Timeout: 10 * time.Second}}
}

type voyageDataResponse struct {
	VoyageData map[string]any `json:"voyageData"`
}

// maxResponseBytes bounds how much of a VMS's response body this client
// will read — same rationale as sensorclient's own constant.
const maxResponseBytes = 1 << 20 // 1 MiB

// FetchVoyageData queries the VMS for its voyage/cargo snapshot at the
// instant `at` and returns its raw field-name -> value map, unmapped to
// any schema. A malformed or oversized response is a validation error
// (fmt.Errorf), not a panic — see this package's own doc comment on
// treating the configured service as untrusted input.
func (c *Client) FetchVoyageData(ctx context.Context, at time.Time) (map[string]any, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("vmsclient: parse base URL: %w", err)
	}
	u := base.JoinPath("voyage-data")
	q := u.Query()
	q.Set("at", at.UTC().Format(time.RFC3339))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("vmsclient: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vmsclient: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vmsclient: VMS returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("vmsclient: read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("vmsclient: response exceeds %d bytes", maxResponseBytes)
	}

	var parsed voyageDataResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("vmsclient: parse response: %w", err)
	}
	return parsed.VoyageData, nil
}
