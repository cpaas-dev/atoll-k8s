# 升级 Kubernetes

```bash
atoll pull ghcr.io/cpaas-dev/atoll/kubernetes:v1.31.10
atoll k8s upgrade ghcr.io/cpaas-dev/atoll/kubernetes:v1.31.10
```

- 不传 `--hosts` 时，先升级全部 master，再升级全部工作节点。
- 也可以分批升级：先升级所有 master，再用 `--hosts` 分批升级工作节点。分批期间，工作节点可以停留在旧版本。

## 版本规则

atoll 遵循 kubeadm 和 kubelet 的版本规则，不满足时拒绝执行：

- 每次只能升级一个小版本，例如从 v1.30 升到 v1.31。
- 先升级 master，再升级工作节点。
- 工作节点的版本不能高于控制面，也不能比控制面低超过 3 个小版本。

## 执行过程

升级前，atoll 先把新镜像里的组件镜像推送到仓库。然后逐台主机执行：

1. drain 节点。
2. 用新镜像替换主机上的 `kubernetes/` 目录，执行 `scripts/init.sh`。
3. 用新镜像的 `bin/kubeadm` 执行升级：第一个升级的 master 执行 `kubeadm upgrade apply`，其余 master 和工作节点执行 `kubeadm upgrade node`。然后替换 kubelet 并重启。
4. uncordon 节点，在 Clusterfile 中记录新镜像。
