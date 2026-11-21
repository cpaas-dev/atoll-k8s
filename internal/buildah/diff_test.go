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

	"go.podman.io/storage/pkg/archive"
)

func TestFilterPatchFileRequired(t *testing.T) {
	tests := []struct {
		change archive.Change
		want   bool
	}{
		{archive.Change{Path: "/registry/library/nginx/blobs/sha256/0a1b", Kind: archive.ChangeAdd}, true},
		{archive.Change{Path: "/registry/library/nginx/index.json", Kind: archive.ChangeModify}, true},
		{archive.Change{Path: "/registry/library/nginx/oci-layout", Kind: archive.ChangeModify}, true},
		{archive.Change{Path: "/registry/library/nginx/blobs/sha256/0a1b", Kind: archive.ChangeModify}, false},
		{archive.Change{Path: "/registry/library/nginx", Kind: archive.ChangeModify}, false},
		{archive.Change{Path: "/etc/kubeadm.yml", Kind: archive.ChangeModify}, true},
		{archive.Change{Path: "/charts/index.json", Kind: archive.ChangeModify}, true},
		{archive.Change{Path: "/registry/library/nginx/index.json", Kind: archive.ChangeDelete}, false},
	}
	for _, tt := range tests {
		if got := filterPatchFileRequired(tt.change); got != tt.want {
			t.Errorf("filterPatchFileRequired(%s) = %v, want %v", tt.change.String(), got, tt.want)
		}
	}
}
