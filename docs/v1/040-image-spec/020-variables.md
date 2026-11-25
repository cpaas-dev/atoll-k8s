# 环境变量与模板

## 变量来源

脚本和模板能用到的变量来自以下来源，后面的覆盖前面的：

1. 镜像的 `ENV`，作为默认值。
2. Clusterfile 的 `spec.env`。
3. Clusterfile 中对应主机的 `hosts[].env`。
4. atoll 提供的 `ATOLL_SYS_*` 变量。用户在 Clusterfile 里设置的 `ATOLL_SYS_*` 会被忽略。

## atoll 提供的变量

| 变量 | 提供给 | 说明 | 示例 |
| --- | --- | --- | --- |
| `ATOLL_SYS_ATOLL_VERSION` | 全部 | atoll 版本 | `v1.0.0` |
| `ATOLL_SYS_CLUSTER_NAME` | 全部 | 集群名 | `default` |
| `ATOLL_SYS_HOST_IP` | 全部 | 当前主机 IP | `10.0.0.4` |
| `ATOLL_SYS_RUN_MODE` | 全部 | 当前主机的角色，逗号分隔 | `master,registry,amd64` |
| `ATOLL_SYS_REGISTRY_ADDRESS` | 全部 | 镜像引用中的仓库地址 | `atoll.hub:5000` |
| `ATOLL_SYS_REGISTRY_PREFIX` | 全部 | 仓库路径前缀，zot 为空 | `library` |
| `ATOLL_SYS_REGISTRY_ENDPOINTS` | 全部 | 实际访问端点，逗号分隔，按优先级排序 | `http://10.0.0.1:5000,http://10.0.0.2:5000` |
| `ATOLL_SYS_REGISTRY_USERNAME` | 全部 | 仓库账号 | `admin` |
| `ATOLL_SYS_REGISTRY_PASSWORD` | 全部 | 仓库密码 | `passw0rd` |
| `ATOLL_SYS_REGISTRY_INSECURE` | 全部 | 是否跳过 TLS 证书校验 | `false` |
| `ATOLL_SYS_REGISTRY_PORT` | 全部，仅 zot | zot 监听端口 | `5000` |
| `ATOLL_SYS_REGISTRY_DATA` | 全部，仅 zot | zot 数据目录 | `/var/lib/registry` |
| `ATOLL_SYS_KUBE_VERSION` | `kubernetes` | 镜像的 Kubernetes 版本 | `v1.30.14` |
| `ATOLL_SYS_IMAGE_ENDPOINT` | `kubernetes` | 本机容器运行时镜像声明的镜像服务端点 | `unix:///var/run/image-cri-shim.sock` |

## 模板

- `etc/`、`scripts/`、`manifests/` 下以 `.tmpl` 结尾的文件是模板。atoll 在每台主机上用该主机的变量渲染，输出到去掉 `.tmpl` 后缀的同名文件，并删除模板文件。
- 模板使用 Go `text/template` 语法，用 `{{ .变量名 }}` 引用变量，例如 `{{ .ATOLL_SYS_REGISTRY_ADDRESS }}`。
- 未定义的变量会被渲染成 `<no value>`。需要默认值时，在镜像的 `ENV` 里给出。
