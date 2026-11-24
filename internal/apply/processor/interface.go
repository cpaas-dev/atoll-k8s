// Copyright © 2021 Alibaba Group Holding Ltd.
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
	"fmt"

	"slices"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/exec"
	"github.com/cpaas-dev/atoll-k8s/internal/filesystem/registry"
	"github.com/cpaas-dev/atoll-k8s/internal/ssh"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/confirm"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/maps"
)

type Interface interface {
	// Execute :according to the difference of desired cluster to do cluster apply.
	Execute(cluster *v2.Cluster) error
}

// SyncClusterStatus fills cluster.Status.Mounts from the mount records of the cluster
// when the status has none: every recorded mount with reset, otherwise the mounts of
// images in cluster.Spec.Image.
func SyncClusterStatus(cluster *v2.Cluster, reset bool) error {
	if cluster.Status.Mounts == nil {
		recorded, err := recordedMounts(cluster.Name)
		if err != nil {
			return err
		}
		cluster.Status.Mounts = make([]v2.MountImage, 0)
		for _, mount := range recorded {
			if reset || slices.Contains(cluster.Spec.Image, mount.ImageName) {
				cluster.Status.Mounts = append(cluster.Status.Mounts, mount)
			}
		}
	}
	logger.Debug("sync cluster status is: %s", cluster.String())
	return nil
}

func ConfirmDeleteNodes() error {
	if !ForceDelete {
		prompt := "are you sure to delete these nodes?"
		cancel := "you have canceled to delete these nodes !"
		if pass, err := confirm.Confirm(prompt, cancel); err != nil {
			return err
		} else if !pass {
			return ErrCancelled
		}
	}
	return nil
}

// MirrorRegistry pushes the images in the registry dirs of mounts to the running registry
// of every registry host. The machine running atoll must reach each registry host's
// registry port.
func MirrorRegistry(cluster *v2.Cluster, mounts []v2.MountImage) error {
	registries := cluster.GetRegistryIPAndPortList()
	logger.Debug("registry nodes is: %+v", registries)
	sshClient := ssh.NewCacheClientFromCluster(cluster, true)
	execer, err := exec.New(sshClient)
	if err != nil {
		return err
	}
	syncer := registry.New(constants.NewPathResolver(cluster.GetName()), execer, mounts)
	return syncer.Sync(context.Background(), registries...)
}

func getIndexOfContainerInMounts(mounts []v2.MountImage, imageName string) int {
	for idx, m := range mounts {
		if m.ImageName == imageName {
			return idx
		}
	}
	return -1
}

// MountClusterImages exports the rootfs of every image in cluster.Spec.Image that is not
// exported yet and records it in cluster.Status.Mounts. The images must include a rootfs
// image.
func MountClusterImages(ctx context.Context, images *ClusterImages, cluster *v2.Cluster) error {
	if cluster.Status.Mounts == nil {
		cluster.Status.Mounts = make([]v2.MountImage, 0)
	}
	var hasRootfsType bool
	for _, img := range cluster.Spec.Image {
		info, err := images.Inspect(ctx, img)
		if err != nil {
			return err
		}
		if info.IsRootFs() {
			imageVersion := maps.GetFromKeys(info.Labels, v2.ImageVersionKeys...)
			if !slices.Contains(v2.ImageVersionList, imageVersion) {
				return fmt.Errorf("can't apply rootfs type images and version %s not %+v",
					imageVersion, v2.ImageVersionList)
			}
			hasRootfsType = true
		}

		idx := getIndexOfContainerInMounts(cluster.Status.Mounts, img)
		var existing *v2.MountImage
		if idx >= 0 {
			existing = &cluster.Status.Mounts[idx]
		}
		mount, err := images.Mount(ctx, img, existing)
		if err != nil {
			return err
		}
		if idx >= 0 {
			mount.Env = maps.Merge(mount.Env, cluster.Status.Mounts[idx].Env)
			cluster.Status.Mounts[idx] = mount
		} else {
			cluster.Status.Mounts = append(cluster.Status.Mounts, mount)
		}
	}

	if !hasRootfsType {
		return fmt.Errorf("cluster images %v include no %s type image", cluster.Spec.Image, v2.RootfsImage)
	}
	return nil
}

// DropObsoleteRootfs removes from cluster.Spec.Image, and from cluster.Status.Mounts with
// their exports, the rootfs images other than the active one in cluster.Status.Mounts.
// Older inventories retained replaced rootfs names in spec.image; scaling must distribute
// the committed rootfs, including when an old image is listed before the active image.
func DropObsoleteRootfs(ctx context.Context, images *ClusterImages, cluster *v2.Cluster) error {
	active := cluster.GetRootfsImage()
	if active == nil {
		return nil
	}
	var obsolete []string
	for _, img := range cluster.Spec.Image {
		if img == active.ImageName {
			continue
		}
		info, err := images.Inspect(ctx, img)
		if err != nil {
			return err
		}
		if info.IsRootFs() {
			obsolete = append(obsolete, img)
		}
	}
	kept := cluster.Spec.Image[:0]
	for _, image := range cluster.Spec.Image {
		if !slices.Contains(obsolete, image) {
			kept = append(kept, image)
		}
	}
	cluster.Spec.Image = kept
	mounts := cluster.Status.Mounts[:0]
	for _, mount := range cluster.Status.Mounts {
		if !slices.Contains(obsolete, mount.ImageName) {
			mounts = append(mounts, mount)
			continue
		}
		if err := images.Remove(mount); err != nil {
			return err
		}
	}
	cluster.Status.Mounts = mounts
	return nil
}
