# 校验规则

atoll 在每个命令开始时检查以下规则，不满足时直接报错，不修改集群。

| 时机 | 规则 |
| --- | --- |
| 读取镜像 | `atoll.io.type` 必须是三种类型之一；`atoll.io.version` 必须是 `v1`；镜像必须有目标主机架构的平台 |
| `create` / `add` | 主机可以通过 SSH 访问；主机名在集群内唯一；一台主机只能是 master 和 node 之一 |
| `registry install` | 目标主机都在清单内；尚未配置仓库 |
| `registry external` | 清单中没有 registry 角色；尚未配置仓库 |
| `cri install` | 已配置仓库；目标主机尚未安装容器运行时 |
| `k8s init` | 所有 master 都已安装容器运行时；集群尚未初始化 |
| `k8s join` | 集群已初始化；目标主机已安装容器运行时、尚未加入集群 |
| `k8s upgrade` | 只升一个小版本；先 master 后工作节点；工作节点与控制面的版本差不超过 3 个小版本 |
| `delete` | 不删除最后一个 master 和最后一个 registry 主机 |
