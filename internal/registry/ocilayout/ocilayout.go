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

// Package ocilayout reads and writes registry roots: directories holding one OCI image
// layout per repository, which is the native storage format of the zot registry.
//
//	<root>/<repository>/oci-layout
//	<root>/<repository>/index.json      one descriptor per tag (org.opencontainers.image.ref.name)
//	                                    plus untagged descriptors for digest-only manifests
//	<root>/<repository>/blobs/sha256/*  manifests, configs and layers, byte for byte
//
// Repositories may be nested (library/nginx). zot resolves tags and digests only through
// the top-level descriptors of index.json, so the child manifests of an image index are
// registered there as untagged descriptors; otherwise pulling a platform manifest by
// digest fails. Manifests are never converted, so Docker schema2 manifests and Docker
// manifest lists keep their digests.
//
// All writers serialize on an in-process mutex per repository directory, replace
// index.json atomically after the blobs it references are in place, and leave files
// with mode 0644 and directories with mode 0755 so a registry running as another user
// can read them.
package ocilayout

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
)

var (
	// ErrTagConflict reports that the same tag points to different digests in the
	// destination and the source of a merge.
	ErrTagConflict = errors.New("tag points to different digests")
	// ErrLegacyLayout reports a registry root holding a distribution/distribution
	// storage directory (docker/registry/v2) instead of OCI image layouts.
	ErrLegacyLayout = errors.New("registry root uses the legacy distribution storage layout")
)

// legacyStorageDir is the storage directory of distribution/distribution, relative to its root.
var legacyStorageDir = filepath.Join("docker", "registry", "v2")

var (
	// repositoryPattern is the repository name grammar of the OCI distribution spec. It
	// rejects empty components, "." and "..", absolute paths and backslashes.
	repositoryPattern = regexp.MustCompile(`^[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*$`)
	// tagPattern is the tag grammar of the OCI distribution spec.
	tagPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
)

// RepositoryDir returns <root>/<repository> using filepath.FromSlash.
func RepositoryDir(root, repository string) string {
	return filepath.Join(root, filepath.FromSlash(repository))
}

// Repositories lists repository names (slash separated, sorted) under root: every
// directory containing an oci-layout file. blobs directories and hidden directories
// (such as zot's .uploads) are not descended into.
func Repositories(root string) ([]string, error) {
	var repositories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && skipWhenListing(entry.Name()) {
			return filepath.SkipDir
		}
		isRepository, err := fileExists(filepath.Join(path, ispec.ImageLayoutFile))
		if err != nil || !isRepository {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve %s relative to %s: %w", path, root, err)
		}
		repositories = append(repositories, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list repositories in registry root %s: %w", root, err)
	}
	slices.Sort(repositories)
	return repositories, nil
}

// skipWhenListing reports whether a directory cannot contain repositories.
func skipWhenListing(name string) bool {
	return name == ispec.ImageBlobsDir || strings.HasPrefix(name, ".")
}

// Descriptors returns the top-level descriptors of the repository's index.json.
func Descriptors(root, repository string) ([]v1.Descriptor, error) {
	if err := validateRepository(repository); err != nil {
		return nil, err
	}
	index, err := readIndex(RepositoryDir(root, repository))
	if err != nil {
		return nil, fmt.Errorf("read descriptors of repository %s in %s: %w", repository, root, err)
	}
	return index.Manifests, nil
}

// CheckRoot returns ErrLegacyLayout (wrapped with the path) if root holds a distribution
// storage directory, and an error if root does not exist. An empty root is valid.
func CheckRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("check registry root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("check registry root: %s is not a directory", root)
	}
	legacy := filepath.Join(root, legacyStorageDir)
	isLegacy, err := fileExists(legacy)
	if err != nil {
		return fmt.Errorf("check registry root %s: %w", root, err)
	}
	if isLegacy {
		return fmt.Errorf("%w: %s exists", ErrLegacyLayout, legacy)
	}
	return nil
}

// validateRepository rejects names that are not valid OCI distribution repository names,
// which also keeps the repository directory inside the registry root.
func validateRepository(repository string) error {
	if repository == "" {
		return errors.New("repository name must not be empty")
	}
	if !repositoryPattern.MatchString(repository) {
		return fmt.Errorf(
			"invalid repository name %q: must be a relative, slash separated path of lowercase components matching %s",
			repository, repositoryPattern,
		)
	}
	return nil
}

// validateReference rejects invalid repository names and tags.
func validateReference(repository, tag string) error {
	if err := validateRepository(repository); err != nil {
		return err
	}
	if tag == "" {
		return fmt.Errorf("tag for repository %s must not be empty", repository)
	}
	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("invalid tag %q for repository %s: must match %s", tag, repository, tagPattern)
	}
	return nil
}
