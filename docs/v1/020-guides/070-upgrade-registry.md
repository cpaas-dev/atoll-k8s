# 升级镜像仓库

本页只适用于 atoll 部署的 zot。外部仓库由用户自行维护。

## 升级 zot

```bash
atoll pull ghcr.io/cpaas-dev/atoll/zot:v2.2.0
atoll registry upgrade ghcr.io/cpaas-dev/atoll/zot:v2.2.0
```

atoll 逐台 registry 主机执行新镜像的 `scripts/init.sh`。zot 的数据目录保留不动，一台主机升级期间，其他 registry 主机继续提供服务。

## 修改仓库密码

```bash
atoll registry passwd
```

修改 zot 的账号密码：更新 registry 主机上的 htpasswd 并重启 zot，同时把新凭据同步到每台主机的容器运行时配置。
