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

	"github.com/cpaas-dev/atoll-k8s/internal/bootstrap"
	"github.com/cpaas-dev/atoll-k8s/internal/checker"
	"github.com/cpaas-dev/atoll-k8s/internal/clusterfile"
	"github.com/cpaas-dev/atoll-k8s/internal/config"
	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/filesystem/rootfs"
	"github.com/cpaas-dev/atoll-k8s/internal/guest"
	"github.com/cpaas-dev/atoll-k8s/internal/runtime"
	"github.com/cpaas-dev/atoll-k8s/internal/runtime/factory"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/maps"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/yaml"
	"golang.org/x/sync/errgroup"
)

type ScaleProcessor struct {
	maintenanceContext context.Context
	ClusterFile        clusterfile.Interface
	Runtime            runtime.Interface
	Images             *ClusterImages
	pullImages         []string
	MastersToJoin      []string
	MastersToDelete    []string
	NodesToJoin        []string
	NodesToDelete      []string
	IsScaleUp          bool
	Guest              guest.Interface
}

func (c *ScaleProcessor) Execute(cluster *v2.Cluster) error {
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

func (c *ScaleProcessor) GetPipeLine() ([]func(cluster *v2.Cluster) error, error) {
	var todoList []func(cluster *v2.Cluster) error
	if c.IsScaleUp {
		todoList = append(todoList,
			c.JoinCheck,
			c.PreProcess,
			c.PreProcessImage,
			c.RunConfig,
			c.MountRootfs,
			c.Bootstrap,
			// s.GetPhasePluginFunc(plugin.PhasePreJoin),
			c.Join,
			c.RunGuest,
			// s.GetPhasePluginFunc(plugin.PhasePostJoin),
		)
		return todoList, nil
	}

	todoList = append(todoList,
		c.DeleteCheck,
		c.PreProcess,
		c.Delete,
		c.UndoBootstrap,
		// c.ApplyCleanPlugin,
		c.UnMountRootfs,
	)
	return todoList, nil
}

func (c *ScaleProcessor) RunGuest(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline RunGuest in ScaleProcessor.")
	hosts := append(c.MastersToJoin, c.NodesToJoin...)
	err := c.Guest.Apply(cluster, cluster.Status.Mounts, hosts)
	if err != nil {
		return fmt.Errorf("%s: %w", RunGuestFailed, err)
	}
	return nil
}

func (c *ScaleProcessor) Delete(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline Delete in ScaleProcessor.")
	err := c.Runtime.ScaleDown(c.MastersToDelete, c.NodesToDelete)
	if err != nil {
		return err
	}
	if len(c.MastersToDelete) > 0 {
		return c.Runtime.SyncNodeIPVS(
			cluster.GetMasterIPAndPortList(),
			cluster.GetNodeIPAndPortList(),
		)
	}
	return nil
}

func (c *ScaleProcessor) Join(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline Join in ScaleProcessor.")
	err := c.Runtime.ScaleUp(c.MastersToJoin, c.NodesToJoin)
	if err != nil {
		return err
	}
	if len(c.MastersToJoin) > 0 {
		return c.Runtime.SyncNodeIPVS(
			cluster.GetMasterIPAndPortList(),
			cluster.GetNodeIPAndPortList(),
		)
	}
	return c.Runtime.SyncNodeIPVS(cluster.GetMasterIPAndPortList(), c.NodesToJoin)
}

func (c ScaleProcessor) UnMountRootfs(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline UnMountRootfs in ScaleProcessor.")
	hosts := append(c.MastersToDelete, c.NodesToDelete...)
	if cluster.Status.Mounts == nil {
		logger.Warn("delete process unmount rootfs skip is cluster not mount rootfs")
		return nil
	}
	fs, err := rootfs.NewRootfsMounter(cluster.Status.Mounts)
	if err != nil {
		return err
	}
	return fs.UnMountRootfs(c.ClusterFile.GetCluster(), hosts)
}

func (c *ScaleProcessor) JoinCheck(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline JoinCheck in ScaleProcessor.")
	var ips, scales []string
	ips = append(ips, cluster.GetMaster0IPAndPort())
	scales = append(c.MastersToJoin, c.NodesToJoin...)
	ips = append(ips, scales...)
	return NewCheckError(
		checker.RunCheckList(
			[]checker.Interface{
				checker.NewIPsHostChecker(ips),
				checker.NewContainerdChecker(scales),
			},
			cluster,
			checker.PhasePre,
		),
	)
}

func (c *ScaleProcessor) DeleteCheck(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline DeleteCheck in ScaleProcessor.")
	var ips []string
	ips = append(ips, cluster.GetMaster0IPAndPort())
	// ips = append(ips, c.MastersToDelete...)
	// ips = append(ips, c.NodesToDelete...)
	return NewCheckError(
		checker.RunCheckList(
			[]checker.Interface{checker.NewIPsHostChecker(ips)},
			cluster,
			checker.PhasePre,
		),
	)
}

func (c *ScaleProcessor) PreProcess(cluster *v2.Cluster) error {
	return NewPreProcessError(c.preProcess(cluster))
}

func (c *ScaleProcessor) preProcess(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline PreProcess in ScaleProcessor.")
	err := c.ClusterFile.Process()
	if err != nil {
		return err
	}
	if c.IsScaleUp {
		if err = DropObsoleteRootfs(c.maintenanceContext, c.Images, cluster); err != nil {
			return err
		}
		// cluster status might be overwrite by inappropriate usage, add mounts if loss.
		if err = MountClusterImages(c.maintenanceContext, c.Images, cluster); err != nil {
			return err
		}
		if cluster.GetRootfsImage().KubeVersion() == "" {
			return fmt.Errorf("rootfs image not found kube version")
		}
		clusterPath := constants.Clusterfile(cluster.Name)
		obj := []any{cluster}
		if configs := c.ClusterFile.GetConfigs(); len(configs) > 0 {
			for i := range configs {
				obj = append(obj, configs[i])
			}
		}
		if !cluster.IsStandaloneControlPlane() {
			if err = yaml.MarshalFile(clusterPath, obj...); err != nil {
				return err
			}
		}
	}
	if err = SyncClusterStatus(cluster, false); err != nil {
		return err
	}

	var rt runtime.Interface
	if c.IsScaleUp {
		rt, err = factory.New(cluster, c.ClusterFile.GetRuntimeConfig())
	} else {
		rt, err = factory.New(c.ClusterFile.GetCluster(), c.ClusterFile.GetRuntimeConfig())
	}
	if err != nil {
		return fmt.Errorf("failed to init runtime: %v", err)
	}
	c.Runtime = rt
	setMaintenanceContext(rt, c.maintenanceContext)

	return err
}

func (c *ScaleProcessor) PreProcessImage(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline PreProcessImage in ScaleProcessor.")

	for i, mount := range cluster.Status.Mounts {
		exported, err := c.Images.Mount(c.maintenanceContext, mount.ImageName, &mount)
		if err != nil {
			return err
		}
		exported.Env = maps.Merge(exported.Env, mount.Env)
		cluster.Status.Mounts[i] = exported
	}

	return nil
}

func (c *ScaleProcessor) RunConfig(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline RunConfig in ScaleProcessor.")
	eg, _ := errgroup.WithContext(context.Background())
	for _, cManifest := range cluster.Status.Mounts {
		manifest := cManifest
		eg.Go(func() error {
			cfg := config.NewConfiguration(
				manifest.ImageName,
				manifest.MountPoint,
				c.ClusterFile.GetConfigs(),
			)
			return cfg.Dump()
		})
	}
	return eg.Wait()
}

func (c *ScaleProcessor) MountRootfs(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline MountRootfs in ScaleProcessor.")
	hosts := append(c.MastersToJoin, c.NodesToJoin...)
	mounts, err := mountsInImageOrder(cluster)
	if err != nil {
		return err
	}
	fs, err := rootfs.NewRootfsMounter(mounts)
	if err != nil {
		return err
	}
	return fs.MountRootfs(cluster, hosts)
}

// mountsInImageOrder returns the mounts of cluster.Spec.Image in the order of the images.
func mountsInImageOrder(cluster *v2.Cluster) ([]v2.MountImage, error) {
	ret := make([]v2.MountImage, 0)
	for _, img := range cluster.Spec.Image {
		idx := getIndexOfContainerInMounts(cluster.Status.Mounts, img)
		if idx == -1 {
			return ret, fmt.Errorf("image %s not mount", img)
		}
		ret = append(ret, cluster.Status.Mounts[idx])
	}
	return ret, nil
}

func (c *ScaleProcessor) Bootstrap(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline Bootstrap in ScaleProcessor")
	hosts := append(c.MastersToJoin, c.NodesToJoin...)
	bs := bootstrap.New(cluster, c.maintenanceContext)
	return bs.Apply(hosts...)
}

func (c *ScaleProcessor) UndoBootstrap(_ *v2.Cluster) error {
	logger.Info("Executing pipeline UndoBootstrap in ScaleProcessor")
	hosts := append(c.MastersToDelete, c.NodesToDelete...)
	bs := bootstrap.New(c.ClusterFile.GetCluster(), c.maintenanceContext)
	return bs.Delete(hosts...)
}

func NewScaleProcessor(
	clusterFile clusterfile.Interface,
	name string,
	images v2.ImageList,
	masterToJoin, masterToDelete, nodeToJoin, nodeToDelete []string,
	contexts ...context.Context,
) (Interface, error) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	clusterImages, err := NewClusterImages(ctx, name)
	if err != nil {
		return nil, err
	}
	gs, err := guest.NewGuestManager()
	if err != nil {
		return nil, err
	}
	return &ScaleProcessor{
		maintenanceContext: ctx,
		MastersToDelete:    masterToDelete,
		MastersToJoin:      masterToJoin,
		NodesToDelete:      nodeToDelete,
		NodesToJoin:        nodeToJoin,
		ClusterFile:        clusterFile,
		Images:             clusterImages,
		pullImages:         images,
		IsScaleUp:          len(masterToJoin) > 0 || len(nodeToJoin) > 0,
		Guest:              gs,
	}, nil
}
