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

package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadZotConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "rendered cluster image config",
			content: `{
  "distSpecVersion": "1.1.1",
  "storage": {"rootDirectory": "/var/lib/registry", "dedupe": true, "gc": false},
  "http": {"address": "0.0.0.0", "port": "5000", "compat": ["docker2s2"],
           "auth": {"htpasswd": {"path": "/etc/registry/registry_htpasswd"}}},
  "log": {"level": "info"}
}`,
		},
		{
			name:    "missing required fields",
			content: `{"storage": {"rootDirectory": "/var/lib/registry", "dedupe": true}, "http": {"address": "0.0.0.0", "port": "5000"}}`,
			wantErr: "missing required fields: storage.gc, http.auth.htpasswd.path",
		},
		{
			name:    "invalid json",
			content: `storage: {}`,
			wantErr: "parse registry config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "registry_config.json")
			if err := os.WriteFile(file, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := readZotConfig(file)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("readZotConfig() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("readZotConfig() error = %v", err)
			}
			if cfg.Storage.RootDirectory != "/var/lib/registry" || !*cfg.Storage.Dedupe || *cfg.Storage.GC ||
				cfg.HTTP.Address != "0.0.0.0" || cfg.HTTP.Port != "5000" ||
				cfg.HTTP.Auth.Htpasswd.Path != "/etc/registry/registry_htpasswd" {
				t.Fatalf("readZotConfig() = %+v, unexpected values", cfg)
			}
		})
	}
}
