# Clusterfile

Clusterfile 保存在运维机的 `~/.atoll/<cluster>/Clusterfile`，分为两部分：

- `spec`：清单和配置，包括主机、角色、SSH、环境变量、仓库配置和 Kubernetes 集群级配置。用户可以修改 `env`，以及末尾附加的 kubeadm 配置；其他字段由命令维护。
- `status`：每台主机实际安装的组件，记录镜像名和 manifest digest，只由 atoll 命令写入。

## 示例

```yaml
apiVersion: apps.atoll.io/v1beta1
kind: Cluster
metadata:
  name: default
spec:
  ssh:
    user: root
    passwd: xxx
    port: 22
  env:
  - TZ=Asia/Shanghai
  hosts:
  - ips: [10.0.0.1:22, 10.0.0.2:22, 10.0.0.3:22]
    roles: [master, registry, amd64]
  - ips: [10.0.0.4:22, 10.0.0.5:22]
    roles: [node, amd64]
  - ips: [10.0.0.6:22]
    roles: [node, arm64]
    env:
    - DATA_DISK=/data
  registry:
    zot:
      domain: atoll.hub
      port: 5000
      username: admin
      password: passw0rd
      data: /var/lib/registry
  kubernetes:
    vip: 10.103.97.2
status:
  hosts:
  - host: 10.0.0.1:22
    registry:
      image: ghcr.io/cpaas-dev/atoll/zot:v2.1.8
      digest: sha256:...
    containerRuntime:
      image: ghcr.io/cpaas-dev/atoll/containerd:v2.1.4
      digest: sha256:...
    kubernetes:
      image: ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14
      digest: sha256:...
  - host: 10.0.0.6:22
    containerRuntime:
      image: ghcr.io/cpaas-dev/atoll/docker:v28.3
      digest: sha256:...
    kubernetes:
      image: ghcr.io/cpaas-dev/atoll/kubernetes:v1.29.15
      digest: sha256:...
---
# Optional kubeadm overrides, merged last.
apiVersion: kubeadm.k8s.io/v1beta3
kind: ClusterConfiguration
networking:
  podSubnet: 100.64.0.0/10
```

## 字段说明

| 字段 | 说明 |
| --- | --- |
| `spec.hosts[].roles` | 主机角色，见 [主机清单与角色](020-hosts.md#主机清单与角色) |
| `spec.env`、`spec.hosts[].env` | 传给镜像脚本和模板的变量，主机级覆盖集群级，见 [环境变量与模板](../040-image-spec/020-variables.md) |
| `spec.registry` | 只能有 `zot` 和 `external` 中的一个，见 [镜像仓库](040-registry.md) |
| `spec.kubernetes.vip` | 工作节点通过 lvscare 访问 API Server 的虚拟 IP，在 `atoll k8s init` 时确定 |
| `status.hosts[]` | 每台主机安装的 registry、容器运行时和 Kubernetes 镜像 |
| 末尾的 kubeadm 文档 | 覆盖 kubeadm 配置，见 [自定义 kubeadm 配置](../020-guides/010-kubeadm-config.md) |
