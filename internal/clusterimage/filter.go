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
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	// RootfsDir is the image directory whose content is the cluster rootfs.
	RootfsDir = "cpaas"

	whiteoutPrefix = ".wh."
	opaqueWhiteout = ".wh..wh..opq"
)

// filterCpaas copies the layer tar stream src to dst, keeping only the entries under
// /cpaas with that prefix removed. See Export for the entries it rejects.
func filterCpaas(src io.Reader, dst io.Writer) error {
	reader := tar.NewReader(src)
	writer := tar.NewWriter(dst)
	for {
		hdr, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read layer tar: %w", err)
		}
		out, keep, err := rewriteEntry(hdr)
		if err != nil {
			return err
		}
		if !keep {
			continue
		}
		if err := writer.WriteHeader(out); err != nil {
			return fmt.Errorf("write entry %s: %w", out.Name, err)
		}
		if out.Typeflag == tar.TypeReg {
			if _, err := io.Copy(writer, reader); err != nil {
				return fmt.Errorf("copy entry %s: %w", hdr.Name, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish filtered layer tar: %w", err)
	}
	return nil
}

// rewriteEntry maps an image entry to its rootfs entry. keep is false only for the image
// root directory, which has no counterpart in the rootfs.
func rewriteEntry(hdr *tar.Header) (*tar.Header, bool, error) {
	name := cleanEntryName(hdr.Name)
	out := *hdr
	// Xattrs duplicates the SCHILY.xattr entries of PAXRecords and is deprecated.
	out.Xattrs = nil                  //nolint:staticcheck // cleared so the writer encodes PAXRecords only
	if hdr.Typeflag == tar.TypeRegA { //nolint:staticcheck // legacy type of regular files
		out.Typeflag = tar.TypeReg
	}

	switch {
	case name == ".":
		if hdr.Typeflag != tar.TypeDir {
			return nil, false, fmt.Errorf("image root entry has type %q, want a directory", hdr.Typeflag)
		}
		return nil, false, nil
	case name == RootfsDir:
		if hdr.Typeflag != tar.TypeDir {
			return nil, false, fmt.Errorf("/%s has type %q, want a directory", RootfsDir, hdr.Typeflag)
		}
		out.Name = "./"
		return &out, true, nil
	case name == opaqueWhiteout || name == whiteoutPrefix+RootfsDir:
		// Hiding the lower image root or /cpaas hides the whole lower rootfs.
		out.Name = opaqueWhiteout
		out.Typeflag = tar.TypeReg
		out.Size = 0
		return &out, true, nil
	case !strings.HasPrefix(name, RootfsDir+"/"):
		return nil, false, fmt.Errorf("entry /%s is outside /%s", name, RootfsDir)
	}

	rel := strings.TrimPrefix(name, RootfsDir+"/")
	out.Name = rel
	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeRegA, tar.TypeDir: //nolint:staticcheck // legacy type of regular files
	case tar.TypeSymlink:
		if err := checkSymlink(name, hdr.Linkname); err != nil {
			return nil, false, err
		}
	case tar.TypeLink:
		target := cleanEntryName(hdr.Linkname)
		if !strings.HasPrefix(target, RootfsDir+"/") {
			return nil, false, fmt.Errorf("hard link /%s points to /%s outside /%s", name, target, RootfsDir)
		}
		out.Linkname = strings.TrimPrefix(target, RootfsDir+"/")
	default:
		return nil, false, fmt.Errorf("entry /%s has unsupported type %q", name, hdr.Typeflag)
	}
	return &out, true, nil
}

// checkSymlink rejects a symbolic link at /name whose target is absolute or resolves
// outside /cpaas.
func checkSymlink(name, target string) error {
	if path.IsAbs(target) {
		return fmt.Errorf("symbolic link /%s has absolute target %s", name, target)
	}
	resolved := path.Join(path.Dir(name), target)
	if resolved != RootfsDir && !strings.HasPrefix(resolved, RootfsDir+"/") {
		return fmt.Errorf("symbolic link /%s target %s leaves /%s", name, target, RootfsDir)
	}
	return nil
}

// cleanEntryName returns a tar entry name relative to the image root, without "./" or "/"
// prefixes; "." names the root itself. Cleaning against "/" turns ".." escapes into paths
// under the root, which then fail the /cpaas checks.
func cleanEntryName(name string) string {
	cleaned := path.Clean("/" + name)
	if cleaned == "/" {
		return "."
	}
	return strings.TrimPrefix(cleaned, "/")
}
