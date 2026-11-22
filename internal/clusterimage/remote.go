// Copyright © 2026 atoll.
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

package clusterimage

import (
	"errors"
	"fmt"
	"strings"

	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/oras-project/oras-go/v3/registry/remote/config"
	"github.com/oras-project/oras-go/v3/registry/remote/credentials"
	"github.com/oras-project/oras-go/v3/registry/remote/policy"
	"github.com/oras-project/oras-go/v3/registry/remote/properties"
	"github.com/oras-project/oras-go/v3/registry/remote/signature"
)

// ErrShortNameNotSearchable is returned for a reference without a registry host when
// registries.conf neither aliases it nor lists unqualified-search-registries.
var ErrShortNameNotSearchable = errors.New("short image name needs a registries.conf alias or unqualified-search-registries")

// remoteResolver turns image references into configured remote repositories, following
// the containers configuration files loaded into configs: registries.conf (aliases,
// unqualified search, location rewrites, mirrors, insecure), credentials from Docker
// config.json and containers auth.json, policy.json and certs.d.
type remoteResolver struct {
	configs *config.Configs
	builder *remote.ClientBuilder
}

func newRemoteResolver(configs *config.Configs) (*remoteResolver, error) {
	builder := remote.NewClientBuilder()
	if configs.DockerConfig != nil || configs.ContainersAuthConfig != nil {
		store, err := configs.CredentialStore(credentials.StoreOptions{})
		if err != nil {
			return nil, fmt.Errorf("load registry credentials: %w", err)
		}
		builder.CredentialStore = store
	}
	if configs.PolicyConfig != nil {
		verifier := signature.NewSignedByVerifierFromConfig(configs.RegistriesDConfig)
		evaluator, err := configs.PolicyEvaluator(policy.WithSignedByVerifier(verifier))
		if err != nil {
			return nil, fmt.Errorf("load image policy: %w", err)
		}
		builder.PolicyEvaluator = evaluator
	}
	return &remoteResolver{configs: configs, builder: builder}, nil
}

// qualifiedNames returns the fully qualified names to try for ref, in order. A qualified
// ref yields itself; a short name yields its alias, or one name per unqualified search
// registry. Each name carries a tag ("latest" when ref has neither tag nor digest).
func (r *remoteResolver) qualifiedNames(ref string) ([]string, error) {
	if isQualified(ref) {
		name, err := canonicalName(ref)
		if err != nil {
			return nil, err
		}
		return []string{name}, nil
	}
	registries := r.configs.RegistriesConfig
	if registries != nil {
		if alias, ok := registries.ResolveAlias(ref); ok {
			name, err := canonicalName(alias)
			if err != nil {
				return nil, fmt.Errorf("alias %q of %q: %w", alias, ref, err)
			}
			return []string{name}, nil
		}
	}
	if registries == nil || len(registries.UnqualifiedSearchRegistries) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrShortNameNotSearchable, ref)
	}
	names := make([]string, 0, len(registries.UnqualifiedSearchRegistries))
	for _, registry := range registries.UnqualifiedSearchRegistries {
		name, err := canonicalName(registry + "/" + ref)
		if err != nil {
			return nil, fmt.Errorf("search registry %q for %q: %w", registry, ref, err)
		}
		names = append(names, name)
	}
	return names, nil
}

// repository returns the remote repository serving name and the tag or digest to fetch.
func (r *remoteResolver) repository(name string, transport Transport) (*remote.Repository, string, error) {
	props, err := r.configs.RegistryProperties(name)
	if err != nil {
		return nil, "", fmt.Errorf("registry properties of %s: %w", name, err)
	}
	props.Transport.PlainHTTP = transport.PlainHTTP
	if transport.SkipTLSVerify {
		props.Transport.Insecure = true
	}
	repo, err := r.builder.NewRepository(props)
	if err != nil {
		return nil, "", fmt.Errorf("remote repository of %s: %w", name, err)
	}
	return repo, props.Reference.ReferenceOrDefault(), nil
}

// isQualified reports whether ref starts with a registry host, using the Docker rule: the
// first path component is a host when it contains "." or ":" or is "localhost".
func isQualified(ref string) bool {
	host, _, found := strings.Cut(ref, "/")
	if !found {
		return false
	}
	return strings.ContainsAny(host, ".:") || host == "localhost"
}

// canonicalName parses a fully qualified reference and returns it in the form used as the
// local store tag: registry/repository:tag, registry/repository@digest or both.
func canonicalName(ref string) (string, error) {
	parsed, err := properties.NewReference(ref)
	if err != nil {
		return "", fmt.Errorf("parse image reference %q: %w", ref, err)
	}
	if parsed.Tag == "" && parsed.Digest == "" {
		parsed.Tag = "latest"
	}
	return parsed.String(), nil
}
