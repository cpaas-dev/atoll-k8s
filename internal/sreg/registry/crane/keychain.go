// Copyright 2018 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package crane

import (
	"strings"
	"sync"

	"github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	dockertypes "github.com/moby/moby/api/types/registry"
)

// defaultKeychain implements Keychain with the semantics of the standard Docker
// credential keychain.
type defaultKeychain struct {
	mu    sync.Mutex
	auths map[string]dockertypes.AuthConfig
}

const (
	// DefaultAuthKey is the key used for dockerhub in config files, which
	// is hardcoded for historical reasons.
	DefaultAuthKey = "https://" + name.DefaultRegistry + "/v1/"
)

// dockerHubHosts are the other host names Docker Hub credentials are stored under:
// containers/image (and so GetAuthInfo) returns them as docker.io.
var dockerHubHosts = []string{"docker.io", "registry-1.docker.io"}

func NewDefaultKeychain(auths map[string]dockertypes.AuthConfig) authn.Keychain {
	return &defaultKeychain{auths: auths}
}

// Resolve implements Keychain.
func (dk *defaultKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	dk.mu.Lock()
	defer dk.mu.Unlock()

	// See:
	// https://github.com/google/ko/issues/90
	// https://github.com/moby/moby/blob/fc01c2b481097a6057bec3cd1ab2d7b4488c50c4/registry/config.go#L397-L404
	var cfg, empty types.AuthConfig
	for _, key := range authKeys(target) {
		if auth, ok := dk.auths[key]; ok {
			cfg = types.AuthConfig(auth)
			break
		}
		// cf.GetAuthConfig automatically sets the ServerAddress attribute. Since
		// we don't make use of it, clear the value for a proper "is-empty" test.
		// See: https://github.com/google/go-containerregistry/issues/1510
		cfg.ServerAddress = ""
		if cfg != empty {
			break
		}
	}
	if cfg == empty {
		return authn.Anonymous, nil
	}

	return authn.FromConfig(authn.AuthConfig{
		Username:      cfg.Username,
		Password:      cfg.Password,
		Auth:          cfg.Auth,
		IdentityToken: cfg.IdentityToken,
		RegistryToken: cfg.RegistryToken,
	}), nil
}

// authKeys returns the auth keys to look up for target, most specific first like
// containers/image: the repository, its parent namespaces and the registry, for each host
// name of the registry. Docker Hub (index.docker.io for go-containerregistry) is also looked
// up under dockerHubHosts and DefaultAuthKey.
func authKeys(target authn.Resource) []string {
	registry := target.RegistryStr()
	hosts := []string{registry}
	if registry == name.DefaultRegistry {
		hosts = append(hosts, dockerHubHosts...)
	}
	// repository is "/<repository>" for a repository target and empty for a registry target.
	repository := strings.TrimPrefix(target.String(), registry)
	var keys []string
	for _, host := range hosts {
		for namespace := repository; namespace != ""; namespace = namespace[:strings.LastIndex(namespace, "/")] {
			keys = append(keys, host+namespace)
		}
		keys = append(keys, host)
	}
	if registry == name.DefaultRegistry {
		keys = append(keys, DefaultAuthKey)
	}
	return keys
}
