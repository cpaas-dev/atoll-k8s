# 准备镜像

集群命令只使用运维机本地镜像库（`~/.atoll/images`）里的镜像，不会从远端拉取。创建集群前，先把三类镜像放进本地镜像库。

## 在线拉取

```bash
atoll pull ghcr.io/cpaas-dev/atoll/zot:v2.1.8 \
  ghcr.io/cpaas-dev/atoll/containerd:v2.1.4 \
  ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14
```

- 默认拉取运维机自己的架构。集群里有其他架构的主机时，用 `--platform linux/arm64` 再拉一次。
- 仓库只提供 HTTP 时加 `--plain-http`，使用自签名证书时加 `--insecure`。
- 认证、镜像加速和短名沿用 `docker login` 写入的凭据和 `registries.conf`。

## 离线导入

离线环境使用 OCI 格式的镜像包：

```bash
atoll import zot.tar containerd.tar kubernetes.tar
```

下一步：[创建集群](030-create-cluster.md)。
