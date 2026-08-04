// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"testing"
	"time"

	"github.com/captv89/ovl/vessel/bootstrap"
)

func TestNextNightlyRun(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before today's run time schedules later today",
			now:  time.Date(2026, 7, 5, 0, 30, 0, 0, time.UTC),
			want: time.Date(2026, 7, 5, 2, 0, 0, 0, time.UTC),
		},
		{
			name: "exactly at the run time schedules tomorrow (not now)",
			now:  time.Date(2026, 7, 5, 2, 0, 0, 0, time.UTC),
			want: time.Date(2026, 7, 6, 2, 0, 0, 0, time.UTC),
		},
		{
			name: "after today's run time schedules tomorrow",
			now:  time.Date(2026, 7, 5, 14, 0, 0, 0, time.UTC),
			want: time.Date(2026, 7, 6, 2, 0, 0, 0, time.UTC),
		},
		{
			name: "crosses a month boundary correctly",
			now:  time.Date(2026, 1, 31, 23, 0, 0, 0, time.UTC),
			want: time.Date(2026, 2, 1, 2, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextNightlyRun(tt.now)
			if !got.Equal(tt.want) {
				t.Errorf("nextNightlyRun(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestBindAddr(t *testing.T) {
	tests := []struct {
		name string
		cfg  *bootstrap.Config
		want string
	}{
		{
			name: "nil config (before wizard step 1) binds loopback only",
			cfg:  nil,
			want: "127.0.0.1:8420",
		},
		{
			name: "standalone mode binds loopback only",
			cfg:  &bootstrap.Config{Mode: bootstrap.ModeStandalone},
			want: "127.0.0.1:8420",
		},
		{
			name: "server mode binds every interface",
			cfg:  &bootstrap.Config{Mode: bootstrap.ModeServer},
			want: "0.0.0.0:8420",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bindAddr(tt.cfg, 8420); got != tt.want {
				t.Errorf("bindAddr(%+v, 8420) = %q, want %q", tt.cfg, got, tt.want)
			}
		})
	}
}
