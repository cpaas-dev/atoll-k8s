# 自定义 kubeadm 配置

需要修改 Pod 网段、Service 网段等 kubeadm 配置时，在 `atoll k8s init` 之前修改 Clusterfile。

## 步骤

1. 导出 Kubernetes 镜像的默认 kubeadm 配置，作为参考：

    ```bash
    atoll k8s config ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14
    ```

2. 把需要修改的 kubeadm 文档追加到 `~/.atoll/<cluster>/Clusterfile` 末尾，只保留要覆盖的字段：

    ```yaml
    ---
    apiVersion: kubeadm.k8s.io/v1beta3
    kind: ClusterConfiguration
    networking:
      podSubnet: 100.64.0.0/10
      serviceSubnet: 10.96.0.0/22
    ```

3. 执行 `atoll k8s init`。之后的 `k8s join` 和 `k8s upgrade` 都使用同一份配置。

## 合并顺序

kubeadm 配置按以下顺序合并，后者覆盖前者：

1. atoll 内置的默认值。
2. Kubernetes 镜像里的 `etc/kubeadm.yml`。
3. Clusterfile 里的 kubeadm 文档。

以下字段由 atoll 按每台主机填写，不需要手动配置：CRI socket、cgroup driver、镜像服务端点、advertise 地址、API Server 访问地址和 VIP。
