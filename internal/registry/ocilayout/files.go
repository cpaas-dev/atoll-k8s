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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// fileMode makes files readable by a registry running as another user.
	fileMode fs.FileMode = 0o644
	// dirMode makes directories traversable by a registry running as another user.
	dirMode fs.FileMode = 0o755
)

var (
	// repositoryLocksMu guards repositoryLocks.
	repositoryLocksMu sync.Mutex
	// repositoryLocks holds one mutex per absolute repository directory. go-containerregistry
	// writes blobs and index.json without locking, so every writer of a repository holds it.
	repositoryLocks = map[string]*sync.Mutex{}
)

// lockRepositories locks the mutexes of dirs in sorted order, so concurrent callers
// cannot deadlock, and returns the function that unlocks them.
func lockRepositories(dirs []string) (func(), error) {
	keys := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve absolute path of repository directory %s: %w", dir, err)
		}
		keys = append(keys, abs)
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	locked := make([]*sync.Mutex, 0, len(keys))
	for _, key := range keys {
		mu := repositoryLock(key)
		mu.Lock()
		locked = append(locked, mu)
	}
	return func() {
		for _, mu := range slices.Backward(locked) {
			mu.Unlock()
		}
	}, nil
}

// repositoryLock returns the mutex of the absolute repository directory key.
func repositoryLock(key string) *sync.Mutex {
	repositoryLocksMu.Lock()
	defer repositoryLocksMu.Unlock()
	mu, ok := repositoryLocks[key]
	if !ok {
		mu = &sync.Mutex{}
		repositoryLocks[key] = mu
	}
	return mu
}

// fileExists reports whether name exists; errors other than non-existence are returned.
func fileExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", name, err)
}

// emptyIndex returns an OCI image index without descriptors.
func emptyIndex() *v1.IndexManifest {
	return &v1.IndexManifest{SchemaVersion: 2, MediaType: types.OCIImageIndex, Manifests: []v1.Descriptor{}}
}

// readIndex parses the index.json of the repository directory dir.
func readIndex(dir string) (*v1.IndexManifest, error) {
	name := filepath.Join(dir, ispec.ImageIndexFile)
	file, err := os.Open(filepath.Clean(name))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	index, err := v1.ParseIndexManifest(file)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("parse %s: %w", name, err), closeFile(file))
	}
	if err := closeFile(file); err != nil {
		return nil, err
	}
	if index.Manifests == nil {
		index.Manifests = []v1.Descriptor{}
	}
	return index, nil
}

// readIndexOrEmpty parses the index.json of dir, or returns an empty index when dir has none yet.
func readIndexOrEmpty(dir string) (*v1.IndexManifest, error) {
	exists, err := fileExists(filepath.Join(dir, ispec.ImageIndexFile))
	if err != nil {
		return nil, err
	}
	if !exists {
		return emptyIndex(), nil
	}
	return readIndex(dir)
}

// writeIndex atomically replaces the index.json of the repository directory dir.
func writeIndex(dir string, index *v1.IndexManifest) error {
	raw, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshal index of %s: %w", dir, err)
	}
	return writeFileAtomic(filepath.Join(dir, ispec.ImageIndexFile), raw)
}

// ensureLayoutFiles creates the blobs directory and the oci-layout file of the repository
// directory dir, keeping an existing oci-layout file.
func ensureLayoutFiles(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, ispec.ImageBlobsDir), dirMode); err != nil {
		return fmt.Errorf("create repository directory %s: %w", dir, err)
	}
	name := filepath.Join(dir, ispec.ImageLayoutFile)
	exists, err := fileExists(name)
	if err != nil || exists {
		return err
	}
	raw, err := json.Marshal(ispec.ImageLayout{Version: ispec.ImageLayoutVersion})
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	return writeFileAtomic(name, raw)
}

// writeFileAtomic replaces name with data through replaceFile.
func writeFileAtomic(name string, data []byte) error {
	return replaceFile(name, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// replaceFile writes the content produced by fill to a temporary file next to name and
// renames it over name, so readers see either the old or the new content.
func replaceFile(name string, fill func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", name, err)
	}
	if err := fillTemporaryFile(tmp, fill); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", name, err), removeTemporaryFile(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return errors.Join(fmt.Errorf("rename %s to %s: %w", tmp.Name(), name, err), removeTemporaryFile(tmp.Name()))
	}
	return nil
}

// fillTemporaryFile writes the content produced by fill to file, sets fileMode and closes it.
func fillTemporaryFile(file *os.File, fill func(io.Writer) error) error {
	if err := fill(file); err != nil {
		return errors.Join(err, closeFile(file))
	}
	if err := file.Chmod(fileMode); err != nil {
		return errors.Join(fmt.Errorf("chmod %s: %w", file.Name(), err), closeFile(file))
	}
	return closeFile(file)
}

// closeFile closes file and names it in the error.
func closeFile(file *os.File) error {
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", file.Name(), err)
	}
	return nil
}

// removeTemporaryFile removes a temporary file left behind by a failed write.
func removeTemporaryFile(name string) error {
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove temporary file %s: %w", name, err)
	}
	return nil
}

// normalizeModes makes a repository readable by a registry running as another user.
// go-containerregistry creates layer blobs with mode 0600 and other files and directories
// with 0777&^umask, so the repository directory, its parent directories below root and the
// directories under blobs get dirMode, while oci-layout, index.json and every blob get
// fileMode. Nested repositories are left to their own writers.
func normalizeModes(root, repository string) error {
	for dir := repository; dir != "."; dir = path.Dir(dir) {
		if err := setMode(RepositoryDir(root, dir), dirMode); err != nil {
			return err
		}
	}
	dir := RepositoryDir(root, repository)
	for _, name := range []string{ispec.ImageLayoutFile, ispec.ImageIndexFile} {
		if err := setMode(filepath.Join(dir, name), fileMode); err != nil {
			return err
		}
	}
	blobs := filepath.Join(dir, ispec.ImageBlobsDir)
	err := filepath.WalkDir(blobs, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return setMode(name, dirMode)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", name)
		}
		return setMode(name, fileMode)
	})
	if err != nil {
		return fmt.Errorf("normalize file modes under %s: %w", blobs, err)
	}
	return nil
}

// setMode changes the permission bits of name to mode unless they already match.
func setMode(name string, mode fs.FileMode) error {
	info, err := os.Stat(name)
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	if info.Mode().Perm() == mode {
		return nil
	}
	if err := os.Chmod(name, mode); err != nil {
		return fmt.Errorf("chmod %s to %o: %w", name, mode, err)
	}
	return nil
}
