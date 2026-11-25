# 创建集群

下面的示例创建一个 3 master、2 worker 的集群。集群名默认为 `default`，可用 `--cluster` 指定其他名称。各步骤必须按顺序执行，原因见 [安装顺序](../030-design/010-architecture.md#安装顺序)。

## 1. 建立主机清单

```bash
atoll create --masters 10.0.0.1-10.0.0.3 --nodes 10.0.0.4,10.0.0.5 -p <ssh-password>
```

- 主机地址可以写成逗号分隔的列表，也可以写成 `10.0.0.1-10.0.0.3` 这样的范围；SSH 端口不是 22 时写成 `10.0.0.1:2222`。
- SSH 参数：`-u` 用户名（默认 root）、`-p` 密码、`--pk` 私钥文件、`--pk-passwd` 私钥密码、`--port` 默认端口。
- 这一步只检查 SSH 连通性和主机名、探测主机架构，然后生成 `~/.atoll/default/Clusterfile`，不在主机上安装任何东西。

## 2. 部署镜像仓库

```bash
atoll registry install ghcr.io/cpaas-dev/atoll/zot:v2.1.8
```

- 默认安装在全部 master 上。用 `--hosts 10.0.0.1,10.0.0.2` 可以指定其他主机，这些主机必须在清单内。
- 可选参数：`--domain`（默认 `atoll.hub`）、`--port`（默认 `5000`）、`--username`、`--password`、`--data`（默认 `/var/lib/registry`）。

如果使用已有的 Harbor 或 Nexus，就不安装 zot，改为 [使用外部仓库](../020-guides/020-external-registry.md)。

## 3. 安装容器运行时

```bash
atoll cri install ghcr.io/cpaas-dev/atoll/containerd:v2.1.4
```

默认安装到所有还没有容器运行时的主机，同时在这些主机上写入仓库域名的 hosts 解析。混用多种运行时时，用 `--hosts` 分批安装：

```bash
atoll cri install ghcr.io/cpaas-dev/atoll/containerd:v2.1.4 --hosts 10.0.0.1-10.0.0.4
atoll cri install ghcr.io/cpaas-dev/atoll/docker:v28.3 --hosts 10.0.0.5
```

## 4. 初始化 Kubernetes

需要修改 Pod 网段等 kubeadm 配置时，先按 [自定义 kubeadm 配置](../020-guides/010-kubeadm-config.md) 修改 Clusterfile。然后初始化控制面，再让工作节点加入：

```bash
atoll k8s init ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14
atoll k8s join ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14
```

- `k8s init` 先把镜像里的组件镜像推送到仓库，在 master0 上执行 `kubeadm init`，再让其余 master 以控制面身份加入。
- `--vip` 指定工作节点访问 API Server 的虚拟 IP，默认 `10.103.97.2`。
- `k8s join` 默认处理所有已安装容器运行时、尚未加入集群的主机，也可以用 `--hosts` 指定。

??? note "每台主机上做了什么"

    1. 执行 Kubernetes 镜像的 `scripts/check.sh` 和 `scripts/init.sh`，完成系统准备，安装 kubelet、kubeadm、kubectl。
    2. 生成 kubeadm 配置，执行 `kubeadm init` 或 `kubeadm join`。
    3. master 写入 `apiserver.cluster.local` 解析，并把 `statics/` 下的审计策略等文件复制到 `/etc/kubernetes`。
    4. 工作节点写入 lvscare 的 IPVS 规则和静态 Pod，通过 VIP 访问各 master 上的 API Server。

## 5. 部署 CNI

!!! warning "atoll 不部署 CNI"

    集群初始化后，节点在 CNI 就绪前保持 NotReady。用你们自己的方式部署 CNI，Pod 网段与 kubeadm 配置中的 `podSubnet` 保持一致。

## 6. 查看状态

```bash
atoll status
```

`status` 列出每台主机的角色，以及当前安装的 registry、容器运行时、Kubernetes 镜像和节点状态。
