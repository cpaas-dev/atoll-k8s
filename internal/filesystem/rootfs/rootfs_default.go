/*
Copyright 2022 cuisongliu@qq.com.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rootfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/env"
	"github.com/cpaas-dev/atoll-k8s/internal/exec"
	"github.com/cpaas-dev/atoll-k8s/internal/filesystem"
	"github.com/cpaas-dev/atoll-k8s/internal/ssh"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	executils "github.com/cpaas-dev/atoll-k8s/internal/utils/exec"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/file"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/hash"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/maps"
	stringsutil "github.com/cpaas-dev/atoll-k8s/internal/utils/strings"
)

type defaultRootfs struct {
	mounts []v2.MountImage
}

func (f *defaultRootfs) MountRootfs(cluster *v2.Cluster, hosts []string) error {
	return f.mountRootfs(cluster, hosts)
}

func (f *defaultRootfs) UnMountRootfs(cluster *v2.Cluster, hosts []string) error {
	return f.unmountRootfs(cluster, hosts)
}

func (f *defaultRootfs) mountRootfs(cluster *v2.Cluster, ipList []string) error {
	pathResolver := constants.NewPathResolver(cluster.Name)
	target := pathResolver.RootFSPath()
	ctx := context.Background()
	eg, _ := errgroup.WithContext(ctx)
	envProcessor := env.NewEnvProcessor(cluster)
	// TODO: remove this when rendering on client side is GA
	for _, mount := range f.mounts {
		src := mount
		eg.Go(func() error {
			if !file.IsExist(src.MountPoint) {
				logger.Debug("Image %s not exist, render env continue", src.ImageName)
				return nil
			}
			// TODO: if we are planing to support rendering templates for each host,
			// then move this rendering process before ssh.CopyDir and do it one by one.
			envs := v2.MergeEnvWithBuiltinKeys(src.Env, src)
			err := renderTemplatesWithEnv(src.MountPoint, ipList, envProcessor, envs)
			if err != nil {
				return fmt.Errorf("failed to render env: %w", err)
			}
			dirs, err := file.StatDir(src.MountPoint, true)
			if err != nil {
				return fmt.Errorf("failed to stat files: %w", err)
			}
			if len(dirs) != 0 {
				_, err = executils.RunBashCmd(fmt.Sprintf(constants.DefaultChmodBash, src.MountPoint))
				if err != nil {
					return fmt.Errorf("run chmod to rootfs failed: %w", err)
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	sshClient := ssh.NewCacheClientFromCluster(cluster, true)
	execer, err := exec.New(sshClient)
	if err != nil {
		return err
	}

	notRegistryDirFilter := func(entry fs.DirEntry) bool { return !constants.IsRegistryDir(entry) }

	copyFn := func(m v2.MountImage, targetHost, targetDir string) error {
		logger.Debug("send mount image, target: %s, image: %s, type: %s", targetHost, m.ImageName, m.Type)
		if err := ssh.CopyDir(execer, targetHost, m.MountPoint, targetDir, notRegistryDirFilter); err != nil {
			logger.Error("error occur while sending mount image %s: %v", m.Name, err)
			return err
		}
		return nil
	}

	// only care about envs from rootfs
	rootfs := cluster.GetRootfsImage()
	// would never happened
	if rootfs == nil {
		return errors.New("cannot mount a cluster without rootfs, this is an unexpected bug")
	}
	rootfsEnvs := v2.MergeEnvWithBuiltinKeys(rootfs.Env, *rootfs)
	localAtollctlPath := findLocalAtollctl()

	for idx := range ipList {
		ip := ipList[idx]
		eg.Go(func() error {
			if len(f.mounts) == 0 {
				return nil
			}
			for i := range f.mounts {
				// contents in rootfs/patch type images cannot be replicated asynchronously
				if err := copyFn(f.mounts[i], ip, target); err != nil {
					return err
				}
			}
			envs := envProcessor.Getenv(ip)
			envs = maps.Merge(rootfsEnvs, envs)
			envs[v2.ImageRunModeEnvSysKey] = strings.Join(cluster.GetRolesByIP(ip), ",")
			if err := syncAtollctlToRootfs(execer, ip, localAtollctlPath, pathResolver.RootFSAtollctlPath()); err != nil {
				logger.Warn("failed to sync atollctl to %s: %v", ip, err)
			}
			renderCommand := getRenderCommand(pathResolver.RootFSAtollctlPath(), target)

			return execer.CmdAsync(ip, stringsutil.RenderShellWithEnv(renderCommand, envs))
		})
	}
	return eg.Wait()
}

func getRenderCommand(binary string, target string) string {
	// skip if atollctl doesn't has subcommand render
	return fmt.Sprintf("%s render --debug=%v --clear %s >/dev/null 2>&1 || true", binary,
		logger.IsDebugMode(),
		strings.Join([]string{
			filepath.Join(target, constants.EtcDirName),
			filepath.Join(target, constants.ScriptsDirName),
			filepath.Join(target, constants.ManifestsDirName),
		}, " "))
}

// SyncLocalAtollctl copies the local atollctl into an existing rootfs when available
// and compatible with the target architecture. Missing local binaries are skipped.
func SyncLocalAtollctl(execer exec.Interface, host, remoteAtollctlPath string) error {
	return syncAtollctlToRootfs(execer, host, findLocalAtollctl(), remoteAtollctlPath)
}

func findLocalAtollctl() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	var candidates []string
	if path := os.Getenv("ATOLL_ATOLLCTL_PATH"); path != "" {
		candidates = append(candidates, path)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "atollctl"))
	}
	if path, err := osexec.LookPath("atollctl"); err == nil {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, "/tmp/atollctl")

	for _, candidate := range candidates {
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0111 != 0
}

func syncAtollctlToRootfs(execer exec.Interface, host, localAtollctlPath, remoteAtollctlPath string) error {
	if localAtollctlPath == "" {
		return nil
	}
	if !isRemoteArchCompatible(execer, host) {
		return nil
	}
	if isRemoteAtollctlCurrent(execer, host, localAtollctlPath, remoteAtollctlPath) {
		return nil
	}

	tmpPath := remoteAtollctlPath + ".new"
	if err := execer.Copy(host, localAtollctlPath, tmpPath); err != nil {
		return err
	}
	return execer.CmdAsync(host, fmt.Sprintf("install -m 0755 %s %s && rm -f %s",
		shellQuote(tmpPath),
		shellQuote(remoteAtollctlPath),
		shellQuote(tmpPath),
	))
}

func isRemoteAtollctlCurrent(execer exec.Interface, host, localAtollctlPath, remoteAtollctlPath string) bool {
	localDigest := hash.FileDigest(localAtollctlPath)
	if localDigest == "" {
		return false
	}
	output, err := execer.Cmd(host, fmt.Sprintf("if [ -f %s ]; then sha256sum %s | cut -d' ' -f1; fi",
		shellQuote(remoteAtollctlPath),
		shellQuote(remoteAtollctlPath),
	))
	if err != nil {
		logger.Debug("failed to get remote atollctl digest on host %s: %v", host, err)
		return false
	}
	return strings.TrimSpace(string(output)) == localDigest
}

func isRemoteArchCompatible(execer exec.Interface, host string) bool {
	output, err := execer.Cmd(host, "uname -m")
	if err != nil {
		logger.Debug("failed to detect remote arch on host %s: %v", host, err)
		return false
	}
	return normalizeArch(strings.TrimSpace(string(output))) == runtime.GOARCH
}

func normalizeArch(arch string) string {
	switch arch {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return arch
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (f *defaultRootfs) unmountRootfs(cluster *v2.Cluster, ipList []string) error {
	clusterRootfsDir := constants.NewPathResolver(cluster.Name).Root()
	rmRootfs := fmt.Sprintf("rm -rf %s", clusterRootfsDir)
	deleteHomeDirCmd := fmt.Sprintf("rm -rf %s", constants.ClusterDir(cluster.Name))
	eg, _ := errgroup.WithContext(context.Background())
	sshClient := ssh.NewCacheClientFromCluster(cluster, true)
	execer, err := exec.New(sshClient)
	if err != nil {
		return err
	}

	for _, IP := range ipList {
		ip := IP
		eg.Go(func() error {
			return execer.CmdAsync(ip, rmRootfs, deleteHomeDirCmd)
		})
	}
	return eg.Wait()
}

func renderTemplatesWithEnv(mountDir string, ipList []string, p env.Interface, envs map[string]string) error {
	// currently only render once
	return p.RenderAll(ipList[0], mountDir, envs)
}

func newDefaultRootfs(mounts []v2.MountImage) (filesystem.Mounter, error) {
	return &defaultRootfs{mounts: mounts}, nil
}

// NewRootfsMounter :according to the Metadata file content to determine what kind of Filesystem will be load.
func NewRootfsMounter(images []v2.MountImage) (filesystem.Mounter, error) {
	return newDefaultRootfs(images)
}
