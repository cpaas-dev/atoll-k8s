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

package ocilayout

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// WriteImage stores img as repository:tag, replacing any previous descriptor for that tag.
func WriteImage(root, repository, tag string, img v1.Image) error {
	if err := validateReference(repository, tag); err != nil {
		return err
	}
	desc, err := imageDescriptor(img)
	if err != nil {
		return fmt.Errorf("write image %s:%s: %w", repository, tag, err)
	}
	err = storeInRepository(root, repository,
		func(p layout.Path) error { return p.WriteImage(img) },
		func(index *v1.IndexManifest) { putTagged(index, tag, desc) },
	)
	if err != nil {
		return fmt.Errorf("write image %s:%s (%s) to %s: %w", repository, tag, desc.Digest, root, err)
	}
	return nil
}

// WriteIndex stores idx as repository:tag and registers its child manifests as untagged descriptors.
func WriteIndex(root, repository, tag string, idx v1.ImageIndex) error {
	if err := validateReference(repository, tag); err != nil {
		return err
	}
	desc, err := indexDescriptor(idx)
	if err != nil {
		return fmt.Errorf("write index %s:%s: %w", repository, tag, err)
	}
	children, err := childDescriptors(idx)
	if err != nil {
		return fmt.Errorf("write index %s:%s (%s): %w", repository, tag, desc.Digest, err)
	}
	err = storeInRepository(root, repository,
		func(p layout.Path) error { return p.WriteIndex(idx) },
		func(index *v1.IndexManifest) {
			putTagged(index, tag, desc)
			addUntagged(index, children)
		},
	)
	if err != nil {
		return fmt.Errorf("write index %s:%s (%s) to %s: %w", repository, tag, desc.Digest, root, err)
	}
	return nil
}

// WriteUntaggedImage stores img in repository without a tag (for digest-only references).
func WriteUntaggedImage(root, repository string, img v1.Image) error {
	if err := validateRepository(repository); err != nil {
		return err
	}
	desc, err := imageDescriptor(img)
	if err != nil {
		return fmt.Errorf("write untagged image to repository %s: %w", repository, err)
	}
	err = storeInRepository(root, repository,
		func(p layout.Path) error { return p.WriteImage(img) },
		func(index *v1.IndexManifest) { addUntagged(index, []v1.Descriptor{desc}) },
	)
	if err != nil {
		return fmt.Errorf("write untagged image %s@%s to %s: %w", repository, desc.Digest, root, err)
	}
	return nil
}

// WriteUntaggedIndex stores idx and its children in repository without a tag.
func WriteUntaggedIndex(root, repository string, idx v1.ImageIndex) error {
	if err := validateRepository(repository); err != nil {
		return err
	}
	desc, err := indexDescriptor(idx)
	if err != nil {
		return fmt.Errorf("write untagged index to repository %s: %w", repository, err)
	}
	children, err := childDescriptors(idx)
	if err != nil {
		return fmt.Errorf("write untagged index %s@%s: %w", repository, desc.Digest, err)
	}
	err = storeInRepository(root, repository,
		func(p layout.Path) error { return p.WriteIndex(idx) },
		func(index *v1.IndexManifest) { addUntagged(index, append([]v1.Descriptor{desc}, children...)) },
	)
	if err != nil {
		return fmt.Errorf("write untagged index %s@%s to %s: %w", repository, desc.Digest, root, err)
	}
	return nil
}

// storeInRepository writes blobs with writeBlobs, ensures the layout files, then applies
// update to index.json and replaces it atomically, holding the repository lock throughout.
func storeInRepository(
	root, repository string,
	writeBlobs func(layout.Path) error,
	update func(*v1.IndexManifest),
) error {
	dir := RepositoryDir(root, repository)
	unlock, err := lockRepositories([]string{dir})
	if err != nil {
		return err
	}
	defer unlock()

	if err := writeBlobs(layout.Path(dir)); err != nil {
		return fmt.Errorf("write blobs to %s: %w", dir, err)
	}
	if err := ensureLayoutFiles(dir); err != nil {
		return err
	}
	index, err := readIndexOrEmpty(dir)
	if err != nil {
		return err
	}
	update(index)
	if err := writeIndex(dir, index); err != nil {
		return err
	}
	return normalizeModes(root, repository)
}

