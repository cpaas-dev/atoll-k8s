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

package save

import (
	"context"
	"errors"
	"fmt"
	"os"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"go.podman.io/common/pkg/retry"
	imagecopy "go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/docker/reference"
	ocitransport "go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/transports"
	itype "go.podman.io/image/v5/types"

	"github.com/cpaas-dev/atoll-k8s/internal/sreg/registry/sync"
)

// layoutImageName names the copied image in the temporary OCI image layout.
const layoutImageName = "source"

// localSource is an image of a containers/image transport other than docker, such as
// containers-storage:, docker-daemon:, docker-archive: or oci-archive:, saved as repository:tag.
type localSource struct {
	image      string
	ref        itype.ImageReference
	repository string
	tag        string
}

// newLocalImageSource returns the source of a containers-storage: or docker-daemon: image,
// saved under the repository path (without registry host) and tag of its image name, like
// registry references.
func newLocalImageSource(image string) (source, error) {
	// ImageNameToReference re-executes the process in a user namespace when containers-storage
	// needs capabilities the process lacks; sources are parsed before any image is saved.
	ref, err := sync.ImageNameToReference(nil, image, nil)
	if err != nil {
		return nil, fmt.Errorf("parse image %s: %w", image, err)
	}
	named := ref.DockerReference()
	if named == nil {
		return nil, fmt.Errorf("image %s has no image name to save it under", image)
	}
	if _, ok := named.(reference.Digested); ok {
		return nil, fmt.Errorf(
			"image %s is referenced by digest, which saving from the %s transport cannot preserve: reference it by tag",
			image, ref.Transport().Name(),
		)
	}
	tagged, ok := named.(reference.NamedTagged)
	if !ok {
		return nil, fmt.Errorf("image %s has no tag to save it under", image)
	}
	return localSource{image: image, ref: ref, repository: reference.Path(named), tag: tagged.Tag()}, nil
}

func (s localSource) name() string {
	return s.image
}

// save copies the image with containers/image into a temporary OCI image layout and stores it
// from there. OCI image layouts hold OCI manifests only, so containers/image converts Docker
// schema2 manifests and Docker manifest lists to OCI: unlike registry sources, images from
// local transports may get digests that differ from their source.
func (s localSource) save(ctx context.Context, root string, selection Selection) (err error) {
	dir, err := os.MkdirTemp("", "atoll-save-*")
	if err != nil {
		return fmt.Errorf("create temporary OCI layout directory: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove temporary OCI layout %s: %w", dir, removeErr))
		}
	}()
	if err := copyToLayout(ctx, s.ref, dir, selection); err != nil {
		return err
	}
	return saveFromLayout(dir, taggedDestination{root: root, repository: s.repository, tag: s.tag}, selection)
}

// copyToLayout copies ref with containers/image into the OCI image layout dir, choosing from
// image lists as selection does.
func copyToLayout(ctx context.Context, ref itype.ImageReference, dir string, selection Selection) (err error) {
	destRef, err := ocitransport.NewReference(dir, layoutImageName)
	if err != nil {
		return fmt.Errorf("reference OCI layout %s: %w", dir, err)
	}
	policyContext, err := signature.NewPolicyContext(&signature.Policy{
		Default: signature.PolicyRequirements{signature.NewPRInsecureAcceptAnything()},
	})
	if err != nil {
		return fmt.Errorf("create signature policy context: %w", err)
	}
	defer func() {
		if destroyErr := policyContext.Destroy(); destroyErr != nil {
			err = errors.Join(err, fmt.Errorf("destroy signature policy context: %w", destroyErr))
		}
	}()
	listSelection, sys := selection.localCopy()
	return retry.IfNecessary(ctx, func() error {
		_, err := imagecopy.Image(ctx, policyContext, destRef, ref, &imagecopy.Options{
			SourceCtx:          sys,
			DestinationCtx:     sys,
			ImageListSelection: listSelection,
			ReportWriter:       os.Stdout,
		})
		if err != nil {
			return fmt.Errorf("copy %s to OCI layout %s: %w", transports.ImageName(ref), dir, err)
		}
		return nil
	}, &retry.Options{MaxRetry: maxRetry})
}

// saveFromLayout stores the only top-level manifest of the OCI image layout dir in dst.
func saveFromLayout(dir string, dst destination, selection Selection) error {
	idx, err := layout.ImageIndexFromPath(dir)
	if err != nil {
		return fmt.Errorf("open OCI layout %s: %w", dir, err)
	}
	index, err := idx.IndexManifest()
	if err != nil {
		return fmt.Errorf("parse index of OCI layout %s: %w", dir, err)
	}
	if len(index.Manifests) != 1 {
		return fmt.Errorf("OCI layout %s holds %d manifests, want exactly 1", dir, len(index.Manifests))
	}
	desc := index.Manifests[0]
	return saveManifest(selection, dst, desc.MediaType, layoutManifest{index: idx, digest: desc.Digest})
}

// layoutManifest opens a top-level manifest of an OCI image layout.
type layoutManifest struct {
	index  v1.ImageIndex
	digest v1.Hash
}

func (m layoutManifest) Image() (v1.Image, error) {
	return m.index.Image(m.digest)
}

func (m layoutManifest) ImageIndex() (v1.ImageIndex, error) {
	return m.index.ImageIndex(m.digest)
}
