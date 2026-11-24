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

package guest

import (
	"fmt"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
)

// FormalizeWorkingCommand returns cmd run in the rootfs work dir of the cluster
// clusterName, or "" for an empty cmd.
func FormalizeWorkingCommand(clusterName string, cmd string) string {
	if cmd == "" {
		return ""
	}
	return fmt.Sprintf(constants.CdAndExecCmd, constants.GetRootWorkDir(clusterName), cmd)
}
