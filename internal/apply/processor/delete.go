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
	"github.com/cpaas-dev/atoll-k8s/internal/clusterfile"
	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/filesystem/rootfs"
	"github.com/cpaas-dev/atoll-k8s/internal/runtime/factory"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	fileutil "github.com/cpaas-dev/atoll-k8s/internal/utils/file"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/strings"
)

var ForceDelete bool

type DeleteProcessor struct {
	ClusterFile clusterfile.Interface
}

// Execute :according to the different of desired cluster to delete cluster.
func (d DeleteProcessor) Execute(cluster *v2.Cluster) (err error) {
	pipLine, err := d.GetPipeLine()
	if err != nil {
		return err
	}
	return runResetPipeline(cluster, pipLine)
}

func runResetPipeline(cluster *v2.Cluster, pipeline []func(*v2.Cluster) error) error {
	// TODO if error is exec net process ???
	for _, f := range pipeline {
		if err := f(cluster); err != nil {
			logger.Warn("failed to exec delete process, %s", err.Error())
		}
	}

	return nil
}

func (d DeleteProcessor) GetPipeLine() ([]func(cluster *v2.Cluster) error, error) {
	var todoList []func(cluster *v2.Cluster) error
	todoList = append(todoList,
		d.PreProcess,
		d.Reset,
		d.UndoBootstrap,
		d.UnMountRootfs,
		// CleanFS also removes the rootfs exports in the mounts dir of the cluster.
		d.CleanFS,
	)
	return todoList, nil
}

func (d *DeleteProcessor) PreProcess(cluster *v2.Cluster) error {
	return NewPreProcessError(SyncClusterStatus(cluster, true))
}

func (d *DeleteProcessor) UndoBootstrap(cluster *v2.Cluster) error {
	logger.Info("Executing pipeline Bootstrap in DeleteProcessor")
	hosts := append(cluster.GetMasterIPAndPortList(), cluster.GetNodeIPAndPortList()...)
	var cls *v2.Cluster
	if v := d.ClusterFile.GetCluster(); v != nil {
		cls = v
	} else {
		cls = cluster
	}
	// Whole-cluster reset continues offline after its API lease disappears.
	bs := bootstrap.New(cls, context.Background())
	return bs.Delete(hosts...)
}

func (d *DeleteProcessor) Reset(cluster *v2.Cluster) error {
	rt, err := factory.New(cluster, d.ClusterFile.GetRuntimeConfig())
	if err != nil {
		return fmt.Errorf("failed to delete runtime, %v", err)
	}
	return rt.Reset()
}

func (d *DeleteProcessor) UnMountRootfs(cluster *v2.Cluster) error {
	hosts := append(cluster.GetMasterIPAndPortList(), cluster.GetNodeIPAndPortList()...)
	if strings.NotInIPList(hosts, cluster.GetRegistryIPAndPort()) {
		hosts = append(hosts, cluster.GetRegistryIPAndPort())
	}
	// umount don't care imageMounts
	fs, err := rootfs.NewRootfsMounter(nil)
	if err != nil {
		return err
	}
	var cls *v2.Cluster
	if v := d.ClusterFile.GetCluster(); v != nil {
		cls = v
	} else {
		cls = cluster
	}

	return fs.UnMountRootfs(cls, hosts)
}

func (d *DeleteProcessor) CleanFS(cluster *v2.Cluster) error {
	workDir := constants.ClusterDir(cluster.Name)
	dataDir := constants.NewPathResolver(cluster.Name).Root()
	return fileutil.CleanFiles(workDir, dataDir)
}

func NewDeleteProcessor(name string, clusterFile clusterfile.Interface) (Interface, error) {
	return DeleteProcessor{
		ClusterFile: clusterFile,
	}, nil
}
