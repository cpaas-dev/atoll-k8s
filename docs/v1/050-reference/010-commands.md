# 命令速查

| 命令 | 作用 | 默认目标主机 |
| --- | --- | --- |
| `atoll pull IMAGE...` | 拉取镜像到运维机本地镜像库 | 不涉及 |
| `atoll import ARCHIVE...` | 导入 OCI 镜像包 | 不涉及 |
| `atoll create` | 新建集群的主机清单 | 不涉及 |
| `atoll add` | 向清单添加主机 | 不涉及 |
| `atoll registry install IMAGE` | 安装 zot | 全部 master |
| `atoll registry external` | 登记外部仓库 | 不涉及 |
| `atoll registry upgrade IMAGE` | 升级 zot | 全部 registry 主机 |
| `atoll registry passwd` | 修改仓库密码 | 全部主机 |
| `atoll cri install IMAGE` | 安装容器运行时 | 尚无容器运行时的主机 |
| `atoll cri upgrade IMAGE` | 升级容器运行时 | 使用同一镜像仓库名的主机 |
| `atoll k8s config IMAGE` | 输出默认 kubeadm 配置 | 不涉及 |
| `atoll k8s init IMAGE` | 初始化控制面 | 全部 master |
| `atoll k8s join IMAGE` | 加入集群 | 已有容器运行时、尚未加入的主机 |
| `atoll k8s upgrade IMAGE` | 升级 Kubernetes | 全部主机，先 master 后工作节点 |
| `atoll delete --hosts` | 删除节点 | 必须指定 |
| `atoll status` | 查看集群和各主机组件 | 全部主机 |

所有集群命令都用 `--cluster` 指定集群，默认为 `default`。需要选择主机的命令用 `--hosts` 覆盖默认目标主机。

`atoll cert`、`atoll exec`、`atoll scp`、`atoll version` 的用法不变，见 `atoll <command> --help`。
