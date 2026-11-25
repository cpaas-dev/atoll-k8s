# 容器运行时镜像

容器运行时镜像安装 CRI 和 image-cri-shim。每台主机只装一个，同一集群可以混用不同的容器运行时镜像。

## 内容

以 containerd 为例：

```text
/cpaas/
├── bin/
│   ├── containerd
│   ├── containerd-shim-runc-v2
│   ├── ctr
│   ├── runc
│   ├── crun                      可选
│   ├── runsc                     可选，gVisor
│   ├── crictl
│   └── image-cri-shim
├── etc/
│   ├── containerd/config.toml.tmpl
│   ├── containerd/hosts.toml.tmpl
│   ├── image-cri-shim.yaml.tmpl
│   ├── containerd.service
│   └── image-cri-shim.service
└── scripts/
    ├── check.sh
    ├── init.sh
    └── clean.sh
```

## init.sh

- 安装二进制和 systemd 服务，启动 containerd 和 image-cri-shim，并设置开机启动。
- 使用 systemd cgroup driver。atoll 在安装完成后，用 atollctl 在主机上检测 CRI socket 和 cgroup driver，写入 kubelet 配置。
- 把 CRI socket 放在标准位置，以便被检测到：containerd 为 `/run/containerd/containerd.sock`，docker 通过 cri-dockerd 提供 `/var/run/cri-dockerd.sock`。
- 升级时原地替换二进制和配置并重启服务，不删除 `/var/lib/containerd`。

### 仓库访问

- containerd 的 `config_path` 设为 `/etc/containerd/certs.d`。
- 按 `ATOLL_SYS_REGISTRY_ENDPOINTS` 的顺序，为 `ATOLL_SYS_REGISTRY_ADDRESS` 写入 `/etc/containerd/certs.d/<address>/hosts.toml`。前面的端点不可用时，containerd 会尝试下一个。
- 只管理集群仓库对应的目录，不删除 `certs.d` 下的其他目录，以保留用户配置的 [上游镜像加速](../020-guides/030-registry-mirrors.md)。
- 配置仓库账号密码。端点是 HTTP，或者 `ATOLL_SYS_REGISTRY_INSECURE` 为 `true` 时，跳过 TLS 校验。

### image-cri-shim

- 转发到本机 CRI socket。
- 按 `<ATOLL_SYS_REGISTRY_ADDRESS>/<ATOLL_SYS_REGISTRY_PREFIX>/<原镜像完整名称>` 改写镜像名，规则见 [镜像路径规则](../030-design/040-registry.md#镜像路径规则)。
- 使用仓库账号密码。
- 监听标签 `atoll.io.image-endpoint` 声明的 socket。

### 其他运行时

crun、runsc、sysbox 等低层运行时注册为 containerd 的 runtime handler。atoll 不创建 RuntimeClass，需要时由用户创建。

## clean.sh

停止并删除 containerd、image-cri-shim 的服务，删除二进制、配置和 `/var/lib/containerd`。

## docker

docker 镜像的约定相同，内容换成 dockerd、cri-dockerd 和 image-cri-shim：image-cri-shim 转发到 cri-dockerd，仓库端点写入 docker 的配置。
