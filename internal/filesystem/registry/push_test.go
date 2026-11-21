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

package registry

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/go-containerregistry/pkg/v1/validate"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
)

// testAuth is the only account of the test registries.
var testAuth = &authn.Basic{Username: "admin", Password: "passw0rd"}

// newRegistryHandler returns an in-memory OCI registry that requires basic auth with testAuth.
func newRegistryHandler() http.Handler {
	reg := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != testAuth.Username || password != testAuth.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		reg.ServeHTTP(w, r)
	})
}

// serve serves handler over plain HTTP until the test ends and returns its address.
func serve(t *testing.T, handler http.Handler) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.Listener.Addr().String()
}

// newTestTarget returns the target for the registry at address with millisecond retries.
func newTestTarget(address string) target {
	return target{
		address:   address,
		auth:      testAuth,
		transport: newTransport(),
		backoff:   remote.Backoff{Duration: time.Millisecond, Factor: 2, Steps: 3},
	}
}

// newMount returns a mounted image whose registry dir is filled by write.
func newMount(t *testing.T, image string, write func(root string) error) v2.MountImage {
	t.Helper()
	mountPoint := t.TempDir()
	if err := write(filepath.Join(mountPoint, constants.RegistryDirName)); err != nil {
		t.Fatalf("write registry dir of %s: %v", image, err)
	}
	return v2.MountImage{ImageName: image, MountPoint: mountPoint}
}

// push pushes the registry roots of mounts to targets.
func push(t *testing.T, mounts []v2.MountImage, targets ...target) error {
	t.Helper()
	roots, err := registryRoots(mounts)
	if err != nil {
		t.Fatalf("registry roots: %v", err)
	}
	return pushToTargets(t.Context(), roots, targets)
}

// newDockerImage returns a random image with a Docker schema2 manifest.
func newDockerImage(t *testing.T) v1.Image {
	t.Helper()
	img, err := random.Image(256, 2)
	if err != nil {
		t.Fatalf("create random image: %v", err)
	}
	return mutate.MediaType(img, types.DockerManifestSchema2)
}

// newOCIImage returns a random image with an OCI image manifest.
func newOCIImage(t *testing.T) v1.Image {
	t.Helper()
	return mutate.ConfigMediaType(mutate.MediaType(newDockerImage(t), types.OCIManifestSchema1), types.OCIConfigJSON)
}

// newDockerManifestList returns a Docker manifest list of linux/amd64 and linux/arm64 images.
func newDockerManifestList(t *testing.T) v1.ImageIndex {
	t.Helper()
	var addenda []mutate.IndexAddendum
	for _, arch := range []string{"amd64", "arm64"} {
		addenda = append(addenda, newPlatformAddendum(t, arch))
	}
	return mutate.IndexMediaType(mutate.AppendManifests(empty.Index, addenda...), types.DockerManifestList)
}

// newPlatformAddendum returns a Docker schema2 image for linux/arch whose config and index
// descriptor both declare the platform.
func newPlatformAddendum(t *testing.T, arch string) mutate.IndexAddendum {
	t.Helper()
	img := newDockerImage(t)
	config, err := img.ConfigFile()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	config = config.DeepCopy()
	config.OS, config.Architecture = "linux", arch
	img, err = mutate.ConfigFile(img, config)
	if err != nil {
		t.Fatalf("set config platform: %v", err)
	}
	return mutate.IndexAddendum{
		Add:        img,
		Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: arch}},
	}
}

// newOCIIndex returns an OCI image index of one OCI image.
func newOCIIndex(t *testing.T) v1.ImageIndex {
	t.Helper()
	return mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: newOCIImage(t)})
}

// manifest is an image or an image index.
type manifest interface {
	partial.Describable
	RawManifest() ([]byte, error)
}

// mustDigest returns the digest of m.
func mustDigest(t *testing.T, m partial.Describable) v1.Hash {
	t.Helper()
	digest, err := m.Digest()
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}
	return digest
}

