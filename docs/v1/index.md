# Atoll-K8s

Atoll-K8s 用来在物理机或虚拟机上部署和维护 Kubernetes 集群。它把集群基础设施拆成三个组件，每个组件有独立的集群镜像和命令，可以逐台主机安装和升级：

| 组件 | 内容 | 命令 |
| --- | --- | --- |
| 镜像仓库 | zot，或者已有的 Harbor、Nexus | `atoll registry` |
| 容器运行时 | containerd 或 docker，以及 image-cri-shim | `atoll cri` |
| Kubernetes | kubeadm、kubelet、kubectl、lvscare 和控制面组件镜像 | `atoll k8s` |

!!! info "文档状态"

    本文档描述 v1 的设计。代码正在按此实现，部分命令尚不可用。

## 从哪里开始

- 第一次使用：先 [安装 atoll](010-getting-started/010-install.md)，再 [准备镜像](010-getting-started/020-prepare-images.md) 和 [创建集群](010-getting-started/030-create-cluster.md)。
- 日常维护：[新增节点](020-guides/040-add-nodes.md)、[升级 Kubernetes](020-guides/060-upgrade-kubernetes.md)、[删除节点](020-guides/080-delete-nodes.md)。
- 构建集群镜像：从 [镜像打包的通用约定](040-image-spec/010-conventions.md) 开始。
- 了解设计：从 [架构](030-design/010-architecture.md) 开始。
- 查命令：[命令速查](050-reference/010-commands.md)。
