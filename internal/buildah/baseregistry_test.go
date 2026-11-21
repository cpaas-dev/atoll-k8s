// Copyright © 2026 atoll.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package buildah

import (
	"testing"

	"go.podman.io/buildah/define"
	"go.podman.io/image/v5/types"
)

func TestFinalStageBase(t *testing.T) {
	const multiStage = "FROM builder:v1 AS build\nRUN make\nFROM app:v1\nCOPY --from=build /out /out\n"
	tests := []struct {
		name          string
		containerfile string
		options       define.BuildOptions
		want          string
		wantErr       bool
	}{
		{
			name:          "single stage",
			containerfile: "FROM labring/kubernetes:v1.25\nCOPY registry ./registry\n",
			want:          "labring/kubernetes:v1.25",
		},
		{
			name:          "scratch",
			containerfile: "FROM scratch\nCOPY . .\n",
			want:          "scratch",
		},
		{
			name:          "build arg overrides heading arg",
			containerfile: "ARG VERSION=v1\nFROM base:${VERSION}\n",
			options:       define.BuildOptions{Args: map[string]string{"VERSION": "v2"}},
			want:          "base:v2",
		},
		{
			name:          "selected platform",
			containerfile: "FROM base:v1-${TARGETARCH}\n",
			options:       define.BuildOptions{SystemContext: &types.SystemContext{OSChoice: "linux", ArchitectureChoice: "arm64"}},
			want:          "base:v1-arm64",
		},
		{
			name:          "last stage",
			containerfile: multiStage,
			want:          "app:v1",
		},
		{
			name:          "target stage",
			containerfile: multiStage,
			options:       define.BuildOptions{Target: "build"},
			want:          "builder:v1",
		},
		{
			name:          "from flag replaces the first FROM",
			containerfile: "FROM base:v1\nCOPY registry ./registry\n",
			options:       define.BuildOptions{From: "base:v2"},
			want:          "base:v2",
		},
		{
			name:          "earlier stage",
			containerfile: "FROM base:v1 AS base\nFROM base\n",
			wantErr:       true,
		},
		{
			name:          "transport",
			containerfile: "FROM oci-archive:/tmp/base.tar\n",
			wantErr:       true,
		},
		{
			name:          "platform flag",
			containerfile: "FROM --platform=linux/arm64 base:v1\n",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := finalStageBase([][]byte{[]byte(tt.containerfile)}, tt.options)
			if (err != nil) != tt.wantErr {
				t.Fatalf("finalStageBase() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("finalStageBase() = %q, want %q", got, tt.want)
			}
		})
	}
}
