# 待定事项

以下问题还没有结论，v1 暂不支持：

- standalone 控制面模式（`controlPlaneMode: standalone`）是否保留。
- 删除 master0 时如何重新选择 master0，并更新其他 master 的 `apiserver.cluster.local` 解析。
- 外部仓库使用自签名 CA 时如何分发 CA 证书。v1 只提供 `insecure`。
- 创建之后增减 registry 主机。
