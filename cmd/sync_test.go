// SPDX-License-Identifier: TODO

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/m4schini/robbe/app/sync"
	"github.com/m4schini/robbe/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestResolveRuntime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		runtime  string
		want     string
		wantWarn bool
	}{
		{name: "configured", runtime: "/run/user/1000/robbe", want: "/run/user/1000/robbe", wantWarn: false},
		{name: "unset falls back to cache", runtime: "", want: "/home/x/.cache/robbe", wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			core, logs := observer.New(zap.WarnLevel)

			var cfg config.Config
			cfg.Cache = "/home/x/.cache/robbe"
			cfg.Runtime = tt.runtime

			got := resolveRuntime(cfg, zap.New(core))
			if got != tt.want {
				t.Errorf("resolveRuntime() = %q, want %q", got, tt.want)
			}

			warnings := logs.FilterMessage("XDG_RUNTIME_DIR unset, lock falls back to cache").All()
			if (len(warnings) > 0) != tt.wantWarn {
				t.Errorf("warning logged = %v, want %v (logs: %+v)", len(warnings) > 0, tt.wantWarn, logs.All())
			}

			if tt.wantWarn && warnings[0].ContextMap()["path"] != "/home/x/.cache/robbe/lock" {
				t.Errorf("warning path = %v, want /home/x/.cache/robbe/lock", warnings[0].ContextMap()["path"])
			}
		})
	}
}

func TestPrintHeader_RedactsPassword(t *testing.T) {
	t.Parallel()

	var cfg config.Config
	cfg.Repo.URL = "https://user:s3cret@example.com/x.git"
	cfg.Repo.Ref = "main"

	var (
		out bytes.Buffer
		res sync.Result
	)

	printHeader(&out, cfg, res)

	if strings.Contains(out.String(), "s3cret") {
		t.Errorf("header leaks password:\n%s", out.String())
	}

	if !strings.Contains(out.String(), "repo    https://user:xxxxx@example.com/x.git ref=main") {
		t.Errorf("header missing redacted url:\n%s", out.String())
	}
}
