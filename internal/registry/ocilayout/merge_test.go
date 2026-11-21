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
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// snapshotTree maps every path below root to its mode and, for files, its content.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		snapshot[rel] = info.Mode().String()
		if !entry.IsDir() {
			snapshot[rel] += " " + string(readFile(t, name))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snapshot
}

// mustWriteImage writes img as repository:tag.
func mustWriteImage(t *testing.T, root, repository, tag string, img v1.Image) {
	t.Helper()
	if err := WriteImage(root, repository, tag, img); err != nil {
		t.Fatalf("write %s:%s: %v", repository, tag, err)
	}
}

func TestMergeRootUnion(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	nginxV1 := newDockerImage(t)
	nginxV2 := newOCIImage(t)
	busybox := newDockerImage(t)
	calico := newDockerManifestList(t)
	pinned := newOCIImage(t)

	mustWriteImage(t, dst, "library/nginx", "v2", nginxV2)
	mustWriteImage(t, dst, "library/busybox", "latest", busybox)
	mustWriteImage(t, src, "library/nginx", "v1", nginxV1)
	if err := WriteIndex(src, "calico/node", "v3", calico); err != nil {
		t.Fatalf("write calico index: %v", err)
	}
	if err := WriteUntaggedImage(src, "calico/node", pinned); err != nil {
		t.Fatalf("write untagged image: %v", err)
	}

	if err := MergeRoot(dst, src); err != nil {
		t.Fatalf("merge: %v", err)
	}

	repositories, err := Repositories(dst)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if want := []string{"calico/node", "library/busybox", "library/nginx"}; !reflect.DeepEqual(repositories, want) {
		t.Fatalf("repositories = %v, want %v", repositories, want)
	}
	wantNginx := map[string]v1.Hash{"v1": mustDigest(t, nginxV1), "v2": mustDigest(t, nginxV2)}
	if got := tagsOf(t, mustDescriptors(t, dst, "library/nginx")); !maps.Equal(got, wantNginx) {
		t.Fatalf("library/nginx tags = %v, want %v", got, wantNginx)
	}
	requireValidImage(t, dst, "library/nginx", mustDigest(t, nginxV1))
	requireValidImage(t, dst, "library/nginx", mustDigest(t, nginxV2))
	requireValidImage(t, dst, "library/busybox", mustDigest(t, busybox))

	got, want := mustDescriptors(t, dst, "calico/node"), mustDescriptors(t, src, "calico/node")
	if !descriptorsEqual(got, want) {
		t.Fatalf("calico/node descriptors = %+v, want the source descriptors %+v", got, want)
	}
	requireStoredManifest(t, dst, "calico/node", calico)
	requireValidIndex(t, dst, "calico/node", mustDigest(t, calico))
	requireValidImage(t, dst, "calico/node", mustDigest(t, pinned))
}

func TestMergeRootCreatesDestination(t *testing.T) {
	src := t.TempDir()
	img := newDockerImage(t)
	mustWriteImage(t, src, "library/nginx", "v1", img)
	dst := filepath.Join(t.TempDir(), "registry")

	if err := MergeRoot(dst, src); err != nil {
		t.Fatalf("merge into missing destination: %v", err)
	}
	if got := tagsOf(t, mustDescriptors(t, dst, "library/nginx")); got["v1"] != mustDigest(t, img) {
		t.Fatalf("library/nginx tags = %v, want v1 -> %s", got, mustDigest(t, img))
	}
	requireValidImage(t, dst, "library/nginx", mustDigest(t, img))

	emptyDst := filepath.Join(t.TempDir(), "empty")
	if err := MergeRoot(emptyDst, t.TempDir()); err != nil {
		t.Fatalf("merge empty source into missing destination: %v", err)
	}
	if err := CheckRoot(emptyDst); err != nil {
		t.Fatalf("destination of an empty merge: %v", err)
	}
}

