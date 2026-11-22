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
	"strconv"
	"strings"

	"github.com/cpaas-dev/atoll-k8s/internal/types/v1beta1"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

type CommandOptions interface {
	Args() []string
}

// LifeCycleOptions atoll run/apply/delete/reset/create/add/cert options
type RunOptions struct {
	Cluster    string
	Debug      bool
	Cmd        []string
	ConfigFile []string
	Env        []string
	Force      bool
	Masters    []string
	Nodes      []string
	Images     []string
	SSH        *v1beta1.SSH
	Transport  string
}

type ApplyOptions struct {
	Clusterfile string
	Debug       bool
	ConfigFile  []string
	Env         []string
	Set         []string
	Values      []string
}

type AddOptions struct {
	Cluster string
	Debug   bool
	Masters []string
	Nodes   []string
}

type DeleteOptions struct {
	Cluster string
	Force   bool
	Debug   bool
	Masters []string
	Nodes   []string
}

type ResetOptions struct {
	Cluster string
	Force   bool
	Debug   bool
	Masters []string
	Nodes   []string
	SSH     *v1beta1.SSH
}

type CertOptions struct {
	Cluster string
	Debug   bool
	AltName []string
}

func (ro *RunOptions) Args() []string {
	if ro.SSH == nil {
		ro.SSH = &v1beta1.SSH{}
	}
	var args Args = []string{}
	return args.appendFlagsWithValues("--cluster", ro.Cluster).
		appendFlagsWithValues("--debug", ro.Debug).
		appendFlagsWithValues("--masters", strings.Join(ro.Masters, ",")).
		appendFlagsWithValues("--nodes", strings.Join(ro.Nodes, ",")).
		appendFlagsWithValues("", ro.Images).
		appendFlagsWithValues("--cmd", ro.Cmd).
		appendFlagsWithValues("--env", ro.Env).
		appendFlagsWithValues("--config-file", ro.ConfigFile).
		appendFlagsWithValues("--user", ro.SSH.User).
		appendFlagsWithValues("--passwd", ro.SSH.Passwd).
		appendFlagsWithValues("--pk", ro.SSH.Pk).
		appendFlagsWithValues("--pk-passwd", ro.SSH.PkPasswd).
		appendFlagsWithValues("--port", ro.SSH.Port).
		appendFlagsWithValues("--transport", ro.Transport)
}

func (ro *ApplyOptions) Args() []string {
	var args Args = []string{}
	return args.appendFlagsWithValues("-f", ro.Clusterfile).
		appendFlagsWithValues("--debug", ro.Debug).
		appendFlagsWithValues("--config-file", ro.ConfigFile).
		appendFlagsWithValues("--env", ro.Env).
		appendFlagsWithValues("--set", ro.Set).
		appendFlagsWithValues("--values", ro.Values)
}

type Args []string

func (ao *AddOptions) Args() []string {
	var args Args = []string{}
	return args.appendFlagsWithValues("--masters", strings.Join(ao.Masters, ",")).
		appendFlagsWithValues("--debug", ao.Debug).
		appendFlagsWithValues("--nodes", strings.Join(ao.Nodes, ",")).
		appendFlagsWithValues("--cluster", ao.Cluster)
}

func (do *DeleteOptions) Args() []string {
	var args Args = []string{}
	return args.appendFlagsWithValues("--cluster", do.Cluster).
		appendFlagsWithValues("--debug", do.Debug).
		appendFlagsWithValues("--force", do.Force).
		appendFlagsWithValues("--masters", strings.Join(do.Masters, ",")).
		appendFlagsWithValues("--nodes", strings.Join(do.Nodes, ","))
}

func (ro *ResetOptions) Args() []string {
	if ro.SSH == nil {
		ro.SSH = &v1beta1.SSH{}
	}
	var args Args = []string{}
	return args.appendFlagsWithValues("--cluster", ro.Cluster).
		appendFlagsWithValues("--debug", ro.Debug).
		appendFlagsWithValues("--force", ro.Force).
		appendFlagsWithValues("--masters", strings.Join(ro.Masters, ",")).
		appendFlagsWithValues("--nodes", strings.Join(ro.Nodes, ",")).
		appendFlagsWithValues("--user", ro.SSH.User).
		appendFlagsWithValues("--passwd", ro.SSH.Passwd).
		appendFlagsWithValues("--pk", ro.SSH.Pk).
		appendFlagsWithValues("--pk-passwd", ro.SSH.PkPasswd).
		appendFlagsWithValues("--port", ro.SSH.Port)
}

func (co *CertOptions) Args() []string {
	var args Args = []string{}
	return args.appendFlagsWithValues("--cluster", co.Cluster).
		appendFlagsWithValues("--debug", co.Debug).
		appendFlagsWithValues("--alt-names", strings.Join(co.AltName, ","))
}

func (args Args) appendFlagsWithValues(flagName string, values interface{}) Args {
	switch vv := values.(type) {
	case []string:
		if vv == nil {
			return args
		}
		for _, v := range vv {
			if flagName != "" {
				args = append(args, flagName)
			}
			args = append(args, v)
		}
	case string:
		if vv == "" {
			return args
		}
		if flagName != "" {
			args = append(args, flagName)
		}
		args = append(args, vv)
	case bool:
		if vv {
			if flagName != "" {
				args = append(args, flagName)
			}
		}
	case uint16:
		if vv == 0 {
			return args
		}
		if flagName != "" {
			args = append(args, flagName)
		}
		args = append(args, strconv.Itoa(int(vv)))
	case int:
		if vv == 0 {
			return args
		}
		if flagName != "" {
			args = append(args, flagName)
		}
		args = append(args, strconv.Itoa(vv))
	default:
		logger.Error("Unsupported %s type %T", flagName, vv)
	}
	return args
}
