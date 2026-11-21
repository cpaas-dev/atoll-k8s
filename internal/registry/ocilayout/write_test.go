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
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/go-containerregistry/pkg/v1/validate"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// manifest is an image or an image index.
type manifest interface {
	partial.Describable
	RawManifest() ([]byte, error)
}

// requireStoredManifest fails unless the blob of m in repository is byte-identical to its
// raw manifest and hashes to its digest.
func requireStoredManifest(t *testing.T, root, repository string, m manifest) {
	t.Helper()
	digest := mustDigest(t, m)
	raw, err := m.RawManifest()
	if err != nil {
		t.Fatalf("read raw manifest: %v", err)
	}
	stored := readFile(t, blobPath(root, repository, digest))
	if !bytes.Equal(stored, raw) {
		t.Fatalf("stored manifest %s differs from the original bytes", digest)
	}
	got, _, err := v1.SHA256(bytes.NewReader(stored))
	if err != nil {
		t.Fatalf("hash stored manifest: %v", err)
	}
	if got != digest {
		t.Fatalf("stored manifest hashes to %s, want %s", got, digest)
	}
}

// openLayout returns the OCI layout of repository as an image index.
func openLayout(t *testing.T, root, repository string) v1.ImageIndex {
	t.Helper()
	idx, err := layout.ImageIndexFromPath(RepositoryDir(root, repository))
	if err != nil {
		t.Fatalf("open layout of %s: %v", repository, err)
	}
	return idx
}

// requireValidImage fails unless the image digest in repository is complete and consistent.
func requireValidImage(t *testing.T, root, repository string, digest v1.Hash) {
	t.Helper()
	img, err := openLayout(t, root, repository).Image(digest)
	if err != nil {
		t.Fatalf("open image %s@%s: %v", repository, digest, err)
	}
	if err := validate.Image(img); err != nil {
		t.Fatalf("validate image %s@%s: %v", repository, digest, err)
	}
}

// requireValidIndex fails unless the index digest in repository is complete and consistent.
func requireValidIndex(t *testing.T, root, repository string, digest v1.Hash) {
	t.Helper()
	idx, err := openLayout(t, root, repository).ImageIndex(digest)
	if err != nil {
		t.Fatalf("open index %s@%s: %v", repository, digest, err)
	}
	if err := validate.Index(idx); err != nil {
		t.Fatalf("validate index %s@%s: %v", repository, digest, err)
	}
}

func TestWritePreservesManifestDigests(t *testing.T) {
	dockerImage := newDockerImage(t)
	ociImage := newOCIImage(t)
	dockerList := newDockerManifestList(t)
	ociIndex := newOCIIndex(t)
	tests := []struct {
		name      string
		manifest  manifest
		mediaType types.MediaType
		write     func(root string) error
		validate  func(t *testing.T, root, repository string, digest v1.Hash)
	}{
		{
			name:      "Docker schema2 image",
			manifest:  dockerImage,
			mediaType: types.DockerManifestSchema2,
			write:     func(root string) error { return WriteImage(root, "app", "v1", dockerImage) },
			validate:  requireValidImage,
		},
		{
			name:      "OCI image",
			manifest:  ociImage,
			mediaType: types.OCIManifestSchema1,
			write:     func(root string) error { return WriteImage(root, "app", "v1", ociImage) },
			validate:  requireValidImage,
		},
		{
			name:      "Docker manifest list",
			manifest:  dockerList,
			mediaType: types.DockerManifestList,
			write:     func(root string) error { return WriteIndex(root, "app", "v1", dockerList) },
			validate:  requireValidIndex,
		},
		{
			name:      "OCI index",
			manifest:  ociIndex,
			mediaType: types.OCIImageIndex,
			write:     func(root string) error { return WriteIndex(root, "app", "v1", ociIndex) },
			validate:  requireValidIndex,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := tt.write(root); err != nil {
				t.Fatalf("write: %v", err)
			}
			digest := mustDigest(t, tt.manifest)
			descs := mustDescriptors(t, root, "app")
			if got := tagsOf(t, descs); !maps.Equal(got, map[string]v1.Hash{"v1": digest}) {
				t.Fatalf("tags = %v, want v1 -> %s", got, digest)
			}
			if descs[0].MediaType != tt.mediaType {
				t.Fatalf("tagged descriptor media type = %s, want %s", descs[0].MediaType, tt.mediaType)
			}
			requireStoredManifest(t, root, "app", tt.manifest)
			tt.validate(t, root, "app", digest)
		})
	}
}

