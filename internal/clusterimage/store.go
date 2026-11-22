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

// Package clusterimage keeps the cluster images of the atoll init node and exports their
// rootfs.
//
// Images live in one OCI image layout (the local store), tagged with their fully
// qualified reference. They are pulled with oras from registries configured by the
// containers configuration files, or imported from OCI archive tarballs. Only the image
// manifest of the requested platform is stored. Pull is the only operation that contacts a
// registry; everything else works on the local store.
//
// A cluster image keeps its rootfs under /cpaas. Export applies the image layers to a new
// directory holding the content of /cpaas; see Export for the rules an image must follow.
//
// The local store is unrelated to the zot registry that master nodes run for image pulls
// inside the cluster.
package clusterimage

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/oci"
	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/oras-project/oras-go/v3/registry/remote/config"
	"golang.org/x/sys/unix"

	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

const (
	// lockFileName is the file locked while the local store is written.
	lockFileName = ".atoll.lock"
	// containerdImageNameAnnotation holds the full image name in OCI archives written by
	// Docker and BuildKit, where org.opencontainers.image.ref.name holds only the tag.
	containerdImageNameAnnotation = "io.containerd.image.name"
)

// Image is an image manifest in the local store.
type Image struct {
	// Name is the fully qualified reference the image is tagged with in the store.
	Name string
	// Manifest describes the image manifest of the stored platform.
	Manifest ocispec.Descriptor
}

// Store is the local image store of the atoll init node.
type Store struct {
	root   string
	layout *oci.Store
	remote *remoteResolver
}

// Open opens the local store at root, creating the OCI image layout when it is missing.
// configs supplies registry access for Pull and the short-name rules of Pull and Image.
func Open(ctx context.Context, root string, configs *config.Configs) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create image store %s: %w", root, err)
	}
	layout, err := oci.NewWithContext(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("open image store %s: %w", root, err)
	}
	resolver, err := newRemoteResolver(configs)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, layout: layout, remote: resolver}, nil
}

// ErrImageNotFound is returned by Image when the local store has no image of the
// requested platform under any qualified name of the reference.
var ErrImageNotFound = errors.New("image not found in the local store")

// Transport selects how Pull connects to registries, on top of registries.conf.
type Transport struct {
	// PlainHTTP connects over HTTP instead of HTTPS.
	PlainHTTP bool
	// SkipTLSVerify accepts any TLS certificate. Registries marked insecure in
	// registries.conf skip verification as well.
	SkipTLSVerify bool
}

// Pull pulls the image ref for platform into the local store, replacing the image stored
// under the same name. Short names resolve through registries.conf aliases and
// unqualified-search-registries; every candidate registry is tried in order and the
// errors of all attempts are returned together.
func (s *Store) Pull(ctx context.Context, ref string, platform ocispec.Platform, transport Transport) (Image, error) {
	names, err := s.remote.qualifiedNames(ref)
	if err != nil {
		return Image{}, err
	}
	unlock, err := s.lock()
	if err != nil {
		return Image{}, err
	}
	defer unlock()

	var pullErrs []error
	for _, name := range names {
		img, err := s.pull(ctx, name, platform, transport)
		if err == nil {
			return img, nil
		}
		pullErrs = append(pullErrs, fmt.Errorf("pull %s: %w", name, err))
	}
	return Image{}, fmt.Errorf("pull image %s for %s: %w", ref, platformString(platform), errors.Join(pullErrs...))
}

// Image returns the stored image ref for platform without contacting any registry. Short
// names resolve to the same candidates as in Pull; the first stored candidate whose
// platform matches is returned, and ErrImageNotFound when there is none.
func (s *Store) Image(ctx context.Context, ref string, platform ocispec.Platform) (Image, error) {
	names, err := s.remote.qualifiedNames(ref)
	if err != nil {
		return Image{}, err
	}
	for _, name := range names {
		img, found, err := s.localImage(ctx, name, platform)
		if err != nil {
			return Image{}, err
		}
		if found {
			return img, nil
		}
	}
	return Image{}, fmt.Errorf("%w: %s for %s", ErrImageNotFound, ref, platformString(platform))
}

