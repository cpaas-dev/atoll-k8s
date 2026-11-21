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

package crane

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/moby/moby/api/types/registry"
)

func TestDefaultKeychainResolvesStoredKeys(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		image    string
		wantUser string
	}{
		{name: "Docker Hub credentials stored as docker.io", key: "docker.io", image: "nginx", wantUser: "user"},
		{
			name: "Docker Hub credentials stored under the legacy key", key: DefaultAuthKey,
			image: "library/nginx", wantUser: "user",
		},
		{name: "namespaced credentials", key: "quay.io/org", image: "quay.io/org/app:v1", wantUser: "user"},
		{name: "registry credentials", key: "127.0.0.1:5000", image: "127.0.0.1:5000/org/app:v1", wantUser: "user"},
		{name: "credentials of another namespace", key: "quay.io/other", image: "quay.io/org/app:v1", wantUser: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := name.ParseReference(tt.image)
			if err != nil {
				t.Fatalf("parse reference %s: %v", tt.image, err)
			}
			keychain := NewDefaultKeychain(map[string]registry.AuthConfig{tt.key: {Username: "user", Password: "secret"}})
			auth, err := keychain.Resolve(ref.Context())
			if err != nil {
				t.Fatalf("resolve %s: %v", ref.Context(), err)
			}
			cfg, err := auth.Authorization()
			if err != nil {
				t.Fatalf("authorization for %s: %v", ref.Context(), err)
			}
			if cfg.Username != tt.wantUser {
				t.Fatalf("username for %s with credentials under %q = %q, want %q", tt.image, tt.key, cfg.Username, tt.wantUser)
			}
		})
	}
}
