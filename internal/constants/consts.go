// Copyright © 2021 sealos.
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

package constants

const (
	DefaultClusterFileName = "Clusterfile"
)

const TemplateSuffix = ".tmpl"

const (
	LvsCareStaticPodName    = "kube-atoll-lvscare"
	YamlFileSuffix          = "yaml"
	DefaultRegistryDomain   = "atoll.hub"
	DefaultRegistryPort     = "5000"
	DefaultRegistryUsername = "admin"
	DefaultRegistryPassword = "passw0rd"
	DefaultRegistryData     = "/var/lib/registry"
	DefaultLvscareDomain    = "lvscare.node.ip"
	DefaultHostsPath        = "/etc/hosts"
)

// Registry node contract fulfilled by the cluster image, see README.md.
const (
	RegistryServiceName      = "zot"
	RegistryConfigDir        = "/etc/registry"
	RegistryConfigFileName   = "registry_config.json"
	RegistryHtpasswdFileName = "registry_htpasswd"
)

const (
	DefaultAPIServerDomain = "apiserver.cluster.local"
	DefaultDNSDomain       = "cluster.local"
	DefaultAPIServerPort   = 6443
)

// CRD kind
const (
	Config  = "Config"
	Cluster = "Cluster"
)

var AppName = "atoll"

var Contact = `
    _     _____   ___   _      _               _  __  ___   ____
   / \   |_   _| / _ \ | |    | |             | |/ / ( _ ) / ___|
  / _ \    | |  | | | || |    | |      _____  | ' /  / _ \ \___ \
 / ___ \   | |  | |_| || |___ | |___  |_____| | . \ | (_) | ___) |
/_/   \_\  |_|   \___/ |_____||_____|         |_|\_\ \___/ |____/

                  Address: github.com/cpaas-dev/atoll-k8s
                  Version: %s
`