// Import copies every image of the OCI archive tarball at archivePath into the local
// store and returns them in archive order. Each image is stored under its full name,
// taken from the io.containerd.image.name annotation, or from
// org.opencontainers.image.ref.name when that holds a fully qualified reference.
func (s *Store) Import(ctx context.Context, archivePath string, platform ocispec.Platform) ([]Image, error) {
	index, err := readArchiveIndex(archivePath)
	if err != nil {
		return nil, err
	}
	source, err := oci.NewFromTar(ctx, archivePath)
	if err != nil {
		return nil, fmt.Errorf("open OCI archive %s: %w", archivePath, err)
	}
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	images := make([]Image, 0, len(index.Manifests))
	for _, desc := range index.Manifests {
		name, err := archiveImageName(desc)
		if err != nil {
			return nil, fmt.Errorf("OCI archive %s: %w", archivePath, err)
		}
		manifest, err := oras.Copy(ctx, source, desc.Digest.String(), s.layout, name, platformCopyOptions(platform))
		if err != nil {
			return nil, fmt.Errorf("import %s for %s from OCI archive %s: %w", name, platformString(platform), archivePath, err)
		}
		logger.Info("imported image %s (%s) from %s", name, manifest.Digest, archivePath)
		images = append(images, Image{Name: name, Manifest: manifest})
	}
	return images, nil
}

// Config returns the image configuration of img.
func (s *Store) Config(ctx context.Context, img Image) (ocispec.Image, error) {
	manifest, err := s.manifest(ctx, img)
	if err != nil {
		return ocispec.Image{}, err
	}
	return s.config(ctx, img, manifest)
}

func (s *Store) pull(ctx context.Context, name string, platform ocispec.Platform, transport Transport) (Image, error) {
	repo, reference, err := s.remote.repository(name, transport)
	if err != nil {
		return Image{}, err
	}
	logger.Info("pulling image %s for %s", name, platformString(platform))
	manifest, err := oras.Copy(ctx, repo, reference, s.layout, name, platformCopyOptions(platform))
	if err != nil {
		return Image{}, err
	}
	logger.Info("pulled image %s (%s)", name, manifest.Digest)
	return Image{Name: name, Manifest: manifest}, nil
}

// localImage returns the image stored under name when its platform matches.
func (s *Store) localImage(ctx context.Context, name string, platform ocispec.Platform) (Image, bool, error) {
	desc, err := s.layout.Resolve(ctx, name)
	if errors.Is(err, errdef.ErrNotFound) {
		return Image{}, false, nil
	}
	if err != nil {
		return Image{}, false, fmt.Errorf("resolve %s in image store %s: %w", name, s.root, err)
	}
	img := Image{Name: name, Manifest: desc}
	cfg, err := s.Config(ctx, img)
	if err != nil {
		return Image{}, false, err
	}
	if !platformMatches(cfg.Platform, platform) {
		logger.Info("stored image %s is %s, not %s", name, platformString(cfg.Platform), platformString(platform))
		return Image{}, false, nil
	}
	return img, true, nil
}

