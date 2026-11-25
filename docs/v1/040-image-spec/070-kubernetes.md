# Kubernetes 镜像

Kubernetes 镜像提供 kubeadm、kubelet、kubectl、控制面组件镜像和 lvscare。不同主机可以使用不同版本的 Kubernetes 镜像，版本规则见 [升级 Kubernetes](../020-guides/060-upgrade-kubernetes.md#版本规则)。

## 内容

```text
/cpaas/
├── bin/
│   ├── kubeadm
│   ├── kubelet
│   └── kubectl
├── etc/
│   ├── kubeadm.yml.tmpl          可选，kubeadm 默认配置
│   ├── kubelet.service
│   └── 10-kubeadm.conf
├── statics/
│   └── audit-policy.yml          复制到 master 的 /etc/kubernetes
├── scripts/
│   ├── check.sh
│   ├── init.sh
│   └── clean.sh
└── registry/                     控制面组件镜像和 lvscare
```

`registry/` 的格式和生成方法见 [registry 目录](040-registry-dir.md)。

## init.sh

- 完成系统准备：关闭 swap，加载 `br_netfilter`、`ip_vs` 等内核模块，设置 sysctl。
- 安装 `bin/` 下的二进制和 kubelet 的 systemd 服务，设置 kubelet 开机启动。
- 不执行 kubeadm。`kubeadm init`、`join`、`upgrade` 和 `reset` 都由 atoll 执行；升级时 atoll 直接使用新镜像里的 `bin/kubeadm` 和 `bin/kubelet`。

## clean.sh

在 atoll 执行 `kubeadm reset` 之后运行，删除 kubelet 服务、二进制、`/etc/kubernetes`、`/var/lib/kubelet` 和 `/var/lib/etcd`。

## kubeadm 默认配置

- `etc/kubeadm.yml` 提供镜像自带的默认值，合并顺序见 [自定义 kubeadm 配置](../020-guides/010-kubeadm-config.md#合并顺序)。
- `imageRepository` 保持上游名称，例如 `registry.k8s.io`。集群内的镜像改写由 image-cri-shim 完成。
- 由 atoll 按主机填写的字段不需要写在这个文件里。