// requireManifest fails unless ref resolves to the exact bytes and media type of m and
// everything m references can be pulled.
func requireManifest(t *testing.T, ref string, m manifest) {
	t.Helper()
	parsed, err := name.ParseReference(ref, name.Insecure)
	if err != nil {
		t.Fatalf("parse %s: %v", ref, err)
	}
	desc, err := remote.Get(parsed, remote.WithAuth(testAuth), remote.WithContext(t.Context()))
	if err != nil {
		t.Fatalf("get %s: %v", ref, err)
	}
	raw, err := m.RawManifest()
	if err != nil {
		t.Fatalf("read raw manifest: %v", err)
	}
	mediaType, err := m.MediaType()
	if err != nil {
		t.Fatalf("read media type: %v", err)
	}
	if want := mustDigest(t, m); desc.Digest != want {
		t.Fatalf("%s has digest %s, want %s", ref, desc.Digest, want)
	}
	if !bytes.Equal(desc.Manifest, raw) || desc.MediaType != mediaType {
		t.Fatalf("%s serves a %s manifest that differs from the pushed %s manifest", ref, desc.MediaType, mediaType)
	}
	if mediaType.IsIndex() {
		idx, err := desc.ImageIndex()
		if err != nil {
			t.Fatalf("open index %s: %v", ref, err)
		}
		if err := validate.Index(idx); err != nil {
			t.Fatalf("validate index %s: %v", ref, err)
		}
		return
	}
	img, err := desc.Image()
	if err != nil {
		t.Fatalf("open image %s: %v", ref, err)
	}
	if err := validate.Image(img); err != nil {
		t.Fatalf("validate image %s: %v", ref, err)
	}
}

// requireTags fails unless repository on the registry at address has exactly tags.
func requireTags(t *testing.T, address, repository string, tags ...string) {
	t.Helper()
	repo, err := name.NewRepository(address+"/"+repository, name.Insecure)
	if err != nil {
		t.Fatalf("parse repository %s: %v", repository, err)
	}
	got, err := remote.List(repo, remote.WithAuth(testAuth), remote.WithContext(t.Context()))
	if err != nil {
		t.Fatalf("list tags of %s: %v", repo, err)
	}
	slices.Sort(got)
	slices.Sort(tags)
	if !slices.Equal(got, tags) {
		t.Fatalf("tags of %s = %v, want %v", repo, got, tags)
	}
}

func TestPushPreservesManifestsOnEveryRegistry(t *testing.T) {
	list := newDockerManifestList(t)
	ociImage := newOCIImage(t)
	untaggedImage := newDockerImage(t)
	untaggedIndex := newOCIIndex(t)
	pause := newDockerImage(t)
	mount := newMount(t, "cluster:v1", func(root string) error {
		return errors.Join(
			ocilayout.WriteIndex(root, "library/app", "v1", list),
			ocilayout.WriteImage(root, "library/app", "oci", ociImage),
			ocilayout.WriteUntaggedImage(root, "library/app", untaggedImage),
			ocilayout.WriteUntaggedIndex(root, "library/app", untaggedIndex),
			ocilayout.WriteImage(root, "registry.k8s.io/pause", "3.10", pause),
		)
	})
	addresses := []string{serve(t, newRegistryHandler()), serve(t, newRegistryHandler())}

	if err := push(t, []v2.MountImage{mount}, newTestTarget(addresses[0]), newTestTarget(addresses[1])); err != nil {
		t.Fatalf("push: %v", err)
	}

	listManifest, err := list.IndexManifest()
	if err != nil {
		t.Fatalf("parse manifest list: %v", err)
	}
	for _, address := range addresses {
		app := address + "/library/app"
		requireManifest(t, app+":v1", list)
		for _, child := range listManifest.Manifests {
			img, err := list.Image(child.Digest)
			if err != nil {
				t.Fatalf("open list child %s: %v", child.Digest, err)
			}
			requireManifest(t, app+"@"+child.Digest.String(), img)
		}
		requireManifest(t, app+":oci", ociImage)
		requireManifest(t, app+"@"+mustDigest(t, untaggedImage).String(), untaggedImage)
		requireManifest(t, app+"@"+mustDigest(t, untaggedIndex).String(), untaggedIndex)
		requireTags(t, address, "library/app", "oci", "v1")
		requireManifest(t, address+"/registry.k8s.io/pause:3.10", pause)
	}
}

