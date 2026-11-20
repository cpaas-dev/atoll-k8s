# image-cri-shim ConfigMap 配置功能详解

## 📋 功能概述

image-cri-shim 支持通过 Kubernetes ConfigMap 动态配置 registry 信息，实现**配置热更新**，无需重启服务即可应用新的 registry 配置。

---

## 🎯 核心功能

### 1. **动态配置同步**
- 自动从 Kubernetes ConfigMap 读取配置
- 定期同步到本地配置文件
- 无需重启 image-cri-shim 服务

### 2. **支持的字段**
ConfigMap 可以配置以下内容：
- ✅ atoll.hub 地址和认证信息
- ✅ atoll.hub 优先级（offlinePriority）
- ✅ 多个 registry 配置（包括优先级）
- ✅ Debug 模式开关
- ✅ 重载间隔（reloadInterval）
- ✅ gRPC 超时时间（timeout）
- ✅ 缓存配置（cache）

### 3. **完全支持优先级配置**（✅ 新增）
- ✅ **priority** - 每个 registry 的优先级
- ✅ **offlinePriority** - atoll.hub 的优先级

---

## 🔧 ConfigMap 配置规范

### ConfigMap 基本信息

```yaml
namespace: kube-system
name: image-cri-shim
dataKey: registries.yaml
```

### ConfigMap 数据结构（支持优先级）

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: image-cri-shim
  namespace: kube-system
data:
  registries.yaml: |
    version: v1
    offlinePriority: 1000  # atoll.hub 优先级（可选）

    atoll:
      address: http://atoll.hub:5000
      auth:
        username: admin
        password: passw0rd

    registries:
      - address: docker.io
        auth:
          username: dockeruser
          password: dockerpass
        priority: 600  # docker.io 优先级

      - address: registry.example.com
        auth:
          username: reguser
          password: regpass
        priority: 800  # registry.example.com 优先级

      - address: fast-registry.io
        auth:
          username: fastuser
          password: fastpass
        priority: 1200  # 高于 atoll.hub

    reloadInterval: 15s
    debug: false
    timeout: 15m

    cache:
      imageCacheSize: 1024
      imageCacheTTL: 30m
      domainCacheTTL: 10m
      statsLogInterval: 1m
      disableStats: false
```

---

## 📝 字段详解

### 1. atoll 配置

```yaml
atoll:
  address: http://atoll.hub:5000  # atoll.hub 地址
  auth:
    username: admin                # 认证用户名
    password: passw0rd             # 认证密码
```

**说明:**
- 配置 atoll.hub（离线 registry）的地址和认证信息
- 如果 ConfigMap 中未配置，会保留本地配置文件的值

### 2. registries 配置

```yaml
registries:
  - address: docker.io
    auth:
      username: user1
      password: pass1

  - address: registry.example.com
    auth:
      username: user2
      password: pass2
```

**说明:**
- 配置额外的 registry 列表
- 每个 registry 包含地址和认证信息
- **注意**: ConfigMap 中的 registry 配置**完全替换**本地配置，而不是合并

### 3. 全局配置

```yaml
reloadInterval: 15s   # 配置重载间隔
debug: false          # Debug 模式
timeout: 15m          # gRPC 超时时间
force: false          # 强制模式
```

### 4. cache 配置

```yaml
cache:
  imageCacheSize: 1024        # 镜像缓存大小
  imageCacheTTL: 30m          # 镜像缓存过期时间
  domainCacheTTL: 10m         # 域名缓存过期时间
  statsLogInterval: 1m        # 统计日志输出间隔
  disableStats: false         # 禁用统计
```

---

## 🔄 工作流程

### 1. 启动时同步

```
image-cri-shim 启动
    ↓
读取本地配置文件
    ↓
调用 SyncConfigFromConfigMap()
    ↓
连接 Kubernetes API
    ↓
读取 kube-system/image-cri-shim ConfigMap
    ↓
合并配置到本地文件
    ↓
继续正常启动流程
```

### 2. 定期同步

```
每个 reloadInterval (默认 15 秒)
    ↓
调用 SyncConfigFromConfigMap()
    ↓
检查 ConfigMap 是否存在
    ↓
如果存在，读取并合并配置
    ↓
如果配置有变化，更新本地文件
    ↓
