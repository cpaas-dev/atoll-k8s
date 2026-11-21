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

package registry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"

	"github.com/cpaas-dev/atoll-k8s/internal/registry/ocilayout"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
)

const (
	// repositoryJobs bounds the repositories of a registry root pushed to one registry at once.
	repositoryJobs = 4
	// blobJobs bounds the blobs and child manifests of one manifest uploaded at once.
	blobJobs = 4

	dialTimeout         = 30 * time.Second
	keepAlive           = 30 * time.Second
	tlsHandshakeTimeout = 10 * time.Second
	// responseHeaderTimeout bounds the wait for a response once a request is sent; it is
	// generous because committing a large blob makes the registry verify its digest first.
	responseHeaderTimeout = 5 * time.Minute
	idleConnTimeout       = 90 * time.Second
	expectContinueTimeout = time.Second
)

// pushBackoff makes up to 5 attempts of a registry request, waiting about 1s, 2s, 4s and 8s.
var pushBackoff = remote.Backoff{Duration: time.Second, Factor: 2, Jitter: 0.1, Steps: 5}

// retryStatusCodes are the registry response codes treated as transient failures.
var retryStatusCodes = []int{
	http.StatusRequestTimeout,
	http.StatusTooManyRequests,
	http.StatusInternalServerError,
	http.StatusBadGateway,
	http.StatusServiceUnavailable,
	http.StatusGatewayTimeout,
}

// registryRoot is the registry root of a mounted cluster image.
type registryRoot struct {
	// image names the mounted image in messages.
	image string
	path  string
}

// target is the registry of one registry host.
type target struct {
	// address is <host IP>:<registry port>.
	address   string
	auth      authn.Authenticator
	transport http.RoundTripper
	// backoff paces the retries of transient request failures.
	backoff remote.Backoff
}

// newTransport returns the plain HTTP transport for registry requests with explicit
// timeouts. go-containerregistry wraps it with its retry transport.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAlive}).DialContext,
		MaxIdleConnsPerHost:   repositoryJobs * blobJobs,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
		IdleConnTimeout:       idleConnTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
	}
}

// pushToTargets pushes roots to every target, targets in parallel, and joins the errors
// of all failed targets.
func pushToTargets(ctx context.Context, roots []registryRoot, targets []target) error {
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() { errs[i] = pushRoots(ctx, roots, t) })
	}
	wg.Wait()
	return errors.Join(errs...)
}

// pushRoots pushes roots to t one after another, so a tag in a later root wins.
func pushRoots(ctx context.Context, roots []registryRoot, t target) error {
	for _, root := range roots {
		logger.Info("pushing images of %s to registry %s", root.image, t.address)
		if err := pushRoot(ctx, root.path, t); err != nil {
			return fmt.Errorf("push images of %s from %s to registry %s: %w", root.image, root.path, t.address, err)
		}
	}
	return nil
}

// pushRoot pushes every repository of the registry root to t, repositoryJobs at a time.
func pushRoot(ctx context.Context, root string, t target) error {
	repositories, err := ocilayout.Repositories(root)
	if err != nil {
		return err
	}
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(repositoryJobs)
	for _, repository := range repositories {
		eg.Go(func() error { return pushRepository(egCtx, root, repository, t) })
	}
	return eg.Wait()
}

