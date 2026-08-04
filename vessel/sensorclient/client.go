// SPDX-License-Identifier: AGPL-3.0-only

// Package sensorclient talks to a vessel's configured onboard sensor-data
// REST service (18.07.26 manual-test items 4/9). Vessel-initiated only,
// matching this codebase's standing communication philosophy (CLAUDE.md;
// office sync is the other example) — the external system only needs to
// expose raw readings for a queried time window, never OVD/schema
// knowledge, and never pushes anything to vessel unsolicited.
//
// Contract: GET {baseURL}/readings?from=<RFC3339>&to=<RFC3339>, bearer
// API key, returns {"readings": {"<sensor field name>": <number, boolean,
// or string>, ...}}. Field names are the sensor service's own vocabulary
// (e.g. "latitude", "speed_kn") — vessel/httpapi's sensorFieldMapFor is
// what translates those into curated OVD schema field names; this
// package knows nothing about schemas.
package sensorclient

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

// Client fetches sensor readings for a configured base URL/API key.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// New returns a Client with a bounded-timeout http.Client — a stalled
// sensor service must not hang the report form indefinitely. Trims
// whitespace off both baseURL and apiKey: a stray trailing space is
// otherwise an opaque url.Parse failure ("invalid port \":8422 \" after
// host") rather than a clear "check your sensor source config" — worth
// tolerating here regardless of whether the value came from a freshly
// submitted form (already trimmed server-side) or a config saved before
// that trimming existed.
func New(baseURL, apiKey string) *Client {
	return &Client{BaseURL: strings.TrimSpace(baseURL), APIKey: strings.TrimSpace(apiKey), HTTPClient: &http.Client{Timeout: 10 * time.Second}}
}

type readingsResponse struct {
	Readings map[string]any `json:"readings"`
}

// maxResponseBytes bounds how much of a sensor service's response body
// this client will read — an external service (even one the operator
// configured themselves) is untrusted input; an oversized or malformed
// response must not exhaust vessel memory.
const maxResponseBytes = 1 << 20 // 1 MiB — generous for a flat readings map, nowhere near "reasonable" for anything else

// FetchReadings queries the sensor service for the window [from, to] and
// returns its raw field-name -> value map, unmapped to any schema. A
// malformed or oversized response is a validation error (fmt.Errorf), not
// a panic — see this package's own doc comment on treating the
// configured service as untrusted input. Values are number, boolean, or
// string (json.Unmarshal into `any` yields float64/bool/string natively
// for those JSON types) — see vessel/httpapi's sensorFieldMapFor for how
// each is mapped onto a schema field.
func (c *Client) FetchReadings(ctx context.Context, from, to time.Time) (map[string]any, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("sensorclient: parse base URL: %w", err)
	}
	u := base.JoinPath("readings")
	q := u.Query()
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("sensorclient: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sensorclient: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sensorclient: sensor service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("sensorclient: read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("sensorclient: response exceeds %d bytes", maxResponseBytes)
	}

	var parsed readingsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("sensorclient: parse response: %w", err)
	}
	return parsed.Readings, nil
}
