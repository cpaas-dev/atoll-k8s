# 架构

## 范围

atoll 只负责集群的基础设施，由三个组件组成：

| 组件 | 镜像类型 | 安装在哪些主机 | 每台主机最多 | 命令 |
| --- | --- | --- | --- | --- |
| 镜像仓库 | `registry` | registry 主机，默认全部 master | 一个 | `atoll registry` |
| 容器运行时 | `container-runtime` | 每台主机 | 一个 | `atoll cri` |
| Kubernetes | `kubernetes` | 每台主机 | 一个 | `atoll k8s` |

- 镜像仓库：集群内拉取镜像的来源，使用 zot，也可以直接使用已有的 Harbor 或 Nexus，见 [镜像仓库](040-registry.md)。
- 容器运行时：containerd 或 docker，连同 image-cri-shim 一起安装。
- Kubernetes：kubeadm、kubelet、kubectl、lvscare 和控制面组件镜像。

三个组件各有独立的集群镜像和命令，按主机分步骤安装和升级。每台主机可以使用不同的容器运行时镜像和不同的 Kubernetes 镜像，例如一部分节点用 containerd、另一部分用 docker，或者在滚动升级期间新旧 Kubernetes 版本并存。

以下内容不在 v1 范围内：

- CNI、存储、监控等应用。集群建好后由用户另行部署，atoll 不执行镜像里的 CMD 或 ENTRYPOINT。
- 重置整个集群。atoll 只提供删除节点。
- K3s 等 kubeadm 以外的发行版。

## 组件之间的关系

- 每类镜像在主机上有自己的目录，目录里的脚本负责安装和清理。
- 镜像之间不读取对方的文件。需要的信息由 atoll 通过 `ATOLL_SYS_*` 环境变量传入，例如容器运行时镜像通过 `ATOLL_SYS_REGISTRY_*` 得到仓库地址。
- atoll 按目标主机的架构，从多架构镜像中选择对应的平台导出。混合架构的集群需要在运维机上拉取所有用到的平台。

目录和变量的约定见 [镜像打包](../040-image-spec/010-conventions.md)。

## 安装顺序

三个组件必须按顺序安装，atoll 在每一步检查前一步是否完成：

1. `atoll create` / `atoll add`：建立主机清单，只做 SSH 和架构检查。
2. `atoll registry install` 安装 zot，或者 `atoll registry external` 登记外部仓库。
3. `atoll cri install`：要求集群已配置镜像仓库。
4. `atoll k8s init` / `atoll k8s join`：要求目标主机已安装容器运行时。

按这个顺序的原因：

- 容器运行时依赖仓库配置：image-cri-shim 和 containerd 需要知道仓库地址、凭据和访问端点。
- Kubernetes 依赖容器运行时：atoll 在主机上用 atollctl 检测 CRI socket 和 cgroup driver，写入 kubelet 和 kubeadm 配置。
- 部署一个新的 Kubernetes 或容器运行时镜像时，atoll 先把镜像里 `registry/` 目录下的镜像推送到仓库，再执行安装，见 [推送镜像](040-registry.md#推送镜像)。
