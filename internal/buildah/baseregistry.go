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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/openshift/imagebuilder"
	"github.com/openshift/imagebuilder/dockerfile/command"
	"github.com/openshift/imagebuilder/dockerfile/parser"
	"go.podman.io/buildah"
	"go.podman.io/buildah/define"
	"go.podman.io/image/v5/transports"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
)

// mergeBaseImageRegistry merges the registry root of the base image of the final build stage
// into the registry directory of the build context, before the build.
//
// A Kubefile "FROM <base>" followed by "COPY registry ./registry" replaces the index.json of
// every repository that the base image and the context both hold, which drops the tags of
// the base. Merging the base registry root into the context first makes the copied
// index.json a superset. The context is modified in place, like the images runSaveImages
// stores there, so it must run after runSaveImages: a tag pointing to different digests in
// the base and the context then fails the build before anything is written.
//
// Nothing is done unless the context has a registry directory and the base image has one.
// Only the final stage produces the image, so only its base applies (see finalStageBase);
// registry directories copied from other stages or images with COPY --from are not merged.
// Builds for several platforms fail, since the context registry directory is shared by all
// platforms while each platform has its own base image.
func mergeBaseImageRegistry(ctx context.Context, store storage.Store, options define.BuildOptions, containerfiles []string) error {
	contextRegistry := filepath.Join(options.ContextDirectory, constants.RegistryDirName)
	exists, err := pathExists(contextRegistry)
	if err != nil || !exists {
		return err
	}
	contents, err := readContainerfiles(options.ContextDirectory, containerfiles)
	if err != nil {
		return fmt.Errorf("determine the base image to merge into %s: %w", contextRegistry, err)
	}
	base, err := finalStageBase(contents, options)
	if err != nil {
		return fmt.Errorf("determine the base image to merge into %s: %w", contextRegistry, err)
	}
	if base == buildah.BaseImageFakeName {
		return nil
	}
	if len(options.Platforms) > 1 || options.AllPlatforms {
		return fmt.Errorf("cannot merge the registry directory of base image %s into %s for several platforms: "+
			"build each platform separately and combine the images with a manifest list", base, contextRegistry)
	}
	if options.SourcePolicyFile != "" {
		return fmt.Errorf("cannot merge the registry directory of base image %s into %s: "+
			"source policy %s may replace the base image during the build", base, contextRegistry, options.SourcePolicyFile)
	}
	return mergeImageRegistryInto(ctx, store, options, base, contextRegistry)
}

// mergeImageRegistryInto pulls base like the build does, merges its registry root into dst
// through a working container and deletes the container.
func mergeImageRegistryInto(ctx context.Context, store storage.Store, options define.BuildOptions, base, dst string) (err error) {
	builder, err := buildah.NewBuilder(ctx, store, buildah.BuilderOptions{
		FromImage:           base,
		PullPolicy:          options.PullPolicy,
		Registry:            options.Registry,
		BlobDirectory:       options.BlobDirectory,
		SignaturePolicyPath: options.SignaturePolicyPath,
		ReportWriter:        options.ReportWriter,
		SystemContext:       options.SystemContext,
		Isolation:           options.Isolation,
		IDMappingOptions:    options.IDMappingOptions,
		CommonBuildOpts:     options.CommonBuildOpts,
		MaxPullRetries:      options.MaxPullPushRetries,
		PullRetryDelay:      options.PullPushRetryDelay,
		OciDecryptConfig:    options.OciDecryptConfig,
	})
	if err != nil {
		return fmt.Errorf("create working container of base image %s: %w", base, err)
	}
	defer func() {
		if deleteErr := builder.Delete(); deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("delete working container %s of base image %s: %w", builder.Container, base, deleteErr))
		}
	}()
	mountPoint, err := builder.Mount(builder.MountLabel)
	if err != nil {
		return fmt.Errorf("mount working container %s of base image %s: %w", builder.Container, base, err)
	}
	if err := mergeRootfsRegistry(dst, mountPoint); err != nil {
		return fmt.Errorf("merge the registry directory of base image %s into the build context: %w", base, err)
	}
	return nil
}

