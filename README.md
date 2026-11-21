<p align="center">
  <img src="./docs/img/cpaas.svg" alt="CPaaS" width="240" />
</p>

<h1 align="center">Atoll-K8s</h1>

<p align="center">Kubernetes 集群生命周期管理工具</p>

<p align="center">简体中文 | <a href="./README_en.md">English</a></p>

## 概述

Atoll-K8s 是一个零依赖的独立二进制工具，用于管理 Kubernetes 集群的完整生命周期：安装、扩缩容、证书管理、升级与重置。它既能部署单节点开发环境，也能部署生产级高可用集群，并通过集群镜像在 Kubernetes 之上组装应用。

本项目基于 [labring/sealos](https://github.com/labring/sealos)（commit [`c0f62a9`](https://github.com/labring/sealos/commit/c0f62a9110450f7069707215acf8e77df0815d37)）修改而来。

## 组件

| 二进制 | 说明 |
|---|---|
| `atoll` | 命令行入口，负责创建、扩缩容、重置集群，以及构建、管理集群镜像 |
| `atollctl` | 节点侧辅助工具，由 `atoll` 分发到各节点执行 |
| `lvscare` | 基于 IPVS 的本地负载均衡，为 API Server 提供高可用 |
| `image-cri-shim` | CRI 代理，自动将镜像名重写到集群私有仓库，便于离线交付 |

## 主要特性

- 单个二进制，不依赖 Ansible、HAProxy 或 Keepalived。
- 支持 amd64 与 arm64，容器运行时支持 containerd 与 Docker。
- 离线安装：不同版本的 Kubernetes 只需切换不同的集群镜像。
- 证书有效期 99 年，支持证书更新、集群备份与升级。
- 通过 IPVS 本地负载均衡实现 API Server 高可用，资源占用低。
- 节点增删便捷。
- 集群镜像基于 Buildah SDK 构建，完全兼容 OCI 标准，可按需组合存储、数据库等组件。

## 快速开始

安装 `atoll`：

```bash
curl -sfL https://raw.githubusercontent.com/cpaas-dev/atoll-k8s/main/scripts/install.sh \
  | sh -s <version> cpaas-dev/atoll-k8s
```

创建集群：

```bash
atoll run <registry>/kubernetes:<version> <registry>/helm:<version> <registry>/calico:<version> \
  --masters 192.168.64.2,192.168.64.22,192.168.64.20 \
  --nodes 192.168.64.21,192.168.64.19 \
  -p <ssh-password>
```

集群镜像需要为 Atoll-K8s 构建。上游 Sealos 的集群镜像（如 `labring/kubernetes`）依赖 `SEALOS_*` 环境变量和 `sealctl`，不能直接使用。

构建并运行自定义集群镜像：

```bash
atoll build -t my-kubernetes:v1.30.0 .
atoll run my-kubernetes:v1.30.0 --masters ... --nodes ...
```

## 从源码构建

环境要求：Linux、Go 1.27 及以上、Make。`atoll` 依赖 CGO（overlay 存储驱动），只能在 Linux 上构建。

```bash
sudo apt-get install -y pkg-config libdevmapper-dev libbtrfs-dev libgpgme-dev build-essential
make build BINS=atoll     # 构建单个二进制
make build                # 构建全部二进制，产物位于 bin/linux_<arch>/
```

更多细节见 [开发指南](./DEVELOP_GUIDE.md)。

## 目录结构

```text
cmd/        各二进制的入口
internal/   核心逻辑，包括 lvscare（internal/lvscare）与 image-cri-shim（internal/imagecrishim）
test/e2e/   端到端测试（独立 Go 模块）
docker/     镜像构建文件
scripts/    安装、构建与发布脚本
```

## 许可证

本项目沿用上游的 [Sealos Sustainable Use License](./LICENSE.md)，它不是标准开源许可证，主要限制如下：

- 允许：企业内部使用、个人非商业使用
- 禁止：向第三方提供任何形式的云服务（包括私有云与托管服务）
- 禁止：除免费的非商业分发外，向他人分发本软件

使用前请阅读 [完整许可证条款](./LICENSE.md)。源码中保留了上游的版权声明。
