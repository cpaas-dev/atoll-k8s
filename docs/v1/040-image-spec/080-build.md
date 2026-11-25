# 构建与发布

## 构建示例

以 Kubernetes 镜像为例：

```dockerfile
FROM scratch
ARG TARGETARCH
LABEL atoll.io.type="kubernetes" \
      atoll.io.version="v1" \
      atoll.io.kubernetes-version="v1.30.14" \
      atoll.io.lvscare-image="ghcr.io/cpaas-dev/atoll/lvscare:v1.0.0"
COPY bin/${TARGETARCH}/ /cpaas/bin/
COPY etc/ /cpaas/etc/
COPY statics/ /cpaas/statics/
COPY scripts/ /cpaas/scripts/
COPY registry/ /cpaas/registry/
```

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/cpaas-dev/atoll/kubernetes:v1.30.14 --push .
```

另外两类镜像的 Dockerfile 结构相同，替换标签和 `COPY` 的内容即可。标签见 [通用约定](010-conventions.md#标签)。

## 发布前检查

- [ ] 所有内容都在 `/cpaas` 下，没有指向 `/cpaas` 之外的链接。
- [ ] 必填标签齐全，`atoll.io.version` 为 `v1`。
- [ ] `scripts/init.sh` 和 `scripts/clean.sh` 存在，`init.sh` 重复执行结果相同。
- [ ] 模板里引用的变量都有来源：镜像 `ENV` 或 `ATOLL_SYS_*`。
- [ ] `registry/` 的仓库路径包含原仓库域名，用 `skopeo copy --all` 生成。
- [ ] Kubernetes 镜像：`atoll.io.kubernetes-version` 与 `bin/kubeadm version` 一致，`atoll.io.lvscare-image` 在 `registry/` 里。
- [ ] 容器运行时镜像：`atoll.io.image-endpoint` 与 image-cri-shim 的监听 socket 一致。
