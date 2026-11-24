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
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterfile"
	"github.com/cpaas-dev/atoll-k8s/internal/config"
	"github.com/cpaas-dev/atoll-k8s/internal/filesystem/rootfs"
	"github.com/cpaas-dev/atoll-k8s/internal/guest"
	"github.com/cpaas-dev/atoll-k8s/internal/runtime"
	"github.com/cpaas-dev/atoll-k8s/internal/runtime/factory"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/confirm"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/maps"
	stringsutil "github.com/cpaas-dev/atoll-k8s/internal/utils/strings"
)

var ForceOverride bool

type InstallProcessor struct {
	maintenanceContext context.Context
	ClusterFile        clusterfile.Interface
	Images             *ClusterImages
	Runtime            runtime.Interface
	Guest              guest.Interface
	NewMounts          []v2.MountImage
	NewImages          []string
	ExtraEnvs          map[string]string // parsing from CLI arguments
	imagesToOverride   []string
}

func (c *InstallProcessor) Execute(cluster *v2.Cluster) error {
	pipLine, err := c.GetPipeLine()
	if err != nil {
		return err
	}

	for _, f := range pipLine {
		if err := checkMaintenanceContext(c.maintenanceContext); err != nil {
			return err
		}
		if err = f(cluster); err != nil {
			return err
		}
	}

	return nil
}

func (c *InstallProcessor) GetPipeLine() ([]func(cluster *v2.Cluster) error, error) {
	var todoList []func(cluster *v2.Cluster) error
	todoList = append(todoList,
		c.SyncStatusAndCheck,
		c.ConfirmOverrideImages,
		c.PreProcess,
		c.RunConfig,
		c.MountRootfs,
		c.MirrorRegistry,
		c.UpgradeIfNeed,
		// i.GetPhasePluginFunc(plugin.PhasePreGuest),
		c.RunGuest,
		c.PostProcess,
		// i.GetPhasePluginFunc(plugin.PhasePostInstall),
	)
	return todoList, nil
}

func (c *InstallProcessor) SyncStatusAndCheck(_ *v2.Cluster) error {
	logger.Info("Executing SyncStatusAndCheck Pipeline in InstallProcessor")
	err := c.ClusterFile.Process()
	if err != nil {
		return err
	}
	current := c.ClusterFile.GetCluster()
	if err = SyncClusterStatus(current, false); err != nil {
		return err
	}
	imageList := sets.NewString(current.Spec.Image...)
	for _, img := range c.NewImages {
		if imageList.Has(img) {
			c.imagesToOverride = append(c.imagesToOverride, img)
		}
	}
	return nil
}

func (c *InstallProcessor) ConfirmOverrideImages(_ *v2.Cluster) error {
	logger.Info("Executing ConfirmOverrideImages Pipeline in InstallProcessor")

	if ForceOverride || len(c.imagesToOverride) == 0 {
		return nil
	}

	prompt := fmt.Sprintf("are you sure to override these following images? \n%s\t", strings.Join(c.imagesToOverride, "\n"))
	cancelledMsg := "you have canceled to override these images"
	pass, err := confirm.Confirm(prompt, cancelledMsg)
	if err != nil {
		return err
	}
	if !pass {
		// return a cancelled error to stop apply process.
		return ErrCancelled
	}
	ForceOverride = true
	return nil
}

