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

package e2e

import (
	"fmt"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/utils"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/suites/operators"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/suites/checkers"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("E2E_atoll_run_test", func() {
	var (
		fakeClient         *operators.FakeClient
		err                error
		fakeCheckInterface checkers.FakeInterface
	)
	fakeClient = operators.NewFakeClient("")
	Context("atoll run suit", func() {
		AfterEach(func() {
			err = fakeClient.Cluster.Reset()
			utils.CheckErr(err, fmt.Sprintf("failed to reset cluster for earch cluster: %v", err))
		})
		It("atoll run single by containerd", func() {
			images := []string{"labring/kubernetes:v1.25.0"}
			err = fakeClient.Cluster.Run(images...)
			utils.CheckErr(err, fmt.Sprintf("failed to Run new cluster for single: %v", err))
			fakeCheckInterface, err = checkers.NewFakeGroupClient("default", &checkers.FakeOpts{
				Images: images,
			})
			utils.CheckErr(err, fmt.Sprintf("failed to get cluster interface: %v", err))
			err = fakeCheckInterface.Verify()
			utils.CheckErr(err, fmt.Sprintf("failed to verify cluster for single: %v", err))
		})

	})

})
