<p align="center">
  <img src="./docs/v1/assets/images/cpaas.svg" alt="CPaaS" width="240" />
</p>

<h1 align="center">Atoll-K8s</h1>

<p align="center">Kubernetes cluster lifecycle management</p>

<p align="center"><a href="./README.md">简体中文</a> | English</p>

## Overview

Atoll-K8s is a standalone, zero-dependency binary for managing the full lifecycle of a Kubernetes cluster: installation, scaling, certificate management, upgrade and reset. It deploys anything from a single-node development environment to a production-grade highly available cluster.

This project is a modified version of [labring/sealos](https://github.com/labring/sealos) (commit [`c0f62a9`](https://github.com/labring/sealos/commit/c0f62a9110450f7069707215acf8e77df0815d37)).

## Components

| Binary | Description |
|---|---|
| `atoll` | The CLI: create, scale and reset clusters; build and manage cluster images |
| `atollctl` | Node-side helper, distributed to every node and invoked by `atoll` |
| `lvscare` | IPVS-based local load balancer that keeps the API server highly available |
| `image-cri-shim` | CRI proxy that rewrites image names to the in-cluster registry for offline delivery |

## Key features

- A single binary with no dependency on Ansible, HAProxy or Keepalived.
- amd64 and arm64, with containerd or Docker as the container runtime.
- Offline installation: switching Kubernetes versions only means switching cluster images.
- 99-year certificates, with certificate renewal, cluster backup and upgrade.
- API server high availability through an IPVS local load balancer with a small footprint.
- Easy node addition and removal.
- Cluster images are standard OCI images, built with common tools such as docker buildx.

## Quick start

Install `atoll`:

```bash
curl -sfL https://raw.githubusercontent.com/cpaas-dev/atoll-k8s/main/scripts/install.sh \
  | sh -s <version> cpaas-dev/atoll-k8s
```

Pull the cluster images and create a cluster. `run` and the other cluster commands only use local images, so pull them first with `atoll pull`:

```bash
atoll pull <registry>/kubernetes:<version>
atoll run <registry>/kubernetes:<version> \
  --masters 192.168.64.2,192.168.64.22,192.168.64.20 \
  --nodes 192.168.64.21,192.168.64.19 \
  -p <ssh-password>
```

Add `--plain-http` to `atoll pull` for a registry served over HTTP, and `--insecure` for one with a self-signed certificate. Credentials, mirrors and short names come from `docker login` and `registries.conf`. Offline, pass OCI image archives directly, for example `atoll run kubernetes.tar ...`.

Cluster images must be built for Atoll-K8s: the rootfs lives under `/cpaas` in the image, and atoll exports only that directory. Images declare their type with the `atoll.io.type` label; only `rootfs` and `patch` are supported, application images such as Helm or Calico are not. Upstream Sealos cluster images (such as `labring/kubernetes`) depend on `SEALOS_*` environment variables and `sealctl`, and cannot be used directly.

atoll has no build command; build cluster images with `docker buildx`, copying the content with `COPY rootfs/ /cpaas/` in the Dockerfile:

```bash
docker buildx build -t <registry>/my-kubernetes:v1.30.0 --push .
atoll pull <registry>/my-kubernetes:v1.30.0
atoll run <registry>/my-kubernetes:v1.30.0 --masters ... --nodes ...
```

## Building from source

Requirements: Go 1.27 or later, Make. Every binary is built with CGO disabled, so no C compiler or system development libraries are needed and any host can cross-compile; the binaries only run on Linux.

```bash
make build BINS=atoll     # build a single binary
make build                # build all binaries into bin/linux_<arch>/
```

See the [development guide](./DEVELOP_GUIDE.md) for details.

## Repository layout

```text
cmd/        entry points of each binary
internal/   core logic, including lvscare (internal/lvscare) and image-cri-shim (internal/imagecrishim)
test/e2e/   end-to-end tests (separate Go module)
docker/     image build files
scripts/    install, build and release scripts
```

## License

This project keeps the upstream [Sealos Sustainable Use License](./LICENSE.md). It is not a standard open source license; its main terms are:

- Allowed: internal business use and personal non-commercial use
- Not allowed: providing any form of cloud service to third parties, including private and managed services
- Not allowed: distributing the software to others, except free distribution for non-commercial purposes

Read the [full license terms](./LICENSE.md) before use. Upstream copyright notices are preserved in the source code.
