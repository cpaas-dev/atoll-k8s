# registry 目录

需要推送到集群仓库的镜像放在镜像的 `/cpaas/registry/` 下。Kubernetes 镜像用它携带控制面组件镜像；容器运行时镜像需要时也可以携带。推送的时机和目标见 [推送镜像](../030-design/040-registry.md#推送镜像)。

## 格式

- 每个仓库是一个 OCI image layout，目录路径就是原镜像的完整名称，包含原仓库域名。Docker Hub 的短名写成 `docker.io/library/<name>`。
- tag 记录在 `index.json` 的 `org.opencontainers.image.ref.name` 注解中。manifest 按原样推送，digest 保持不变，Docker schema2 manifest 和 manifest list 也可以。
- 不接受 distribution 的旧存储格式（`docker/registry/v2`）。

```text
registry/
├── registry.k8s.io/
│   ├── pause/                    oci-layout、index.json、blobs/sha256/...
│   ├── kube-apiserver/
│   ├── kube-controller-manager/
│   ├── kube-scheduler/
│   ├── kube-proxy/
│   ├── etcd/
│   └── coredns/coredns/
└── ghcr.io/cpaas-dev/atoll/lvscare/
```

## 生成

用 skopeo 生成：

```bash
for image in $(kubeadm config images list --kubernetes-version v1.30.14) \
             ghcr.io/cpaas-dev/atoll/lvscare:v1.0.0; do
  name=${image%:*}
  tag=${image##*:}
  skopeo copy --all "docker://${image}" "oci:registry/${name}:${tag}"
done
```

!!! warning "使用 `--all`"

    `--all` 保留多架构 index，这样每个平台的镜像都带着完全相同的 `registry/`。混合架构的集群必须这样做：不同平台的镜像推送同一个 tag 时 digest 不一致，atoll 会报 tag 冲突。
