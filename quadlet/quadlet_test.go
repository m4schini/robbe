// SPDX-License-Identifier: TODO

package quadlet

import "testing"

func TestIsUnitFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rel  string
		want bool
	}{
		{"a.container", true},
		{"a.pod", true},
		{"a.volume", true},
		{"a.network", true},
		{"a.image", true},
		{"a.build", true},
		{"a.kube", true},
		{"a.env", false},
		{"Makefile", false},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			t.Parallel()

			if got := IsUnitFile(tt.rel); got != tt.want {
				t.Errorf("IsUnitFile(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestIsWorkload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rel  string
		want bool
	}{
		{"a.container", true},
		{"a.pod", true},
		{"a.volume", false},
		{"a.network", false},
		{"a.image", false},
		{"a.build", false},
		{"a.kube", false},
		{"a.env", false},
		{"dir/a.container", true},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			t.Parallel()

			if got := IsWorkload(tt.rel); got != tt.want {
				t.Errorf("IsWorkload(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestConventionalUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rel  string
		want string
	}{
		{"x.container", "x.service"},
		{"x.pod", "x-pod.service"},
		{"x.volume", "x-volume.service"},
		{"x.network", "x-network.service"},
		{"x.image", "x-image.service"},
		{"x.build", "x-build.service"},
		{"dir/x.container", "x.service"},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			t.Parallel()

			if got := ConventionalUnit(tt.rel); got != tt.want {
				t.Errorf("ConventionalUnit(%q) = %q, want %q", tt.rel, got, tt.want)
			}
		})
	}
}
