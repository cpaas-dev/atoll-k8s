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
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
)

// destination is where a source image is stored in a registry root.
type destination interface {
	fmt.Stringer
	// writeImage stores img.
	writeImage(img v1.Image) error
	// writeIndex stores idx with all its child manifests.
	writeIndex(idx v1.ImageIndex) error
	// writePlatform stores the part of idx that serves platform.
	writePlatform(idx v1.ImageIndex, platform ispec.Platform) error
}

// taggedDestination stores a source image referenced by tag as repository:tag.
type taggedDestination struct {
	root       string
	repository string
	tag        string
}

func (d taggedDestination) String() string {
	return d.repository + ":" + d.tag
}

func (d taggedDestination) writeImage(img v1.Image) error {
	return ocilayout.WriteImage(d.root, d.repository, d.tag, img)
}

func (d taggedDestination) writeIndex(idx v1.ImageIndex) error {
	return ocilayout.WriteIndex(d.root, d.repository, d.tag, idx)
}

// writePlatform stores the image of platform under the tag, like containers/image
// copy.CopySystemImage.
func (d taggedDestination) writePlatform(idx v1.ImageIndex, platform ispec.Platform) error {
	img, err := platformImage(idx, platform)
	if err != nil {
		return fmt.Errorf("select platform for %s: %w", d, err)
	}
	return d.writeImage(img)
}

// untaggedDestination stores a source image referenced by digest in repository without a tag.
type untaggedDestination struct {
	root       string
	repository string
	// digest is the digest of the reference, for messages.
	digest string
}

func (d untaggedDestination) String() string {
	return d.repository + "@" + d.digest
}

func (d untaggedDestination) writeImage(img v1.Image) error {
	return ocilayout.WriteUntaggedImage(d.root, d.repository, img)
}

func (d untaggedDestination) writeIndex(idx v1.ImageIndex) error {
	return ocilayout.WriteUntaggedIndex(d.root, d.repository, idx)
}

// writePlatform stores the whole index after checking that it provides platform: the digest
// reference names the index, so storing only the platform's image would leave the reference
// unresolvable in the registry root.
func (d untaggedDestination) writePlatform(idx v1.ImageIndex, platform ispec.Platform) error {
	if err := requirePlatforms(idx, []ispec.Platform{platform}); err != nil {
		return fmt.Errorf("check platform for %s: %w", d, err)
	}
	return d.writeIndex(idx)
}
