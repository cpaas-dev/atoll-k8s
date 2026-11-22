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

package clusterimage

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci/oci/layer"

	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

const (
	dockerManifestMediaType  = "application/vnd.docker.distribution.manifest.v2+json"
	dockerLayerGzipMediaType = "application/vnd.docker.image.rootfs.diff.tar.gzip"
)

// errUnpackStopped interrupts the layer filter once the unpack of its output has ended.
var errUnpackStopped = errors.New("layer unpack stopped")

// Export writes the rootfs of img, the content of /cpaas in the image, to target, which
// must not exist. Layers are applied in order with OCI whiteout semantics; the uncompressed
// digest of every layer must match the DiffID in the image configuration.
//
// Export fails, leaving no target behind, when a layer has
//   - an entry outside /cpaas, other than the image root directory, or a whiteout of
//     anything at the image root except /cpaas itself;
//   - a symbolic link with an absolute target, or a relative target that leaves /cpaas;
//   - a hard link to a path outside /cpaas;
//   - a device, FIFO or other special file.
//
// Owners, permissions, extended attributes and modification times are restored, so the
// caller must run as root.
func (s *Store) Export(ctx context.Context, img Image, target string) error {
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("export target %s already exists", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check export target %s: %w", target, err)
	}
	manifest, err := s.manifest(ctx, img)
	if err != nil {
		return err
	}
	cfg, err := s.config(ctx, img, manifest)
	if err != nil {
		return err
	}
	if len(cfg.RootFS.DiffIDs) != len(manifest.Layers) {
		return fmt.Errorf("image %s lists %d layers but %d diff IDs", img.Name, len(manifest.Layers), len(cfg.RootFS.DiffIDs))
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create parent of export target %s: %w", target, err)
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(target)+".export-")
	if err != nil {
		return fmt.Errorf("create staging dir for %s: %w", target, err)
	}
	// The /cpaas entry of the image, when present, overrides this mode.
	if err := os.Chmod(staging, 0o755); err != nil {
		return fmt.Errorf("set mode of staging dir %s: %w", staging, err)
	}
	if err := s.applyLayers(ctx, img, manifest.Layers, cfg.RootFS.DiffIDs, staging); err != nil {
		if rmErr := os.RemoveAll(staging); rmErr != nil {
			logger.Warn("remove staging dir %s: %v", staging, rmErr)
		}
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		if rmErr := os.RemoveAll(staging); rmErr != nil {
			logger.Warn("remove staging dir %s: %v", staging, rmErr)
		}
		return fmt.Errorf("move exported rootfs of %s to %s: %w", img.Name, target, err)
	}
	logger.Info("exported rootfs of image %s to %s", img.Name, target)
	return nil
}

func (s *Store) applyLayers(ctx context.Context, img Image, layers []ocispec.Descriptor, diffIDs []digest.Digest, root string) error {
	for i, desc := range layers {
		if err := s.applyLayer(ctx, desc, diffIDs[i], root); err != nil {
			return fmt.Errorf("image %s layer %d (%s): %w", img.Name, i, desc.Digest, err)
		}
	}
	return nil
}

func (s *Store) applyLayer(ctx context.Context, desc ocispec.Descriptor, diffID digest.Digest, root string) error {
	blob, err := s.layout.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("read layer blob: %w", err)
	}
	defer blob.Close()
	uncompressed, err := decompress(desc.MediaType, blob)
	if err != nil {
		return err
	}
	defer uncompressed.Close()

	if err := diffID.Validate(); err != nil {
		return fmt.Errorf("invalid diff ID %q: %w", diffID, err)
	}
	verifier := diffID.Algorithm().Digester()
	source := io.TeeReader(uncompressed, verifier.Hash())

	filtered, writer := io.Pipe()
	filterDone := make(chan error, 1)
	go func() {
		err := filterCpaas(source, writer)
		if err == nil {
			// Hash the end-of-archive padding the tar reader leaves unread.
			_, err = io.Copy(io.Discard, source)
		}
		writer.CloseWithError(err)
		filterDone <- err
	}()
	unpackErr := layer.UnpackLayer(root, filtered, &layer.UnpackOptions{OnDiskFormat: layer.DirRootfs{}})
	if unpackErr == nil {
		// Read what the filter still writes after the end-of-archive marker.
		_, unpackErr = io.Copy(io.Discard, filtered)
	}
	filtered.CloseWithError(errUnpackStopped)
	// A filter error explains a failed unpack better than the read error it caused.
	if filterErr := <-filterDone; filterErr != nil && !errors.Is(filterErr, errUnpackStopped) {
		return filterErr
	}
	if unpackErr != nil {
		return fmt.Errorf("apply layer: %w", unpackErr)
	}
	if got := verifier.Digest(); got != diffID {
		return fmt.Errorf("uncompressed digest %s does not match diff ID %s", got, diffID)
	}
	return nil
}

// decompress returns the uncompressed tar stream of a layer blob of mediaType.
func decompress(mediaType string, blob io.Reader) (io.ReadCloser, error) {
	switch mediaType {
	case ocispec.MediaTypeImageLayer:
		return io.NopCloser(blob), nil
	case ocispec.MediaTypeImageLayerGzip, dockerLayerGzipMediaType:
		reader, err := gzip.NewReader(blob)
		if err != nil {
			return nil, fmt.Errorf("open gzip layer: %w", err)
		}
		return reader, nil
	case ocispec.MediaTypeImageLayerZstd:
		decoder, err := zstd.NewReader(blob)
		if err != nil {
			return nil, fmt.Errorf("open zstd layer: %w", err)
		}
		return decoder.IOReadCloser(), nil
	default:
		return nil, fmt.Errorf("unsupported layer media type %s", mediaType)
	}
}