func (s *Store) manifest(ctx context.Context, img Image) (ocispec.Manifest, error) {
	if img.Manifest.MediaType != ocispec.MediaTypeImageManifest && img.Manifest.MediaType != dockerManifestMediaType {
		return ocispec.Manifest{}, fmt.Errorf("image %s: %s is a %s, not an image manifest", img.Name, img.Manifest.Digest, img.Manifest.MediaType)
	}
	data, err := content.FetchAll(ctx, s.layout, img.Manifest)
	if err != nil {
		return ocispec.Manifest{}, fmt.Errorf("read manifest %s of image %s: %w", img.Manifest.Digest, img.Name, err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("decode manifest %s of image %s: %w", img.Manifest.Digest, img.Name, err)
	}
	return manifest, nil
}

func (s *Store) config(ctx context.Context, img Image, manifest ocispec.Manifest) (ocispec.Image, error) {
	data, err := content.FetchAll(ctx, s.layout, manifest.Config)
	if err != nil {
		return ocispec.Image{}, fmt.Errorf("read config %s of image %s: %w", manifest.Config.Digest, img.Name, err)
	}
	var cfg ocispec.Image
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ocispec.Image{}, fmt.Errorf("decode config %s of image %s: %w", manifest.Config.Digest, img.Name, err)
	}
	return cfg, nil
}

// lock takes the exclusive lock of the local store, shared with other atoll processes.
func (s *Store) lock() (func(), error) {
	path := filepath.Join(s.root, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open image store lock %s: %w", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock image store %s: %w", path, err)
	}
	return func() {
		if err := f.Close(); err != nil {
			logger.Warn("close image store lock %s: %v", path, err)
		}
	}, nil
}

func platformCopyOptions(platform ocispec.Platform) oras.CopyOptions {
	opts := oras.CopyOptions{}
	opts.WithTargetPlatform(&platform)
	return opts
}

// HostPlatform returns the platform of this host, the platform of the cluster images
// atoll uses.
func HostPlatform() ocispec.Platform {
	return ocispec.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}
}

// platformMatches reports whether an image of platform got serves want, with the rule
// oras applies when selecting a manifest: OS and architecture must be equal, variant and
// OS version only when want sets them.
func platformMatches(got, want ocispec.Platform) bool {
	if got.OS != want.OS || got.Architecture != want.Architecture {
		return false
	}
	if want.Variant != "" && got.Variant != want.Variant {
		return false
	}
	return want.OSVersion == "" || got.OSVersion == want.OSVersion
}

func platformString(p ocispec.Platform) string {
	if p.Variant == "" {
		return p.OS + "/" + p.Architecture
	}
	return p.OS + "/" + p.Architecture + "/" + p.Variant
}

// readArchiveIndex reads index.json of the OCI archive tarball at path.
func readArchiveIndex(path string) (ocispec.Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return ocispec.Index{}, fmt.Errorf("open OCI archive %s: %w", path, err)
	}
	defer f.Close()
	reader := tar.NewReader(f)
	for {
		hdr, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return ocispec.Index{}, fmt.Errorf("%s has no %s: only OCI image layout archives are supported, "+
				"such as the output of docker buildx build --output type=oci", path, ocispec.ImageIndexFile)
		}
		if err != nil {
			return ocispec.Index{}, fmt.Errorf("read OCI archive %s: %w", path, err)
		}
		if filepath.Clean(hdr.Name) != ocispec.ImageIndexFile {
			continue
		}
		var index ocispec.Index
		if err := json.NewDecoder(reader).Decode(&index); err != nil {
			return ocispec.Index{}, fmt.Errorf("decode %s of OCI archive %s: %w", ocispec.ImageIndexFile, path, err)
		}
		if len(index.Manifests) == 0 {
			return ocispec.Index{}, fmt.Errorf("OCI archive %s lists no images", path)
		}
		return index, nil
	}
}

// archiveImageName returns the fully qualified name of an image listed in an archive index.
func archiveImageName(desc ocispec.Descriptor) (string, error) {
	if name := desc.Annotations[containerdImageNameAnnotation]; name != "" {
		return canonicalName(name)
	}
	if name := desc.Annotations[ocispec.AnnotationRefName]; isQualified(name) {
		return canonicalName(name)
	}
	return "", fmt.Errorf("image %s has neither %s nor a fully qualified %s annotation",
		desc.Digest, containerdImageNameAnnotation, ocispec.AnnotationRefName)
}
