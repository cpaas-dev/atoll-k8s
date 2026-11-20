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

package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/template"
	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/utils"
)

const (
	//	ImageDockerfile = `FROM scratch
	//COPY . .`
	RootfsTemplateDockerfile = `FROM {{ .BaseImage }}
MAINTAINER cpaas-dev
{{- if .Copys }}
{{- range .Copys }}
COPY {{.}}
{{- end }}
{{- end }}`
)

type RootfsDockerfile struct {
	Images            []string
	KubeadmYaml       string
	BaseImage         string
	dockerfileContent string
	Copys             []string
}

func (d *RootfsDockerfile) Write() (string, error) {
	tmpdir, err := utils.MkTmpdir("")
	if err != nil {
		return "", fmt.Errorf("create tmpdir failed: %w", err)
	}
	if d.BaseImage == "" {
		d.BaseImage = "scratch"
	}
	if len(d.Images) != 0 {
		if err := os.MkdirAll(path.Join(tmpdir, "images", "shim"), 0755); err != nil {
			return "", fmt.Errorf("create images dir failed: %w", err)
		}
		for i, image := range d.Images {
			if err := os.WriteFile(path.Join(tmpdir, "images", "shim", fmt.Sprintf("image%d", i)), []byte(image), 0644); err != nil {
				return "", fmt.Errorf("write shim image failed: %w", err)
			}
		}
		d.Copys = append(d.Copys, "registry registry")
	}

	if d.KubeadmYaml != "" {
		if err := os.WriteFile(tmpdir+"/kubeadm.yml", []byte(d.KubeadmYaml), 0644); err != nil {
			return "", fmt.Errorf("write kubeadm.yml failed: %w", err)
		}
		d.Copys = append(d.Copys, "kubeadm.yml etc/")
	}

	t, _, err := template.TryParse(RootfsTemplateDockerfile)
	if err != nil {
		return "", err
	}
	out := bytes.NewBuffer(nil)
	if err = t.Execute(out, d); err != nil {
		return "", err
	}
	d.dockerfileContent = out.String()
	if d.dockerfileContent == "" {
		return "", errors.New("dockerfile content is not set")
	}
	if err := os.WriteFile(tmpdir+"/Dockerfile", []byte(d.dockerfileContent), 0644); err != nil {
		return "", fmt.Errorf("write RootfsDockerfile failed: %w", err)
	}
	return tmpdir, nil
}
