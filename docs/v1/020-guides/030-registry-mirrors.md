# 配置上游镜像加速

集群内拉取镜像时，image-cri-shim 先把镜像名改写到集群仓库。集群仓库里没有的镜像，例如之后自行部署的 CNI 和应用镜像，containerd 会直接访问原仓库。原仓库访问慢或者无法访问时，可以在每台主机上为 containerd 配置上游镜像加速。

!!! note "适用范围"

    本页只适用于使用 containerd 的主机。

## 镜像站

| 原仓库 | 镜像站，按优先级排列 | 验证用的镜像 |
| --- | --- | --- |
| `docker.io` | `docker.linkos.org`、`docker.m.daocloud.io`、`docker.1ms.run` | `library/python:3.13` |
| `gcr.io` | `gcr.linkos.org` | `distroless/static:nonroot` |
| `ghcr.io` | `ghcr.linkos.org`、`ghcr.1ms.run` | `containerd/busybox:1.36` |
| `nvcr.io` | `nvcr.nju.edu.cn` | `nvidia/gpu-operator:v26.3.0` |
| `quay.io` | `quay.linkos.org` | `coreos/etcd:v3.5.0` |
| `registry.k8s.io` | `k8s.linkos.org` | `pause:3.10` |
| `mcr.microsoft.com` | `mcr.1ms.run` | `playwright/mcp:v0.0.86` |

这些都是第三方镜像站，可用性随时可能变化，使用前先 [验证](#验证)。

## 配置

containerd 从 `/etc/containerd/certs.d/<原仓库>/hosts.toml` 读取每个原仓库的访问端点：按顺序尝试各个镜像站，都不可用时再访问 `server` 指定的原仓库。atoll 的容器运行时镜像已经把 containerd 的 `config_path` 设为 `/etc/containerd/certs.d`，新增或修改 `hosts.toml` 后不需要重启 containerd。

在每台主机上执行需要的原仓库对应的命令：

=== "docker.io"

    ```bash
    mkdir -p /etc/containerd/certs.d/docker.io
    cat <<'EOF' >/etc/containerd/certs.d/docker.io/hosts.toml
    server = "https://registry-1.docker.io"

    [host."https://docker.linkos.org"]
      capabilities = ["pull", "resolve"]
    [host."https://docker.m.daocloud.io"]
      capabilities = ["pull", "resolve"]
    [host."https://docker.1ms.run"]
      capabilities = ["pull", "resolve"]
    EOF
    ```

=== "gcr.io"

    ```bash
    mkdir -p /etc/containerd/certs.d/gcr.io
    cat <<'EOF' >/etc/containerd/certs.d/gcr.io/hosts.toml
    server = "https://gcr.io"

    [host."https://gcr.linkos.org"]
      capabilities = ["pull", "resolve"]
    EOF
    ```

=== "ghcr.io"

    ```bash
    mkdir -p /etc/containerd/certs.d/ghcr.io
    cat <<'EOF' >/etc/containerd/certs.d/ghcr.io/hosts.toml
    server = "https://ghcr.io"

    [host."https://ghcr.linkos.org"]
      capabilities = ["pull", "resolve"]
    [host."https://ghcr.1ms.run"]
      capabilities = ["pull", "resolve"]
    EOF
    ```

=== "nvcr.io"

    ```bash
    mkdir -p /etc/containerd/certs.d/nvcr.io
    cat <<'EOF' >/etc/containerd/certs.d/nvcr.io/hosts.toml
    server = "https://nvcr.io"

    [host."https://nvcr.nju.edu.cn"]
      capabilities = ["pull", "resolve"]
    EOF
    ```

=== "quay.io"

    ```bash
    mkdir -p /etc/containerd/certs.d/quay.io
    cat <<'EOF' >/etc/containerd/certs.d/quay.io/hosts.toml
    server = "https://quay.io"

    [host."https://quay.linkos.org"]
      capabilities = ["pull", "resolve"]
    EOF
    ```

=== "registry.k8s.io"

    ```bash
    mkdir -p /etc/containerd/certs.d/registry.k8s.io
    cat <<'EOF' >/etc/containerd/certs.d/registry.k8s.io/hosts.toml
    server = "https://registry.k8s.io"

    [host."https://k8s.linkos.org"]
      capabilities = ["pull", "resolve"]
    EOF
    ```

=== "mcr.microsoft.com"

    ```bash
    mkdir -p /etc/containerd/certs.d/mcr.microsoft.com
    cat <<'EOF' >/etc/containerd/certs.d/mcr.microsoft.com/hosts.toml
    server = "https://mcr.microsoft.com"

    [host."https://mcr.1ms.run"]
      capabilities = ["pull", "resolve"]
    EOF
    ```

这些目录与集群仓库的目录互不影响。`atoll cri upgrade` 不会改动它们；`atoll delete` 删除主机时，会随容器运行时一起清理。

## 验证

检查某个镜像站是否可用，直接从镜像站拉取表中的验证镜像：

```bash
ctr -n k8s.io images pull docker.linkos.org/library/python:3.13
```

检查配置是否生效，用原仓库的镜像名，按 `hosts.toml` 拉取：

```bash
ctr -n k8s.io images pull --hosts-dir /etc/containerd/certs.d docker.io/library/python:3.13
```