等待下一个周期
```

### 3. 配置合并策略

ConfigMap 配置与本地配置文件的**合并策略**:

| 配置项 | ConfigMap 未设置 | ConfigMap 已设置 |
|--------|-----------------|-----------------|
| atoll.address | 保留本地值 | 使用 ConfigMap 值 |
| atoll.auth | 保留本地值 | 使用 ConfigMap 值 |
| registries | 保留本地值 | **替换**为 ConfigMap 值 |
| debug | 保留本地值 | 使用 ConfigMap 值 |
| timeout | 保留本地值 | 使用 ConfigMap 值 |
| reloadInterval | 保留本地值 | 使用 ConfigMap 值 |
| cache.* | 保留本地值 | 使用 ConfigMap 值 |

**注意**: `registries` 是完全替换，不是合并！

---

## 🚀 使用场景

### 场景 1: 集群管理员统一配置

**需求**: 集群管理员希望统一管理所有节点的 registry 配置

**解决方案**:
```bash
# 1. 创建 ConfigMap
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: image-cri-shim
  namespace: kube-system
data:
  registries.yaml: |
    version: v1
    atoll:
      address: http://atoll.hub:5000
      auth:
        username: admin
        password: passw0rd
    registries:
      - address: docker.io
        auth:
          username: dockeruser
          password: dockerpass
EOF

# 2. 等待最多 15 秒，配置自动同步到所有节点
# 3. 验证配置生效
kubectl logs -n kube-system daemonset/image-cri-shim | grep "synced image-cri-shim config"
```

### 场景 2: 动态更新 Registry

**需求**: 添加新的 private registry，无需重启节点

**解决方案**:
```bash
# 1. 更新 ConfigMap
kubectl patch configmap -n kube-system image-cri-shim --patch-file=/tmp/new-registries.yaml

# 2. 等待最多 15 秒，配置自动重载
# 3. 查看日志确认
journalctl -u image-cri-shim -f | grep "reloaded shim auth configuration"
```

### 场景 3: 临时启用 Debug 模式

**需求**: 临时开启 debug 日志进行问题排查

**解决方案**:
```yaml
# ConfigMap 中设置
debug: true
```

---

## ⚠️ 限制和注意事项

### 1. **Priority 字段完全支持** ✅

**状态**: ConfigMap 配置中**完全支持** priority 和 offlinePriority 字段

**支持的优先级配置**:
- ✅ **priority** - 每个 registry 的优先级 (0-10000)
- ✅ **offlinePriority** - atoll.hub 的优先级 (0-10000)

**验证方法**:
```bash
# 1. 查看 ConfigMap
kubectl get cm -n kube-system image-cri-shim -o yaml

# 2. 查看本地配置（已同步）
cat /etc/image-cri-shim.yaml

# 3. 查看日志确认同步
kubectl logs -n kube-system daemonset/image-cri-shim | grep "configmap:"
```

**期望日志**:
```
configmap: synced offlinePriority=1000
configmap: syncing registry docker.io with priority=600
configmap: merged 2 registries into config
synced image-cri-shim config from ConfigMap into /etc/image-cri-shim.yaml
```

### 2. **Registries 完全替换**

**问题**: ConfigMap 的 registries 配置会**完全替换**本地配置，不是合并

**影响**:
- 如果 ConfigMap 只配置了 1 个 registry，本地配置的其他 registry 会丢失
- 需要在 ConfigMap 中包含所有需要的 registry

**示例**:
```yaml
# 本地配置有 3 个 registries
registries:
  - address: docker.io
  - address: registry1.com
  - address: registry2.com

# ConfigMap 只配置了 1 个
data:
  registries.yaml: |
    registries:
      - address: docker.io  # 只有这个会被保留

# 结果：registry1.com 和 registry2.com 丢失
```

### 3. **ConfigMap 必须存在**

如果 ConfigMap 不存在：
- ✅ 不会报错
- ✅ image-cri-shim 正常运行
- ✅ 使用本地配置文件
- ⚠️ 不会自动创建 ConfigMap

### 4. **认证信息安全**

ConfigMap 中的认证信息是**明文存储**：
```yaml
auth:
  username: admin
  password: passw0rd  # 明文密码
```

**安全建议**:
- 使用 Kubernetes Secrets + RBAC
- 限制 ConfigMap 的访问权限
- 定期轮换密码

---

## 🔍 日志和调试

### 启动日志

```
config: processing 2 configured registries
config: registry[1] address=docker.io domain=index.docker.io priority=500 (default)
config: offline_registry address=http://atoll.hub:5000 domain=atoll.hub priority=1000 (default)
```

### ConfigMap 同步日志

```
synced image-cri-shim config from ConfigMap into /etc/image-cri-shim.yaml
syncing image-cri-shim config from ConfigMap completed
```

### 配置重载日志

```
reloaded shim auth configuration from /etc/image-cri-shim.yaml
```

### Debug 日志

如果看不到日志，检查:
```bash
# 1. ConfigMap 是否存在
kubectl get cm -n kube-system image-cri-shim

# 2. ConfigMap 内容是否正确
kubectl get cm -n kube-system image-cri-shim -o yaml

