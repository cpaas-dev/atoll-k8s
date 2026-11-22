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

// Package clusterimagetest builds cluster images for tests: layer tarballs, OCI image
// manifests and indexes pushed to any oras target, and images tagged into an OCI image
// layout such as the local store of package clusterimage.
package clusterimagetest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/oci"
)

// Mtime is the modification time of every layer entry.
var Mtime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// Entry is one tar entry of a layer.
type Entry struct {
	Name, Link, Body string
	Type             byte
	Mode             int64
	UID, GID         int
}

// Dir returns a directory entry.
func Dir(name string, mode int64) Entry { return Entry{Name: name, Type: tar.TypeDir, Mode: mode} }

// File returns a regular file entry.
func File(name, body string, mode int64) Entry {
	return Entry{Name: name, Body: body, Type: tar.TypeReg, Mode: mode}
}

// Symlink returns a symbolic link entry.
func Symlink(name, target string) Entry {
	return Entry{Name: name, Link: target, Type: tar.TypeSymlink, Mode: 0o777}
}

// LayerTar returns an uncompressed layer tarball holding entries.
func LayerTar(t *testing.T, entries ...Entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.Name, Linkname: e.Link, Typeflag: e.Type, Mode: e.Mode,
			Uid: e.UID, Gid: e.GID, ModTime: Mtime, Format: tar.FormatPAX}
		if e.Type == tar.TypeReg {
			hdr.Size = int64(len(e.Body))
		}
		if err := writer.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %s: %v", e.Name, err)
		}
		if _, err := writer.Write([]byte(e.Body)); err != nil {
			t.Fatalf("write tar body %s: %v", e.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.Bytes()
}

// Image is an image to push: its configuration and uncompressed layer tarballs.
type Image struct {
	Platform ocispec.Platform
	Config   ocispec.ImageConfig
	Layers   [][]byte
	// DiffIDs are the diff IDs the configuration declares, the real ones from NewImage.
	DiffIDs []digest.Digest
}

// NewImage returns an image of platform with layers and their real diff IDs.
func NewImage(platform ocispec.Platform, layers ...[]byte) Image {
	img := Image{Platform: platform, Layers: layers}
	for _, layer := range layers {
		img.DiffIDs = append(img.DiffIDs, digest.FromBytes(layer))
	}
	return img
}

// Push pushes img into target as a gzip-layered OCI image and returns its manifest.
func Push(t *testing.T, target content.Pusher, img Image) ocispec.Descriptor {
	t.Helper()
	manifest := ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest}
	for _, layer := range img.Layers {
		var gz bytes.Buffer
		writer := gzip.NewWriter(&gz)
		if _, err := writer.Write(layer); err != nil {
			t.Fatalf("gzip layer: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close gzip: %v", err)
		}
		manifest.Layers = append(manifest.Layers, pushBytes(t, target, ocispec.MediaTypeImageLayerGzip, gz.Bytes()))
	}
	cfg := ocispec.Image{Platform: img.Platform, Config: img.Config, RootFS: ocispec.RootFS{Type: "layers", DiffIDs: img.DiffIDs}}
	manifest.Config = pushJSON(t, target, ocispec.MediaTypeImageConfig, cfg)
	desc := pushJSON(t, target, ocispec.MediaTypeImageManifest, manifest)
	desc.Platform = &img.Platform
	return desc
}

// PushIndex pushes an image index of manifests into target.
func PushIndex(t *testing.T, target content.Pusher, manifests ...ocispec.Descriptor) ocispec.Descriptor {
	t.Helper()
	index := ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, Manifests: manifests}
	return pushJSON(t, target, ocispec.MediaTypeImageIndex, index)
}

// Tag pushes img into the OCI image layout at layoutDir and tags it as name. The layout
// must not be open in a store, which would not see the new tag.
func Tag(t *testing.T, layoutDir, name string, img Image) ocispec.Descriptor {
	t.Helper()
	layout, err := oci.New(layoutDir)
	if err != nil {
		t.Fatalf("open OCI layout %s: %v", layoutDir, err)
	}
	desc := Push(t, layout, img)
	if err := layout.Tag(context.Background(), desc, name); err != nil {
		t.Fatalf("tag %s: %v", name, err)
	}
	return desc
}

func pushJSON(t *testing.T, target content.Pusher, mediaType string, v any) ocispec.Descriptor {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %s: %v", mediaType, err)
	}
	return pushBytes(t, target, mediaType, data)
}

func pushBytes(t *testing.T, target content.Pusher, mediaType string, data []byte) ocispec.Descriptor {
	t.Helper()
	desc, err := oras.PushBytes(context.Background(), target, mediaType, data)
	if err != nil {
		t.Fatalf("push %s: %v", mediaType, err)
	}
	return desc
}
