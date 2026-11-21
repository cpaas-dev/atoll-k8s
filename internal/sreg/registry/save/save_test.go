// Copyright © 2021 sealos.
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

package save

import (
	"bytes"
	"errors"
	"io"
	"log"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/go-containerregistry/pkg/v1/validate"
	"github.com/moby/moby/api/types/registry"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
)

// startRegistry serves handler over plain HTTP and returns its host:port.
func startRegistry(t *testing.T, handler http.Handler) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse registry URL %s: %v", server.URL, err)
	}
	return u.Host
}

// newRegistryHandler returns an in-memory OCI registry.
func newRegistryHandler() http.Handler {
	return gcrregistry.New(gcrregistry.Logger(log.New(io.Discard, "", 0)))
}

// newPlatformImage returns a random image with a Docker schema2 manifest whose config
// declares linux/arch.
func newPlatformImage(t *testing.T, arch string) v1.Image {
	t.Helper()
	img, err := random.Image(256, 1)
	if err != nil {
		t.Fatalf("create random image: %v", err)
	}
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
	return mutate.MediaType(img, types.DockerManifestSchema2)
}

// newOCIPlatformImage returns newPlatformImage with an OCI image manifest.
func newOCIPlatformImage(t *testing.T, arch string) v1.Image {
	t.Helper()
	img := mutate.MediaType(newPlatformImage(t, arch), types.OCIManifestSchema1)
	return mutate.ConfigMediaType(img, types.OCIConfigJSON)
}

// newDockerManifestList returns a Docker manifest list of linux/amd64 and linux/arm64
// entries without variant, and the arm64 image.
func newDockerManifestList(t *testing.T) (v1.ImageIndex, v1.Image) {
	t.Helper()
	amd64, arm64 := newPlatformImage(t, "amd64"), newPlatformImage(t, "arm64")
	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{
			Add:        amd64,
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}},
		},
		mutate.IndexAddendum{
			Add:        arm64,
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}},
		},
	)
	return mutate.IndexMediaType(idx, types.DockerManifestList), arm64
}

// newOCIIndex returns an OCI image index of linux/amd64 and linux/arm64/v8 OCI images.
func newOCIIndex(t *testing.T) v1.ImageIndex {
	t.Helper()
	return mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{
			Add:        newOCIPlatformImage(t, "amd64"),
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}},
		},
		mutate.IndexAddendum{
			Add:        newOCIPlatformImage(t, "arm64"),
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"}},
		},
	)
}

// pushImage pushes img to the registry reference ref.
func pushImage(t *testing.T, ref string, img v1.Image, options ...remote.Option) {
	t.Helper()
	if err := remote.Write(mustParseReference(t, ref), img, options...); err != nil {
		t.Fatalf("push image %s: %v", ref, err)
	}
}

// pushIndex pushes idx to the registry reference ref.
func pushIndex(t *testing.T, ref string, idx v1.ImageIndex) {
	t.Helper()
	if err := remote.WriteIndex(mustParseReference(t, ref), idx); err != nil {
		t.Fatalf("push index %s: %v", ref, err)
	}
}

// mustParseReference parses a registry reference.
func mustParseReference(t *testing.T, ref string) name.Reference {
	t.Helper()
	r, err := name.ParseReference(ref)
	if err != nil {
		t.Fatalf("parse reference %s: %v", ref, err)
	}
	return r
}

// mustDigest returns the digest of an image or an image index.
func mustDigest(t *testing.T, m interface{ Digest() (v1.Hash, error) }) v1.Hash {
	t.Helper()
	digest, err := m.Digest()
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}
	return digest
}

// mustSelectPlatforms returns SelectPlatforms(platforms).
func mustSelectPlatforms(t *testing.T, platforms ...ispec.Platform) Selection {
	t.Helper()
	selection, err := SelectPlatforms(platforms)
	if err != nil {
		t.Fatalf("select platforms %v: %v", platforms, err)
	}
	return selection
}

