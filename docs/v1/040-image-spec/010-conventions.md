# 通用约定

本节是 v1 集群镜像的格式约定，供构建 registry、容器运行时和 Kubernetes 三类镜像的人使用。三类镜像在集群中的作用见 [架构](../030-design/010-architecture.md)。

## 镜像格式

- 集群镜像是标准 OCI 镜像，用 `docker buildx` 等常用工具构建，基础镜像使用 `FROM scratch`。
- 建议发布为多架构镜像（`linux/amd64`、`linux/arm64`）。每个平台只放对应架构的二进制，atoll 按目标主机的架构选择平台。
- 镜像内容全部放在 `/cpaas` 目录下。atoll 只导出 `/cpaas`，并去掉这一层前缀，再分发到主机上该类型的目录。
- 以下内容会让导出失败：`/cpaas` 之外的文件、绝对路径的符号链接、指向 `/cpaas` 之外的链接、设备文件。
- atoll 不执行镜像的 CMD 和 ENTRYPOINT。

| 镜像类型 | 主机目录 |
| --- | --- |
| `registry` | `/var/lib/atoll/data/<cluster>/registry/` |
| `container-runtime` | `/var/lib/atoll/data/<cluster>/container-runtime/` |
| `kubernetes` | `/var/lib/atoll/data/<cluster>/kubernetes/` |

## 标签

| 标签 | 适用类型 | 说明 |
| --- | --- | --- |
| `atoll.io.type` | 全部 | `registry`、`container-runtime` 或 `kubernetes` |
| `atoll.io.version` | 全部 | 镜像格式版本，固定为 `v1` |
| `atoll.io.kubernetes-version` | `kubernetes` | Kubernetes 版本，例如 `v1.30.14`，必须与 `bin/kubeadm` 一致 |
| `atoll.io.lvscare-image` | `kubernetes` | lvscare 的原镜像名，例如 `ghcr.io/cpaas-dev/atoll/lvscare:v1.0.0`，这个镜像必须在 `registry/` 里 |
| `atoll.io.image-endpoint` | `container-runtime` | kubelet 使用的镜像服务端点，即 image-cri-shim 的 socket，例如 `unix:///var/run/image-cri-shim.sock` |

表中的标签都是必填。缺少标签或取值不对的镜像，atoll 会直接报错。

## 目录结构

```text
/cpaas/
├── scripts/      生命周期脚本：check.sh、init.sh、clean.sh
├── etc/          配置文件，可以是模板
├── manifests/    Kubernetes 清单，可以是模板
├── bin/          二进制
├── statics/      不渲染的静态文件
└── registry/     需要推送到集群仓库的镜像
```

- 除 `registry/` 外，整个目录都会复制到主机上。`registry/` 只留在运维机，由 atoll 推送到仓库，格式见 [registry 目录](040-registry-dir.md)。
- 其他目录名可以自由使用，脚本按相对路径引用即可。
- 脚本和模板的约定见 [生命周期脚本](030-scripts.md) 和 [环境变量与模板](020-variables.md)。
