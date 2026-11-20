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

<p align="center">Kubernetes 集群生命周期管理。</p>

<p align="center"><a href="./README.md">English</a></p>

## 概述

Sealos 是一个零依赖的独立二进制工具，用于管理 Kubernetes 集群的整个生命周期。既可以安装单节点开发环境，也可以构建生产级高可用集群，并通过集群镜像在 Kubernetes 之上组装应用。

本仓库包含 [`lifecycle`](./lifecycle) 模块：

- `sealos`：用于创建、扩缩容、重置、构建集群及集群镜像的命令行工具。
- `sealctl`：`sealos` 在节点侧使用的辅助工具。
- `lvscare`：基于 IPVS 的本地负载均衡，为 API Server 提供高可用。
- `image-cri-shim`：自动识别镜像名称，让离线交付更方便。
- `sreg`：集群镜像仓库工具。

## 主要特性

- 支持 ARM，v1.20 及以上版本的离线包支持 containerd 与 Docker。
- 提供 99 年证书，支持集群备份与升级。
- 不依赖 Ansible、HAProxy 或 Keepalived。
- 支持离线安装，不同版本的 Kubernetes 只需使用不同的集群镜像。
- 方便地添加/删除集群节点。
- 支持集群镜像，可按需自定义和组合所需的集群组件。
- 使用 Buildah 的 SDK 统一镜像格式，完全兼容 OCI 标准。

## 开始使用

```bash
$ curl -sfL  https://raw.githubusercontent.com/labring/sealos/v5.1.0/scripts/install.sh \
    | sh -s v5.1.0 labring/sealos
# 创建集群
$ sealos run labring/kubernetes:v1.25.0-4.2.0 labring/helm:v3.8.2 labring/calico:v3.24.1 \
     --masters 192.168.64.2,192.168.64.22,192.168.64.20 \
     --nodes 192.168.64.21,192.168.64.19 -p [your-ssh-passwd]
```

构建并运行自定义集群镜像：

```shell
sealos build -t my-kubernetes:v1.25.0 .
sealos run my-kubernetes:v1.25.0 ...
```

更多内容请参考 [lifecycle/README.md](./lifecycle/README.md)。

## 🏘️ 社区与支持

- 🌐 访问 [Sealos官网](https://sealos.io/) 获取完整的文档和实用链接。

- 💬 加入我们的 [Discord服务器](https://discord.gg/qzBmGGZGk7)，与 Sealos 开发者和终端用户进行交流。这是了解 Sealos 和 Kubernetes 以及提问和分享经验的理想之地。

- 🐦 在 [Twitter](https://twitter.com/Sealos_io) 上关注我们。

- 🐞 请将任何 Sealos 的 Bug、问题和需求提交到 [GitHub Issue](https://github.com/labring/sealos/issues/new/choose)。

## 👩‍💻 贡献与开发

翻阅[现有的 Issue](https://github.com/labring/sealos/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-desc) 和 [Pull Requests](https://github.com/labring/sealos/pulls?q=is%3Apr+is%3Aopen+sort%3Aupdated-desc)，看看您是否能提供帮助。如果您想要新增需求或报告 Bug，请使用我们提供的模板[创建一个 GitHub Issue](https://github.com/labring/sealos/issues/new/choose)。

📖 [查看贡献指南 →](./CONTRIBUTING.md)

🔧 [查看开发指南 →](./lifecycle/DEVELOPGUIDE.md)

## 🔗 链接

- [Buildah](https://github.com/containers/buildah) 在 Sealos 4.0 中，我们广泛地利用了 Buildah 的能力，以确保集群镜像与 OCI 标准兼容。

## 📄 许可证

Sealos 采用 [Sealos 可持续使用许可证](./LICENSE.md)，这是一个定制许可证，允许：
- ✅ 内部商业用途和个人非商业用途
- ❌ 向第三方提供云服务

**这不是标准的开源许可证** - 使用前请查看 [完整许可证条款](./LICENSE.md)。

**贡献代码**: 通过贡献代码，您同意我们的 [贡献者许可协议](./CONTRIBUTOR_LICENSE_AGREEMENT.md) 和许可证变更条款。

<!-- ## License -->

<!-- [![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Flabring%2Fsealos.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Flabring%2Fsealos?ref=badge_large) -->
