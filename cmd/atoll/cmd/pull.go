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

package cmd

import (
	"context"
	"fmt"

	"github.com/containerd/platforms"
	"github.com/oras-project/oras-go/v3/registry/remote/config"
	"github.com/spf13/cobra"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterimage"
	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/file"
)

var examplePull = `
pull a cluster image for the platform of this host:
    atoll pull ghcr.io/cpaas-dev/atoll/kubernetes:v1.33.6

pull an arm64 image from a registry served over plain HTTP:
    atoll pull --platform linux/arm64 --plain-http harbor.example.com/atoll/kubernetes:v1.33.6

pull from an HTTPS registry with a self-signed certificate:
    atoll pull --insecure harbor.example.com/atoll/kubernetes:v1.33.6
`

func newPullCmd() *cobra.Command {
	var (
		platform  string
		transport clusterimage.Transport
	)
	cmd := &cobra.Command{
		Use:   "pull IMAGE [IMAGE...]",
		Short: "Pull cluster images into the local image store",
		Long: `Pull cluster images into the local image store, the only images run, apply and the
other cluster commands use. Registry credentials, mirrors, short names and image policy
come from the standard files: Docker config.json, containers auth.json, registries.conf,
policy.json and certs.d.`,
		Example: examplePull,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := platforms.Parse(platform)
			if err != nil {
				return fmt.Errorf("invalid --platform %q: %w", platform, err)
			}
			store, err := openImageStore(cmd.Context())
			if err != nil {
				return err
			}
			for _, ref := range args {
				img, err := store.Pull(cmd.Context(), ref, target, transport)
				if err != nil {
					return err
				}
				fmt.Printf("%s %s\n", img.Name, img.Manifest.Digest)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&platform, "platform", platforms.Format(clusterimage.HostPlatform()),
		"platform of the image to pull, in the form os/arch[/variant]")
	cmd.Flags().BoolVar(&transport.PlainHTTP, "plain-http", false,
		"connect to the registry over HTTP instead of HTTPS")
	cmd.Flags().BoolVar(&transport.SkipTLSVerify, "insecure", false,
		"accept any TLS certificate of the registry")
	return cmd
}

// openImageStore opens the local image store with the registry client configuration of
// this host.
func openImageStore(ctx context.Context) (*clusterimage.Store, error) {
	configs, err := config.LoadConfigs()
	if err != nil {
		return nil, fmt.Errorf("load registry client configuration: %w", err)
	}
	return clusterimage.Open(ctx, constants.ImageStorePath(), configs)
}

// importArchives imports the OCI archive tarballs among refs into the local image store
// and returns refs with each tarball replaced by the names of its images.
func importArchives(ctx context.Context, refs []string) ([]string, error) {
	var (
		store  *clusterimage.Store
		images []string
	)
	for _, ref := range refs {
		if !file.IsTarFile(ref) {
			images = append(images, ref)
			continue
		}
		if store == nil {
			opened, err := openImageStore(ctx)
			if err != nil {
				return nil, err
			}
			store = opened
		}
		imported, err := store.Import(ctx, ref, clusterimage.HostPlatform())
		if err != nil {
			return nil, err
		}
		for _, img := range imported {
			images = append(images, img.Name)
		}
	}
	return images, nil
}
