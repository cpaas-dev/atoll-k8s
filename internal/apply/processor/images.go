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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/registry/remote/config"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterimage"
	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/file"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/maps"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/rand"
)

// mountRecordSuffix names the file next to an export that records its mount.
const mountRecordSuffix = ".json"

// ClusterImages serves the images of one cluster from the local image store, for the
// platform of this host. Images must be pulled before; nothing is fetched from registries.
// The rootfs of each image is exported to the mounts dir of the cluster, and every export
// is recorded in a file next to it so the mounts survive a Clusterfile without status.
type ClusterImages struct {
	store     *clusterimage.Store
	mountsDir string
	platform  ocispec.Platform
}

// NewClusterImages opens the local image store for the cluster clusterName.
func NewClusterImages(ctx context.Context, clusterName string) (*ClusterImages, error) {
	configs, err := config.LoadConfigs()
	if err != nil {
		return nil, fmt.Errorf("load registry client configuration: %w", err)
	}
	store, err := clusterimage.Open(ctx, constants.ImageStorePath(), configs)
	if err != nil {
		return nil, err
	}
	return &ClusterImages{
		store:     store,
		mountsDir: constants.ClusterMountsDir(clusterName),
		platform:  clusterimage.HostPlatform(),
	}, nil
}

// Inspect returns the mount of the stored image ref without exporting it: image name,
// digest, type, labels, env without PATH, and cmd and entrypoint without "/bin/sh -c".
// Images that are neither rootfs nor patch images are rejected.
func (c *ClusterImages) Inspect(ctx context.Context, ref string) (v2.MountImage, error) {
	_, mount, err := c.lookup(ctx, ref)
	return mount, err
}

func (c *ClusterImages) lookup(ctx context.Context, ref string) (clusterimage.Image, v2.MountImage, error) {
	img, err := c.store.Image(ctx, ref, c.platform)
	if errors.Is(err, clusterimage.ErrImageNotFound) {
		return clusterimage.Image{}, v2.MountImage{}, fmt.Errorf("%w; pull it with: atoll pull %s", err, ref)
	}
	if err != nil {
		return clusterimage.Image{}, v2.MountImage{}, err
	}
	cfg, err := c.store.Config(ctx, img)
	if err != nil {
		return clusterimage.Image{}, v2.MountImage{}, err
	}
	mount := v2.MountImage{
		ImageName:  ref,
		Digest:     img.Manifest.Digest.String(),
		Env:        maps.FromSlice(cfg.Config.Env),
		Labels:     cfg.Config.Labels,
		Entrypoint: withoutShell(cfg.Config.Entrypoint),
		Cmd:        withoutShell(cfg.Config.Cmd),
		Type:       v2.ImageType(maps.GetFromKeys(cfg.Config.Labels, v2.ImageTypeKeys...)),
	}
	if mount.Type != v2.RootfsImage && mount.Type != v2.PatchImage {
		return clusterimage.Image{}, v2.MountImage{}, fmt.Errorf(
			"image %s has type %q in label %v, only %s and %s images are supported",
			ref, mount.Type, v2.ImageTypeKeys, v2.RootfsImage, v2.PatchImage)
	}
	delete(mount.Env, "PATH")
	return img, mount, nil
}

// Mount returns the mount of the stored image ref with its rootfs exported. existing is
// the current mount of ref, nil when there is none. It is returned unchanged when its
// export still holds the stored image; otherwise the image is exported again under the
// same name, or under a new name when existing is nil.
func (c *ClusterImages) Mount(ctx context.Context, ref string, existing *v2.MountImage) (v2.MountImage, error) {
	img, mount, err := c.lookup(ctx, ref)
	if err != nil {
		return v2.MountImage{}, err
	}
	mount.Name = rand.Generator(8)
	if existing != nil {
		current, err := c.exported(*existing)
		if err != nil {
			return v2.MountImage{}, err
		}
		if current && existing.Digest == mount.Digest {
			return *existing, nil
		}
		if err := c.Remove(*existing); err != nil {
			return v2.MountImage{}, err
		}
		if existing.Name != "" {
			mount.Name = existing.Name
		}
	}
	mount.MountPoint = filepath.Join(c.mountsDir, mount.Name)
	if err := c.store.Export(ctx, img, mount.MountPoint); err != nil {
		return v2.MountImage{}, err
	}
	record, err := json.Marshal(mount)
	if err != nil {
		return v2.MountImage{}, fmt.Errorf("encode mount record of %s: %w", ref, err)
	}
	if err := file.AtomicWriteFile(c.recordPath(mount.Name), record, 0o600); err != nil {
		return v2.MountImage{}, fmt.Errorf("write mount record of %s: %w", ref, err)
	}
	return mount, nil
}

// Remove deletes the export of mount and its record. Mounts without an export are
// skipped, such as legacy buildah mounts outside the mounts dir.
func (c *ClusterImages) Remove(mount v2.MountImage) error {
	if mount.Name == "" || mount.MountPoint != filepath.Join(c.mountsDir, mount.Name) {
		return nil
	}
	if err := os.RemoveAll(mount.MountPoint); err != nil {
		return fmt.Errorf("remove rootfs export %s of %s: %w", mount.MountPoint, mount.ImageName, err)
	}
	if err := os.Remove(c.recordPath(mount.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove mount record of %s: %w", mount.ImageName, err)
	}
	return nil
}

// exported reports whether the export of mount exists in the mounts dir.
func (c *ClusterImages) exported(mount v2.MountImage) (bool, error) {
	if mount.Name == "" || mount.MountPoint != filepath.Join(c.mountsDir, mount.Name) {
		return false, nil
	}
	info, err := os.Stat(mount.MountPoint)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check rootfs export %s of %s: %w", mount.MountPoint, mount.ImageName, err)
	}
	return info.IsDir(), nil
}

func (c *ClusterImages) recordPath(name string) string {
	return filepath.Join(c.mountsDir, name+mountRecordSuffix)
}

// recordedMounts returns the mounts recorded in the mounts dir of the cluster clusterName.
func recordedMounts(clusterName string) ([]v2.MountImage, error) {
	dir := constants.ClusterMountsDir(clusterName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read mounts dir %s: %w", dir, err)
	}
	var mounts []v2.MountImage
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), mountRecordSuffix) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read mount record %s: %w", path, err)
		}
		var mount v2.MountImage
		if err := json.Unmarshal(data, &mount); err != nil {
			return nil, fmt.Errorf("decode mount record %s: %w", path, err)
		}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

// withoutShell drops the "/bin/sh" and "-c" arguments the image builder wraps commands in.
func withoutShell(args []string) []string {
	var out []string
	for _, arg := range args {
		if arg == "/bin/sh" || arg == "-c" {
			continue
		}
		out = append(out, arg)
	}
	return out
}