// readContainerfiles reads the containerfiles, resolving a missing relative path against
// contextDir like imagebuildah.BuildDockerfiles. Remote, stdin and preprocessed (.in)
// containerfiles cannot be read before the build and fail.
func readContainerfiles(contextDir string, containerfiles []string) ([][]byte, error) {
	contents := make([][]byte, 0, len(containerfiles))
	for _, name := range containerfiles {
		if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") ||
			name == os.Stdin.Name() || strings.HasSuffix(name, ".in") {
			return nil, fmt.Errorf("containerfile %s must be a local file without .in preprocessing", name)
		}
		data, err := os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) && !strings.HasPrefix(name, contextDir) {
			data, err = os.ReadFile(filepath.Join(contextDir, name))
		}
		if err != nil {
			return nil, fmt.Errorf("read containerfile: %w", err)
		}
		contents = append(contents, data)
	}
	return contents, nil
}

// finalStageBase returns the base image of the stage that produces the image: the --target
// stage, otherwise the last stage. The containerfiles are parsed like imagebuildah does:
// they are concatenated, --from replaces the first FROM and build args, heading ARGs and the
// TARGET* args of the selected platform are expanded. "scratch" is returned as is.
//
// Bases whose file system is unknown before the build fail: an earlier stage, an additional
// build context, a transport reference (such as oci-archive:) and FROM --platform.
func finalStageBase(contents [][]byte, options define.BuildOptions) (string, error) {
	if len(contents) == 0 {
		return "", errors.New("no containerfile")
	}
	node, err := imagebuilder.ParseDockerfile(bytes.NewReader(contents[0]))
	if err != nil {
		return "", fmt.Errorf("parse containerfile: %w", err)
	}
	for _, data := range contents[1:] {
		extra, err := imagebuilder.ParseDockerfile(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("parse additional containerfile: %w", err)
		}
		node.Children = append(node.Children, extra.Children...)
	}
	if options.From != "" {
		replaceFirstFrom(node.Children, options.From)
	}
	b := imagebuilder.NewBuilder(options.Args)
	maps.Copy(b.BuiltinArgDefaults, targetPlatformArgs(options.SystemContext))
	stages, err := imagebuilder.NewStages(node, b)
	if err != nil {
		return "", fmt.Errorf("read stages: %w", err)
	}
	stages, ok := stages.ThroughTarget(options.Target)
	if !ok {
		return "", fmt.Errorf("target stage %q not found", options.Target)
	}
	final := stages[len(stages)-1]
	base, err := final.Builder.From(final.Node)
	if err != nil {
		return "", fmt.Errorf("resolve FROM of stage %s: %w", final.Name, err)
	}
	if base == buildah.BaseImageFakeName {
		return base, nil
	}
	if final.Builder.Platform != "" {
		return "", fmt.Errorf("stage %s uses FROM --platform=%s, which is not supported", final.Name, final.Builder.Platform)
	}
	for _, stage := range stages[:len(stages)-1] {
		if base == stage.Name || base == strconv.Itoa(stage.Position) {
			return "", fmt.Errorf("stage %s is based on the earlier stage %s, which is not supported", final.Name, base)
		}
	}
	if _, ok := options.AdditionalBuildContexts[base]; ok {
		return "", fmt.Errorf("stage %s is based on the additional build context %s, which is not supported", final.Name, base)
	}
	if transport, _, found := strings.Cut(base, ":"); found && transports.Get(transport) != nil {
		return "", fmt.Errorf("stage %s is based on %s, whose %s transport is not supported", final.Name, base, transport)
	}
	return base, nil
}

// replaceFirstFrom sets the image of the first FROM instruction to from, like the --from
// flag of imagebuildah.
func replaceFirstFrom(instructions []*parser.Node, from string) {
	for _, instruction := range instructions {
		if strings.EqualFold(instruction.Value, command.From) && instruction.Next != nil {
			instruction.Next.Value = from
			return
		}
	}
}

// targetPlatformArgs returns the TARGET* build arg defaults imagebuildah derives from the
// architecture selected in sc (with the host OS unless an OS is selected too), or nil when no
// architecture is selected.
func targetPlatformArgs(sc *types.SystemContext) map[string]string {
	if sc == nil || sc.ArchitectureChoice == "" {
		return nil
	}
	targetOS := sc.OSChoice
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	platform := targetOS + "/" + sc.ArchitectureChoice
	if sc.VariantChoice != "" {
		platform += "/" + sc.VariantChoice
	}
	return map[string]string{
		"TARGETOS":       targetOS,
		"TARGETARCH":     sc.ArchitectureChoice,
		"TARGETVARIANT":  sc.VariantChoice,
		"TARGETPLATFORM": platform,
	}
}
