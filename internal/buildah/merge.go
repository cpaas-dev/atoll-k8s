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

package buildah

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/image"

	"go.podman.io/buildah/pkg/parse"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"go.podman.io/buildah"
	buildahcli "go.podman.io/buildah/pkg/cli"
	"go.podman.io/buildah/util"

	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/rand"
)

func newMergeCommand() *cobra.Command {
	layerFlagsResults := buildahcli.LayerResults{}
	buildFlagResults := buildahcli.BudResults{}
	fromAndBudResults := buildahcli.FromAndBudResults{}
	userNSResults := buildahcli.UserNSResults{}
	namespaceResults := buildahcli.NameSpaceResults{}
	buildahInfo := &buildah.BuilderInfo{}
	sopts := saverOptions{}
	mergeCommand := &cobra.Command{
		Use:   "merge",
		Short: "merge multiple images into one",
		RunE: func(cmd *cobra.Command, args []string) error {
			br := buildahcli.BuildOptions{
				LayerResults:      &layerFlagsResults,
				BudResults:        &buildFlagResults,
				UserNSResults:     &userNSResults,
				FromAndBudResults: &fromAndBudResults,
				NameSpaceResults:  &namespaceResults,
			}
			logger.Debug("save enable: %+v", sopts.enabled)
			return buildCmd(cmd, []string{buildahInfo.MountPoint}, sopts, br)
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			tag := getTagsFromFlags(cmd)
			var err error
			flagSetters := []FlagSetter{}
			if flagChanged(cmd, "platform") {
				platformFlags := getPlatformFromFlags(cmd)
				if len(platformFlags) != 1 {
					return fmt.Errorf("only one platform is allowed")
				}
				oss, arch, variant, err := parse.Platform(platformFlags[0])
				if err != nil {
					return err
				}
				logger.Debug("os: %s, arch: %s, variant: %s, tag: %+v", oss, arch, variant, tag)
				platform := v1.Platform{
					Architecture: arch,
					OS:           oss,
					Variant:      variant,
				}
				flagSetters = append(flagSetters, WithPlatformOption(platform))
			}

			buildahInfo, err = mergeImagesWithScratchContainer(tag[0], args, flagSetters)
			if err != nil {
				return err
			}
			return setDefaultFlagsWithSetters(cmd, setDefaultTLSVerifyFlag, setDefaultSaveImageFlag)
		},
		PostRunE: func(cmd *cobra.Command, args []string) error {
			tag := getTagsFromFlags(cmd)
			logger.Debug("images %s is merged to %+v", strings.Join(args, ","), tag)
			if buildahInfo != nil {
				b, err := New("")
				if err != nil {
					return err
				}
				if err = b.Delete(buildahInfo.Container); err != nil {
					return err
				}
			}
			return nil
		},
		Args: cobra.MinimumNArgs(2),
		Example: fmt.Sprintf(`
  %[1]s merge -t new:0.1.0 kubernetes:v1.19.9 mysql:5.7.0 redis:6.0.0`, rootCmd.CommandPath()),
	}
	mergeCommand.SetUsageTemplate(UsageTemplate())

	flags := mergeCommand.Flags()
	flags.SetInterspersed(false)

	// build is a all common flags
	buildFlags := buildahcli.GetBudFlags(&buildFlagResults)
	buildFlags.StringVar(&buildFlagResults.Runtime, "runtime", util.Runtime(), "`path` to an alternate runtime. Use BUILDAH_RUNTIME environment variable to override.")
	layerFlags := buildahcli.GetLayerFlags(&layerFlagsResults)
	fromAndBudFlags, err := buildahcli.GetFromAndBudFlags(&fromAndBudResults, &userNSResults, &namespaceResults)
	bailOnError(err, "failed to setup From and Build flags")

	sopts.RegisterFlags(flags)
	flags.AddFlagSet(&buildFlags)
	flags.AddFlagSet(&layerFlags)
	flags.AddFlagSet(&fromAndBudFlags)
	flags.SetNormalizeFunc(buildahcli.AliasFlags)
	bailOnError(markFlagsHidden(flags, "save-image"), "")
	bailOnError(markFlagsHidden(flags, "tls-verify"), "")
	bailOnError(markFlagsHidden(flags, append(flagsInBuildCommandToBeHidden(), flagsAssociatedWithPlatform()...)...), "")
	return mergeCommand
}

// mergeImagesWithScratchContainer prepares the build context of the merge in the mount point of
// a scratch container: the registry roots of all images merged into registry/ and a Sealfile
// copying the file systems of the images without their registry directories, then the merged
// registry root. The scratch container is deleted when preparing fails.
func mergeImagesWithScratchContainer(newImageName string, images []string, setters []FlagSetter) (_ *buildah.BuilderInfo, err error) {
	b, err := New("")
	if err != nil {
		return nil, err
	}
	cName := fmt.Sprintf("%s-%s", newImageName, rand.Generator(8))
	bInfo, err := b.Create(cName, "scratch", setters...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			if deleteErr := b.Delete(cName); deleteErr != nil {
				err = errors.Join(err, fmt.Errorf("delete scratch container %s: %w", cName, deleteErr))
			}
		}
	}()
	logger.Debug("mergeDir: %s", bInfo.MountPoint)
	if err = b.Pull(images, append(setters, WithPullPolicyOption(PullIfMissing.String()))...); err != nil {
		return nil, err
	}

	imageObjList := make([]map[string]v1.Image, 0)
	for _, i := range images {
		obj, err := b.InspectImage(i)
		if err != nil {
			return nil, fmt.Errorf("inspect image %s: %w", i, err)
		}
		imageObjList = append(imageObjList, map[string]v1.Image{i: *obj.OCIv1})
	}

	mergeDir := bInfo.MountPoint
	registryDir := filepath.Join(mergeDir, constants.RegistryDirName)
	for idx, i := range images {
		if err = mergeImageRegistry(b, fmt.Sprintf("%s-%d", cName, idx), i, registryDir, setters); err != nil {
			return nil, err
		}
	}
	dockerfile, err := image.MergeDockerfileFromImages(imageObjList)
	if err != nil {
		return nil, err
	}
	hasRegistry, err := pathExists(registryDir)
	if err != nil {
		return nil, err
	}
	if hasRegistry {
		dockerfile += "\n" + image.CopyRegistryInstruction
	}
	err = os.WriteFile(path.Join(mergeDir, "Sealfile"), []byte(dockerfile), 0755)
	if err != nil {
		return nil, err
	}
	logger.Debug("buildOptions file: %s", path.Join(mergeDir, "Sealfile"))
	logger.Debug("buildOptions tag: %s", newImageName)
	logger.Debug("buildOptions contextDir: %s", mergeDir)
	return &bInfo, nil
}

// mergeImageRegistry merges the registry root of the pulled image imageName into dst through
// the working container containerName, which is deleted afterwards.
func mergeImageRegistry(b Interface, containerName, imageName, dst string, setters []FlagSetter) (err error) {
	info, err := b.Create(containerName, imageName, setters...)
	if err != nil {
		return fmt.Errorf("create working container %s of image %s: %w", containerName, imageName, err)
	}
	defer func() {
		if deleteErr := b.Delete(containerName); deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("delete working container %s of image %s: %w", containerName, imageName, deleteErr))
		}
	}()
	if err := mergeRootfsRegistry(dst, info.MountPoint); err != nil {
		return fmt.Errorf("merge the registry directory of image %s: %w", imageName, err)
	}
	return nil
}
