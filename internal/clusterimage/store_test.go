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
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/content/oci"
	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/oras-project/oras-go/v3/registry/remote/config"
	"github.com/oras-project/oras-go/v3/registry/remote/properties"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterimage/clusterimagetest"
)

// testRegistry is an in-process registry that counts the requests it serves.
type testRegistry struct {
	server   *httptest.Server
	host     string
	requests atomic.Int64
}

// startRegistry serves a registry with newServer, httptest.NewTLSServer for HTTPS with a
// self-signed certificate or httptest.NewServer for plain HTTP.
func startRegistry(t *testing.T, newServer func(http.Handler) *httptest.Server) *testRegistry {
	t.Helper()
	reg := &testRegistry{}
	handler := registry.New()
	reg.server = newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reg.requests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(reg.server.Close)
	serverURL, err := url.Parse(reg.server.URL)
	if err != nil {
		t.Fatalf("parse registry URL: %v", err)
	}
	reg.host = serverURL.Host
	return reg
}

// push pushes a two-platform index of rootfs images to repository:tag.
func (r *testRegistry) push(t *testing.T, repository, tag string) {
	t.Helper()
	props, err := properties.NewRegistry(r.host + "/" + repository + ":" + tag)
	if err != nil {
		t.Fatalf("registry properties: %v", err)
	}
	props.Transport.PlainHTTP = strings.HasPrefix(r.server.URL, "http://")
	props.Transport.Insecure = true
	repo, err := remote.NewClientBuilder().NewRepository(props)
	if err != nil {
		t.Fatalf("remote repository: %v", err)
	}
	amd64 := clusterimagetest.Push(t, repo, clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.File("cpaas/arch", "amd64\n", 0o644))))
	arm64 := clusterimagetest.Push(t, repo, clusterimagetest.NewImage(arm64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.File("cpaas/arch", "arm64\n", 0o644))))
	index := clusterimagetest.PushIndex(t, repo, amd64, arm64)
	if err := repo.Tag(testCtx, index, tag); err != nil {
		t.Fatalf("tag %s: %v", tag, err)
	}
}

// registriesConf writes content as registries.conf and returns the options loading it.
func registriesConf(t *testing.T, content string) config.LoadConfigsOptions {
	t.Helper()
	path := filepath.Join(t.TempDir(), "registries.conf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write registries.conf: %v", err)
	}
	return config.LoadConfigsOptions{RegistriesConfigPath: path}
}

func searchConf(t *testing.T, host string) config.LoadConfigsOptions {
	t.Helper()
	return registriesConf(t, fmt.Sprintf("unqualified-search-registries = [%q]\n", host))
}

func TestPullStoresRequestedPlatformAndImageStaysLocal(t *testing.T) {
	reg := startRegistry(t, httptest.NewTLSServer)
	reg.push(t, "cluster/kubernetes", "v1")
	store := openTestStore(t, searchConf(t, reg.host))
	name := reg.host + "/cluster/kubernetes:v1"
	skipVerify := Transport{SkipTLSVerify: true}

	pulled, err := store.Pull(testCtx, name, arm64Linux, skipVerify)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	cfg, err := store.Config(testCtx, pulled)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if pulled.Name != name || cfg.Architecture != "arm64" {
		t.Fatalf("pulled %s for %s, want %s for arm64", pulled.Name, cfg.Architecture, name)
	}

	requests := reg.requests.Load()
	local, err := store.Image(testCtx, "cluster/kubernetes:v1", arm64Linux)
	if err != nil {
		t.Fatalf("image by short name: %v", err)
	}
	if local.Name != pulled.Name || local.Manifest.Digest != pulled.Manifest.Digest {
		t.Fatalf("short name resolved to %+v, want %+v", local, pulled)
	}
	_, err = store.Image(testCtx, name, amd64Linux)
	if !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("image for amd64 error = %v, want %v", err, ErrImageNotFound)
	}
	if got := reg.requests.Load() - requests; got != 0 {
		t.Fatalf("local lookups sent %d registry requests", got)
	}

	other, err := store.Pull(testCtx, name, amd64Linux, skipVerify)
	if err != nil {
		t.Fatalf("pull amd64: %v", err)
	}
	if cfg, err := store.Config(testCtx, other); err != nil || cfg.Architecture != "amd64" {
		t.Fatalf("amd64 config = %+v, %v", cfg.Platform, err)
	}
}

