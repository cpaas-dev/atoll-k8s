# 镜像仓库

集群内所有镜像都从一个镜像仓库拉取。仓库有两种来源，只能选一种：

- zot：由 atoll 部署在 registry 主机上。
- 外部仓库：已有的 Harbor 或 Nexus，atoll 不部署任何东西，用法见 [使用外部仓库](../020-guides/020-external-registry.md)。

## zot

`atoll registry install` 在 registry 主机上安装 zot。

- registry 主机在安装时确定，默认是全部 master，并以 `registry` 角色写入 Clusterfile。之后增减 master 不会改变 registry 主机。
- 仓库的域名、端口、账号、密码和数据目录写在 `spec.registry.zot`，默认值依次为 `atoll.hub`、`5000`、`admin`、`passw0rd`、`/var/lib/registry`。
- atoll 根据账号密码生成 htpasswd，放入 registry 镜像目录后再执行 `init.sh`。
- 每台集群主机的 `/etc/hosts` 把仓库域名解析到第一个 registry 主机。
- 容器运行时按顺序配置所有 registry 主机作为访问端点（`ATOLL_SYS_REGISTRY_ENDPOINTS`）。前一个端点不可用时，containerd 自动尝试下一个。

## 外部仓库

| 仓库 | 是否推送 | 说明 |
| --- | --- | --- |
| Harbor | 是 | 像对待 zot 一样推送镜像。路径前缀对应的项目需要事先建好，账号需要推送权限 |
| Nexus | 否 | 只把容器运行时指向它。镜像由 Nexus 代理上游或者事先同步 |

使用外部仓库时，清单里不能有 registry 角色。

## 镜像路径规则

集群内的镜像名统一按以下规则改写，zot 和外部仓库使用同一套规则：

```text
<仓库地址>/<前缀>/<原镜像的完整名称>
```

原镜像的完整名称包含原仓库域名；Docker Hub 的短名先补全为 `docker.io/library/...`。前缀为空时省略这一段。

| 原镜像 | zot（前缀为空） | Harbor（前缀 `library`） |
| --- | --- | --- |
| `registry.k8s.io/pause:3.9` | `atoll.hub:5000/registry.k8s.io/pause:3.9` | `harbor.example.com/library/registry.k8s.io/pause:3.9` |
| `nginx:1.27` | `atoll.hub:5000/docker.io/library/nginx:1.27` | `harbor.example.com/library/docker.io/library/nginx:1.27` |
| `ghcr.io/cpaas-dev/atoll/lvscare:v1.0.0` | `atoll.hub:5000/ghcr.io/cpaas-dev/atoll/lvscare:v1.0.0` | `harbor.example.com/library/ghcr.io/cpaas-dev/atoll/lvscare:v1.0.0` |

- kubelet 拉取镜像时，由容器运行时镜像里的 image-cri-shim 按这个规则改写。
- 保留原域名，可以避免不同来源的同名仓库互相覆盖，例如 `docker.io/library/nginx` 和 `quay.io/library/nginx`。

## 推送镜像

- 时机：部署一个新的 Kubernetes 或容器运行时镜像时，包括 `k8s init`、`k8s join`、`k8s upgrade`、`cri install`、`cri upgrade`。atoll 先推送镜像里 `registry/` 目录下的镜像，再执行安装。
- 目标：推送到每一个 zot 主机，各主机内容相同；使用外部仓库时，只推送到开启了推送的 Harbor。
- 仓库名：在镜像 `registry/` 目录里的仓库路径前面加上前缀，例如 `registry.k8s.io/pause` 推送为 `library/registry.k8s.io/pause`。
- manifest 按原样推送，digest 保持不变；仓库里已经存在的内容会跳过。

`registry/` 目录的格式见 [registry 目录](../040-image-spec/040-registry-dir.md)。