// storedManifests returns the tags (tag -> digest) and the untagged digests of repository.
func storedManifests(t *testing.T, root, repository string) (map[string]v1.Hash, []v1.Hash) {
	t.Helper()
	descs, err := ocilayout.Descriptors(root, repository)
	if err != nil {
		t.Fatalf("read descriptors of %s: %v", repository, err)
	}
	tags := map[string]v1.Hash{}
	var untagged []v1.Hash
	for _, desc := range descs {
		if tag, ok := desc.Annotations[ispec.AnnotationRefName]; ok {
			tags[tag] = desc.Digest
			continue
		}
		untagged = append(untagged, desc.Digest)
	}
	return tags, untagged
}

// requireTags fails unless repository has exactly the tags want.
func requireTags(t *testing.T, root, repository string, want map[string]v1.Hash) {
	t.Helper()
	if got, _ := storedManifests(t, root, repository); !maps.Equal(got, want) {
		t.Fatalf("tags of %s = %v, want %v", repository, got, want)
	}
}

// requireStoredBytes fails unless the manifest blob of m in repository equals its raw manifest.
func requireStoredBytes(t *testing.T, root, repository string, m interface {
	Digest() (v1.Hash, error)
	RawManifest() ([]byte, error)
},
) {
	t.Helper()
	digest := mustDigest(t, m)
	raw, err := m.RawManifest()
	if err != nil {
		t.Fatalf("read raw manifest: %v", err)
	}
	blob := filepath.Join(ocilayout.RepositoryDir(root, repository), ispec.ImageBlobsDir, digest.Algorithm, digest.Hex)
	stored, err := os.ReadFile(filepath.Clean(blob))
	if err != nil {
		t.Fatalf("read stored manifest %s: %v", digest, err)
	}
	if !bytes.Equal(stored, raw) {
		t.Fatalf("stored manifest %s differs from the source bytes", digest)
	}
}

