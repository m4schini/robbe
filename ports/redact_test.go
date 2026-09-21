// SPDX-License-Identifier: TODO

package ports_test

import (
	"testing"

	"github.com/m4schini/robbe/ports"
)

func TestRedactURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "https with password", in: "https://user:s3cret@example.com/x.git", want: "https://user:xxxxx@example.com/x.git"},
		{name: "https without userinfo", in: "https://example.com/x.git", want: "https://example.com/x.git"},
		{name: "https with user only", in: "https://user@example.com/x.git", want: "https://user@example.com/x.git"},
		{name: "scp-like unchanged", in: "git@github.com:me/x.git", want: "git@github.com:me/x.git"},
		{name: "ssh with user unchanged", in: "ssh://git@example.com/x.git", want: "ssh://git@example.com/x.git"},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ports.RedactURL(tt.in); got != tt.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