# 3. image-cri-shim 日志
kubectl logs -n kube-system daemonset/image-cri-shim
```

---

## 📊 最佳实践

### 1. **生产环境推荐配置**

```yaml
# ConfigMap 配置
apiVersion: v1
kind: ConfigMap
metadata:
  name: image-cri-shim
  namespace: kube-system
data:
  registries.yaml: |
    version: v1
    atoll:
      address: http://atoll.hub:5000
      auth:
        username: ${ATOLL_USERNAME}  # 使用环境变量
        password: ${ATOLL_PASSWORD}

    registries:
      - address: docker.io
        auth:
          username: ${DOCKER_USERNAME}
          password: ${DOCKER_PASSWORD}

    reloadInterval: 30s  # 较长的间隔减少 API 调用
    debug: false
    timeout: 15m
```

### 2. **混合使用 ConfigMap 和本地文件**

```yaml
# /etc/image-cri-shim.yaml - 配置优先级（ConfigMap 不支持）
address: http://atoll.hub:5000
auth: admin:passw0rd
offlinePriority: 1000

registries:
  - address: docker.io
    auth: user:pass
    priority: 800  # 本地文件设置优先级

# ConfigMap - 配置认证信息（可动态更新）
data:
  registries.yaml: |
    version: v1
    atoll:
      address: http://atoll.hub:5000  # 可以覆盖
      auth:
        username: admin
        password: newpassword  # 动态更新密码

    registries:
      - address: new-registry.com  # 添加新 registry
        auth:
          username: newuser
          password: newpass
```

**结果**:
- ✓ ConfigMap 可以动态更新认证信息
- ✓ 本地文件的优先级配置保留
- ⚠️ ConfigMap 的 registries 会替换本地配置

### 3. **验证配置生效**

```bash
# 1. 查看 ConfigMap
kubectl get cm -n kube-system image-cri-shim -o yaml

# 2. 查看本地配置（已同步）
cat /etc/image-cri-shim.yaml

# 3. 查看 image-cri-shim 日志
journalctl -u image-cri-shim -n 50 | grep -E "synced|reloaded"

# 4. 测试拉取镜像
crictl pull nginx:latest
journalctl -u image-cri-shim -n 20 | grep "priority match"
```

---

## 🔧 故障排查

### 问题 1: ConfigMap 配置未生效

**症状**: 更新 ConfigMap 后，配置没有变化

**排查步骤**:
```bash
# 1. 检查 ConfigMap 是否存在
kubectl get cm -n kube-system image-cri-shim

# 2. 检查 ConfigMap 内容
kubectl get cm -n kube-system image-cri-shim -o jsonpath='{.data.registries\.yaml}'

# 3. 检查 image-cri-shim 日志
kubectl logs -n kube-system daemonset/image-cri-shim | tail -50

# 4. 检查本地配置文件
cat /etc/image-cri-shim.yaml

# 5. 等待一个 reloadInterval 周期（最多 15 秒）
```

### 问题 2: Priority 配置丢失

**症状**: 通过 ConfigMap 配置后，registry 优先级都变成了 500

**原因**: ConfigMap 不支持 priority 字段

**解决方案**: 使用本地配置文件设置优先级

### 问题 3: ConfigMap 找不到

**日志**:
```
skip syncing image-cri-shim config; unable to create kube client: ...
configmap kube-system/image-cri-shim not found; skip syncing
```

**说明**:
- 这些是**正常**的 debug 日志
- image-cri-shim 会继续使用本地配置
- 不影响服务运行

---

## 📝 总结

### ✅ ConfigMap 配置的优势

1. **动态更新**: 无需重启节点即可更新配置
2. **统一管理**: 集群管理员可以集中配置所有节点
3. **灵活控制**: 可以动态开关 debug 模式、调整缓存参数

### ⚠️ ConfigMap 配置的限制

1. **完全替换**: registries 配置会完全替换本地配置
2. **明文存储**: 认证信息以明文存储在 ConfigMap 中

### 🎯 推荐使用方式

**纯 ConfigMap 配置模式** (推荐):
- ✅ **ConfigMap**: 配置所有内容(地址、认证、优先级、缓存等)
- ✅ **优势**: 动态更新,无需重启,完全支持优先级

**配置文件分层**:
```
动态配置（ConfigMap）- 推荐主要使用:
  - Registry 地址和认证
  - Priority 配置 ✅ 完全支持
  - OfflinePriority 配置 ✅ 完全支持
  - Debug 开关
  - 缓存参数
  - 重载间隔

静态配置（本地文件）- 可选:
  - Runtime socket 路径（如果需要自定义）
  - 基础网络参数（如果需要自定义）
```

这样既能享受 ConfigMap 的动态更新便利，又能完全支持优先级等高级配置功能！
