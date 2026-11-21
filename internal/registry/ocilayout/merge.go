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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// MergeRoot merges the registry root src into dst (dst is created if missing).
//
// Every repository of src is validated before anything is written: a tag present in both
// roots with different digests is an ErrTagConflict (all conflicts are reported together)
// and leaves dst untouched. Then, per repository, the blobs missing in dst are copied
// (verified against their sha256 name, written to a temporary file and renamed), oci-layout
// is created if missing and index.json is replaced atomically, so a registry serving dst
// never sees a descriptor whose blobs are missing. Tagged descriptors are keyed by tag
// (same tag and digest is skipped); untagged descriptors are keyed by digest and skipped
// when dst already references the digest, tagged or untagged. Merging the same source
// twice leaves dst unchanged.
func MergeRoot(dst, src string) error {
	if err := CheckRoot(src); err != nil {
		return fmt.Errorf("merge registry root %s into %s: source: %w", src, dst, err)
	}
	if err := checkDestinationRoot(dst); err != nil {
		return fmt.Errorf("merge registry root %s into %s: destination: %w", src, dst, err)
	}
	repositories, err := Repositories(src)
	if err != nil {
		return fmt.Errorf("merge registry root %s into %s: %w", src, dst, err)
	}
	dstDirs := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		dstDirs = append(dstDirs, RepositoryDir(dst, repository))
	}
	unlock, err := lockRepositories(dstDirs)
	if err != nil {
		return fmt.Errorf("merge registry root %s into %s: %w", src, dst, err)
	}
	defer unlock()

	plans, err := planMerges(dst, src, repositories)
	if err != nil {
		return fmt.Errorf("merge registry root %s into %s: %w", src, dst, err)
	}
	if err := os.MkdirAll(dst, dirMode); err != nil {
		return fmt.Errorf("merge registry root %s into %s: create destination: %w", src, dst, err)
	}
	for _, plan := range plans {
		if err := plan.apply(); err != nil {
			return fmt.Errorf("merge repository %s from %s into %s: %w", plan.repository, src, dst, err)
		}
	}
	return nil
}

// checkDestinationRoot accepts a missing dst and otherwise applies CheckRoot.
func checkDestinationRoot(dst string) error {
	exists, err := fileExists(dst)
	if err != nil || !exists {
		return err
	}
	return CheckRoot(dst)
}

// repositoryMerge is the validated merge of one source repository into the destination root.
type repositoryMerge struct {
	dstRoot    string
	srcRoot    string
	repository string
	// index is the merged destination index.json.
	index *v1.IndexManifest
	// writeIndex is false when the destination index.json exists and gains no descriptor.
	writeIndex bool
}

