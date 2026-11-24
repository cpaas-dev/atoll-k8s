// Copyright © 2026 sealos.
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
	"os"
	"reflect"
	"testing"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
)

func TestScaleRetainsCommittedRootfsFromLegacyImageList(t *testing.T) {
	requireRoot(t)
	useTempRuntimeRoot(t)
	const (
		oldRootfs    = "registry.example.com/example/kubernetes:old"
		patch        = "registry.example.com/example/patch:v1"
		activeRootfs = "registry.example.com/example/kubernetes:active"
	)
	storeClusterImage(t, oldRootfs, v2.RootfsImage, "old")
	storeClusterImage(t, patch, v2.PatchImage, "patch")
	storeClusterImage(t, activeRootfs, v2.RootfsImage, "active")
	cluster := &v2.Cluster{}
	cluster.Name = testCluster
	cluster.Spec.Image = []string{oldRootfs, patch, activeRootfs}
	cluster.Status.Mounts = []v2.MountImage{
		{
			Type:      v2.RootfsImage,
			ImageName: activeRootfs,
		},
	}

	images := newTestClusterImages(t)
	if err := DropObsoleteRootfs(context.Background(), images, cluster); err != nil {
		t.Fatal(err)
	}
	if err := MountClusterImages(context.Background(), images, cluster); err != nil {
		t.Fatal(err)
	}

	want := []string{patch, activeRootfs}
	if !reflect.DeepEqual([]string(cluster.Spec.Image), want) {
		t.Fatalf("obsolete image retained in inventory: %v", cluster.Spec.Image)
	}
	if rootfs := cluster.GetRootfsImage(); rootfs.ImageName != activeRootfs || readMarker(t, *rootfs) != "active" {
		t.Fatalf("scale changed active rootfs to %+v", rootfs)
	}
	exports, err := os.ReadDir(constants.ClusterMountsDir(testCluster))
	if err != nil {
		t.Fatalf("read mounts dir: %v", err)
	}
	var dirs int
	for _, export := range exports {
		if export.IsDir() {
			dirs++
		}
	}
	if dirs != len(want) || len(cluster.Status.Mounts) != len(want) {
		t.Fatalf("scale exported %d images into %d mounts, want %d", dirs, len(cluster.Status.Mounts), len(want))
	}
}
