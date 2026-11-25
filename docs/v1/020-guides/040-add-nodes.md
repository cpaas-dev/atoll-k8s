# 新增节点

## 新增工作节点

```bash
atoll add --nodes 10.0.0.6 -p <ssh-password>
atoll cri install ghcr.io/cpaas-dev/atoll/containerd:v2.1.4 --hosts 10.0.0.6
atoll k8s join ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14 --hosts 10.0.0.6
```

1. `atoll add` 把主机加入清单，只做 SSH 和架构检查。SSH 参数与 [创建集群](../010-getting-started/030-create-cluster.md#1-建立主机清单) 相同。
2. `atoll cri install` 在新主机上安装容器运行时。
3. `atoll k8s join` 让新主机加入集群。

新主机使用的容器运行时镜像和 Kubernetes 镜像可以与其他主机不同，Kubernetes 版本需要满足 [版本规则](060-upgrade-kubernetes.md#版本规则)。

## 新增 master

把 `--nodes` 换成 `--masters`：

```bash
atoll add --masters 10.0.0.7 -p <ssh-password>
atoll cri install ghcr.io/cpaas-dev/atoll/containerd:v2.1.4 --hosts 10.0.0.7
atoll k8s join ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14 --hosts 10.0.0.7
```

新 master 以控制面身份加入，atoll 随后同步各工作节点的 lvscare 后端列表。

新增的主机不会自动成为 registry 主机。
