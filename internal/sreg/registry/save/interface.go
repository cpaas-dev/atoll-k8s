// Copyright © 2021 sealos.
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

package save

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/moby/moby/api/types/registry"
	"golang.org/x/sync/errgroup"

	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

// Registry saves images into a registry root: a directory holding one OCI image layout per
// repository (see package ocilayout).
type Registry interface {
	// SaveImages saves images into the registry root dir, choosing from image indexes with
	// selection, and returns the saved images in input order.
	SaveImages(images []string, dir string, selection Selection) ([]string, error)
}

// imageSaver saves registry references and images of the containers-storage: and
// docker-daemon: transports.
type imageSaver struct {
	ctx          context.Context
	maxPullProcs int
	auths        map[string]registry.AuthConfig
}

// tarSaver saves images of archives given as transport:path@repository[:tag].
type tarSaver struct {
	ctx          context.Context
	maxPullProcs int
}

// NewImageSaver returns a Registry saving up to maxPullProcs images at a time from registry
// references (optionally prefixed with docker://), authenticated with auths, and from the
// containers-storage: and docker-daemon: transports.
func NewImageSaver(ctx context.Context, maxPullProcs int, auths map[string]registry.AuthConfig) Registry {
	return &imageSaver{ctx: ctx, maxPullProcs: maxPullProcs, auths: auths}
}

// NewImageTarSaver returns a Registry saving up to maxPullProcs images at a time from
// archives given as transport:path@repository[:tag], such as
// docker-archive:/root/config_main.tar@library/config_main.
func NewImageTarSaver(ctx context.Context, maxPullProcs int) Registry {
	return &tarSaver{ctx: ctx, maxPullProcs: maxPullProcs}
}

// source is one image to save into a registry root.
type source interface {
	// name returns the image as the caller gave it.
	name() string
	// save stores the image in the registry root according to selection.
	save(ctx context.Context, root string, selection Selection) error
}

// saveSources checks the registry root and saves sources, at most maxPullProcs at a time. It
// returns the names of sources in input order, or the first error after cancelling the
// remaining saves.
func saveSources(
	ctx context.Context,
	maxPullProcs int,
	root string,
	sources []source,
	selection Selection,
) ([]string, error) {
	if maxPullProcs < 1 {
		return nil, fmt.Errorf("maximum number of parallel pulls is %d, must be at least 1", maxPullProcs)
	}
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(maxPullProcs)
	for _, src := range sources {
		group.Go(func() error {
			logger.Info("saving image %s into %s", src.name(), root)
			if err := src.save(groupCtx, root, selection); err != nil {
				return fmt.Errorf("save image %s into %s: %w", src.name(), root, err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(sources))
	for _, src := range sources {
		names = append(names, src.name())
	}
	return names, nil
}

// checkRoot refuses an existing registry root that is not made of OCI image layouts, such as
// a legacy distribution storage directory. A missing root is created by the first write.
func checkRoot(root string) error {
	_, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check registry root %s: %w", root, err)
	}
	return ocilayout.CheckRoot(root)
}