// pushRepository pushes the top-level descriptors of a layout repository to t: tagged
// descriptors by tag, untagged descriptors by digest unless an index of the repository
// reaches them, in which case they are pushed with that index.
func pushRepository(ctx context.Context, root, repository string, t target) error {
	descs, err := ocilayout.Descriptors(root, repository)
	if err != nil {
		return err
	}
	index, err := layout.ImageIndexFromPath(ocilayout.RepositoryDir(root, repository))
	if err != nil {
		return fmt.Errorf("open repository %s in %s: %w", repository, root, err)
	}
	children, err := indexChildren(index, descs)
	if err != nil {
		return fmt.Errorf("repository %s in %s: %w", repository, root, err)
	}
	p, err := newRepositoryPush(t, repository, index)
	if err != nil {
		return err
	}
	for _, desc := range descs {
		if tag, tagged := desc.Annotations[ispec.AnnotationRefName]; tagged {
			err = p.pushTagged(ctx, tag, desc)
		} else if !children[desc.Digest] {
			err = p.pushUntagged(ctx, desc)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// indexChildren returns the digests of the manifests reachable from the image indexes
// among descs. Image manifests reference no other manifests.
func indexChildren(index v1.ImageIndex, descs []v1.Descriptor) (map[v1.Hash]bool, error) {
	children := map[v1.Hash]bool{}
	for _, desc := range descs {
		if !desc.MediaType.IsIndex() {
			continue
		}
		idx, err := index.ImageIndex(desc.Digest)
		if err != nil {
			return nil, fmt.Errorf("open index %s: %w", desc.Digest, err)
		}
		if err := addIndexChildren(children, idx); err != nil {
			return nil, fmt.Errorf("index %s: %w", desc.Digest, err)
		}
	}
	return children, nil
}

// addIndexChildren adds the digests of the manifests below idx, recursively, to children.
func addIndexChildren(children map[v1.Hash]bool, idx v1.ImageIndex) error {
	manifest, err := idx.IndexManifest()
	if err != nil {
		return fmt.Errorf("parse index manifest: %w", err)
	}
	for _, child := range manifest.Manifests {
		children[child.Digest] = true
		if !child.MediaType.IsIndex() {
			continue
		}
		nested, err := idx.ImageIndex(child.Digest)
		if err != nil {
			return fmt.Errorf("open nested index %s: %w", child.Digest, err)
		}
		if err := addIndexChildren(children, nested); err != nil {
			return fmt.Errorf("nested index %s: %w", child.Digest, err)
		}
	}
	return nil
}

// repositoryPush pushes manifests of one layout repository to the same repository of one registry.
type repositoryPush struct {
	// repository is the repository reference on the registry, <address>/<repository>.
	repository string
	layout     v1.ImageIndex
	pusher     *remote.Pusher
	puller     *remote.Puller
}

// newRepositoryPush returns the push of the layout repository index to repository on t.
func newRepositoryPush(t target, repository string, index v1.ImageIndex) (repositoryPush, error) {
	ref, err := name.NewRepository(t.address+"/"+repository, name.Insecure, name.StrictValidation)
	if err != nil {
		return repositoryPush{}, fmt.Errorf("repository %s on registry %s: %w", repository, t.address, err)
	}
	opts := []remote.Option{
		remote.WithAuth(t.auth),
		remote.WithTransport(t.transport),
		remote.WithJobs(blobJobs),
		remote.WithRetryBackoff(t.backoff),
		remote.WithRetryStatusCodes(retryStatusCodes...),
		remote.WithRetryPredicate(retryTransient(ref.Name(), t.backoff.Steps)),
	}
	pusher, err := remote.NewPusher(opts...)
	if err != nil {
		return repositoryPush{}, fmt.Errorf("create pusher for %s: %w", ref, err)
	}
	puller, err := remote.NewPuller(opts...)
	if err != nil {
		return repositoryPush{}, fmt.Errorf("create puller for %s: %w", ref, err)
	}
	return repositoryPush{repository: ref.Name(), layout: index, pusher: pusher, puller: puller}, nil
}

// pushTagged points tag at desc on the registry. A tag already pointing at desc is left
// alone; a tag pointing at another digest is overwritten with a warning.
func (p repositoryPush) pushTagged(ctx context.Context, tag string, desc v1.Descriptor) error {
	ref, err := name.NewTag(p.repository+":"+tag, name.Insecure, name.StrictValidation)
	if err != nil {
		return fmt.Errorf("tag %q of %s (%s): %w", tag, p.repository, desc.Digest, err)
	}
	existing, err := p.puller.Head(ctx, ref)
	switch {
	case isNotFound(err):
	case err != nil:
		return fmt.Errorf("check existing tag %s: %w", ref, err)
	case existing.Digest == desc.Digest:
		logger.Debug("tag %s already points to %s", ref, desc.Digest)
		return nil
	default:
		logger.Warn("overwriting tag %s: %s -> %s", ref, existing.Digest, desc.Digest)
	}
	m, err := p.manifest(desc)
	if err != nil {
		return err
	}
	if err := p.pusher.Push(ctx, ref, m); err != nil {
		return fmt.Errorf("push %s (%s): %w", ref, desc.Digest, err)
	}
	logger.Debug("pushed %s (%s)", ref, desc.Digest)
	return nil
}

// pushUntagged pushes desc by digest; manifests already on the registry are skipped.
func (p repositoryPush) pushUntagged(ctx context.Context, desc v1.Descriptor) error {
	ref, err := name.NewDigest(p.repository+"@"+desc.Digest.String(), name.Insecure, name.StrictValidation)
	if err != nil {
		return fmt.Errorf("digest reference for %s in %s: %w", desc.Digest, p.repository, err)
	}
	m, err := p.manifest(desc)
	if err != nil {
		return err
	}
	if err := p.pusher.Push(ctx, ref, m); err != nil {
		return fmt.Errorf("push %s: %w", ref, err)
	}
	logger.Debug("pushed %s", ref)
	return nil
}

// manifest opens the image or image index of the top-level descriptor desc.
func (p repositoryPush) manifest(desc v1.Descriptor) (remote.Taggable, error) {
	var (
		m   remote.Taggable
		err error
	)
	switch {
	case desc.MediaType.IsIndex():
		m, err = p.layout.ImageIndex(desc.Digest)
	case desc.MediaType.IsImage():
		m, err = p.layout.Image(desc.Digest)
	default:
		return nil, fmt.Errorf(
			"manifest %s for %s has media type %q, which is neither an image nor an image index",
			desc.Digest, p.repository, desc.MediaType,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("open manifest %s for %s: %w", desc.Digest, p.repository, err)
	}
	return m, nil
}

// isNotFound reports whether err is a registry response with status 404.
func isNotFound(err error) bool {
	var terr *transport.Error
	return errors.As(err, &terr) && terr.StatusCode == http.StatusNotFound
}

// retryTransient returns the retry predicate for requests about repository: transient
// failures are logged as warnings and retried, up to attempts in total.
func retryTransient(repository string, attempts int) func(error) bool {
	return func(err error) bool {
		if !isTransient(err) {
			return false
		}
		logger.Warn("transient failure of a registry request for %s, retrying (at most %d attempts): %v",
			repository, attempts, err)
		return true
	}
}

// isTransient reports whether err is a network blip or a temporary registry failure,
// matching go-containerregistry's default retry predicate. Cancellation and deadlines are
// never transient.
func isTransient(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, net.ErrClosed)
}
