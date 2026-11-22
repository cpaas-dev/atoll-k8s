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
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/cpaas-dev/atoll-k8s/internal/clusterimage/clusterimagetest"
)

// node describes one exported file system entry.
type node struct {
	kind     string // "dir", "file" or "symlink"
	mode     fs.FileMode
	uid, gid int
	data     string // file content or symlink target
}

// snapshot records every entry under root by its path relative to root.
func snapshot(t *testing.T, root string) map[string]node {
	t.Helper()
	nodes := map[string]node{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		stat := info.Sys().(*syscall.Stat_t)
		n := node{mode: info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky), uid: int(stat.Uid), gid: int(stat.Gid)}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			n.kind = "symlink"
			n.mode = 0
			if n.data, err = os.Readlink(path); err != nil {
				return err
			}
		case info.IsDir():
			n.kind = "dir"
		default:
			n.kind = "file"
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			n.data = string(data)
		}
		if !info.ModTime().Equal(clusterimagetest.Mtime) {
			t.Errorf("%s modified at %s, want %s", rel, info.ModTime().UTC(), clusterimagetest.Mtime)
		}
		nodes[rel] = n
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return nodes
}

func TestExportAppliesLayersUnderCpaas(t *testing.T) {
	requireRoot(t)
	store := openTestStore(t, noRegistries)
	base := clusterimagetest.LayerTar(t,
		clusterimagetest.Dir("./", 0o755),
		clusterimagetest.Dir("cpaas/", 0o755),
		clusterimagetest.Entry{Name: "cpaas/a/", Type: tar.TypeDir, Mode: 0o750, UID: 1000, GID: 1000},
		clusterimagetest.File("cpaas/a/keep.txt", "old\n", 0o644),
		clusterimagetest.File("cpaas/a/remove.txt", "bye\n", 0o600),
		clusterimagetest.Dir("cpaas/b/", 0o755),
		clusterimagetest.File("cpaas/b/old.txt", "replaced\n", 0o644),
		clusterimagetest.Dir("cpaas/b/sub/", 0o755),
		clusterimagetest.File("cpaas/b/sub/deep.txt", "deep\n", 0o644),
		clusterimagetest.Dir("cpaas/bin/", 0o755),
		clusterimagetest.Entry{Name: "cpaas/bin/tool", Body: "#!/bin/sh\n", Type: tar.TypeReg, Mode: 0o4755},
		clusterimagetest.Entry{Name: "cpaas/bin/hard", Link: "cpaas/bin/tool", Type: tar.TypeLink, Mode: 0o4755},
		clusterimagetest.Entry{Name: "cpaas/bin/group", Body: "g\n", Type: tar.TypeReg, Mode: 0o2750, GID: 42},
		clusterimagetest.Symlink("cpaas/link-rel", "a/keep.txt"),
		clusterimagetest.Dir("cpaas/scripts/", 0o755),
		clusterimagetest.Symlink("cpaas/scripts/tool", "../bin/tool"),
		clusterimagetest.Dir("cpaas/shared/", 0o1777),
	)
	top := clusterimagetest.LayerTar(t,
		clusterimagetest.Entry{Name: "cpaas/a/", Type: tar.TypeDir, Mode: 0o750, UID: 1000, GID: 1000},
		clusterimagetest.File("cpaas/a/.wh.remove.txt", "", 0o644),
		clusterimagetest.Entry{Name: "cpaas/a/keep.txt", Body: "new\n", Type: tar.TypeReg, Mode: 0o640, UID: 33, GID: 33},
		clusterimagetest.Dir("cpaas/b/", 0o700),
		// The opaque marker follows an entry of the same layer, which it must keep.
		clusterimagetest.File("cpaas/b/new.txt", "fresh\n", 0o644),
		clusterimagetest.File("cpaas/b/.wh..wh..opq", "", 0o644),
		clusterimagetest.Dir("cpaas/c/", 0o755),
		clusterimagetest.File("cpaas/c/x.txt", "x\n", 0o644),
	)
	img := storeImage(t, store, "registry.example.com/cluster/edge:v1", clusterimagetest.NewImage(amd64Linux, base, top))
	target := filepath.Join(t.TempDir(), "rootfs")

	if err := store.Export(testCtx, img, target); err != nil {
		t.Fatalf("export: %v", err)
	}

	want := map[string]node{
		".":            {kind: "dir", mode: 0o755},
		"a":            {kind: "dir", mode: 0o750, uid: 1000, gid: 1000},
		"a/keep.txt":   {kind: "file", mode: 0o640, uid: 33, gid: 33, data: "new\n"},
		"b":            {kind: "dir", mode: 0o700},
		"b/new.txt":    {kind: "file", mode: 0o644, data: "fresh\n"},
		"bin":          {kind: "dir", mode: 0o755},
		"bin/tool":     {kind: "file", mode: 0o755 | fs.ModeSetuid, data: "#!/bin/sh\n"},
		"bin/hard":     {kind: "file", mode: 0o755 | fs.ModeSetuid, data: "#!/bin/sh\n"},
		"bin/group":    {kind: "file", mode: 0o750 | fs.ModeSetgid, gid: 42, data: "g\n"},
		"c":            {kind: "dir", mode: 0o755},
		"c/x.txt":      {kind: "file", mode: 0o644, data: "x\n"},
		"link-rel":     {kind: "symlink", data: "a/keep.txt"},
		"scripts":      {kind: "dir", mode: 0o755},
		"scripts/tool": {kind: "symlink", data: "../bin/tool"},
		"shared":       {kind: "dir", mode: 0o777 | fs.ModeSticky},
	}
	got := snapshot(t, target)
	paths := make([]string, 0, len(want)+len(got))
	for path := range want {
		paths = append(paths, path)
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		if got[path] != want[path] {
			t.Errorf("%s: got %+v, want %+v", path, got[path], want[path])
		}
	}

	tool, err := os.Stat(filepath.Join(target, "bin/tool"))
	if err != nil {
		t.Fatalf("stat bin/tool: %v", err)
	}
	hard, err := os.Stat(filepath.Join(target, "bin/hard"))
	if err != nil {
		t.Fatalf("stat bin/hard: %v", err)
	}
	if !os.SameFile(tool, hard) {
		t.Errorf("bin/hard is not a hard link of bin/tool")
	}
}

