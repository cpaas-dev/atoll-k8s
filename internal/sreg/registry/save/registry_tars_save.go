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
	"fmt"
	"strings"

	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/transports/alltransports"
)

func (s *tarSaver) SaveImages(tars []string, dir string, selection Selection) ([]string, error) {
	sources := make([]source, 0, len(tars))
	for _, tar := range tars {
		// Tar lists read by buildimage.TarList keep empty lines.
		if strings.TrimSpace(tar) == "" {
			continue
		}
		src, err := parseTar(tar)
		if err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	return saveSources(s.ctx, s.maxPullProcs, dir, sources, selection)
}

// parseTar returns the source of tar, given as transport:path@repository[:tag].
func parseTar(tar string) (source, error) {
	archive, target, ok := strings.Cut(tar, "@")
	if !ok {
		return nil, fmt.Errorf("tar image %s must have the form transport:path@repository[:tag]", tar)
	}
	ref, err := alltransports.ParseImageName(archive)
	if err != nil {
		return nil, fmt.Errorf("invalid source name %s of tar image %s: %w", archive, tar, err)
	}
	repository, tag, err := tarDestination(target)
	if err != nil {
		return nil, fmt.Errorf("invalid destination of tar image %s: %w", tar, err)
	}
	return localSource{image: tar, ref: ref, repository: repository, tag: tag}, nil
}

// tarDestination returns the repository and tag of target, given as repository[:tag]. The
// repository is kept whole and a missing tag means latest, as when the image was pushed to
// <registry>/<target>.
func tarDestination(target string) (repository, tag string, err error) {
	ref, err := reference.Parse(target)
	if err != nil {
		return "", "", fmt.Errorf("parse %q as repository[:tag]: %w", target, err)
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return "", "", fmt.Errorf("%q has no repository", target)
	}
	tagged, ok := reference.TagNameOnly(named).(reference.NamedTagged)
	if !ok {
		return "", "", fmt.Errorf("%q has no tag", target)
	}
	return named.Name(), tagged.Tag(), nil
}
