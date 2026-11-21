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
	"errors"
	"fmt"
	"path"
	"slices"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	imagecopy "go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/manifest"
	itype "go.podman.io/image/v5/types"

	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

// Selection decides what is saved from the top-level manifest of a source image. It is
// created by SelectPlatforms or SelectAll.
type Selection interface {
	// saveIndex stores idx, the top-level manifest of a source image, in dst.
	saveIndex(dst destination, idx v1.ImageIndex) error
	// saveImage stores img, the top-level manifest of a source image, in dst.
	saveImage(dst destination, img v1.Image) error
	// localCopy returns how containers/image copies image lists from local transports and
	// the system context choosing their platform.
	localCopy() (imagecopy.ImageListSelection, *itype.SystemContext)
}

// platformSelection saves the requested platforms of image indexes.
type platformSelection struct {
	platforms []ispec.Platform
}

// allSelection saves image indexes with every platform.
type allSelection struct{}

// SelectPlatforms saves the given platforms of image indexes. With exactly one platform, a
// tag reference stores the image of that platform, like containers/image
// copy.CopySystemImage. With more platforms, the original index is stored once after checking
// that it provides every platform. Every platform needs an OS and an architecture.
func SelectPlatforms(platforms []ispec.Platform) (Selection, error) {
	if len(platforms) == 0 {
		return nil, errors.New("select platforms: no platform requested")
	}
	for _, platform := range platforms {
		if platform.OS == "" || platform.Architecture == "" {
			return nil, fmt.Errorf("select platforms: platform %q needs an OS and an architecture", platformString(platform))
		}
	}
	return platformSelection{platforms: platforms}, nil
}

// SelectAll saves image indexes unchanged with every platform, like containers/image
// copy.CopyAllImages (--all).
func SelectAll() Selection {
	return allSelection{}
}

func (s platformSelection) saveIndex(dst destination, idx v1.ImageIndex) error {
	if len(s.platforms) == 1 {
		return dst.writePlatform(idx, s.platforms[0])
	}
	if err := requirePlatforms(idx, s.platforms); err != nil {
		return fmt.Errorf("check platforms for %s: %w", dst, err)
	}
	return dst.writeIndex(idx)
}

func (s platformSelection) saveImage(dst destination, img v1.Image) error {
	if err := warnUnrequestedPlatform(dst, img, s.platforms); err != nil {
		return err
	}
	return dst.writeImage(img)
}

func (s platformSelection) localCopy() (imagecopy.ImageListSelection, *itype.SystemContext) {
	if len(s.platforms) == 1 {
		return imagecopy.CopySystemImage, platformContext(s.platforms[0])
	}
	return imagecopy.CopyAllImages, &itype.SystemContext{}
}

func (allSelection) saveIndex(dst destination, idx v1.ImageIndex) error {
	return dst.writeIndex(idx)
}

func (allSelection) saveImage(dst destination, img v1.Image) error {
	return dst.writeImage(img)
}

func (allSelection) localCopy() (imagecopy.ImageListSelection, *itype.SystemContext) {
	return imagecopy.CopyAllImages, &itype.SystemContext{}
}

// platformImage returns the image manifest that platformDigest chooses from idx.
func platformImage(idx v1.ImageIndex, platform ispec.Platform) (v1.Image, error) {
	digest, err := platformDigest(idx, platform)
	if err != nil {
		return nil, err
	}
	index, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("parse index manifest: %w", err)
	}
	i := slices.IndexFunc(index.Manifests, func(child v1.Descriptor) bool { return child.Digest == digest })
	if i < 0 {
		return nil, fmt.Errorf("platform %s resolves to %s, which the index does not list", platformString(platform), digest)
	}
	if mediaType := index.Manifests[i].MediaType; !mediaType.IsImage() {
		return nil, fmt.Errorf(
			"platform %s resolves to %s with media type %s, which is not an image manifest",
			platformString(platform), digest, mediaType,
		)
	}
	img, err := idx.Image(digest)
	if err != nil {
		return nil, fmt.Errorf("open image %s of platform %s: %w", digest, platformString(platform), err)
	}
	return img, nil
}

// requirePlatforms fails unless idx provides every platform, so storing a whole index fails
// where storing each platform's image would.
func requirePlatforms(idx v1.ImageIndex, platforms []ispec.Platform) error {
	var errs []error
	for _, platform := range platforms {
		if _, err := platformDigest(idx, platform); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// platformDigest returns the digest of the child manifest that containers/image
// copy.CopySystemImage chooses from idx for platform. Unlike go-containerregistry's strict
// matcher, it matches an entry without variant for any requested variant (linux/arm64 for
// linux/arm64/v8) and a request without variant to the variants containers/image knows for
// the architecture (linux/arm64/v8 for linux/arm64), preferring the most compatible entry.
func platformDigest(idx v1.ImageIndex, platform ispec.Platform) (v1.Hash, error) {
	raw, err := idx.RawManifest()
	if err != nil {
		return v1.Hash{}, fmt.Errorf("read index manifest: %w", err)
	}
	mediaType, err := idx.MediaType()
	if err != nil {
		return v1.Hash{}, fmt.Errorf("read index media type: %w", err)
	}
	list, err := manifest.ListFromBlob(raw, string(mediaType))
	if err != nil {
		return v1.Hash{}, fmt.Errorf("parse index manifest of media type %s: %w", mediaType, err)
	}
	chosen, err := list.ChooseInstance(platformContext(platform))
	if err != nil {
		return v1.Hash{}, fmt.Errorf("choose platform %s: %w", platformString(platform), err)
	}
	digest, err := v1.NewHash(chosen.String())
	if err != nil {
		return v1.Hash{}, fmt.Errorf("parse digest %s chosen for platform %s: %w", chosen, platformString(platform), err)
	}
	return digest, nil
}

// platformContext returns the containers/image system context choosing platform.
func platformContext(platform ispec.Platform) *itype.SystemContext {
	return &itype.SystemContext{
		OSChoice:           platform.OS,
		ArchitectureChoice: platform.Architecture,
		VariantChoice:      platform.Variant,
	}
}

// warnUnrequestedPlatform logs a warning when img, a single-manifest image, is built for none
// of platforms; OS and architecture are compared because image configs often omit the
// variant. The image is still saved unchanged: there is nothing to choose from, and
// containers/image copy.CopySystemImage copies such an image to a registry without checking
// its platform either.
func warnUnrequestedPlatform(dst destination, img v1.Image, platforms []ispec.Platform) error {
	config, err := img.ConfigFile()
	if err != nil {
		return fmt.Errorf("read image config of %s to check its platform: %w", dst, err)
	}
	built := ispec.Platform{OS: config.OS, Architecture: config.Architecture, Variant: config.Variant}
	if slices.ContainsFunc(platforms, func(platform ispec.Platform) bool {
		return platform.OS == built.OS && platform.Architecture == built.Architecture
	}) {
		return nil
	}
	requested := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		requested = append(requested, platformString(platform))
	}
	logger.Warn("image %s is built for platform %q, none of the requested platforms %v; saving it unchanged",
		dst, platformString(built), requested)
	return nil
}

// platformString formats platform as os/architecture[/variant].
func platformString(platform ispec.Platform) string {
	return path.Join(platform.OS, platform.Architecture, platform.Variant)
}
