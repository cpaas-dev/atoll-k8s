# 升级容器运行时

```bash
atoll pull ghcr.io/cpaas-dev/atoll/containerd:v2.2.0
atoll cri upgrade ghcr.io/cpaas-dev/atoll/containerd:v2.2.0 --hosts 10.0.0.4
```

不传 `--hosts` 时，升级所有当前使用同一镜像仓库名的主机。例如上面的镜像会升级所有正在使用 `ghcr.io/cpaas-dev/atoll/containerd` 的主机，使用 docker 镜像的主机不受影响。

## 执行过程

atoll 逐台主机执行：

1. 主机已加入集群时，先 drain 节点。
2. 用新镜像替换主机上的 `container-runtime/` 目录，执行新镜像的 `scripts/init.sh`，原地升级，保留容器运行时的数据目录。
3. uncordon 节点，在 Clusterfile 中记录新镜像。

一台主机失败时立即停止，不再处理后面的主机。

镜像需要支持原地升级，要求见 [生命周期脚本](../040-image-spec/030-scripts.md)。
