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

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/utils"

	"github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"

	cmd2 "github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/cmd"
	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/kube"

	"sigs.k8s.io/yaml"

	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/retry"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/terraform"
	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/settings"
)

type Applier struct {
	LocalCmd       cmd2.Interface
	RemoteCmd      cmd2.Interface
	RemoteAtollCmd *cmd2.AtollCmd
	k8sClient      kube.K8s
	Infra          *terraform.InfraDetail
	RunImages      []string
	ClusterName    string
	ImageName      string
	TestDir        string
	SSH            *v1beta1.SSH
}

func NewApplier(infra *terraform.InfraDetail) (*Applier, error) {
	publicSSHConfig := &v1beta1.SSH{
		User:   "root",
		Port:   22,
		Passwd: infra.Public.Password,
	}
	remoteSSH := cmd2.NewRemoteCmd(infra.Public.PublicIP, publicSSHConfig)
	remoteAtollCmd := cmd2.NewAtollCmd(settings.E2EConfig.AtollBinPath, remoteSSH)
	a := &Applier{
		Infra:          infra,
		SSH:            publicSSHConfig,
		RemoteAtollCmd: remoteAtollCmd,
		RemoteCmd:      remoteSSH,
		LocalCmd:       cmd2.LocalCmd{},
		ClusterName:    settings.GetEnvWithDefault(settings.TestClusterName, "default"),
		ImageName:      os.Getenv(settings.TestImageName),
		RunImages:      strings.Split(os.Getenv(settings.TestRunImages), ","),
		TestDir:        settings.DefaultTestDir,
	}
	if len(a.RunImages) == 0 {
		a.RunImages = []string{settings.HelmImageName, settings.CalicoImageName}
	}
	if err := a.WaitSSHReady(); err != nil {
		return nil, fmt.Errorf("wait for ssh ready: %v", err)
	}
	a.Infra.Nodes = append([]terraform.Host{*a.Infra.Public}, a.Infra.Nodes...)
	if err := a.initImage(); err != nil {
		return nil, fmt.Errorf("init image: %v", err)
	}
	return a, nil
}

func (a *Applier) initImage() error {
	if err := a.RemoteCmd.Copy(settings.E2EConfig.AtollBinPath, settings.E2EConfig.AtollBinPath); err != nil {
		return fmt.Errorf("copy atoll bin to remote: %v", err)
	}

	if err := a.RemoteCmd.AsyncExec("chmod +x " + settings.E2EConfig.AtollBinPath); err != nil {
		return fmt.Errorf("chmod atoll bin: %v", err)
	}
	if err := a.RemoteAtollCmd.ImagePull(&cmd2.PullOptions{
		ImageRefs: []string{a.ImageName},
	}); err != nil {
		return fmt.Errorf("pull image: %v", err)
	}
	return nil
}

func (a *Applier) FetchRemoteKubeConfig() {
	localConf := filepath.Join(a.TestDir, "kube", "admin.conf")
	if utils.IsFileExist(localConf) {
		utils.CheckErr(os.Rename(localConf, localConf+".bak"+time.Now().Format("20060102150405")))
	}
	utils.CheckErr(a.RemoteCmd.CopyR(localConf, "/root/.kube/config"))
	content, err := os.ReadFile(localConf)
	utils.CheckErr(err)

	certOpts := &cmd2.CertOptions{Cluster: a.ClusterName, AltName: []string{a.Infra.Public.PublicIP}}
	logger.Info("certOpts: %v", certOpts)
	/*
		 issue: output: Error: open /root/.atoll/e2e_test/etc/kubeadm-init.yaml: no such file or directory
		exec not in master0
	*/
	//pattern := regexp.MustCompile(`(?m)^(\s+server:\s+).+$`)
	//newData := pattern.ReplaceAllString(string(content), fmt.Sprintf("${1}%s:6443", a.EIp))
	newData := strings.Replace(string(content), "server: https://apiserver.cluster.local:6443", "server: https://"+a.Infra.Public.PublicIP+":6443", -1)
	utils.CheckErr(utils.WriteFile(localConf, []byte(newData)))
	utils.CheckErr(a.RemoteAtollCmd.Cert(certOpts))
	time.Sleep(30 * time.Second)
}

func (a *Applier) CheckNodeNum(num int) {
	notReady := make(map[string]struct{})
	err := retry.Retry(5, 5*time.Second, func() error {
		var err error
		a.k8sClient, err = kube.NewK8sClient(filepath.Join(a.TestDir, "kube", "admin.conf"), "https://"+a.Infra.Public.PublicIP+":6443")
		if err != nil {
			return err
		}
		nodes, err := a.k8sClient.ListNodes()
		if err != nil {
			return err
		}

		//kubectl get nodes --no-headers=true | awk '$2 != "Ready" {print}'
		//not_ready_nodes=$(kubectl get nodes --no-headers | awk '{ if ($2 != "Ready") print $1 }')
		for _, node := range nodes.Items {
			for _, condition := range node.Status.Conditions {
				if condition.Reason != "KubeletReady" {
					continue
				}
				if condition.Type != "Ready" {
					notReady[node.Name] = struct{}{}
					return fmt.Errorf("node %s is not ready： %s", node.Name, condition.Type)
				}
				logger.Info("node %s is ready", node.Name)
				delete(notReady, node.Name)
			}
		}

		return nil
	})
	utils.CheckErr(err)
	nodes, listErr := a.k8sClient.ListNodes()
	utils.CheckErr(listErr)
	for i := range nodes.Items {
		nodeInfo, _ := yaml.Marshal(nodes.Items[i])
		fmt.Printf("node info: %s \n", string(nodeInfo))
		if _, ok := notReady[nodes.Items[i].Name]; !ok {
			continue
		}
		logger.Error("node %s is not ready: %v", nodes.Items[i].Name, err)
	}
	utils.CheckEqual(len(nodes.Items), num)
}

func (a *Applier) WaitSSHReady() error {
	var err error
	for i := 0; i < 10; i++ {
		if err = a.RemoteCmd.AsyncExec("date"); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i) * time.Second)
	}
	return fmt.Errorf("wait for host %s ssh ready timeout: %v", a.Infra.Public.PublicIP, err)
}
