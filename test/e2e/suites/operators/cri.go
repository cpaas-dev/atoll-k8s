/*
Copyright 2023 cuisongliu@qq.com.

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

package operators

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/json"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/cmd"
)

type fakeCRIClient struct {
	AtollCmd *cmd.AtollCmd
}

func newCRIClient(atollCmd *cmd.AtollCmd) FakeCRIInterface {
	return &fakeCRIClient{
		AtollCmd: atollCmd,
	}
}

var _ FakeCRIInterface = &fakeCRIClient{}

func (f *fakeCRIClient) Pull(name string) error {
	if f.AtollCmd.CriBinPath == "" {
		if err := f.AtollCmd.SetCriBinPath(); err != nil {
			return err
		}
	}
	return f.AtollCmd.CRIImagePull(name)
}
func (f *fakeCRIClient) ImageList() (*ImageStruct, error) {
	if f.AtollCmd.CriBinPath == "" {
		if err := f.AtollCmd.SetCriBinPath(); err != nil {
			return nil, err
		}
	}
	data, err := f.AtollCmd.CRIImageList(false)
	if err != nil {
		return nil, err
	}
	var image ImageStruct
	err = json.Unmarshal(data, &image)
	if err != nil {
		return nil, err
	}
	return &image, nil
}
func (f *fakeCRIClient) HasImage(name string) error {
	data, err := f.ImageList()
	if err != nil {
		return err
	}
	for _, v := range data.Images {
		for _, tag := range v.RepoTags {
			if tag == name {
				return nil
			}
		}
	}
	return fmt.Errorf("image %s not found", name)
}