func TestPushLaterMountWins(t *testing.T) {
	first, second, third := newDockerImage(t), newOCIImage(t), newDockerImage(t)
	mounts := []v2.MountImage{
		newMount(t, "cluster:v1", func(root string) error {
			return errors.Join(
				ocilayout.WriteImage(root, "library/app", "v1", first),
				ocilayout.WriteImage(root, "library/app", "stable", first),
			)
		}),
		newMount(t, "patch:v1", func(root string) error {
			return errors.Join(
				ocilayout.WriteImage(root, "library/app", "v1", second),
				ocilayout.WriteImage(root, "library/app", "v2", third),
			)
		}),
	}
	address := serve(t, newRegistryHandler())

	// The second sync re-points v1 to the first image and back, as a re-run does.
	for range 2 {
		if err := push(t, mounts, newTestTarget(address)); err != nil {
			t.Fatalf("push: %v", err)
		}
	}

	app := address + "/library/app"
	requireManifest(t, app+":v1", second)
	requireManifest(t, app+":stable", first)
	requireManifest(t, app+":v2", third)
	requireTags(t, address, "library/app", "stable", "v1", "v2")
}

func TestRegistryRoots(t *testing.T) {
	withoutRegistry := v2.MountImage{ImageName: "app:v1", MountPoint: t.TempDir()}
	withRegistry := newMount(t, "cluster:v1", func(root string) error {
		return ocilayout.WriteImage(root, "app", "v1", newDockerImage(t))
	})
	legacy := v2.MountImage{ImageName: "legacy:v1", MountPoint: t.TempDir()}
	legacyRoot := filepath.Join(legacy.MountPoint, constants.RegistryDirName)
	if err := os.MkdirAll(filepath.Join(legacyRoot, "docker", "registry", "v2", "repositories"), 0o755); err != nil {
		t.Fatalf("create legacy registry dir: %v", err)
	}

	roots, err := registryRoots([]v2.MountImage{withoutRegistry, withRegistry})
	if err != nil {
		t.Fatalf("registry roots: %v", err)
	}
	want := []registryRoot{{image: "cluster:v1", path: filepath.Join(withRegistry.MountPoint, constants.RegistryDirName)}}
	if !slices.Equal(roots, want) {
		t.Fatalf("registry roots = %+v, want %+v", roots, want)
	}

	_, err = registryRoots([]v2.MountImage{withRegistry, legacy})
	if !errors.Is(err, ocilayout.ErrLegacyLayout) {
		t.Fatalf("registry roots error = %v, want ErrLegacyLayout", err)
	}
	for _, want := range []string{"legacy:v1", legacyRoot} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("registry roots error %q does not name %s", err, want)
		}
	}
}

func TestPushNamesUnreachableRegistry(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	unreachable := closed.Listener.Addr().String()
	closed.Close()
	reachable := serve(t, newRegistryHandler())
	img := newDockerImage(t)
	mount := newMount(t, "cluster:v1", func(root string) error {
		return ocilayout.WriteImage(root, "app", "v1", img)
	})

	err := push(t, []v2.MountImage{mount}, newTestTarget(unreachable), newTestTarget(reachable))
	if err == nil {
		t.Fatal("push to an unreachable registry succeeded, want error")
	}
	if !strings.Contains(err.Error(), unreachable) || strings.Contains(err.Error(), reachable) {
		t.Fatalf("push error %q must name only the unreachable registry %s", err, unreachable)
	}
	requireManifest(t, reachable+"/app:v1", img)
}

func TestPushRetriesTransientFailures(t *testing.T) {
	handler := newRegistryHandler()
	var manifestPuts atomic.Int32
	address := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/") && manifestPuts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	img := newDockerImage(t)
	mount := newMount(t, "cluster:v1", func(root string) error {
		return ocilayout.WriteImage(root, "app", "v1", img)
	})

	if err := push(t, []v2.MountImage{mount}, newTestTarget(address)); err != nil {
		t.Fatalf("push: %v", err)
	}
	if got := manifestPuts.Load(); got != 2 {
		t.Fatalf("got %d manifest uploads, want 2 (one failure and one retry)", got)
	}
	requireManifest(t, address+"/app:v1", img)
}
