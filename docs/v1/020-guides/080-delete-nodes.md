# 删除节点

```bash
atoll delete --hosts 10.0.0.5
```

删除一步完成。执行前会要求确认，`--force` 跳过确认。

## 执行过程

对每台主机按安装的反向顺序清理：

1. 主机已加入集群时，先 drain，然后从集群中删除节点，执行 `kubeadm reset`；master 同时移除自己的 etcd 成员。
2. 执行 Kubernetes 镜像的 `scripts/clean.sh`，清理 lvscare 规则和 hosts 解析。
3. 执行容器运行时镜像的 `scripts/clean.sh`。
4. registry 主机执行 registry 镜像的 `scripts/clean.sh`。
5. 删除主机上的 `/var/lib/atoll/data/<cluster>/`，把主机从清单和状态中移除。

删除 master 后，atoll 同步各工作节点的 lvscare 后端列表。

## 限制

- 不能删除最后一个 master。
- 不能删除最后一个 registry 主机。
- atoll 不提供重置整个集群的命令。
