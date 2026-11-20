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

package cmd

import (
	"os"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/settings"

	"github.com/cpaas-dev/atoll-k8s/test/e2e/testhelper/utils"
)

type AtollCmd struct {
	BinPath    string
	CriBinPath string
	Executor   Interface
	ImageService
	ClusterCycle
	CRICycle
}

func NewAtollCmd(binPath string, executor Interface) *AtollCmd {
	return &AtollCmd{
		BinPath:  binPath,
		Executor: executor,
	}
}

func (s *AtollCmd) SetCriBinPath() error {
	binPath, err := utils.GetBinPath("crictl")
	if err != nil {
		return err
	}
	s.CriBinPath = binPath
	return nil
}

type ClusterCycle interface {
	Apply(*ApplyOptions) error
	Build(*BuildOptions) error
	Create(*CreateOptions) ([]byte, error)
	Add(*AddOptions) error
	Delete(*DeleteOptions) error
	Run(*RunOptions) error
	Reset(*ResetOptions) error
	Cert(*CertOptions) error
}

type ImageService interface {
	ImagePull(*PullOptions) error
	ImagePush(image string) error
	ImageList() error
	ImageTag(name, newName string) error
	ImageSave(image string, path string, archive string) error
	ImageLoad(path string) error
	ImageMerge(options *MergeOptions) error
	ImageRemove(images ...string) error
	ImageInspect(image string) error
}

type CRICycle interface {
	CRIImageList(display bool) ([]byte, error)
	CRIProcessList(display bool) ([]byte, error)
	CRIPodList(display bool) ([]byte, error)
	CRIImagePull(name string) error
}

func isDebug() bool {
	if val, _ := os.LookupEnv(settings.TestDEBUG); val == "true" {
		return true
	}
	return false
}

func SetDebug() {
	_ = os.Setenv(settings.TestDEBUG, "true")
}

func (s *AtollCmd) Apply(args *ApplyOptions) error {
	if isDebug() {
		args.Debug = true
	}
	return s.Executor.AsyncExec(s.BinPath, append([]string{"apply"}, args.Args()...)...)
}

func (s *AtollCmd) Build(args *BuildOptions) error {
	if isDebug() {
		args.Debug = true
	}
	return s.Executor.AsyncExec(s.BinPath, append([]string{"build"}, args.Args()...)...)
}

func (s *AtollCmd) Create(args *CreateOptions) ([]byte, error) {
	if isDebug() {
		args.Debug = true
	}
	if args.Short {
		return s.Executor.Exec(s.BinPath, append([]string{"create"}, args.Args()...)...)
	}
	return nil, s.Executor.AsyncExec(s.BinPath, append([]string{"create"}, args.Args()...)...)
}

func (s *AtollCmd) Add(args *AddOptions) error {
	if isDebug() {
		args.Debug = true
	}
	return s.Executor.AsyncExec(s.BinPath, append([]string{"add"}, args.Args()...)...)
}

func (s *AtollCmd) Delete(args *DeleteOptions) error {
	if isDebug() {
		args.Debug = true
	}
	return s.Executor.AsyncExec(s.BinPath, append([]string{"delete"}, args.Args()...)...)
}

func (s *AtollCmd) Run(args *RunOptions) error {
	if isDebug() {
		args.Debug = true
	}
	return s.Executor.AsyncExec(s.BinPath, append([]string{"run"}, args.Args()...)...)
}

func (s *AtollCmd) Reset(args *ResetOptions) error {
	if isDebug() {
		args.Debug = true
	}
	return s.Executor.AsyncExec(s.BinPath, append([]string{"reset"}, args.Args()...)...)
}

func (s *AtollCmd) Cert(args *CertOptions) error {
	if isDebug() {
		args.Debug = true
	}
	return s.Executor.AsyncExec(s.BinPath, append([]string{"cert"}, args.Args()...)...)
}

func (s *AtollCmd) ImagePull(args *PullOptions) error {
	return s.Executor.AsyncExec(s.BinPath, append([]string{"pull"}, args.Args()...)...)
}

func (s *AtollCmd) ImagePush(image string) error {
	return s.Executor.AsyncExec(s.BinPath, "push", image)
}

func (s *AtollCmd) ImageList() error {
	return s.Executor.AsyncExec(s.BinPath, "list")
}

func (s *AtollCmd) ImageSave(image string, path string, archive string) error {
	if archive == "" {
		return s.Executor.AsyncExec(s.BinPath, "save", "-o", path, image)
	}
	return s.Executor.AsyncExec(s.BinPath, "save", "-o", path, "--format", archive, image)
}

func (s *AtollCmd) ImageMultiSave(path string, name ...string) error {
	param := append([]string{"save", "-m", "--format", "docker-archive", "-o", path}, name...)
	return s.Executor.AsyncExec(s.BinPath, param...)
}

func (s *AtollCmd) ImageLoad(path string) error {
	return s.Executor.AsyncExec(s.BinPath, "load", "-i", path)
}

func (s *AtollCmd) ImageTag(name, newName string) error {
	return s.Executor.AsyncExec(s.BinPath, "tag", name, newName)
}

func (s *AtollCmd) ImageMerge(args *MergeOptions) error {
	return s.Executor.AsyncExec(s.BinPath, append([]string{"merge"}, args.Args()...)...)
}

func (s *AtollCmd) ImageRemove(images ...string) error {
	return s.Executor.AsyncExec(s.BinPath, append([]string{"rmi", "-f"}, images...)...)
}

func (s *AtollCmd) ImageInspect(image string) error {
	return s.Executor.AsyncExec(s.BinPath, "inspect", image)
}
func (s *AtollCmd) CRIImageList(display bool) ([]byte, error) {
	if display {
		return nil, s.Executor.AsyncExec(s.CriBinPath, "images")
	}
	return s.Executor.Exec(s.CriBinPath, "images", "-o", "json")
}
func (s *AtollCmd) CRIProcessList(display bool) ([]byte, error) {
	if display {
		return nil, s.Executor.AsyncExec(s.CriBinPath, "ps", "-a")
	}
	return s.Executor.Exec(s.CriBinPath, "ps", "-a", "-o", "json")
}
func (s *AtollCmd) CRIPodList(display bool) ([]byte, error) {
	if display {
		return nil, s.Executor.AsyncExec(s.CriBinPath, "pods")
	}
	data, err := s.Executor.Exec(s.CriBinPath, "pods", "-o", "json")
	if err != nil {
		return nil, err
	}
	return data, nil
}
func (s *AtollCmd) CRIImagePull(name string) error {
	return s.Executor.AsyncExec(s.CriBinPath, "pull", name)
}
