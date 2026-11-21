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

package checker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/moby/moby/api/types/registry"

	"github.com/cpaas-dev/atoll-k8s/internal/sreg/registry/crane"

	"github.com/cpaas-dev/atoll-k8s/internal/exec"
	"github.com/cpaas-dev/atoll-k8s/internal/registry/helpers"

	"github.com/cpaas-dev/atoll-k8s/internal/constants"
	"github.com/cpaas-dev/atoll-k8s/internal/ssh"
	"github.com/cpaas-dev/atoll-k8s/internal/template"
	v2 "github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/iputils"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

type RegistryChecker struct {
}

type RegistryStatus struct {
	Address        string
	Port           string
	Storage        string
	Dedupe         bool
	GC             bool
	Htpasswd       string
	RegistryDomain string
	Auth           string
	Ping           string
	Error          string
}

// zotConfig is the subset of the zot registry config reported by the checker.
type zotConfig struct {
	Storage struct {
		RootDirectory string `json:"rootDirectory"`
		Dedupe        *bool  `json:"dedupe"`
		GC            *bool  `json:"gc"`
	} `json:"storage"`
	HTTP struct {
		Address string `json:"address"`
		Port    string `json:"port"`
		Auth    struct {
			Htpasswd struct {
				Path string `json:"path"`
			} `json:"htpasswd"`
		} `json:"auth"`
	} `json:"http"`
}

// readZotConfig parses the zot JSON config at file and requires the fields reported by the checker.
func readZotConfig(file string) (*zotConfig, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read registry config: %w", err)
	}
	cfg := &zotConfig{}
	if err = json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse registry config %s: %w", file, err)
	}
	var missing []string
	if cfg.Storage.RootDirectory == "" {
		missing = append(missing, "storage.rootDirectory")
	}
	if cfg.Storage.Dedupe == nil {
		missing = append(missing, "storage.dedupe")
	}
	if cfg.Storage.GC == nil {
		missing = append(missing, "storage.gc")
	}
	if cfg.HTTP.Address == "" {
		missing = append(missing, "http.address")
	}
	if cfg.HTTP.Port == "" {
		missing = append(missing, "http.port")
	}
	if cfg.HTTP.Auth.Htpasswd.Path == "" {
		missing = append(missing, "http.auth.htpasswd.path")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("registry config %s is missing required fields: %s", file, strings.Join(missing, ", "))
	}
	return cfg, nil
}

func (n *RegistryChecker) Check(cluster *v2.Cluster, phase string) error {
	if phase != PhasePost {
		return nil
	}
	localAddr, _ := iputils.ListLocalHostAddrs()
	if !iputils.IsLocalIP(cluster.GetRegistryIP(), localAddr) {
		logger.Info("current registry ip is %s,not local addr,skip check.", cluster.GetRegistryIP())
		return nil
	}
	status := &RegistryStatus{}
	defer func() {
		err := n.Output(status)
		if err != nil {
			logger.Error("error output: %+v", err)
		}
	}()

	var errs []error
	if cfg, err := readZotConfig(path.Join(constants.RegistryConfigDir, constants.RegistryConfigFileName)); err != nil {
		errs = append(errs, err)
	} else {
		status.Address = cfg.HTTP.Address
		status.Port = cfg.HTTP.Port
		status.Storage = cfg.Storage.RootDirectory
		status.Dedupe = *cfg.Storage.Dedupe
		status.GC = *cfg.Storage.GC
		status.Htpasswd = cfg.HTTP.Auth.Htpasswd.Path
	}

	sshCtx := ssh.NewCacheClientFromCluster(cluster, false)
	execer, err := exec.New(sshCtx)
	if err != nil {
		return err
	}
	root := constants.NewPathResolver(cluster.Name).RootFSPath()
	regInfo := helpers.GetRegistryInfo(execer, root, cluster.GetRegistryIPAndPort())
	status.Auth = fmt.Sprintf("%s:%s", regInfo.Username, regInfo.Password)
	status.RegistryDomain = fmt.Sprintf("%s:%s", regInfo.Domain, regInfo.Port)
	cfg := registry.AuthConfig{
		Username: regInfo.Username,
		Password: regInfo.Password,
	}
	if _, err = crane.NewRegistry(status.RegistryDomain, cfg); err != nil {
		errs = append(errs, fmt.Errorf("get registry interface error: %w", err))
	} else {
		status.Ping = "ok"
	}
	status.Error = Nil
	if len(errs) > 0 {
		status.Error = errors.Join(errs...).Error()
	}
	return nil
}

func (n *RegistryChecker) Output(status *RegistryStatus) error {
	tpl, isOk, err := template.TryParse(`Registry Service Status
  Address: {{ .Address }}
  Port: {{ .Port }}
  Storage: {{ .Storage }}
  Dedupe: {{ .Dedupe }}
  GC: {{ .GC }}
  Htpasswd: {{ .Htpasswd }}
  RegistryDomain: {{ .RegistryDomain }}
  Auth: {{ .Auth }}
  Ping: {{ .Ping }}
  Error: {{ .Error }}
  Logger: journalctl -xeu ` + constants.RegistryServiceName + `
`)
	if err != nil || !isOk {
		if err != nil {
			logger.Error("failed to render registry checkers template. error: %s", err.Error())
			return err
		}
		return errors.New("convert registry template failed")
	}
	return tpl.Execute(os.Stdout, status)
}

func NewRegistryChecker() Interface {
	return &RegistryChecker{}
}
