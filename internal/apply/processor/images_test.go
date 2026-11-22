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

package processor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterimage"
	cit "github.com/cpaas-dev/atoll-k8s/internal/clusterimage/clusterimagetest"
	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
)

const testCluster = "default"

// useTempRuntimeRoot points the runtime root, holding the image store and cluster dirs,
// at a temporary directory for the test.
func useTempRuntimeRoot(t *testing.T) {
	t.Helper()
	previous := constants.DefaultRuntimeRootDir
	constants.DefaultRuntimeRootDir = t.TempDir()
	t.Cleanup(func() { constants.DefaultRuntimeRootDir = previous })
}

// storeClusterImage tags an image of imageType holding /cpaas/marker in the image store.
func storeClusterImage(t *testing.T, name string, imageType v2.ImageType, marker string) {
	t.Helper()
	img := cit.NewImage(clusterimage.HostPlatform(),
		cit.LayerTar(t, cit.Dir("cpaas/", 0o755), cit.File("cpaas/marker", marker, 0o644)))
	img.Config.Labels = map[string]string{
		v2.ImageTypeKeys[0]:    string(imageType),
		v2.ImageVersionKeys[0]: v2.ImageVersionList[0],
		v2.ImageKubeVersionKey: "v1.28.15",
	}
	cit.Tag(t, constants.ImageStorePath(), name, img)
}

func newTestClusterImages(t *testing.T) *ClusterImages {
	t.Helper()
	images, err := NewClusterImages(context.Background(), testCluster)
	if err != nil {
		t.Fatalf("open cluster images: %v", err)
	}
	return images
}

func readMarker(t *testing.T, mount v2.MountImage) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(mount.MountPoint, "marker"))
	if err != nil {
		t.Fatalf("read marker of %s: %v", mount.ImageName, err)
	}
	return string(data)
}

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("exporting a rootfs restores file owners and needs root")
	}
}

func TestClusterImagesMountKeepsCurrentExport(t *testing.T) {
	requireRoot(t)
	useTempRuntimeRoot(t)
	ctx := context.Background()
	const ref = "registry.example.com/cluster/kubernetes:v1"
	storeClusterImage(t, ref, v2.RootfsImage, "v1")

	first, err := newTestClusterImages(t).Mount(ctx, ref, nil)
	if err != nil {
		t.Fatalf("mount: %v", err)
	}
	rendered := filepath.Join(first.MountPoint, "rendered.conf")
	if err := os.WriteFile(rendered, []byte("rendered"), 0o644); err != nil {
		t.Fatalf("write into export: %v", err)
	}

	again, err := newTestClusterImages(t).Mount(ctx, ref, &first)
	if err != nil {
		t.Fatalf("mount again: %v", err)
	}
	if again.Name != first.Name || again.Digest != first.Digest {
		t.Fatalf("mount again = %+v, want %+v", again, first)
	}
	if _, err := os.Stat(rendered); err != nil {
		t.Fatalf("kept export lost the rendered file: %v", err)
	}

	storeClusterImage(t, ref, v2.RootfsImage, "v2")
	updated, err := newTestClusterImages(t).Mount(ctx, ref, &first)
	if err != nil {
		t.Fatalf("mount updated image: %v", err)
	}
	if updated.Name != first.Name || updated.Digest == first.Digest || readMarker(t, updated) != "v2" {
		t.Fatalf("updated mount %+v does not export the new image into %s", updated, first.Name)
	}
	if _, err := os.Stat(rendered); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("re-export kept the rendered file of the old image: %v", err)
	}
}

func TestClusterImagesNameMissingImagePull(t *testing.T) {
	useTempRuntimeRoot(t)
	const ref = "registry.example.com/cluster/kubernetes:missing"

	_, err := newTestClusterImages(t).Inspect(context.Background(), ref)

	if !errors.Is(err, clusterimage.ErrImageNotFound) || !strings.Contains(err.Error(), "atoll pull "+ref) {
		t.Fatalf("inspect error = %v, want a not found error naming atoll pull %s", err, ref)
	}
}

func TestSyncClusterStatusRestoresRecordedMounts(t *testing.T) {
	requireRoot(t)
	useTempRuntimeRoot(t)
	ctx := context.Background()
	const rootfs, app = "registry.example.com/cluster/kubernetes:v1", "registry.example.com/cluster/helm:v1"
	storeClusterImage(t, rootfs, v2.RootfsImage, "rootfs")
	storeClusterImage(t, app, v2.AppImage, "app")
	images := newTestClusterImages(t)
	for _, ref := range []string{rootfs, app} {
		if _, err := images.Mount(ctx, ref, nil); err != nil {
			t.Fatalf("mount %s: %v", ref, err)
		}
	}

	cluster := &v2.Cluster{}
	cluster.Name = testCluster
	cluster.Spec.Image = []string{rootfs}
	if err := SyncClusterStatus(cluster, false); err != nil {
		t.Fatalf("sync status: %v", err)
	}
	if len(cluster.Status.Mounts) != 1 || cluster.Status.Mounts[0].ImageName != rootfs || readMarker(t, cluster.Status.Mounts[0]) != "rootfs" {
		t.Fatalf("restored mounts %+v, want only %s", cluster.Status.Mounts, rootfs)
	}

	cluster.Status.Mounts = nil
	if err := SyncClusterStatus(cluster, true); err != nil {
		t.Fatalf("sync status for reset: %v", err)
	}
	var names []string
	for _, mount := range cluster.Status.Mounts {
		names = append(names, mount.ImageName)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{app, rootfs}) {
		t.Fatalf("reset restored %v, want every recorded mount", names)
	}
}