func TestWriteIndexRegistersChildManifests(t *testing.T) {
	t.Run("Docker manifest list", func(t *testing.T) {
		root := t.TempDir()
		list := newDockerManifestList(t)
		if err := WriteIndex(root, "app", "v1", list); err != nil {
			t.Fatalf("write index: %v", err)
		}
		listManifest, err := list.IndexManifest()
		if err != nil {
			t.Fatalf("parse manifest list: %v", err)
		}
		want := []v1.Descriptor{{
			MediaType:   types.DockerManifestList,
			Digest:      mustDigest(t, list),
			Size:        mustSize(t, list),
			Annotations: map[string]string{ispec.AnnotationRefName: "v1"},
		}}
		for _, child := range listManifest.Manifests {
			want = append(want, v1.Descriptor{MediaType: child.MediaType, Digest: child.Digest, Size: child.Size})
			img, err := list.Image(child.Digest)
			if err != nil {
				t.Fatalf("open child %s: %v", child.Digest, err)
			}
			requireStoredManifest(t, root, "app", img)
		}
		if got := mustDescriptors(t, root, "app"); !descriptorsEqual(got, want) {
			t.Fatalf("descriptors = %+v, want %+v", got, want)
		}
	})

	t.Run("nested OCI index", func(t *testing.T) {
		root := t.TempDir()
		idx := newOCIIndex(t)
		if err := WriteIndex(root, "app", "v1", idx); err != nil {
			t.Fatalf("write index: %v", err)
		}
		want := reachableManifests(t, idx)
		if len(want) != 3 {
			t.Fatalf("test index reaches %d manifests, want 3 (image, nested index, nested image)", len(want))
		}
		got := untaggedDigests(t, mustDescriptors(t, root, "app"))
		if !slices.Equal(got, want) {
			t.Fatalf("untagged digests = %v, want %v", got, want)
		}
	})
}

// mustSize returns the size of a manifest.
func mustSize(t *testing.T, m partial.Describable) int64 {
	t.Helper()
	size, err := m.Size()
	if err != nil {
		t.Fatalf("compute size: %v", err)
	}
	return size
}

// reachableManifests returns the digests of all manifests below idx in depth-first order.
func reachableManifests(t *testing.T, idx v1.ImageIndex) []v1.Hash {
	t.Helper()
	im, err := idx.IndexManifest()
	if err != nil {
		t.Fatalf("parse index manifest: %v", err)
	}
	var digests []v1.Hash
	for _, child := range im.Manifests {
		digests = append(digests, child.Digest)
		if !child.MediaType.IsIndex() {
			continue
		}
		nested, err := idx.ImageIndex(child.Digest)
		if err != nil {
			t.Fatalf("open nested index %s: %v", child.Digest, err)
		}
		digests = append(digests, reachableManifests(t, nested)...)
	}
	return digests
}

// descriptorsEqual compares descriptors field by field, ignoring nil versus empty maps.
func descriptorsEqual(got, want []v1.Descriptor) bool {
	return slices.EqualFunc(got, want, func(a, b v1.Descriptor) bool {
		return a.MediaType == b.MediaType && a.Digest == b.Digest && a.Size == b.Size &&
			maps.Equal(a.Annotations, b.Annotations) && a.Platform == nil && b.Platform == nil &&
			a.ArtifactType == b.ArtifactType && len(a.URLs) == 0 && len(a.Data) == 0
	})
}

func TestWriteSameTagTwiceIsIdempotent(t *testing.T) {
	root := t.TempDir()
	img := newDockerImage(t)
	list := newDockerManifestList(t)
	if err := WriteImage(root, "app", "image", img); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := WriteIndex(root, "app", "list", list); err != nil {
		t.Fatalf("write index: %v", err)
	}
	indexFile := filepath.Join(RepositoryDir(root, "app"), ispec.ImageIndexFile)
	before := readFile(t, indexFile)

	if err := WriteImage(root, "app", "image", img); err != nil {
		t.Fatalf("rewrite image: %v", err)
	}
	if err := WriteIndex(root, "app", "list", list); err != nil {
		t.Fatalf("rewrite index: %v", err)
	}
	if after := readFile(t, indexFile); !bytes.Equal(before, after) {
		t.Fatalf("index.json changed after rewriting the same tags:\nbefore %s\nafter  %s", before, after)
	}
	descs := mustDescriptors(t, root, "app")
	if len(descs) != 4 {
		t.Fatalf("got %d descriptors, want 4 (two tags and two list children): %+v", len(descs), descs)
	}
}

func TestWriteRepointsTag(t *testing.T) {
	root := t.TempDir()
	first := newDockerImage(t)
	second := newOCIImage(t)
	list := newDockerManifestList(t)

	for _, step := range []struct {
		tag string
		img v1.Image
	}{{"v1", first}, {"stable", first}, {"v1", second}} {
		if err := WriteImage(root, "app", step.tag, step.img); err != nil {
			t.Fatalf("write %s: %v", step.tag, err)
		}
	}
	want := map[string]v1.Hash{"v1": mustDigest(t, second), "stable": mustDigest(t, first)}
	if got := tagsOf(t, mustDescriptors(t, root, "app")); !maps.Equal(got, want) {
		t.Fatalf("tags after re-pointing v1 = %v, want %v", got, want)
	}

	if err := WriteIndex(root, "app", "v1", list); err != nil {
		t.Fatalf("re-point v1 to an index: %v", err)
	}
	want["v1"] = mustDigest(t, list)
	descs := mustDescriptors(t, root, "app")
	if got := tagsOf(t, descs); !maps.Equal(got, want) {
		t.Fatalf("tags after re-pointing v1 to an index = %v, want %v", got, want)
	}
	for _, desc := range descs {
		if desc.Digest == want["v1"] && desc.MediaType != types.DockerManifestList {
			t.Fatalf("v1 descriptor media type = %s, want %s", desc.MediaType, types.DockerManifestList)
		}
	}
}

