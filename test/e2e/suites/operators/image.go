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
	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/cmd"
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

func (f *fakeImageClient) PullImage(images ...string) error {
	return f.AtollCmd.ImagePull(&cmd.PullOptions{
		ImageRefs: images,
	})
}
