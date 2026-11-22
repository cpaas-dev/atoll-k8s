/*
Copyright 2023 cuisongliu@qq.com.

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

package apply

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterimage"
	cit "github.com/cpaas-dev/atoll-k8s/internal/clusterimage/clusterimagetest"
	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
)

const genTestImage = "registry.example.com/cluster/kubernetes:v1.30.3"

// storeGenTestImage points the runtime root at a temporary directory and stores a rootfs
// image there as genTestImage.
func storeGenTestImage(t *testing.T) {
	t.Helper()
	previous := constants.DefaultRuntimeRootDir
	constants.DefaultRuntimeRootDir = t.TempDir()
	t.Cleanup(func() { constants.DefaultRuntimeRootDir = previous })
	img := cit.NewImage(clusterimage.HostPlatform(), cit.LayerTar(t, cit.Dir("cpaas/", 0o755)))
	img.Config.Labels = map[string]string{
		v2.ImageTypeKeys[0]:    string(v2.RootfsImage),
		v2.ImageVersionKeys[0]: v2.ImageVersionList[0],
		v2.ImageKubeVersionKey: "v1.30.3",
		v2.ImageVIPKey:         "$defaultVIP",
	}
	img.Config.Env = []string{"PATH=/usr/bin", "registryPort=5000", "defaultVIP=10.103.97.2"}
	cit.Tag(t, constants.ImageStorePath(), genTestImage, img)
}

func TestNewClusterFromGenArgs(t *testing.T) {
	tests := []struct {
		name    string
		masters string
	}{
		{name: "default", masters: "172.16.1.35"},
		{name: "Success_EmptyHostsAndSSH", masters: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storeGenTestImage(t)
			cmd := &cobra.Command{Use: "mock"}
			cmd.SetContext(context.Background())
			args := &RunArgs{Cluster: &Cluster{Masters: tt.masters, ClusterName: "default"}}

			got, err := NewClusterFromGenArgs(cmd, args, []string{genTestImage})

			if err != nil {
				t.Fatalf("generate Clusterfile: %v", err)
			}
			if !strings.Contains(string(got), genTestImage) {
				t.Fatalf("generated Clusterfile does not use %s:\n%s", genTestImage, got)
			}
		})
	}
}

func TestGenImageInfo(t *testing.T) {
	storeGenTestImage(t)

	got, err := genImageInfo(context.Background(), "default", genTestImage)

	if err != nil {
		t.Fatalf("gen image info: %v", err)
	}
	if got.Name != "default" || got.Type != v2.RootfsImage || got.Env["registryPort"] != "5000" || got.Env["PATH"] != "" {
		t.Fatalf("gen image info = %+v", got)
	}
}

func TestGenImageInfoNeedsPulledImage(t *testing.T) {
	storeGenTestImage(t)

	_, err := genImageInfo(context.Background(), "default", "registry.example.com/cluster/kubernetes:missing")

	if !errors.Is(err, clusterimage.ErrImageNotFound) {
		t.Fatalf("gen image info error = %v, want %v", err, clusterimage.ErrImageNotFound)
	}
}
