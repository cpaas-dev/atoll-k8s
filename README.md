<p align="center">
  <img src="./docs/img/cpaas.svg" alt="CPaaS" width="240" />
</p>

<h1 align="center">Atoll-K8s</h1>

<p align="center">Kubernetes 集群生命周期管理工具</p>

<p align="center">简体中文 | <a href="./README_en.md">English</a></p>

## 概述

Atoll-K8s 是一个零依赖的独立二进制工具，用于管理 Kubernetes 集群的完整生命周期：安装、扩缩容、证书管理、升级与重置。它既能部署单节点开发环境，也能部署生产级高可用集群。

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
- 集群镜像是标准 OCI 镜像，可用 docker buildx 等常用工具构建。

## 快速开始

安装 `atoll`：

```bash
curl -sfL https://raw.githubusercontent.com/cpaas-dev/atoll-k8s/main/scripts/install.sh \
  | sh -s <version> cpaas-dev/atoll-k8s
```

拉取集群镜像并创建集群。`run` 等集群命令只使用本地镜像，需要先用 `atoll pull` 拉取：

```bash
atoll pull <registry>/kubernetes:<version>
atoll run <registry>/kubernetes:<version> \
  --masters 192.168.64.2,192.168.64.22,192.168.64.20 \
  --nodes 192.168.64.21,192.168.64.19 \
  -p <ssh-password>
```

镜像仓库只提供 HTTP 时给 `atoll pull` 加 `--plain-http`，使用自签名证书时加 `--insecure`。认证、镜像加速和短名沿用 `docker login` 写入的凭据和 `registries.conf`。离线环境可以直接传入 OCI 格式的镜像包，例如 `atoll run kubernetes.tar ...`。

集群镜像需要为 Atoll-K8s 构建：rootfs 的内容放在镜像的 `/cpaas` 目录下，atoll 只导出这个目录。镜像必须用 `atoll.io.type` 标签声明类型，只支持 `rootfs` 和 `patch`，不支持 Helm、Calico 这类应用镜像。上游 Sealos 的集群镜像（如 `labring/kubernetes`）依赖 `SEALOS_*` 环境变量和 `sealctl`，不能直接使用。

atoll 不提供构建命令，可以用 `docker buildx` 构建集群镜像，Dockerfile 里用 `COPY rootfs/ /cpaas/` 放入内容：

```bash
docker buildx build -t <registry>/my-kubernetes:v1.30.0 --push .
atoll pull <registry>/my-kubernetes:v1.30.0
atoll run <registry>/my-kubernetes:v1.30.0 --masters ... --nodes ...
```

## 从源码构建

环境要求：Go 1.27 及以上、Make。所有二进制都关闭 CGO 编译，不需要 C 编译器和系统开发库，可以在任意平台交叉编译；编译产物只能在 Linux 上运行。

```bash
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