func TestSaveRegistryImages(t *testing.T) {
	host := startRegistry(t, newRegistryHandler())
	repo := host + "/library/app"
	dockerImage := newPlatformImage(t, "amd64")
	pushImage(t, repo+":docker", dockerImage)
	list, arm64Image := newDockerManifestList(t)
	pushIndex(t, repo+":list", list)
	ociIndex := newOCIIndex(t)
	pushIndex(t, repo+":oci", ociIndex)
	imageDigest, listDigest := mustDigest(t, dockerImage), mustDigest(t, list)
	amd64 := ispec.Platform{OS: "linux", Architecture: "amd64"}
	arm64 := ispec.Platform{OS: "linux", Architecture: "arm64"}
	arm64v8 := ispec.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"}
	s390x := ispec.Platform{OS: "linux", Architecture: "s390x"}

	tests := []struct {
		name         string
		image        string
		selection    Selection
		wantTags     map[string]v1.Hash
		wantUntagged []v1.Hash
	}{
		{
			name:      "tag reference keeps the Docker schema2 manifest",
			image:     repo + ":docker",
			selection: mustSelectPlatforms(t, amd64),
			wantTags:  map[string]v1.Hash{"docker": imageDigest},
		},
		{
			name:      "docker transport prefix",
			image:     "docker://" + repo + ":docker",
			selection: mustSelectPlatforms(t, amd64),
			wantTags:  map[string]v1.Hash{"docker": imageDigest},
		},
		{
			name:         "digest reference is saved untagged",
			image:        repo + "@" + imageDigest.String(),
			selection:    mustSelectPlatforms(t, amd64),
			wantTags:     map[string]v1.Hash{},
			wantUntagged: []v1.Hash{imageDigest},
		},
		{
			name:         "tag and digest reference is saved by digest",
			image:        repo + ":docker@" + imageDigest.String(),
			selection:    mustSelectPlatforms(t, amd64),
			wantTags:     map[string]v1.Hash{},
			wantUntagged: []v1.Hash{imageDigest},
		},
		{
			name:      "single-manifest image of another platform is saved unchanged",
			image:     repo + ":docker",
			selection: mustSelectPlatforms(t, arm64),
			wantTags:  map[string]v1.Hash{"docker": imageDigest},
		},
		{
			name:      "single platform stores the platform image of a manifest list",
			image:     repo + ":list",
			selection: mustSelectPlatforms(t, arm64),
			wantTags:  map[string]v1.Hash{"list": mustDigest(t, arm64Image)},
		},
		{
			name:      "variant request matches an entry without variant",
			image:     repo + ":list",
			selection: mustSelectPlatforms(t, arm64v8),
			wantTags:  map[string]v1.Hash{"list": mustDigest(t, arm64Image)},
		},
		{
			name:      "multiple platforms keep the original manifest list",
			image:     repo + ":list",
			selection: mustSelectPlatforms(t, amd64, arm64),
			wantTags:  map[string]v1.Hash{"list": listDigest},
		},
		{
			name:      "all keeps the original OCI index",
			image:     repo + ":oci",
			selection: SelectAll(),
			wantTags:  map[string]v1.Hash{"oci": mustDigest(t, ociIndex)},
		},
		{
			name:         "digest reference of an index keeps the index for a single platform",
			image:        repo + "@" + listDigest.String(),
			selection:    mustSelectPlatforms(t, arm64),
			wantTags:     map[string]v1.Hash{},
			wantUntagged: []v1.Hash{listDigest},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			saved, err := NewImageSaver(t.Context(), 2, nil).SaveImages([]string{tt.image}, root, tt.selection)
			if err != nil {
				t.Fatalf("save %s: %v", tt.image, err)
			}
			if !slices.Equal(saved, []string{tt.image}) {
				t.Fatalf("saved = %v, want [%s]", saved, tt.image)
			}
			tags, untagged := storedManifests(t, root, "library/app")
			if !maps.Equal(tags, tt.wantTags) {
				t.Fatalf("tags = %v, want %v", tags, tt.wantTags)
			}
			for _, digest := range tt.wantUntagged {
				if !slices.Contains(untagged, digest) {
					t.Fatalf("untagged digests %v lack %s", untagged, digest)
				}
			}
		})
	}

	t.Run("manifests are stored byte for byte", func(t *testing.T) {
		root := t.TempDir()
		images := []string{repo + ":docker", repo + ":list"}
		if _, err := NewImageSaver(t.Context(), 2, nil).SaveImages(images, root, SelectAll()); err != nil {
			t.Fatalf("save %v: %v", images, err)
		}
		requireTags(t, root, "library/app", map[string]v1.Hash{"docker": imageDigest, "list": listDigest})
		requireStoredBytes(t, root, "library/app", dockerImage)
		requireStoredBytes(t, root, "library/app", list)
		requireStoredBytes(t, root, "library/app", arm64Image)
	})

	for _, platforms := range [][]ispec.Platform{{s390x}, {amd64, s390x}} {
		t.Run("missing platform fails for "+platformsName(platforms), func(t *testing.T) {
			_, err := NewImageSaver(t.Context(), 2, nil).SaveImages([]string{repo + ":list"}, t.TempDir(),
				mustSelectPlatforms(t, platforms...))
			if err == nil || !strings.Contains(err.Error(), "s390x") {
				t.Fatalf("save without linux/s390x in the index: error = %v, want one naming s390x", err)
			}
		})
	}
}

// platformsName joins the platforms for a subtest name.
func platformsName(platforms []ispec.Platform) string {
	names := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		names = append(names, platformString(platform))
	}
	return strings.Join(names, ",")
}

func TestSaveRefusesLegacyRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docker", "registry", "v2"), 0o755); err != nil {
		t.Fatalf("create legacy storage directory: %v", err)
	}
	selection := mustSelectPlatforms(t, ispec.Platform{OS: "linux", Architecture: "amd64"})
	_, err := NewImageSaver(t.Context(), 1, nil).SaveImages([]string{"127.0.0.1:1/app:v1"}, root, selection)
	if !errors.Is(err, ocilayout.ErrLegacyLayout) {
		t.Fatalf("save into a legacy root: error = %v, want %v", err, ocilayout.ErrLegacyLayout)
	}
}

