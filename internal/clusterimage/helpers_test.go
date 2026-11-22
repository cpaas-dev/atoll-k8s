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

package clusterimage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/registry/remote/config"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterimage/clusterimagetest"
)

var (
	amd64Linux   = ocispec.Platform{OS: "linux", Architecture: "amd64"}
	arm64Linux   = ocispec.Platform{OS: "linux", Architecture: "arm64"}
	testCtx      = context.Background()
	noRegistries = config.LoadConfigsOptions{}
)

// openTestStore opens a store in a temporary directory with the given client configuration.
func openTestStore(t *testing.T, opts config.LoadConfigsOptions) *Store {
	t.Helper()
	dir := t.TempDir()
	if opts.DockerConfigPath == "" {
		opts.DockerConfigPath = filepath.Join(dir, "absent", "config.json")
	}
	if opts.ContainersAuthPath == "" {
		opts.ContainersAuthPath = filepath.Join(dir, "absent", "auth.json")
	}
	if opts.RegistriesConfigPath == "" {
		opts.RegistriesConfigPath = filepath.Join(dir, "absent", "registries.conf")
	}
	if opts.PolicyConfigPath == "" {
		opts.PolicyConfigPath = filepath.Join(dir, "absent", "policy.json")
	}
	if opts.RegistriesDPath == "" {
		opts.RegistriesDPath = filepath.Join(dir, "absent", "registries.d")
	}
	if opts.CertsDirPaths == nil {
		opts.CertsDirPaths = []string{filepath.Join(dir, "absent", "certs.d")}
	}
	configs, err := config.LoadConfigsWithOptions(opts)
	if err != nil {
		t.Fatalf("load client configs: %v", err)
	}
	store, err := Open(testCtx, filepath.Join(dir, "images"), configs)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

// storeImage pushes img into the local store of s and tags it as name.
func storeImage(t *testing.T, s *Store, name string, img clusterimagetest.Image) Image {
	t.Helper()
	desc := clusterimagetest.Push(t, s.layout, img)
	if err := s.layout.Tag(testCtx, desc, name); err != nil {
		t.Fatalf("tag %s: %v", name, err)
	}
	return Image{Name: name, Manifest: desc}
}

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("export restores file owners and needs root")
	}
}
