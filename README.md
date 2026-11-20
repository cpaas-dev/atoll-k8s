<p align="center">
  <a href="https://sealos.io/">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="./docs/img/sealos-left-dark.png" />
      <source media="(prefers-color-scheme: light)" srcset="./docs/img/sealos-left.png" />
      <img src="./docs/img/sealos-left.png" alt="Sealos" width="260" />
    </picture>
  </a>
</p>

<h1 align="center">Sealos</h1>

<p align="center">Kubernetes cluster lifecycle management.</p>

<p align="center"><a href="./README_zh.md">简体中文</a></p>

## Overview

Sealos is a standalone binary tool with zero dependencies for managing the entire lifecycle of a Kubernetes cluster. It can install a single-node development environment or production-grade highly available clusters, and assemble applications on top of Kubernetes using cluster images.

This repository contains the [`lifecycle`](./lifecycle) module:

- `sealos`: the CLI for creating, scaling, resetting and building clusters and cluster images.
- `sealctl`: the node-side helper used by `sealos`.
- `lvscare`: an IPVS-based local load balancer providing high availability for the API server.
- `image-cri-shim`: recognizes image names automatically, making offline delivery more convenient.
- `sreg`: a registry tool for cluster images.

## Key features

- ARM support. Offline packages v1.20 and above support integration with both containerd and Docker.
- Provides 99-year certificates and supports cluster backup and upgrade.
- Does not rely on Ansible, HAProxy, or Keepalived.
- Provides offline installation. Different versions of Kubernetes only require different cluster images.
- Easy addition/deletion of cluster nodes.
- Supports cluster images, allowing you to customize and combine the cluster components you need.
- Uses the SDK of Buildah to standardize the image format, fully compatible with OCI standards.

## Get started

```bash
$ curl -sfL  https://raw.githubusercontent.com/labring/sealos/v5.1.0/scripts/install.sh \
    | sh -s v5.1.0 labring/sealos
# Create a cluster
$ sealos run labring/kubernetes:v1.25.0-4.2.0 labring/helm:v3.8.2 labring/calico:v3.24.1 \
     --masters 192.168.64.2,192.168.64.22,192.168.64.20 \
     --nodes 192.168.64.21,192.168.64.19 -p [your-ssh-passwd]
```

Build and run a customized cluster image:

```shell
sealos build -t my-kubernetes:v1.25.0 .
sealos run my-kubernetes:v1.25.0 ...
```

See [lifecycle/README.md](./lifecycle/README.md) for more details.

## 🏘️ Community & support

- 🌐 Visit the [Sealos Website](https://sealos.io/) for full documentation and useful links.
- 💬 Join our [Discord Server](https://discord.gg/wdUn538zVP) to chat with the Sealos team and other Sealos users. This is a good place to learn about Sealos and Kubernetes, ask questions, and share your experiences.
- 🐦 Tweet at @Sealos_io on [X/Twitter](https://twitter.com/Sealos_io) and follow us.
- 🐞 Create [GitHub Issues](https://github.com/labring/sealos/issues/new/choose) for bug reports and feature requests.

## 👩‍💻 Contributing & Development

Have a look through [existing issues](https://github.com/labring/sealos/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-desc) and [Pull Requests](https://github.com/labring/sealos/pulls?q=is%3Apr+is%3Aopen+sort%3Aupdated-desc) that you could help with. If you'd like to request a feature or report a bug, please [create a GitHub Issue](https://github.com/labring/sealos/issues/new/choose) using one of the templates provided.

📖 [See contribution guide →](./CONTRIBUTING.md)

🔧 [See development guide →](./lifecycle/DEVELOPGUIDE.md)

## Links

- [Buildah](https://github.com/containers/buildah) The functionalities of Buildah are extensively utilized in Sealos 4.0 to ensure that cluster images are compatible with OCI standard.

## 📄 License

Sealos is licensed under the [Sealos Sustainable Use License](./LICENSE.md), a custom license that allows:

- ✅ Internal business use and personal non-commercial use
- ❌ Providing cloud services to third parties

**Not a standard open source license** - please review the [full license terms](./LICENSE.md) before use.

**Contributing**: By contributing, you agree to our [Contributor License Agreement](./CONTRIBUTOR_LICENSE_AGREEMENT.md) and license change terms.

<!-- ## License -->

<!-- [![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Flabring%2Fsealos.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Flabring%2Fsealos?ref=badge_large) -->
