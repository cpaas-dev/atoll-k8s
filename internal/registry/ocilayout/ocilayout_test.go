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

package ocilayout

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
)

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

// newDockerManifestList returns a Docker manifest list of two Docker schema2 images.
func newDockerManifestList(t *testing.T) v1.ImageIndex {
	t.Helper()
	idx := mutate.AppendManifests(empty.Index, newPlatformAddendum(t, "amd64"), newPlatformAddendum(t, "arm64"))
	return mutate.IndexMediaType(idx, types.DockerManifestList)
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

// newOCIIndex returns an OCI index holding an OCI image and a nested OCI index with one image.
func newOCIIndex(t *testing.T) v1.ImageIndex {
	t.Helper()
	nested := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: newOCIImage(t)})
	return mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: newOCIImage(t)},
		mutate.IndexAddendum{Add: nested},
	)
}

// mustDigest returns the digest of a manifest.
func mustDigest(t *testing.T, manifest interface{ Digest() (v1.Hash, error) }) v1.Hash {
	t.Helper()
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}
	return digest
}

// mustDescriptors returns the top-level descriptors of repository.
func mustDescriptors(t *testing.T, root, repository string) []v1.Descriptor {
	t.Helper()
	descs, err := Descriptors(root, repository)
	if err != nil {
		t.Fatalf("read descriptors of %s: %v", repository, err)
	}
	return descs
}

// tagsOf maps every tag of descs to its digest and fails on duplicate tags.
func tagsOf(t *testing.T, descs []v1.Descriptor) map[string]v1.Hash {
	t.Helper()
	tags := map[string]v1.Hash{}
	for _, desc := range descs {
		tag, ok := desc.Annotations[ispec.AnnotationRefName]
		if !ok {
			continue
		}
		if _, dup := tags[tag]; dup {
			t.Fatalf("tag %s has more than one descriptor in %v", tag, descs)
		}
		tags[tag] = desc.Digest
	}
	return tags
}

// untaggedDigests returns the digests of untagged descs and fails on duplicates.
func untaggedDigests(t *testing.T, descs []v1.Descriptor) []v1.Hash {
	t.Helper()
	var digests []v1.Hash
	seen := map[v1.Hash]bool{}
	for _, desc := range descs {
		if _, ok := desc.Annotations[ispec.AnnotationRefName]; ok {
			continue
		}
		if seen[desc.Digest] {
			t.Fatalf("untagged digest %s has more than one descriptor in %v", desc.Digest, descs)
		}
		seen[desc.Digest] = true
		digests = append(digests, desc.Digest)
	}
	return digests
}

// blobPath returns the path of the blob digest in repository.
func blobPath(root, repository string, digest v1.Hash) string {
	return filepath.Join(RepositoryDir(root, repository), ispec.ImageBlobsDir, digest.Algorithm, digest.Hex)
}

// readFile returns the content of name.
func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

// mkdirAll creates name with its parents.
func mkdirAll(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(name, 0o755); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func TestRepositoryDir(t *testing.T) {
	got := RepositoryDir("/var/lib/registry", "library/nginx")
	want := filepath.Join("/var/lib/registry", "library", "nginx")
	if got != want {
		t.Fatalf("RepositoryDir = %s, want %s", got, want)
	}
}

func TestRepositoriesListsNestedRepositoriesSorted(t *testing.T) {
	root := t.TempDir()
	img := newDockerImage(t)
	for _, repository := range []string{"library/nginx", "library", "library-ext", "calico/node", "registry.k8s.io/pause"} {
		if err := WriteImage(root, repository, "v1", img); err != nil {
			t.Fatalf("write %s: %v", repository, err)
		}
	}
	mkdirAll(t, filepath.Join(root, "not-a-repository", "child"))
	mkdirAll(t, filepath.Join(root, ".uploads", "hidden"))
	if err := os.WriteFile(filepath.Join(root, ".uploads", "hidden", ispec.ImageLayoutFile), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write hidden oci-layout: %v", err)
	}

	got, err := Repositories(root)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	want := []string{"calico/node", "library", "library-ext", "library/nginx", "registry.k8s.io/pause"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Repositories = %v, want %v", got, want)
	}
}

func TestRepositoriesOfEmptyAndMissingRoot(t *testing.T) {
	got, err := Repositories(t.TempDir())
	if err != nil {
		t.Fatalf("list repositories of empty root: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Repositories of empty root = %v, want none", got)
	}

	_, err = Repositories(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Repositories of missing root error = %v, want fs.ErrNotExist", err)
	}
}

func TestDescriptorsOfMissingRepository(t *testing.T) {
	_, err := Descriptors(t.TempDir(), "missing")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Descriptors of missing repository error = %v, want fs.ErrNotExist", err)
	}
}