func TestExportCpaasWhiteoutReplacesLowerRootfs(t *testing.T) {
	requireRoot(t)
	store := openTestStore(t, noRegistries)
	base := clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.File("cpaas/old.txt", "old\n", 0o644))
	top := clusterimagetest.LayerTar(t, clusterimagetest.File(".wh.cpaas", "", 0o644), clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.File("cpaas/new.txt", "new\n", 0o644))
	img := storeImage(t, store, "registry.example.com/cluster/reset:v1", clusterimagetest.NewImage(amd64Linux, base, top))
	target := filepath.Join(t.TempDir(), "rootfs")

	if err := store.Export(testCtx, img, target); err != nil {
		t.Fatalf("export: %v", err)
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	if len(entries) != 1 || entries[0].Name() != "new.txt" {
		t.Fatalf("rootfs holds %v, want only new.txt", entries)
	}
}

func TestExportRejectsInvalidImages(t *testing.T) {
	requireRoot(t)
	valid := clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.File("cpaas/ok.txt", "ok\n", 0o644))
	tests := []struct {
		name    string
		image   func(t *testing.T) clusterimagetest.Image
		wantErr string
	}{
		{
			name: "entry outside cpaas",
			image: func(t *testing.T) clusterimagetest.Image {
				return clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.File("etc/passwd", "x\n", 0o644)))
			},
			wantErr: "entry /etc/passwd is outside /cpaas",
		},
		{
			name: "whiteout outside cpaas",
			image: func(t *testing.T) clusterimagetest.Image {
				return clusterimagetest.NewImage(amd64Linux, valid, clusterimagetest.LayerTar(t, clusterimagetest.File(".wh.etc", "", 0o644)))
			},
			wantErr: "entry /.wh.etc is outside /cpaas",
		},
		{
			name: "absolute symlink",
			image: func(t *testing.T) clusterimagetest.Image {
				return clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.Symlink("cpaas/passwd", "/etc/passwd")))
			},
			wantErr: "symbolic link /cpaas/passwd has absolute target /etc/passwd",
		},
		{
			name: "symlink leaving cpaas",
			image: func(t *testing.T) clusterimagetest.Image {
				return clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755), clusterimagetest.Symlink("cpaas/up", "../etc")))
			},
			wantErr: "symbolic link /cpaas/up target ../etc leaves /cpaas",
		},
		{
			name: "hard link outside cpaas",
			image: func(t *testing.T) clusterimagetest.Image {
				return clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755),
					clusterimagetest.Entry{Name: "cpaas/shadow", Link: "etc/shadow", Type: tar.TypeLink}))
			},
			wantErr: "hard link /cpaas/shadow points to /etc/shadow outside /cpaas",
		},
		{
			name: "device file",
			image: func(t *testing.T) clusterimagetest.Image {
				return clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755),
					clusterimagetest.Entry{Name: "cpaas/null", Type: tar.TypeChar, Mode: 0o666}))
			},
			wantErr: "entry /cpaas/null has unsupported type",
		},
		{
			name: "diff ID mismatch",
			image: func(t *testing.T) clusterimagetest.Image {
				img := clusterimagetest.NewImage(amd64Linux, valid)
				img.DiffIDs = []digest.Digest{digest.FromString("other")}
				return img
			},
			wantErr: "does not match diff ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestStore(t, noRegistries)
			img := storeImage(t, store, "registry.example.com/cluster/bad:v1", tt.image(t))
			parent := t.TempDir()
			target := filepath.Join(parent, "rootfs")

			err := store.Export(testCtx, img, target)

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("export error = %v, want it to contain %q", err, tt.wantErr)
			}
			leftovers, readErr := os.ReadDir(parent)
			if readErr != nil {
				t.Fatalf("read %s: %v", parent, readErr)
			}
			if len(leftovers) != 0 {
				t.Fatalf("failed export left %v in %s", leftovers, parent)
			}
		})
	}
}

func TestExportRefusesExistingTarget(t *testing.T) {
	store := openTestStore(t, noRegistries)
	img := storeImage(t, store, "registry.example.com/cluster/app:v1",
		clusterimagetest.NewImage(amd64Linux, clusterimagetest.LayerTar(t, clusterimagetest.Dir("cpaas/", 0o755))))
	target := t.TempDir()

	err := store.Export(testCtx, img, target)

	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("export error = %v, want an already exists error", err)
	}
}
