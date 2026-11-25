# 使用外部仓库

已有 Harbor 或 Nexus 时，可以不部署 zot，直接让集群使用外部仓库。外部仓库在 `atoll create` 之后、`atoll cri install` 之前登记，并且清单里不能有 registry 角色。

## Harbor

由 atoll 推送镜像：

```bash
atoll registry external --address harbor.example.com --prefix library \
  --username 'robot$atoll' --password xxx --push
```

- `--prefix` 是路径前缀，对应 Harbor 的项目。项目需要事先建好，账号需要推送权限。
- 部署 Kubernetes 或容器运行时镜像时，atoll 把镜像里 `registry/` 目录下的镜像推送到 Harbor，与对待 zot 的方式相同。

## Nexus

镜像由 Nexus 代理上游或者事先同步，atoll 不推送：

```bash
atoll registry external --address nexus.example.com:8082 --username atoll --password xxx
```

Nexus 提供的镜像也要遵循 [镜像路径规则](../030-design/040-registry.md#镜像路径规则)。

## 参数

| 参数 | 说明 |
| --- | --- |
| `--address` | 镜像引用中使用的仓库地址，`host[:port]` |
| `--prefix` | 路径前缀，例如 Harbor 的项目名，可以为空 |
| `--username`、`--password` | 仓库账号 |
| `--push` | 由 atoll 推送镜像，Harbor 使用 |
| `--plain-http` | 仓库只提供 HTTP |
| `--insecure` | 接受任意 TLS 证书，用于自签名证书 |

参数保存在 Clusterfile 的 `spec.registry.external`：

```yaml
spec:
  registry:
    external:
      address: harbor.example.com
      prefix: library
      username: robot$atoll
      password: xxx
      push: true
      plainHTTP: false
      insecure: false
```

集群内的镜像名按 `<地址>/<前缀>/<原镜像完整名称>` 改写，例如 `registry.k8s.io/pause:3.9` 改写为 `harbor.example.com/library/registry.k8s.io/pause:3.9`。
