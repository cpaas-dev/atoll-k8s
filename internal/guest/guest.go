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

package guest

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/cpaas-dev/atoll-k8s/internal/env"
	"github.com/cpaas-dev/atoll-k8s/internal/exec"
	"github.com/cpaas-dev/atoll-k8s/internal/ssh"
	"github.com/cpaas-dev/atoll-k8s/internal/third_party/expansion"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/maps"
	stringsutil "github.com/cpaas-dev/atoll-k8s/internal/utils/strings"
)

type Interface interface {
	Apply(cluster *v2.Cluster, mounts []v2.MountImage, targetHosts []string) error
	Delete(cluster *v2.Cluster) error
}

type Default struct{}

func NewGuestManager() (Interface, error) {
	return &Default{}, nil
}

func (d *Default) Apply(cluster *v2.Cluster, mounts []v2.MountImage, targetHosts []string) error {
	envGetter := env.NewEnvProcessor(cluster)
	sshClient := ssh.NewCacheClientFromCluster(cluster, true)
	execer, err := exec.New(sshClient)
	if err != nil {
		return err
	}

	for i, m := range mounts {
		switch {
		case m.IsRootFs(), m.IsPatch():
			eg, ctx := errgroup.WithContext(context.Background())
			for j := range targetHosts {
				node := targetHosts[j]
				// Standalone control-plane binaries and services are managed
				// by the dedicated lifecycle, so the rootfs image hooks never
				// run on those hosts. Patch images are the supported way to
				// layer host-level changes and still execute everywhere; the
				// patch scripts themselves decide what to do per host.
				if m.IsRootFs() && cluster.IsStandaloneControlPlane() &&
					isControlPlaneHost(cluster, node) {
					continue
				}
				envs := maps.Merge(m.Env, envGetter.Getenv(node))
				cmds := formalizeImageCommands(cluster, i, m, envs)
				eg.Go(func() error {
					return execer.CmdAsyncWithContext(ctx, node,
						stringsutil.RenderShellWithEnv(strings.Join(cmds, "; "), envs),
					)
				})
			}
			if err := eg.Wait(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("mount %s of image %s has unsupported type %q", m.Name, m.ImageName, m.Type)
		}
	}
	return nil
}

func isControlPlaneHost(cluster *v2.Cluster, host string) bool {
	if slices.Contains(cluster.GetMasterIPAndPortList(), host) {
		return true
	}
	return slices.Contains(cluster.GetMasterIPList(), host)
}

func formalizeImageCommands(
	cluster *v2.Cluster,
	index int,
	m v2.MountImage,
	extraEnvs map[string]string,
) []string {
	envs := maps.Merge(m.Env, extraEnvs)
	envs = v2.MergeEnvWithBuiltinKeys(envs, m)
	mapping := expansion.MappingFuncFor(envs)

	cmds := make([]string, 0)
	for i := range m.Entrypoint {
		cmds = append(
			cmds,
			FormalizeWorkingCommand(
				cluster.Name,
				expansion.Expand(m.Entrypoint[i], mapping),
			),
		)
	}
	if index == 0 && len(cluster.Spec.Command) > 0 {
		for i := range cluster.Spec.Command {
			cmds = append(
				cmds,
				FormalizeWorkingCommand(
					cluster.Name,
					expansion.Expand(cluster.Spec.Command[i], mapping),
				),
			)
		}
	} else {
		for i := range m.Cmd {
			cmds = append(
				cmds,
				FormalizeWorkingCommand(
					cluster.Name,
					expansion.Expand(m.Cmd[i], mapping),
				),
			)
		}
	}

	return cmds
}

func (d Default) Delete(_ *v2.Cluster) error {
	panic("not yet implemented")
}