// planMerges validates the merge of every repository and joins all errors, so every tag
// conflict is reported at once.
func planMerges(dst, src string, repositories []string) ([]repositoryMerge, error) {
	plans := make([]repositoryMerge, 0, len(repositories))
	var errs []error
	for _, repository := range repositories {
		plan, err := planMerge(dst, src, repository)
		if err != nil {
			errs = append(errs, fmt.Errorf("repository %s: %w", repository, err))
			continue
		}
		plans = append(plans, plan)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return plans, nil
}

// planMerge computes the merged destination index.json of repository.
func planMerge(dst, src, repository string) (repositoryMerge, error) {
	if err := validateRepository(repository); err != nil {
		return repositoryMerge{}, err
	}
	srcIndex, err := readIndex(RepositoryDir(src, repository))
	if err != nil {
		return repositoryMerge{}, err
	}
	dstDir := RepositoryDir(dst, repository)
	dstIndexExists, err := fileExists(filepath.Join(dstDir, ispec.ImageIndexFile))
	if err != nil {
		return repositoryMerge{}, err
	}
	dstIndex := emptyIndex()
	if dstIndexExists {
		if dstIndex, err = readIndex(dstDir); err != nil {
			return repositoryMerge{}, err
		}
	}
	added, err := missingDescriptors(dstIndex.Manifests, srcIndex.Manifests)
	if err != nil {
		return repositoryMerge{}, err
	}
	dstIndex.Manifests = append(dstIndex.Manifests, added...)
	return repositoryMerge{
		dstRoot:    dst,
		srcRoot:    src,
		repository: repository,
		index:      dstIndex,
		writeIndex: !dstIndexExists || len(added) > 0,
	}, nil
}

// missingDescriptors returns the descriptors of src that dst lacks. Tagged descriptors are
// keyed by tag and fail with ErrTagConflict when the digests differ; untagged descriptors
// are keyed by digest.
func missingDescriptors(dst, src []v1.Descriptor) ([]v1.Descriptor, error) {
	tags := make(map[string]v1.Hash, len(dst))
	digests := make(map[v1.Hash]bool, len(dst))
	for _, desc := range dst {
		digests[desc.Digest] = true
		if tag, ok := desc.Annotations[ispec.AnnotationRefName]; ok {
			tags[tag] = desc.Digest
		}
	}
	var added []v1.Descriptor
	var conflicts []error
	for _, desc := range src {
		tag, tagged := desc.Annotations[ispec.AnnotationRefName]
		if !tagged {
			if !digests[desc.Digest] {
				added = append(added, desc)
				digests[desc.Digest] = true
			}
			continue
		}
		existing, ok := tags[tag]
		if !ok {
			added = append(added, desc)
			tags[tag] = desc.Digest
			digests[desc.Digest] = true
			continue
		}
		if existing != desc.Digest {
			conflicts = append(conflicts, fmt.Errorf(
				"%w: tag %q is %s in the destination and %s in the source", ErrTagConflict, tag, existing, desc.Digest,
			))
		}
	}
	return added, errors.Join(conflicts...)
}

// apply places the missing blobs, the oci-layout file and the merged index.json in the
// destination repository, in that order, and normalizes its file modes.
func (m repositoryMerge) apply() error {
	dstDir := RepositoryDir(m.dstRoot, m.repository)
	if err := copyMissingBlobs(dstDir, RepositoryDir(m.srcRoot, m.repository)); err != nil {
		return err
	}
	if err := ensureLayoutFiles(dstDir); err != nil {
		return err
	}
	if m.writeIndex {
		if err := writeIndex(dstDir, m.index); err != nil {
			return err
		}
	}
	return normalizeModes(m.dstRoot, m.repository)
}

// copyMissingBlobs copies every blob of the repository directory srcDir that dstDir lacks.
func copyMissingBlobs(dstDir, srcDir string) error {
	srcBlobs := filepath.Join(srcDir, ispec.ImageBlobsDir)
	exists, err := fileExists(srcBlobs)
	if err != nil || !exists {
		return err
	}
	return filepath.WalkDir(srcBlobs, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(srcDir, name)
		if err != nil {
			return fmt.Errorf("resolve %s relative to %s: %w", name, srcDir, err)
		}
		target := filepath.Join(dstDir, rel)
		present, err := fileExists(target)
		if err != nil || present {
			return err
		}
		return copyBlob(target, name)
	})
}

// copyBlob copies the blob file source (blobs/sha256/<hex>) to target through a temporary
// file and fails unless its content matches the digest in its path.
func copyBlob(target, source string) error {
	want, err := v1.NewHash(filepath.Base(filepath.Dir(source)) + ":" + filepath.Base(source))
	if err != nil {
		return fmt.Errorf("blob %s is not named after a digest: %w", source, err)
	}
	if want.Algorithm != "sha256" {
		return fmt.Errorf("blob %s uses digest algorithm %s, only sha256 is supported", source, want.Algorithm)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return fmt.Errorf("create blob directory for %s: %w", target, err)
	}
	in, err := os.Open(filepath.Clean(source))
	if err != nil {
		return fmt.Errorf("open blob %s: %w", source, err)
	}
	err = replaceFile(target, func(w io.Writer) error {
		got, _, err := v1.SHA256(io.TeeReader(in, w))
		if err != nil {
			return fmt.Errorf("copy blob %s: %w", source, err)
		}
		if got != want {
			return fmt.Errorf("blob %s has digest %s, want %s", source, got, want)
		}
		return nil
	})
	return errors.Join(err, closeFile(in))
}