func TestWriteUntagged(t *testing.T) {
	t.Run("image", func(t *testing.T) {
		root := t.TempDir()
		img := newDockerImage(t)
		for range 2 {
			if err := WriteUntaggedImage(root, "app", img); err != nil {
				t.Fatalf("write untagged image: %v", err)
			}
		}
		want := []v1.Descriptor{{MediaType: types.DockerManifestSchema2, Digest: mustDigest(t, img), Size: mustSize(t, img)}}
		if got := mustDescriptors(t, root, "app"); !descriptorsEqual(got, want) {
			t.Fatalf("descriptors = %+v, want %+v", got, want)
		}
		requireStoredManifest(t, root, "app", img)
		requireValidImage(t, root, "app", mustDigest(t, img))
	})

	t.Run("index", func(t *testing.T) {
		root := t.TempDir()
		list := newDockerManifestList(t)
		for range 2 {
			if err := WriteUntaggedIndex(root, "app", list); err != nil {
				t.Fatalf("write untagged index: %v", err)
			}
		}
		descs := mustDescriptors(t, root, "app")
		if tags := tagsOf(t, descs); len(tags) != 0 {
			t.Fatalf("untagged index produced tags %v", tags)
		}
		want := append([]v1.Hash{mustDigest(t, list)}, reachableManifests(t, list)...)
		if got := untaggedDigests(t, descs); !slices.Equal(got, want) {
			t.Fatalf("untagged digests = %v, want %v", got, want)
		}
		requireStoredManifest(t, root, "app", list)
		requireValidIndex(t, root, "app", mustDigest(t, list))
	})

	t.Run("digest already tagged", func(t *testing.T) {
		root := t.TempDir()
		img := newDockerImage(t)
		if err := WriteImage(root, "app", "v1", img); err != nil {
			t.Fatalf("write image: %v", err)
		}
		if err := WriteUntaggedImage(root, "app", img); err != nil {
			t.Fatalf("write untagged image: %v", err)
		}
		if descs := mustDescriptors(t, root, "app"); len(descs) != 1 {
			t.Fatalf("got %d descriptors, want only the tagged one: %+v", len(descs), descs)
		}
	})
}

func TestConcurrentWritesToOneRepository(t *testing.T) {
	root := t.TempDir()
	const writers = 8
	images := make([]v1.Image, writers)
	for i := range images {
		images[i] = newDockerImage(t)
	}
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i, img := range images {
		wg.Go(func() { errs[i] = WriteImage(root, "app", fmt.Sprintf("v%d", i), img) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	tags := tagsOf(t, mustDescriptors(t, root, "app"))
	if len(tags) != writers {
		t.Fatalf("got %d tags after concurrent writes, want %d: %v", len(tags), writers, tags)
	}
	for i, img := range images {
		if tags[fmt.Sprintf("v%d", i)] != mustDigest(t, img) {
			t.Fatalf("tag v%d does not point to its image", i)
		}
	}
}

// withRestrictiveUmask sets the process umask to 0077 for the duration of the test, so
// every file and directory created without an explicit chmod is private.
func withRestrictiveUmask(t *testing.T) {
	t.Helper()
	previous := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(previous) })
}

// requireRegistryModes fails unless repository and its parents below root are directories
// with mode 0755 and every file of the repository has mode 0644.
func requireRegistryModes(t *testing.T, root, repository string) {
	t.Helper()
	parent := filepath.Dir(RepositoryDir(root, repository))
	for ; parent != root; parent = filepath.Dir(parent) {
		requireMode(t, parent, dirMode)
	}
	err := filepath.WalkDir(RepositoryDir(root, repository), func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			requireMode(t, name, dirMode)
			return nil
		}
		requireMode(t, name, fileMode)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", repository, err)
	}
}

// requireMode fails unless name has the permission bits mode.
func requireMode(t *testing.T, name string, mode fs.FileMode) {
	t.Helper()
	info, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat %s: %v", name, err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s has mode %o, want %o", name, info.Mode().Perm(), mode)
	}
}

func TestWriteNormalizesFileModes(t *testing.T) {
	withRestrictiveUmask(t)
	root := t.TempDir()
	if err := WriteImage(root, "library/nginx", "v1", newDockerImage(t)); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := WriteIndex(root, "library/nginx", "multi", newDockerManifestList(t)); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := WriteUntaggedImage(root, "calico/node", newOCIImage(t)); err != nil {
		t.Fatalf("write untagged image: %v", err)
	}
	if err := WriteUntaggedIndex(root, "calico/cni", newOCIIndex(t)); err != nil {
		t.Fatalf("write untagged index: %v", err)
	}
	for _, repository := range []string{"library/nginx", "calico/node", "calico/cni"} {
		requireRegistryModes(t, root, repository)
	}
}
