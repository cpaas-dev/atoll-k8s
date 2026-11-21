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
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/validate"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
)

// writeRootfsImage stores a random image as repository:tag in the registry root of rootfs
// and returns its digest.
func writeRootfsImage(t *testing.T, rootfs, repository, tag string) v1.Hash {
	t.Helper()
	img, err := random.Image(256, 2)
	if err != nil {
		t.Fatalf("create random image: %v", err)
	}
	if err := ocilayout.WriteImage(filepath.Join(rootfs, constants.RegistryDirName), repository, tag, img); err != nil {
		t.Fatalf("write %s:%s to %s: %v", repository, tag, rootfs, err)
	}
	digest, err := img.Digest()
	if err != nil {
		t.Fatalf("compute digest of %s:%s: %v", repository, tag, err)
	}
	return digest
}

// requireTags fails unless repository in the registry root holds exactly the tags in want and
// every tagged image is complete.
func requireTags(t *testing.T, root, repository string, want map[string]v1.Hash) {
	t.Helper()
	descs, err := ocilayout.Descriptors(root, repository)
	if err != nil {
		t.Fatalf("read descriptors of %s: %v", repository, err)
	}
	got := map[string]v1.Hash{}
	for _, desc := range descs {
		if tag, ok := desc.Annotations[ispec.AnnotationRefName]; ok {
			got[tag] = desc.Digest
		}
	}
	if !maps.Equal(got, want) {
		t.Fatalf("%s tags = %v, want %v", repository, got, want)
	}
	for tag, digest := range want {
		img, err := layout.Path(ocilayout.RepositoryDir(root, repository)).Image(digest)
		if err != nil {
			t.Fatalf("open %s:%s: %v", repository, tag, err)
		}
		if err := validate.Image(img); err != nil {
			t.Fatalf("validate %s:%s: %v", repository, tag, err)
		}
	}
}

// TestMergeRootfsRegistryMergeCommand merges the registry roots of several images in order,
// like atoll merge does, into a missing destination.
func TestMergeRootfsRegistryMergeCommand(t *testing.T) {
	first, withoutRegistry, last := t.TempDir(), t.TempDir(), t.TempDir()
	nginxV1 := writeRootfsImage(t, first, "library/nginx", "v1")
	nginxV2 := writeRootfsImage(t, last, "library/nginx", "v2")
	pause := writeRootfsImage(t, last, "pause", "3.9")
	dst := filepath.Join(t.TempDir(), constants.RegistryDirName)

	for _, rootfs := range []string{first, withoutRegistry, last} {
		if err := mergeRootfsRegistry(dst, rootfs); err != nil {
			t.Fatalf("merge registry of %s: %v", rootfs, err)
		}
	}

	requireTags(t, dst, "library/nginx", map[string]v1.Hash{"v1": nginxV1, "v2": nginxV2})
	requireTags(t, dst, "pause", map[string]v1.Hash{"3.9": pause})
}

// TestMergeRootfsRegistryBuildContext merges a base image registry into a build context, like
// atoll build does before COPY registry ./registry, so the context index.json of every
// repository is a superset of the base one.
func TestMergeRootfsRegistryBuildContext(t *testing.T) {
	base, contextDir := t.TempDir(), t.TempDir()
	baseApp := writeRootfsImage(t, base, "app", "v1")
	baseOnly := writeRootfsImage(t, base, "calico/node", "v3")
	contextApp := writeRootfsImage(t, contextDir, "app", "v2")
	contextRegistry := filepath.Join(contextDir, constants.RegistryDirName)

	if err := mergeRootfsRegistry(contextRegistry, base); err != nil {
		t.Fatalf("merge base registry: %v", err)
	}

	requireTags(t, contextRegistry, "app", map[string]v1.Hash{"v1": baseApp, "v2": contextApp})
	requireTags(t, contextRegistry, "calico/node", map[string]v1.Hash{"v3": baseOnly})
}

func TestMergeRootfsRegistryTagConflict(t *testing.T) {
	base, contextDir := t.TempDir(), t.TempDir()
	writeRootfsImage(t, base, "app", "v1")
	writeRootfsImage(t, base, "other", "v1")
	contextApp := writeRootfsImage(t, contextDir, "app", "v1")
	contextRegistry := filepath.Join(contextDir, constants.RegistryDirName)
	indexFile := filepath.Join(ocilayout.RepositoryDir(contextRegistry, "app"), ispec.ImageIndexFile)
	before, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatalf("read %s: %v", indexFile, err)
	}

	err = mergeRootfsRegistry(contextRegistry, base)
	if !errors.Is(err, ocilayout.ErrTagConflict) {
		t.Fatalf("merge error = %v, want %v", err, ocilayout.ErrTagConflict)
	}

	after, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatalf("read %s: %v", indexFile, err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("%s changed after a conflicting merge:\n%s\nwant\n%s", indexFile, after, before)
	}
	repositories, err := ocilayout.Repositories(contextRegistry)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(repositories) != 1 {
		t.Fatalf("repositories after a conflicting merge = %v, want only app", repositories)
	}
	requireTags(t, contextRegistry, "app", map[string]v1.Hash{"v1": contextApp})
}

func TestMergeRootfsRegistryLegacyLayout(t *testing.T) {
	rootfs := t.TempDir()
	legacy := filepath.Join(rootfs, constants.RegistryDirName, "docker", "registry", "v2")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("create %s: %v", legacy, err)
	}

	err := mergeRootfsRegistry(filepath.Join(t.TempDir(), constants.RegistryDirName), rootfs)
	if !errors.Is(err, ocilayout.ErrLegacyLayout) {
		t.Fatalf("merge error = %v, want %v", err, ocilayout.ErrLegacyLayout)
	}
}
