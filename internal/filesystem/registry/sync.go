/*
Copyright 2023 fengxsong@outlook.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package registry fills the registries of the registry hosts with the images stored in
// the registry roots (see internal/registry/ocilayout) of mounted cluster images.
//
// Images are pushed through the OCI distribution API from the machine running atoll, so
// that machine must reach the registry port (registry.yml, default 5000) of every
// registry host. Registries are addressed as <host IP>:<registry port> over plain HTTP
// with basic auth, never by the registry domain, which only resolves on cluster hosts.
// Manifests are pushed as stored, so digests are preserved byte for byte, Docker schema2
// manifests and manifest lists included.
package registry

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/exec"
	"github.com/cpaas-dev/atoll-k8s/internal/filesystem"
	"github.com/cpaas-dev/atoll-k8s/internal/registry/helpers"
	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/iputils"
)

type impl struct {
	pathResolver constants.PathResolver
	execer       exec.Interface
	mounts       []v2.MountImage
}

// Sync pushes the registry roots of the mounts to the registry of every host in hosts
// (cluster host addresses, ip:ssh-port). The registries must be running, and the machine
// running atoll must reach every host's registry port. Port and credentials are read from
// registry.yml on the first host, as Bootstrap does.
func (s *impl) Sync(ctx context.Context, hosts ...string) error {
	roots, err := registryRoots(s.mounts)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return nil
	}
	if len(hosts) == 0 {
		return errors.New("cannot push images of mounted registry roots: no registry hosts")
	}
	rc := helpers.GetRegistryInfo(s.execer, s.pathResolver.RootFSPath(), hosts[0])
	targets, err := newTargets(hosts, rc)
	if err != nil {
		return err
	}
	return pushToTargets(ctx, roots, targets)
}

// registryRoots returns the registry roots of mounts in mount order, skipping mounts
// without a registry dir and failing on a root ocilayout.CheckRoot rejects.
func registryRoots(mounts []v2.MountImage) ([]registryRoot, error) {
	var roots []registryRoot
	for _, mount := range mounts {
		root := filepath.Join(mount.MountPoint, constants.RegistryDirName)
		exists, err := pathExists(root)
		if err != nil {
			return nil, fmt.Errorf("registry dir of image %s: %w", mount.ImageName, err)
		}
		if !exists {
			continue
		}
		if err := ocilayout.CheckRoot(root); err != nil {
			return nil, fmt.Errorf("registry dir %s of image %s: %w", root, mount.ImageName, err)
		}
		roots = append(roots, registryRoot{image: mount.ImageName, path: root})
	}
	return roots, nil
}

// pathExists reports whether name exists; errors other than non-existence are returned.
func pathExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", name, err)
}

// newTargets returns the registry of every host, addressed by host IP and the port of rc,
// sharing one transport and the basic auth credentials of rc.
func newTargets(hosts []string, rc *v2.RegistryConfig) ([]target, error) {
	if rc.Port == "" || rc.Username == "" || rc.Password == "" {
		return nil, fmt.Errorf(
			"registry config needs port, username and password to push images, got port %q and username %q",
			rc.Port, rc.Username,
		)
	}
	auth := &authn.Basic{Username: rc.Username, Password: rc.Password}
	transport := newTransport()
	targets := make([]target, 0, len(hosts))
	for _, host := range hosts {
		targets = append(targets, target{
			address:   net.JoinHostPort(iputils.GetHostIP(host), rc.Port),
			auth:      auth,
			transport: transport,
			backoff:   pushBackoff,
		})
	}
	return targets, nil
}

// New returns a syncer pushing the registry roots of mounts; see Sync.
func New(pathResolver constants.PathResolver, execer exec.Interface, mounts []v2.MountImage) filesystem.RegistrySyncer {
	return &impl{pathResolver, execer, mounts}
}
