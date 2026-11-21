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

package password

import (
	"fmt"
	"path"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/exec"
	"github.com/cpaas-dev/atoll-k8s/internal/imagecrishim/types"
	"github.com/cpaas-dev/atoll-k8s/internal/registry/helpers"
	"github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/file"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/passwd"
)

type Upgrade interface {
	UpdateRegistryPasswd(rc *v1beta1.RegistryConfig, target, host string) error
	RestartRegistry(host string) error
	UpdateRegistryConfig(rc *v1beta1.RegistryConfig, target, host string) error
	UpdateImageShimConfig(rc *types.Config, target, host string) error
}

func NewUpgrade(name string, sshInterface exec.Interface) Upgrade {
	return &upgrade{Cluster: name, SSHInterface: sshInterface, mk: &maker{sets.New[string]()}}
}

type maker struct {
	paths sets.Set[string]
}

type upgrade struct {
	Cluster            string
	SSHInterface       exec.Interface
	RegistryConfig     *v1beta1.RegistryConfig
	ImageCRIShimConfig *types.Config
	mk                 *maker
}

func (m *upgrade) UpdateRegistryConfig(rc *v1beta1.RegistryConfig, target, host string) error {
	configPath, err := m.mk.configLocalRegistryConfig(path.Join(constants.ClusterDir(m.Cluster), constants.EtcDirName), rc)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("update registry target path is empty")
	}
	if len(configPath) > 0 {
		if err = m.SSHInterface.Copy(host, configPath, target); err != nil {
			return err
		}
	}
	return nil
}
func (m *upgrade) UpdateImageShimConfig(rc *types.Config, target, host string) error {
	configPath, err := m.mk.configLocalImageShimConfig(path.Join(constants.ClusterDir(m.Cluster), constants.EtcDirName), rc)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("update image shim target path is empty")
	}
	if len(configPath) > 0 {
		if err = m.SSHInterface.Copy(host, configPath, target); err != nil {
			return err
		}
		if err = m.SSHInterface.CmdAsync(host, "systemctl restart image-cri-shim"); err != nil {
			return err
		}
	}
	return nil
}
func (m *upgrade) UpdateRegistryPasswd(rc *v1beta1.RegistryConfig, target, host string) error {
	htpasswdPath, err := m.mk.configLocalHtpasswd(path.Join(constants.ClusterDir(m.Cluster), constants.EtcDirName), rc)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("update registry htpasswd target path is empty")
	}
	if len(htpasswdPath) > 0 {
		if err = m.SSHInterface.Copy(host, htpasswdPath, target); err != nil {
			return err
		}
	}
	return nil
}

// RestartRegistry restarts the registry service on host so that it reloads its htpasswd.
func (m *upgrade) RestartRegistry(host string) error {
	return m.SSHInterface.CmdAsync(host, fmt.Sprintf("systemctl restart %s", constants.RegistryServiceName))
}

func (m *maker) configLocalHtpasswd(cfgBasedir string, rc *v1beta1.RegistryConfig) (string, error) {
	if rc.Username == "" || rc.Password == "" {
		logger.Warn("registry username or password is empty")
		return "", nil
	}
	fp := path.Join(cfgBasedir, constants.RegistryHtpasswdFileName)
	if !m.paths.Has(fp) {
		pwd := passwd.Htpasswd(rc.Username, rc.Password)
		if err := file.WriteFile(fp, []byte(pwd)); err != nil {
			return "", fmt.Errorf("failed to make htpasswd: %v", err)
		}
	}
	return fp, nil
}

func (m *maker) configLocalRegistryConfig(cfgBasedir string, rc *v1beta1.RegistryConfig) (string, error) {
	data, err := yaml.Marshal(rc)
	if err != nil {
		return "", err
	}
	fp := path.Join(cfgBasedir, helpers.RegistryCustomConfig)
	if !m.paths.Has(fp) {
		if err = file.WriteFile(fp, data); err != nil {
			return "", fmt.Errorf("failed to make registry config: %v", err)
		}
	}
	return fp, nil
}

func (m *maker) configLocalImageShimConfig(cfgBasedir string, rc *types.Config) (string, error) {
	data, err := yaml.Marshal(rc)
	if err != nil {
		return "", err
	}
	fp := path.Join(cfgBasedir, "image-cri-shim.yaml")
	if !m.paths.Has(fp) {
		if err = file.WriteFile(fp, data); err != nil {
			return "", fmt.Errorf("failed to make registry config: %v", err)
		}
	}
	return fp, nil
}
