# 运维机与集群主机

## 运维机

运维机是运行 atoll 的机器，可以是集群主机之一，也可以在集群之外。它需要：

- 通过 SSH 访问所有集群主机。
- 访问每个 registry 主机的仓库端口，或者访问外部仓库，用于推送镜像。

运维机上的目录：

| 路径 | 内容 |
| --- | --- |
| `~/.atoll/images` | 本地镜像库，`atoll pull` 和 `atoll import` 写入，OCI image layout 格式 |
| `~/.atoll/<cluster>/Clusterfile` | 集群清单和状态，见 [Clusterfile](030-clusterfile.md) |
| `~/.atoll/<cluster>/mounts/` | 从镜像导出的目录，按主机架构各导出一份 |
| `~/.atoll/<cluster>/pki`、`etc` | 集群证书和 admin.conf |

## 集群主机

集群主机上的目录：

| 路径 | 内容 |
| --- | --- |
| `/var/lib/atoll/data/<cluster>/registry/` | registry 镜像的内容，只在 registry 主机上 |
| `/var/lib/atoll/data/<cluster>/container-runtime/` | 本机容器运行时镜像的内容 |
| `/var/lib/atoll/data/<cluster>/kubernetes/` | 本机 Kubernetes 镜像的内容 |
| `/var/lib/atoll/data/<cluster>/bin/atollctl` | 由 atoll 分发的节点侧工具 |

## atollctl

atollctl 随 atoll 一起发布，版本与 atoll 一致，镜像里不携带 atollctl。atoll 在对一台主机执行第一个步骤之前，把 atollctl 复制到这台主机上：

- 与运维机架构相同的主机，使用 atoll 同目录下的 `atollctl`。
- 其他架构的主机，使用同目录下的 `atollctl-linux-<arch>`，例如 `atollctl-linux-arm64`。

## 主机清单与角色

集群主机先通过 `atoll create` 和 `atoll add` 写入清单，之后的每个步骤都只能操作清单里的主机。

| 角色 | 说明 |
| --- | --- |
| `master` | 控制面节点。第一个 master 称为 master0，执行 `kubeadm init` |
| `node` | 工作节点 |
| `registry` | 运行 zot 的主机，必须同时是 master 或 node |
| `amd64` / `arm64` | 主机架构，加入清单时由 atoll 自动探测 |

一台主机只能是 master 和 node 之一。registry 角色只能附加在清单内的主机上，因此镜像仓库不会部署到集群之外。
