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

package buildimage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseShimImagesMergesImageLists(t *testing.T) {
	dir := t.TempDir()
	lists := map[string]string{
		"base.txt":  "registry.example.com/pause:3.10\n# comment\n\n  \"registry.example.com/coredns:v1.11\"  \n",
		"extra.txt": "registry.example.com/pause:3.10\nregistry.example.com/lvscare:v1\n",
	}
	for name, content := range lists {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, err := ParseShimImages(dir)

	if err != nil {
		t.Fatalf("parse shim images: %v", err)
	}
	want := []string{"registry.example.com/coredns:v1.11", "registry.example.com/lvscare:v1", "registry.example.com/pause:3.10"}
	if !slices.Equal(got, want) {
		t.Fatalf("ParseShimImages() = %q, want %q", got, want)
	}
}
