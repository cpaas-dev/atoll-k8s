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

package buildah

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

// mergeRootfsRegistry merges the registry root of the image file system rootfs into the
// registry root dst with ocilayout.MergeRoot, creating dst when it is missing.
//
// A registry root holds a single index.json per repository, so overlaying registry
// directories file by file (image layers, COPY, rootfs merges) keeps only the last
// index.json and drops tags; every build-time combination of registry directories goes
// through this function instead. A tag pointing to different digests fails with
// ocilayout.ErrTagConflict and leaves dst untouched. A rootfs without a registry directory
// holds no images and is skipped.
func mergeRootfsRegistry(dst, rootfs string) error {
	src := filepath.Join(rootfs, constants.RegistryDirName)
	exists, err := pathExists(src)
	if err != nil {
		return err
	}
	if !exists {
		logger.Debug("skip merging registry root %s into %s: it does not exist", src, dst)
		return nil
	}
	return ocilayout.MergeRoot(dst, src)
}

// pathExists reports whether name exists; errors other than non-existence are returned.
func pathExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", name, err)
}
