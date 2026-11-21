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
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/moby/api/types/registry"
	"go.podman.io/common/pkg/retry"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/transports/alltransports"

	"github.com/cpaas-dev/atoll-k8s/internal/sreg/registry/crane"
)

const (
	// containersStorageTransport and dockerDaemonTransport are the local transports accepted
	// next to registry references.
	containersStorageTransport = "containers-storage"
	dockerDaemonTransport      = "docker-daemon"
	// dockerTransportPrefix is the optional prefix of registry references.
	dockerTransportPrefix = "docker://"
	// maxRetry is how often saving an image is retried after a retryable error.
	maxRetry = 3
)

func (s *imageSaver) SaveImages(images []string, dir string, selection Selection) ([]string, error) {
	sources := make([]source, 0, len(images))
	for _, image := range images {
		src, err := s.parseImage(image)
		if err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	return saveSources(s.ctx, s.maxPullProcs, dir, sources, selection)
}

// parseImage returns the source of image: a registry reference, optionally prefixed with
// docker://, or an image of the containers-storage: or docker-daemon: transport.
func (s *imageSaver) parseImage(image string) (source, error) {
	transport := alltransports.TransportFromImageName(image)
	if transport == nil {
		return s.registrySource(image, image)
	}
	switch transport.Name() {
	case docker.Transport.Name():
		return s.registrySource(image, strings.TrimPrefix(image, dockerTransportPrefix))
	case containersStorageTransport, dockerDaemonTransport:
		return newLocalImageSource(image)
	}
	return nil, fmt.Errorf(
		"image %s uses the %s transport, which is not supported: use a registry reference, %s, %s: or %s:",
		image, transport.Name(), dockerTransportPrefix, containersStorageTransport, dockerDaemonTransport,
	)
}

// registrySource returns the source of image, whose registry reference is reference.
func (s *imageSaver) registrySource(image, reference string) (source, error) {
	// name.Insecure lets go-containerregistry fall back to plain HTTP when HTTPS fails, and
	// crane.GetRemoteOptions skips TLS verification: the saver always set
	// DockerInsecureSkipTLSVerify, with which containers/image does both for every registry.
	ref, err := name.ParseReference(reference, name.Insecure)
	if err != nil {
		return nil, fmt.Errorf("parse image reference %s: %w", image, err)
	}
	return registrySource{image: image, ref: ref, auths: s.auths}, nil
}

// registrySource is an image in a registry.
type registrySource struct {
	image string
	// ref is a name.Tag or a name.Digest. name.ParseReference drops the tag of a reference
	// with both a tag and a digest, which is therefore saved as a digest reference.
	ref   name.Reference
	auths map[string]registry.AuthConfig
}

func (s registrySource) name() string {
	return s.image
}

// save fetches the top-level manifest of the reference and stores it byte for byte, without
// converting Docker manifests to OCI.
func (s registrySource) save(ctx context.Context, root string, selection Selection) error {
	dst, err := registryDestination(root, s.ref)
	if err != nil {
		return err
	}
	return retry.IfNecessary(ctx, func() error {
		desc, err := remote.Get(s.ref, crane.GetRemoteOptions(ctx, s.auths)...)
		if err != nil {
			return fmt.Errorf("fetch manifest of %s: %w", s.ref, err)
		}
		return saveManifest(selection, dst, desc.MediaType, desc)
	}, &retry.Options{MaxRetry: maxRetry})
}

// registryDestination stores a tag reference under its tag and a digest reference untagged,
// in the repository path of the reference without the registry host, the same path the
// image has in its registry: nginx and docker.io/library/nginx are saved as library/nginx,
// registry.k8s.io/pause as pause.
func registryDestination(root string, ref name.Reference) (destination, error) {
	repository := ref.Context().RepositoryStr()
	switch r := ref.(type) {
	case name.Tag:
		return taggedDestination{root: root, repository: repository, tag: r.TagStr()}, nil
	case name.Digest:
		return untaggedDestination{root: root, repository: repository, digest: r.DigestStr()}, nil
	}
	return nil, fmt.Errorf("reference %s is neither a tag nor a digest reference", ref)
}

// manifestOpener opens the top-level manifest of a source image.
type manifestOpener interface {
	Image() (v1.Image, error)
	ImageIndex() (v1.ImageIndex, error)
}

// saveManifest stores the top-level manifest of a source image, which has mediaType, in dst
// according to selection.
func saveManifest(selection Selection, dst destination, mediaType types.MediaType, opener manifestOpener) error {
	switch {
	case mediaType.IsIndex():
		idx, err := opener.ImageIndex()
		if err != nil {
			return fmt.Errorf("open image index for %s: %w", dst, err)
		}
		return selection.saveIndex(dst, idx)
	case mediaType.IsImage():
		img, err := opener.Image()
		if err != nil {
			return fmt.Errorf("open image for %s: %w", dst, err)
		}
		return selection.saveImage(dst, img)
	}
	return fmt.Errorf("cannot save %s: manifest media type %q is neither an image nor an image index", dst, mediaType)
}