// imageDescriptor returns the untagged descriptor of img, which must be an OCI image
// manifest or a Docker schema2 manifest.
func imageDescriptor(img v1.Image) (v1.Descriptor, error) {
	desc, err := untaggedDescriptor(img)
	if err != nil {
		return v1.Descriptor{}, err
	}
	if !desc.MediaType.IsImage() {
		return v1.Descriptor{}, fmt.Errorf(
			"manifest %s has media type %q, which is not an image manifest", desc.Digest, desc.MediaType,
		)
	}
	return desc, nil
}

// indexDescriptor returns the untagged descriptor of idx, which must be an OCI image index
// or a Docker manifest list.
func indexDescriptor(idx v1.ImageIndex) (v1.Descriptor, error) {
	desc, err := untaggedDescriptor(idx)
	if err != nil {
		return v1.Descriptor{}, err
	}
	if !desc.MediaType.IsIndex() {
		return v1.Descriptor{}, fmt.Errorf(
			"manifest %s has media type %q, which is not an image index", desc.Digest, desc.MediaType,
		)
	}
	return desc, nil
}

// untaggedDescriptor builds the minimal descriptor zot itself writes for a manifest pushed
// by digest: media type, digest and size only.
func untaggedDescriptor(d partial.Describable) (v1.Descriptor, error) {
	mediaType, err := d.MediaType()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("read manifest media type: %w", err)
	}
	digest, err := d.Digest()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("compute manifest digest: %w", err)
	}
	size, err := d.Size()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("compute size of manifest %s: %w", digest, err)
	}
	return v1.Descriptor{MediaType: mediaType, Digest: digest, Size: size}, nil
}

// childDescriptors returns untagged descriptors for every image manifest and image index
// reachable from idx, recursing into nested indexes. Other children (such as plain blobs)
// are stored by go-containerregistry but cannot be pulled as manifests, so they are not
// registered.
func childDescriptors(idx v1.ImageIndex) ([]v1.Descriptor, error) {
	manifest, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("parse index manifest: %w", err)
	}
	var descs []v1.Descriptor
	for _, child := range manifest.Manifests {
		if !child.MediaType.IsImage() && !child.MediaType.IsIndex() {
			continue
		}
		descs = append(descs, v1.Descriptor{MediaType: child.MediaType, Digest: child.Digest, Size: child.Size})
		if !child.MediaType.IsIndex() {
			continue
		}
		nested, err := idx.ImageIndex(child.Digest)
		if err != nil {
			return nil, fmt.Errorf("open nested index %s: %w", child.Digest, err)
		}
		nestedDescs, err := childDescriptors(nested)
		if err != nil {
			return nil, fmt.Errorf("nested index %s: %w", child.Digest, err)
		}
		descs = append(descs, nestedDescs...)
	}
	return descs, nil
}

// putTagged points tag at a copy of desc annotated with tag. The first descriptor carrying
// tag is replaced in place, so rewriting a tag leaves index.json unchanged; any further
// descriptor carrying tag is dropped. A new tag is appended.
func putTagged(index *v1.IndexManifest, tag string, desc v1.Descriptor) {
	desc.Annotations = map[string]string{ispec.AnnotationRefName: tag}
	manifests := make([]v1.Descriptor, 0, len(index.Manifests)+1)
	placed := false
	for _, existing := range index.Manifests {
		switch {
		case existing.Annotations[ispec.AnnotationRefName] != tag:
			manifests = append(manifests, existing)
		case !placed:
			manifests = append(manifests, desc)
			placed = true
		}
	}
	if !placed {
		manifests = append(manifests, desc)
	}
	index.Manifests = manifests
}

// addUntagged appends each of descs whose digest the index does not reference yet, tagged
// or untagged.
func addUntagged(index *v1.IndexManifest, descs []v1.Descriptor) {
	present := make(map[v1.Hash]bool, len(index.Manifests))
	for _, existing := range index.Manifests {
		present[existing.Digest] = true
	}
	for _, desc := range descs {
		if present[desc.Digest] {
			continue
		}
		index.Manifests = append(index.Manifests, desc)
		present[desc.Digest] = true
	}
}