func (c *InstallProcessor) PreProcess(cluster *v2.Cluster) error {
	logger.Info("Executing PreProcess Pipeline in InstallProcessor")
	// Fail before changing the cluster when an image is missing from the local store.
	for _, image := range c.NewImages {
		if _, err := c.Images.Inspect(c.maintenanceContext, image); err != nil {
			return err
		}
	}
	// This code ensures that `mount` always contains the latest `MountImage` instances from `cluster.Status.Mounts`
	// and that each `ImageName` is represented by only one corresponding instance in `mounts`.
	mountIndexes := make(map[string]int)
	for i := range cluster.Status.Mounts {
		mountIndexes[cluster.Status.Mounts[i].ImageName] = i
	}

	indexes := make([]int, 0)
	for i := range mountIndexes {
		indexes = append(indexes, mountIndexes[i])
	}
	sort.Ints(indexes)
	mounts := make([]v2.MountImage, 0)
	for _, index := range indexes {
		mounts = append(mounts, cluster.Status.Mounts[index])
	}
	cluster.Status.Mounts = mounts

	for _, img := range c.NewImages {
		index, mount := cluster.FindImage(img)
		if mount != nil {
			if !ForceOverride {
				continue
			}
			logger.Debug("trying to override image %s", img)
			if err := c.Images.Remove(*mount); err != nil {
				return err
			}
		}
		cluster.Spec.Image = stringsutil.Merge(cluster.Spec.Image, img)
		newMount, err := c.Images.Mount(c.maintenanceContext, img, nil)
		if err != nil {
			return err
		}
		newMount.Env = maps.Merge(newMount.Env, c.ExtraEnvs)
		// This code ensures that `cluster.Status.Mounts` always contains the latest `MountImage` instances
		if index >= 0 {
			cluster.Status.Mounts = append(cluster.Status.Mounts[:index], cluster.Status.Mounts[index+1:]...)
		}
		cluster.Status.Mounts = append(cluster.Status.Mounts, newMount)
		c.NewMounts = append(c.NewMounts, newMount)
	}

	rt, err := factory.New(cluster, c.ClusterFile.GetRuntimeConfig())
	if err != nil {
		return fmt.Errorf("failed to init runtime, %v", err)
	}
	c.Runtime = rt
	setMaintenanceContext(rt, c.maintenanceContext)
	return nil
}

func (c *InstallProcessor) UpgradeIfNeed(cluster *v2.Cluster) error {
	logger.Info("Executing UpgradeIfNeed Pipeline in InstallProcessor")
	for _, img := range c.NewMounts {
		version := img.KubeVersion()
		if version == "" {
			continue
		}
		err := c.Runtime.Upgrade(version)
		if err != nil {
			logger.Error("upgrade cluster failed")
			return err
		}
		//upgrade success; replace the old cluster mount
		previous := slices.Clone(cluster.Status.Mounts)
		cluster.ReplaceRootfsImage()
		for _, mount := range previous {
			if !slices.ContainsFunc(cluster.Status.Mounts, func(m v2.MountImage) bool { return m.Name == mount.Name }) {
				if err := c.Images.Remove(mount); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *InstallProcessor) PostProcess(*v2.Cluster) error {
	if len(c.NewMounts) == 0 {
		logger.Info("no images have been changed")
	} else {
		logger.Info("succeeded in applying images to this cluster")
	}
	return nil
}

func (c *InstallProcessor) RunConfig(_ *v2.Cluster) error {
	if len(c.NewMounts) == 0 {
		return nil
	}
	eg, _ := errgroup.WithContext(context.Background())
	for _, cManifest := range c.NewMounts {
		manifest := cManifest
		eg.Go(func() error {
			cfg := config.NewConfiguration(manifest.ImageName, manifest.MountPoint, c.ClusterFile.GetConfigs())
			return cfg.Dump()
		})
	}
	return eg.Wait()
}

func (c *InstallProcessor) MountRootfs(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline MountRootfs in InstallProcessor.")
	if len(c.NewMounts) == 0 {
		return nil
	}
	hosts := append(cluster.GetMasterIPAndPortList(), cluster.GetNodeIPAndPortList()...)
	fs, err := rootfs.NewRootfsMounter(c.NewMounts)
	if err != nil {
		return err
	}
	return fs.MountRootfs(cluster, hosts)
}

func (c *InstallProcessor) MirrorRegistry(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline MirrorRegistry in InstallProcessor.")
	return MirrorRegistry(cluster, c.NewMounts)
}

func (c *InstallProcessor) RunGuest(cluster *v2.Cluster) error {
	if len(c.NewMounts) == 0 {
		return nil
	}
	return c.Guest.Apply(cluster, c.NewMounts, cluster.GetAllIPS())
}

func NewInstallProcessor(ctx context.Context, clusterFile clusterfile.Interface, images []string) (Interface, error) {
	clusterImages, err := NewClusterImages(ctx, clusterFile.GetCluster().Name)
	if err != nil {
		return nil, err
	}

	gs, err := guest.NewGuestManager()
	if err != nil {
		return nil, err
	}

	return &InstallProcessor{
		maintenanceContext: ctx,
		ClusterFile:        clusterFile,
		Images:             clusterImages,
		Guest:              gs,
		NewImages:          images,
		ExtraEnvs:          GetEnvs(ctx),
	}, nil
}
