# 参与贡献 Atoll-K8s

Atoll-K8s 是从 [labring/sealos](https://github.com/labring/sealos) 的集群管理部分 fork 出来的项目。欢迎提 issue 和 PR。

## 报告安全问题

请不要用公开 issue 报告安全漏洞。在仓库的 Security 标签页点击 "Report a vulnerability" 私下提交。

## 提 issue

- 先搜索已有 issue，有相同问题的话直接在下面补充信息。
- 使用 [issue 模板](.github/ISSUE_TEMPLATE)（bug、功能建议、文档），尽量填全版本、环境和复现步骤。
- 贴日志和配置之前，先去掉密码、密钥、内网地址等敏感信息。

## 提交代码

### 准备仓库

有仓库写权限的成员可以直接在本仓库建分支；其他人先 fork，再 clone 自己的 fork：

```bash
git clone https://github.com/<your-username>/atoll-k8s.git
cd atoll-k8s
git remote add upstream https://github.com/cpaas-dev/atoll-k8s.git
git remote set-url --push upstream no-pushing
```

从最新的 `main` 拉分支：

```bash
git fetch upstream
git checkout -b <branch> upstream/main
```

开发环境和编译方法见 [DEVELOP_GUIDE.md](DEVELOP_GUIDE.md)。

### 提交前检查

```bash
make format          # gofmt + goimports, CI does not check formatting
make build           # or: make build BINS="<changed binaries>"
make lint            # CI only fails on issues in new code
make verify-license  # every source file needs a license header
```

新增的源文件用 `make add-license` 添加 license 头。已有文件里的上游版权头不要删改。

### Commit 和 PR 标题

PR 标题由 CI 按 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/v1.0.0/) 检查，scope 必填，不符合的 PR 无法合并。commit message 也用同样的格式。

```
<type>(<scope>): <subject>
```

- type 可选：feat、fix、docs、style、refactor、perf、test、build、ci、chore、revert
- 示例：`feat(cli): add cluster status command`、`fix(atollctl): close ipvs handle on exit`
- 标题不支持 `!` 破坏性变更标记，破坏性变更请在 PR 描述里说明。

### 提 PR

- 目标分支是 `main`，提交前先 rebase 到最新的 `main`。
- 一个 PR 只做一件事，每个 commit 都应能单独通过 CI。
- 修 bug 请关联对应 issue；新增功能请同时补充文档。PR 标题和描述会进入 release note。

## 许可证

本项目沿用上游的 [LICENSE.md](LICENSE.md)（Sealos Sustainable Use License）。它不是 OSI 认可的开源许可证，限制了对外提供云服务和商业分发，贡献前请先阅读。