func TestWriteRejectsInvalidReferences(t *testing.T) {
	img := newDockerImage(t)
	tests := []struct {
		name       string
		repository string
		tag        string
	}{
		{name: "empty repository", repository: "", tag: "v1"},
		{name: "parent traversal", repository: "../escape", tag: "v1"},
		{name: "inner traversal", repository: "library/../../escape", tag: "v1"},
		{name: "absolute path", repository: "/etc/registry", tag: "v1"},
		{name: "dot component", repository: "./library", tag: "v1"},
		{name: "empty component", repository: "library//nginx", tag: "v1"},
		{name: "trailing slash", repository: "library/", tag: "v1"},
		{name: "backslash", repository: `library\nginx`, tag: "v1"},
		{name: "uppercase repository", repository: "Library", tag: "v1"},
		{name: "empty tag", repository: "nginx", tag: ""},
		{name: "tag with leading dot", repository: "nginx", tag: ".v1"},
		{name: "tag with leading dash", repository: "nginx", tag: "-v1"},
		{name: "tag with colon", repository: "nginx", tag: "v1:latest"},
		{name: "tag with slash", repository: "nginx", tag: "v1/latest"},
		{name: "tag too long", repository: "nginx", tag: strings.Repeat("a", 129)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := WriteImage(root, tt.repository, tt.tag, img); err == nil {
				t.Fatalf("WriteImage(%q, %q) succeeded, want error", tt.repository, tt.tag)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatalf("read root: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("root has %d entries after rejected write, want none", len(entries))
			}
		})
	}

	t.Run("untagged writers validate the repository", func(t *testing.T) {
		root := t.TempDir()
		if err := WriteUntaggedImage(root, "../escape", img); err == nil {
			t.Fatal("WriteUntaggedImage with traversal succeeded, want error")
		}
		if err := WriteUntaggedIndex(root, "/abs", newOCIIndex(t)); err == nil {
			t.Fatal("WriteUntaggedIndex with absolute repository succeeded, want error")
		}
	})

	t.Run("longest valid tag", func(t *testing.T) {
		if err := WriteImage(t.TempDir(), "library/nginx", "_"+strings.Repeat("a", 127), img); err != nil {
			t.Fatalf("write 128 character tag: %v", err)
		}
	})
}

func TestWriteRejectsMismatchedManifestKinds(t *testing.T) {
	root := t.TempDir()
	if err := WriteImage(root, "app", "v1", mutate.MediaType(newDockerImage(t), types.OCIImageIndex)); err == nil {
		t.Fatal("WriteImage with an index media type succeeded, want error")
	}
	if err := WriteIndex(root, "app", "v1", mutate.IndexMediaType(newOCIIndex(t), types.OCIManifestSchema1)); err == nil {
		t.Fatal("WriteIndex with an image media type succeeded, want error")
	}
}

func TestCheckRoot(t *testing.T) {
	t.Run("empty root is valid", func(t *testing.T) {
		if err := CheckRoot(t.TempDir()); err != nil {
			t.Fatalf("check empty root: %v", err)
		}
	})

	t.Run("root with repositories is valid", func(t *testing.T) {
		root := t.TempDir()
		if err := WriteImage(root, "library/nginx", "v1", newDockerImage(t)); err != nil {
			t.Fatalf("write image: %v", err)
		}
		if err := CheckRoot(root); err != nil {
			t.Fatalf("check root: %v", err)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		err := CheckRoot(filepath.Join(t.TempDir(), "missing"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("CheckRoot of missing root error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("root is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if err := CheckRoot(file); err == nil {
			t.Fatal("CheckRoot of a file succeeded, want error")
		}
	})

	t.Run("legacy distribution storage", func(t *testing.T) {
		root := t.TempDir()
		legacy := filepath.Join(root, "docker", "registry", "v2")
		mkdirAll(t, filepath.Join(legacy, "repositories"))
		err := CheckRoot(root)
		if !errors.Is(err, ErrLegacyLayout) {
			t.Fatalf("CheckRoot error = %v, want ErrLegacyLayout", err)
		}
		if !strings.Contains(err.Error(), legacy) {
			t.Fatalf("CheckRoot error %q does not name %s", err, legacy)
		}
	})
}