func TestMergeRootSharesBlobs(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	base := newDockerImage(t)
	extra, err := random.Layer(256, types.DockerLayer)
	if err != nil {
		t.Fatalf("create layer: %v", err)
	}
	derived, err := mutate.AppendLayers(base, extra)
	if err != nil {
		t.Fatalf("append layer: %v", err)
	}
	mustWriteImage(t, dst, "app", "base", base)
	mustWriteImage(t, src, "app", "derived", derived)

	baseLayers, err := base.Layers()
	if err != nil {
		t.Fatalf("read base layers: %v", err)
	}
	shared := blobPath(dst, "app", mustDigest(t, baseLayers[0]))
	before, err := os.Stat(shared)
	if err != nil {
		t.Fatalf("stat shared blob: %v", err)
	}

	if err := MergeRoot(dst, src); err != nil {
		t.Fatalf("merge: %v", err)
	}

	after, err := os.Stat(shared)
	if err != nil {
		t.Fatalf("stat shared blob after merge: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatalf("shared blob %s was replaced instead of kept", shared)
	}
	requireValidImage(t, dst, "app", mustDigest(t, base))
	requireValidImage(t, dst, "app", mustDigest(t, derived))
	blobs, err := os.ReadDir(filepath.Join(RepositoryDir(dst, "app"), ispec.ImageBlobsDir, "sha256"))
	if err != nil {
		t.Fatalf("read blobs: %v", err)
	}
	// Two layers, two configs and two manifests of base plus the extra layer of derived.
	if len(blobs) != 7 {
		t.Fatalf("destination has %d blobs, want 7", len(blobs))
	}
}

func TestMergeRootDescriptorKeys(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	img := newDockerImage(t)
	mustWriteImage(t, dst, "app", "v1", img)
	mustWriteImage(t, src, "app", "v1", img)
	mustWriteImage(t, src, "app", "stable", img)
	if err := WriteUntaggedImage(src, "app", img); err != nil {
		t.Fatalf("write untagged image: %v", err)
	}

	if err := MergeRoot(dst, src); err != nil {
		t.Fatalf("merge: %v", err)
	}

	descs := mustDescriptors(t, dst, "app")
	want := map[string]v1.Hash{"v1": mustDigest(t, img), "stable": mustDigest(t, img)}
	if got := tagsOf(t, descs); !maps.Equal(got, want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
	if untagged := untaggedDigests(t, descs); len(untagged) != 0 {
		t.Fatalf("untagged digests = %v, want none because the digest is already tagged", untagged)
	}
}

func TestMergeRootConflictLeavesDestinationUntouched(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	mustWriteImage(t, dst, "library/nginx", "v1", newDockerImage(t))
	mustWriteImage(t, dst, "calico/node", "v3", newDockerImage(t))
	mustWriteImage(t, src, "library/nginx", "v1", newDockerImage(t))
	mustWriteImage(t, src, "calico/node", "v3", newOCIImage(t))
	mustWriteImage(t, src, "library/busybox", "latest", newDockerImage(t))
	mustWriteImage(t, src, "library/nginx", "v2", newDockerImage(t))
	before := snapshotTree(t, dst)

	err := MergeRoot(dst, src)
	if !errors.Is(err, ErrTagConflict) {
		t.Fatalf("merge error = %v, want ErrTagConflict", err)
	}
	for _, conflict := range []string{"library/nginx", "calico/node"} {
		if !strings.Contains(err.Error(), conflict) {
			t.Fatalf("merge error %q does not report the conflict in %s", err, conflict)
		}
	}
	if after := snapshotTree(t, dst); !maps.Equal(before, after) {
		t.Fatalf("destination changed after a conflicting merge: %d entries before, %d after", len(before), len(after))
	}
}

func TestMergeRootTwiceIsIdempotent(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	mustWriteImage(t, dst, "library/nginx", "v2", newDockerImage(t))
	mustWriteImage(t, src, "library/nginx", "v1", newDockerImage(t))
	if err := WriteIndex(src, "calico/node", "v3", newDockerManifestList(t)); err != nil {
		t.Fatalf("write index: %v", err)
	}

	if err := MergeRoot(dst, src); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	first := snapshotTree(t, dst)
	if err := MergeRoot(dst, src); err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if second := snapshotTree(t, dst); !maps.Equal(first, second) {
		t.Fatal("destination changed when merging the same source again")
	}
	for _, repository := range []string{"library/nginx", "calico/node"} {
		descs := mustDescriptors(t, dst, repository)
		tagsOf(t, descs)
		untaggedDigests(t, descs)
	}
}

func TestMergeRootRejectsCorruptBlob(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	img := newDockerImage(t)
	mustWriteImage(t, src, "app", "v1", img)
	config, err := img.ConfigName()
	if err != nil {
		t.Fatalf("read config digest: %v", err)
	}
	if err := os.WriteFile(blobPath(src, "app", config), []byte("corrupt"), 0o644); err != nil {
		t.Fatalf("corrupt config blob: %v", err)
	}

	err = MergeRoot(dst, src)
	if err == nil || !strings.Contains(err.Error(), config.String()) {
		t.Fatalf("merge error = %v, want a digest mismatch for %s", err, config)
	}
	if _, err := os.Stat(blobPath(dst, "app", config)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("corrupt blob reached the destination: %v", err)
	}
	indexFile := filepath.Join(RepositoryDir(dst, "app"), ispec.ImageIndexFile)
	if _, err := os.Stat(indexFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("index.json was written although a blob failed: %v", err)
	}
	var leftovers []string
	err = filepath.WalkDir(dst, func(name string, entry fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(entry.Name(), ".") {
			leftovers = append(leftovers, name)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walk destination: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestMergeRootRejectsLegacyRoots(t *testing.T) {
	legacy := t.TempDir()
	mkdirAll(t, filepath.Join(legacy, "docker", "registry", "v2"))
	root := t.TempDir()
	mustWriteImage(t, root, "app", "v1", newDockerImage(t))

	if err := MergeRoot(t.TempDir(), legacy); !errors.Is(err, ErrLegacyLayout) {
		t.Fatalf("merge from legacy source error = %v, want ErrLegacyLayout", err)
	}
	if err := MergeRoot(legacy, root); !errors.Is(err, ErrLegacyLayout) {
		t.Fatalf("merge into legacy destination error = %v, want ErrLegacyLayout", err)
	}
	if err := MergeRoot(t.TempDir(), filepath.Join(t.TempDir(), "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("merge from missing source error = %v, want fs.ErrNotExist", err)
	}
}

func TestMergeRootNormalizesFileModes(t *testing.T) {
	withRestrictiveUmask(t)
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteImage(t, dst, "library/nginx", "v2", newDockerImage(t))
	for _, repository := range []string{"library/nginx", "calico/node"} {
		mustWriteImage(t, src, repository, "v1", newDockerImage(t))
	}
	// Simulate blobs written by another tool with restrictive modes.
	err := filepath.WalkDir(dst, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		return os.Chmod(name, 0o600)
	})
	if err != nil {
		t.Fatalf("restrict destination modes: %v", err)
	}

	if err := MergeRoot(dst, src); err != nil {
		t.Fatalf("merge: %v", err)
	}
	for _, repository := range []string{"library/nginx", "calico/node"} {
		requireRegistryModes(t, dst, repository)
	}
}

func TestMergeRootReportsEveryConflict(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	tags := []string{"a", "b", "c"}
	for _, tag := range tags {
		mustWriteImage(t, dst, "app", tag, newDockerImage(t))
		mustWriteImage(t, src, "app", tag, newDockerImage(t))
	}
	err := MergeRoot(dst, src)
	if !errors.Is(err, ErrTagConflict) {
		t.Fatalf("merge error = %v, want ErrTagConflict", err)
	}
	for _, tag := range tags {
		if !strings.Contains(err.Error(), fmt.Sprintf("tag %q", tag)) {
			t.Fatalf("merge error %q does not report tag %s", err, tag)
		}
	}
}
