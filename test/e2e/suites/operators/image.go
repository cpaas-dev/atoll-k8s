// Copyright © 2023 sealos.
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

package operators

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/json"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/cmd"
)

const (
	DockerTarFile = "/tmp/docker.tar"
	OCITarFile    = "/tmp/oci.tar"
)

type fakeImageClient struct {
	*cmd.AtollCmd
}

func NewFakeImage(atollCmd *cmd.AtollCmd) FakeImageInterface {
	return &fakeImageClient{
		AtollCmd: atollCmd,
	}
}

var _ FakeImageInterface = &fakeImageClient{}

func (f *fakeImageClient) ListImages(display bool) ([]DisplayImage, error) {
	if display {
		return nil, f.AtollCmd.ImageList()
	}
	data, err := f.AtollCmd.Executor.Exec(f.AtollCmd.BinPath, "images", "--json")
	if err != nil {
		return nil, err
	}
	var images []DisplayImage
	err = json.Unmarshal(data, &images)
	return images, err
}

func (f *fakeImageClient) PullImage(images ...string) error {
	return f.AtollCmd.ImagePull(&cmd.PullOptions{
		ImageRefs: images,
		Quiet:     true,
	})
}

func (f *fakeImageClient) RemoveImage(images ...string) error {
	return f.AtollCmd.ImageRemove(images...)
}

func (f *fakeImageClient) DockerArchiveImage(name string) error {
	return f.AtollCmd.ImageSave(name, DockerTarFile, "docker-archive")
}

func (f *fakeImageClient) OCIArchiveImage(name string) error {
	return f.AtollCmd.ImageSave(name, OCITarFile, "oci-archive")
}

func (f *fakeImageClient) SaveImage(name, file string) error {
	return f.AtollCmd.ImageSave(name, file, "")
}

func (f *fakeImageClient) SaveMultiImage(file string, name ...string) error {
	return f.AtollCmd.ImageMultiSave(file, name...)
}

func (f *fakeImageClient) LoadImage(file string) error {
	return f.AtollCmd.ImageLoad(file)
}

func (f *fakeImageClient) Create(name string, short bool) ([]byte, error) {
	return f.AtollCmd.Create(&cmd.CreateOptions{
		Short: short,
		Image: name,
	})
}

func (f *fakeImageClient) Merge(newImage string, images []string) error {
	return f.AtollCmd.ImageMerge(&cmd.MergeOptions{
		Tag:       []string{newImage},
		ImageRefs: images,
	})
}

func (f *fakeImageClient) FetchImageID(name string) (string, error) {
	data, err := f.AtollCmd.Executor.Exec(f.BinPath, "images", "-q", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
func (f *fakeImageClient) BuildImage(image, context string, opts BuildOptions) error {
	return f.AtollCmd.Build(&cmd.BuildOptions{
		Tag:          image,
		SaveImage:    opts.SaveImage,
		Context:      context,
		MaxPullProcs: opts.MaxPullProcs,
		Pull:         opts.Pull,
		Debug:        true,
	})
}

func (f *fakeImageClient) TagImage(name, newName string) error {
	return f.AtollCmd.ImageTag(name, newName)
}
