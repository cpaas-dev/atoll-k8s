# 安装 atoll

atoll 安装在运维机上。运维机可以是集群主机之一，也可以在集群之外，要求见 [运维机与集群主机](../030-design/020-hosts.md)。

```bash
curl -sfL https://raw.githubusercontent.com/cpaas-dev/atoll-k8s/main/scripts/install.sh \
  | sh -s <version> cpaas-dev/atoll-k8s
```

安装脚本会把 `atoll` 和 `atollctl` 放到同一个目录。atollctl 是节点侧工具，由 atoll 在执行步骤前复制到集群主机上。

集群里有与运维机架构不同的主机时，需要把对应架构的 atollctl 放到同一目录，命名为 `atollctl-linux-<arch>`，例如 `atollctl-linux-arm64`。

下一步：[准备镜像](020-prepare-images.md)。