func TestSaveUsesCredentials(t *testing.T) {
	const user, password = "user", "secret"
	handler := newRegistryHandler()
	host := startRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != user || p != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	img := newPlatformImage(t, "amd64")
	pushImage(t, host+"/app:v1", img, remote.WithAuth(&authn.Basic{Username: user, Password: password}))
	selection := mustSelectPlatforms(t, ispec.Platform{OS: "linux", Architecture: "amd64"})

	_, err := NewImageSaver(t.Context(), 1, nil).SaveImages([]string{host + "/app:v1"}, t.TempDir(), selection)
	if err == nil {
		t.Fatal("save without credentials succeeded, want an authentication error")
	}
	root := t.TempDir()
	auths := map[string]registry.AuthConfig{host: {Username: user, Password: password}}
	if _, err := NewImageSaver(t.Context(), 1, auths).SaveImages([]string{host + "/app:v1"}, root, selection); err != nil {
		t.Fatalf("save with credentials: %v", err)
	}
	requireTags(t, root, "app", map[string]v1.Hash{"v1": mustDigest(t, img)})
}

// concurrencyRecorder records the largest number of manifest requests served at once. Each
// request waits until limit requests are in flight or a timeout passes, so overlapping saves
// are observed deterministically.
type concurrencyRecorder struct {
	next     http.Handler
	limit    int
	mu       sync.Mutex
	inFlight int
	max      int
}

func (c *concurrencyRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.URL.Path, "/manifests/") || r.Method != http.MethodGet {
		c.next.ServeHTTP(w, r)
		return
	}
	c.mu.Lock()
	c.inFlight++
	c.max = max(c.max, c.inFlight)
	c.mu.Unlock()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		c.mu.Lock()
		reached := c.max >= c.limit
		c.mu.Unlock()
		if reached {
			break
		}
	}
	c.next.ServeHTTP(w, r)
	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
}

func TestSaveHonoursMaxPullProcs(t *testing.T) {
	const maxPullProcs = 2
	recorder := &concurrencyRecorder{next: newRegistryHandler(), limit: maxPullProcs}
	host := startRegistry(t, recorder)
	var images []string
	for _, repository := range []string{"a", "b", "c", "d"} {
		pushImage(t, host+"/"+repository+":v1", newPlatformImage(t, "amd64"))
		images = append(images, host+"/"+repository+":v1")
	}
	recorder.mu.Lock()
	recorder.max = 0
	recorder.mu.Unlock()

	selection := mustSelectPlatforms(t, ispec.Platform{OS: "linux", Architecture: "amd64"})
	saved, err := NewImageSaver(t.Context(), maxPullProcs, nil).SaveImages(images, t.TempDir(), selection)
	if err != nil {
		t.Fatalf("save %v: %v", images, err)
	}
	if !slices.Equal(saved, images) {
		t.Fatalf("saved = %v, want %v in input order", saved, images)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.max != maxPullProcs {
		t.Fatalf("at most %d manifests were fetched at once, want %d", recorder.max, maxPullProcs)
	}
}

func TestSaveTarImages(t *testing.T) {
	root := t.TempDir()
	archive := "docker-archive:testregistrytar/images/skopeo/config_main.tar"
	tars := []string{archive + "@library/config_main", "", archive + "@library/config_main:v1"}
	selection := mustSelectPlatforms(t, ispec.Platform{OS: "linux", Architecture: "amd64"})
	saved, err := NewImageTarSaver(t.Context(), 2).SaveImages(tars, root, selection)
	if err != nil {
		t.Fatalf("save %v: %v", tars, err)
	}
	if want := []string{tars[0], tars[2]}; !slices.Equal(saved, want) {
		t.Fatalf("saved = %v, want %v", saved, want)
	}
	tags, _ := storedManifests(t, root, "library/config_main")
	if len(tags) != 2 || tags["latest"] != tags["v1"] {
		t.Fatalf("tags = %v, want latest and v1 on the same manifest", tags)
	}
	idx, err := layout.ImageIndexFromPath(ocilayout.RepositoryDir(root, "library/config_main"))
	if err != nil {
		t.Fatalf("open saved layout: %v", err)
	}
	img, err := idx.Image(tags["latest"])
	if err != nil {
		t.Fatalf("open saved image: %v", err)
	}
	if err := validate.Image(img); err != nil {
		t.Fatalf("validate saved image: %v", err)
	}
	mediaType, err := img.MediaType()
	if err != nil {
		t.Fatalf("read media type: %v", err)
	}
	if mediaType != types.OCIManifestSchema1 {
		t.Fatalf("media type = %s, want %s from the conversion to an OCI layout", mediaType, types.OCIManifestSchema1)
	}
}
