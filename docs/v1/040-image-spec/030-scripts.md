# 生命周期脚本

每类镜像都用 `scripts/` 下固定名称的脚本完成安装和清理：

| 脚本 | 必须 | 执行时机 |
| --- | --- | --- |
| `scripts/check.sh` | 否 | 每次执行 `init.sh` 之前，用于前置检查 |
| `scripts/init.sh` | 是 | 首次安装和升级 |
| `scripts/clean.sh` | 是 | 删除主机时 |

## 执行方式

- atoll 以 root 身份执行 `cd <主机目录>/scripts && bash <脚本>`，所有变量都以环境变量导出，变量见 [环境变量与模板](020-variables.md)。
- 脚本退出码非 0 时，atoll 立即停止当前命令并报错。

## init.sh

- 必须幂等，既用于首次安装，也用于原地升级。
- 升级时，atoll 先用新镜像替换主机目录，再执行新镜像的 `init.sh`；旧版本的 `clean.sh` 不会执行。因此 `init.sh` 要能覆盖旧版本的安装结果，并保留数据目录。

## clean.sh

- 彻底清理本镜像安装的内容，包括服务、配置、二进制和数据目录。
- 删除主机时，三类镜像的 `clean.sh` 按 Kubernetes、容器运行时、registry 的顺序执行，见 [删除节点](../020-guides/080-delete-nodes.md)。
