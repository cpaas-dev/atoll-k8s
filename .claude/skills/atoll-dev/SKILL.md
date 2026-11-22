---
name: atoll-dev
description: 编译、测试、lint、格式化 Atoll-K8s Go 代码库（atoll、atollctl、lvscare、image-cri-shim）并检查 license 头，编写能通过 CI 的 commit message 和 PR 标题。在本仓库编译二进制、跑测试、验证代码改动、准备提交或 PR 时使用。
---

# Atoll-K8s 开发

## 环境

- 所有二进制都用 `CGO_ENABLED=0` 编译，不需要 C 编译器和系统开发库；产物只能在 Linux 上运行。
- Go 1.27（CI 固定为 1.27.3）。

## 目录结构

- `cmd/<binary>`：atoll、atollctl、lvscare、image-cri-shim 的入口
- `internal/`：所有共享逻辑
- `test/e2e`：端到端测试，是独立的 Go module，通过 `replace` 依赖根 module
- `docs/`：文档

## 编译

```bash
make build BINS=atoll             # 编译单个二进制
make build BINS="atoll atollctl"  # 编译多个二进制
make build                        # 编译 cmd/ 下所有二进制
make help                         # 列出所有 target 和选项
```

- 产物输出到 `bin/<os>_<arch>/`，例如 `bin/linux_amd64/atoll`。
- 冷编译要编译整个依赖图，需要几分钟；增量编译约 15 秒。冷编译时 Bash 超时设为 600000 ms（或放到后台运行），不要中途打断。
- 交叉编译用 `make build.multiarch PLATFORMS="linux_arm64 linux_amd64"`，不需要额外工具链。

## 验证改动

1. `make format`
2. `make build BINS="<改动涉及的二进制>"`
3. 对改动的包运行 `go test ./internal/<pkg>/...`
4. 冒烟测试二进制（见下方）
5. `make lint` 和 `make verify-license`

不需要特权就能运行的冒烟命令：

```bash
./bin/linux_amd64/atoll version
./bin/linux_amd64/atoll env
./bin/linux_amd64/atoll <command> --help
./bin/linux_amd64/atoll completion bash > /tmp/completion.bash
./bin/linux_amd64/atollctl hostname
```

集群命令（`run`、`apply`、`add`、`delete`、`reset`、`gen`）只使用 `atoll pull` 拉到本地镜像库（`~/.atoll/images`）的镜像，导出 rootfs 需要 root 权限。测试时用 `ATOLL_RUNTIME_ROOT=/tmp/<dir>` 把运行目录指到临时目录，不要往用户的 `~/.atoll` 写东西；不要执行真正的 `atoll run`，它会在本机安装集群。

## 测试

- 部分单元测试依赖 root、宿主机路径或网络。认定失败是回归之前，先在 `main` 上跑同一个测试，确认是不是环境问题。
- `make coverage` 只对 `internal/env` 和 `internal/apply` 跑 `go test -race`。
- `test/e2e` 需要真实机器，在 `e2e_*.yml` workflow 里运行；除非用户要求，不要在本地跑。

## 格式化、lint、license

- `make format` 依次运行 `gofmt -s`、`goimports -local github.com/cpaas-dev/atoll-k8s` 和 `go mod edit -fmt`。CI 没有格式检查，所以每次提交前都要跑。
- `make lint` 用 `.golangci.yml` 运行 golangci-lint，缺失时会安装到 `tools/`。CI（`ci.yml`）固定 golangci-lint v2.12.2，并以 base commit 传入 `--new-from-rev`，所以只有新代码里的问题会让 CI 失败。本地 `make lint` 也会输出历史问题，只修改动行上的问题即可。
- `make verify-license` 就是 `check-license.yml` 跑的检查。新源文件需要 `scripts/template/LICENSE` 里的头部，可以用 `make add-license` 添加。保留已有的上游 `Copyright © ... sealos.` 头部，license 禁止删除版权声明。

## Commit 和 PR 标题

`check-pr-title-lint-semantic.yml` 强制使用 Conventional Commits，且 scope 必填：

```
<type>(<scope>): <subject>
```

- 可用 type：feat、fix、docs、style、refactor、perf、test、build、ci、chore、revert
- 示例：`feat(cli): add cluster status command`、`fix(atollctl): close ipvs handle on exit`、`ci(workflow): bump golangci-lint`
- 标题正则不接受 `!` 破坏性变更标记，破坏性变更写在 PR 描述里。
- commit message 也使用同样的格式。