func TestPullVerifiesTLSUnlessSkipped(t *testing.T) {
	reg := startRegistry(t, httptest.NewTLSServer)
	reg.push(t, "cluster/kubernetes", "v1")
	name := reg.host + "/cluster/kubernetes:v1"
	insecureConf := fmt.Sprintf("[[registry]]\nprefix = %q\nlocation = %q\ninsecure = true\n", reg.host, reg.host)
	tests := []struct {
		name      string
		conf      config.LoadConfigsOptions
		transport Transport
		wantErr   string
	}{
		{name: "verified by default", conf: noRegistries, wantErr: "certificate"},
		{name: "skipped by flag", conf: noRegistries, transport: Transport{SkipTLSVerify: true}},
		{name: "skipped by registries.conf", conf: registriesConf(t, insecureConf)},
		{name: "plain HTTP against TLS", conf: noRegistries, transport: Transport{PlainHTTP: true}, wantErr: "pull " + name},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestStore(t, tt.conf)

			_, err := store.Pull(testCtx, name, amd64Linux, tt.transport)

			if tt.wantErr == "" && err != nil {
				t.Fatalf("pull: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("pull error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestPullOverPlainHTTPOnlyWhenRequested(t *testing.T) {
	reg := startRegistry(t, httptest.NewServer)
	reg.push(t, "cluster/kubernetes", "v1")
	store := openTestStore(t, noRegistries)
	name := reg.host + "/cluster/kubernetes:v1"

	if _, err := store.Pull(testCtx, name, amd64Linux, Transport{}); err == nil {
		t.Fatalf("pull over HTTPS from an HTTP registry succeeded")
	}
	if _, err := store.Pull(testCtx, name, amd64Linux, Transport{PlainHTTP: true}); err != nil {
		t.Fatalf("pull over plain HTTP: %v", err)
	}
}

func TestPullReportsEveryCandidate(t *testing.T) {
	reg := startRegistry(t, httptest.NewTLSServer)
	store := openTestStore(t, searchConf(t, reg.host))

	_, err := store.Pull(testCtx, "cluster/missing:v1", amd64Linux, Transport{SkipTLSVerify: true})

	if err == nil || !strings.Contains(err.Error(), "pull "+reg.host+"/cluster/missing:v1") {
		t.Fatalf("pull error = %v, want it to name the searched candidate", err)
	}
}

func TestImageRejectsUnsearchableShortName(t *testing.T) {
	store := openTestStore(t, noRegistries)

	_, err := store.Image(testCtx, "cluster/kubernetes:v1", amd64Linux)

	if !errors.Is(err, ErrShortNameNotSearchable) {
		t.Fatalf("image error = %v, want %v", err, ErrShortNameNotSearchable)
	}
}

// writeArchive writes an OCI archive holding a two-platform index annotated by annotate.
func writeArchive(t *testing.T, annotate func(desc *ocispec.Descriptor)) string {
	t.Helper()
	layoutDir := t.TempDir()
	layout, err := oci.New(layoutDir)
	if err != nil {
		t.Fatalf("create layout: %v", err)
	}
	amd64 := clusterimagetest.Push(t, layout, clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.File("cpaas/arch", "amd64\n", 0o644))))
	arm64 := clusterimagetest.Push(t, layout, clusterimagetest.NewImage(arm64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.File("cpaas/arch", "arm64\n", 0o644))))
	index := clusterimagetest.PushIndex(t, layout, amd64, arm64)
	annotate(&index)
	indexJSON, err := json.Marshal(ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{index}})
	if err != nil {
		t.Fatalf("marshal archive index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), indexJSON, 0o644); err != nil {
		t.Fatalf("write archive index: %v", err)
	}
	archive := filepath.Join(t.TempDir(), "image.tar")
	tarDirectory(t, layoutDir, archive)
	return archive
}

func tarDirectory(t *testing.T, root, archive string) {
	t.Helper()
	out, err := os.Create(archive)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	writer := tar.NewWriter(out)
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == root {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := writer.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(writer, f)
		return err
	})
	if err != nil {
		t.Fatalf("tar %s: %v", root, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive tar: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

func TestImportStoresArchiveImagesByFullName(t *testing.T) {
	tests := []struct {
		name     string
		annotate func(desc *ocispec.Descriptor)
	}{
		{
			name: "containerd image name",
			annotate: func(desc *ocispec.Descriptor) {
				desc.Annotations = map[string]string{
					containerdImageNameAnnotation: "registry.example.com/cluster/kubernetes:v1",
					ocispec.AnnotationRefName:     "v1",
				}
			},
		},
		{
			name: "qualified ref name",
			annotate: func(desc *ocispec.Descriptor) {
				desc.Annotations = map[string]string{ocispec.AnnotationRefName: "registry.example.com/cluster/kubernetes:v1"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestStore(t, noRegistries)
			archive := writeArchive(t, tt.annotate)

			images, err := store.Import(testCtx, archive, arm64Linux)
			if err != nil {
				t.Fatalf("import: %v", err)
			}

			if len(images) != 1 || images[0].Name != "registry.example.com/cluster/kubernetes:v1" {
				t.Fatalf("imported %+v", images)
			}
			stored, err := store.Image(testCtx, images[0].Name, arm64Linux)
			if err != nil || stored.Name != images[0].Name || stored.Manifest.Digest != images[0].Manifest.Digest {
				t.Fatalf("stored image = %+v, %v; want %+v", stored, err, images[0])
			}
		})
	}
}

func TestImportRejectsUnnamedImages(t *testing.T) {
	store := openTestStore(t, noRegistries)
	archive := writeArchive(t, func(desc *ocispec.Descriptor) {
		desc.Annotations = map[string]string{ocispec.AnnotationRefName: "v1"}
	})

	_, err := store.Import(testCtx, archive, amd64Linux)

	if err == nil || !strings.Contains(err.Error(), "neither "+containerdImageNameAnnotation) {
		t.Fatalf("import error = %v, want a missing name error", err)
	}
}
