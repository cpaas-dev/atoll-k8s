# registry 镜像

registry 镜像在 registry 主机上安装 zot。

## 内容

```text
/cpaas/
├── bin/zot
├── etc/
│   ├── registry_config.json.tmpl
│   └── zot.service
└── scripts/
    ├── init.sh
    └── clean.sh
```

atoll 在执行 `init.sh` 之前，根据仓库账号密码生成 `etc/registry_htpasswd`，放进主机目录。

## init.sh

- 安装 `bin/zot`，并把配置文件和 htpasswd 安装到 `/etc/registry/registry_config.json` 和 `/etc/registry/registry_htpasswd`。`atoll registry passwd` 会更新这个 htpasswd。
- 以 systemd 服务 `zot` 运行，设置开机启动。`atoll registry passwd` 修改密码后会重启这个服务。
- 监听 `ATOLL_SYS_REGISTRY_PORT`，数据放在 `ATOLL_SYS_REGISTRY_DATA`。atoll 在 `init.sh` 结束后等待这个端口可以连接。
- 开启 dedupe，关闭 gc，开启 `docker2s2` 兼容，以接受 Docker schema2 manifest。

配置模板示例：

```json
{
  "storage": {
    "rootDirectory": "{{ .ATOLL_SYS_REGISTRY_DATA }}",
    "dedupe": true,
    "gc": false
  },
  "http": {
    "address": "0.0.0.0",
    "port": "{{ .ATOLL_SYS_REGISTRY_PORT }}",
    "compat": ["docker2s2"],
    "auth": {
      "htpasswd": {
        "path": "/etc/registry/registry_htpasswd"
      }
    }
  },
  "log": {
    "level": "info"
  }
}
```

## clean.sh

停止并删除 `zot` 服务，删除 `/etc/registry` 和数据目录。
