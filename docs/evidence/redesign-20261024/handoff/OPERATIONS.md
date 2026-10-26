# 操作引导

面向接手者的逐步操作。所有构建、测试、浏览器验收都在 GitHub Actions 上进行，本机不安装依赖。

## 1. 准备环境

1. 工具：`git`、`gh`（GitHub CLI，已登录对 `ForceMind/MyAPI` 有推送权限的账号；`gh auth status` 检查）。读写代码的编辑器即可，不需要 Node、Bun、Go、Docker。
2. 工作区：项目**只放在维护者的移动硬盘上**（exFAT 格式），维护者已决定本机内置磁盘上的旧版本项目会删除，所以**不要在本机内置磁盘另建副本或克隆**。工作区是硬盘上的 `dev` 分支 git 工作区 `/Volumes/Work/Prive/MyAPI/.claude/worktrees/dev-work`，直接在其中工作。它依赖硬盘上主检出 `/Volumes/Work/Prive/MyAPI` 里的 `.git` 目录，**不要删除、移动或重命名 `/Volumes/Work/Prive/MyAPI`**；主检出本身停在旧分支，不要在那里工作。
3. exFAT 注意事项：每条 git 命令加 `-c core.fileMode=false`（否则每个已跟踪文件显示权限位变化）；输出会有大量 `non-monotonic index ... ._pack-*.idx` 报错噪音，用 `2>&1 | grep -v 'non-monotonic'` 过滤，不影响结果；不要 `git add -A`，只添加自己的文件；Go 与 Node 会忽略 `._*` 文件，但统计代码行数或 grep 时要排除它们（用 `git ls-files`、`git grep` 更可靠）。
4. 确认 Git 身份是仓库维护者既有身份，不要修改 Git 用户配置。
5. 不要创建新分支；所有工作在 `dev`。

## 2. 了解当前状态

```bash
git fetch origin
git log --oneline -15 origin/dev
gh run list --repo ForceMind/MyAPI --branch dev --limit 5
```

- 先读 `handoff/README.md`，再读 `TASKS.md`。
- 最后一个已确认全绿的提交：`a53973d4`（CI 运行 40271665685，首次失败于偶发的 `window-maintenance`，重跑通过）。之后的 5b、5c 代码未经过 CI。

## 3. CI 循环（核心操作）

每轮约 30 分钟。同一分支的新推送会**取消**还没跑完的旧运行，所以攒好提交再推送，推完等结果。

### 3.1 看哪个作业、哪个步骤失败

```bash
gh run view <RUN_ID> --repo ForceMind/MyAPI                       # 概览
gh run view <RUN_ID> --repo ForceMind/MyAPI --json jobs > jobs.json
```

`jobs.json` 里每个作业有 `steps`，找 `conclusion` 为 `failure` 的步骤。前端作业叫 "Frontend typecheck and test"，其中的步骤依次是：Typecheck、Lint、Test，然后是几个 Chromium 验收步骤（"Qualify advanced personal maintenance in Chromium"、"Verify real quota charts..."、"Verify model discovery and routing..."、"Verify assigned access..."、"Qualify personal console pages and native-window shares in Chromium"、"Qualify shared UI, roles, settings and responsive pages in Chromium"）。另有 "Isolated credential maintenance browser and real API" 作业（含真实 Go 后端步骤）。

### 3.2 取失败日志（日志很长，必须先写到文件再搜）

```bash
gh run view <RUN_ID> --repo ForceMind/MyAPI --job <JOB_ID> --log-failed > ci.log
grep -n -E "##\[error\]|AssertionError|Error:|FAIL" ci.log | head -30
```

- Lint 的注解不带文件名：`##[error]Expected { after 'if' condition.` 表示有 `if (...)` 后语句体换行但没加花括号；用 `git diff <基线>..HEAD -U0 -- web/src | grep -nE '^\+\s*(\} else )?if \(.*\)\s*$'` 找新增代码里的这种写法。
- 浏览器验收的失败信息在日志里的 `AssertionError [ERR_ASSERTION]: ...` 或 `locator.waitFor: Timeout`；脚本在 `tools/ui/*.mjs`、`tools/runtime/*.mjs`、`tools/quota`、`tools/routing`。
- 只重跑失败作业（判断是否偶发）：`gh run rerun <RUN_ID> --repo ForceMind/MyAPI --failed`。

### 3.3 取浏览器截图（可选，用完删除）

CI 的浏览器步骤会上传截图和报告为构件（步骤名带 "Upload ... screenshots"）：

```bash
gh run download <RUN_ID> --repo ForceMind/MyAPI -D /tmp/myapi-shots
# 看完后删除：rm -rf /tmp/myapi-shots
```

只下载需要的构件（`-n <name>`），看完立即删除，不长期占用本机空间。

## 4. 修复与提交规则

1. 先从日志定位具体断言和文件，再改；不要凭推断改（本批里一次凭推断改错了位置，浪费一轮 CI）。
2. 判据：失败是"真实回归"（改产品代码）还是"合同变化"（需求确实变了，才更新测试，并写明依据）。遮挡、裁切、不可读、功能不可达、用量未知不补零、不重放写入、不刷新登录等断言不得放宽。
3. 没有本机依赖时，改代码要做"人工 typecheck + lint"：导入的符号与路径存在、props 类型与调用点一致、无未使用变量、可选值已处理、被删符号全仓 grep 无残留引用（含测试与 `tools/`）。
4. oxlint 规则：`curly: multi-line`；测试里用 `.querySelector()`；不用 `dangerouslySetInnerHTML`；其余看 `web/.oxlintrc.json`。
5. 路径限定提交（避免带入暂存区里别人的文件）：
   ```bash
   git add <路径...>
   git commit -m "type(scope): 说明" -m "<署名行>" -- <同样的路径...>
   git show --stat HEAD        # 核对只含自己的文件
   ```
   提交信息沿用既有风格：`feat(web): ...`、`fix(web): ...`、`test(tools): ...`、`docs: ...`、`feat(cli): ...`。
6. 推送：`git push origin dev`。不要强推、不要 reset、不要 stash、不要 `git add -A`。
7. 推送后记录 CI 运行号，等结果。通过才能在 `TASKS.md` 里把该步标为已验证。

## 5. 前端修改约定

- 技术栈、组件与目录约定看 `web/AGENTS.md` 与 `.agents/skills/shadcn-ui`（语义令牌、组合规则）。颜色只用设计令牌（`web/src/styles/theme.css`），不写死颜色；图标用 Hugeicons；不新增依赖。
- 文案：所有用户可见文本经 `t('English key')`，英文原文即键。新增键要在 `web/src/i18n/locales/` 的 7 个文件各加一份（en、zh 有译文，其余五种语言用英文回填；追加在文件末尾，7 个文件顺序一致）。`_reports` 由 `bun run i18n:sync` 生成，没有运行环境时不要手改。
- 修改后检查是否影响 `tools/` 下的浏览器脚本：它们按 `data-testid`、可访问名称、标题文字、`.pc-*` 选择器定位。尽量保持这些不变；必须改时同批更新脚本。
- 圆角：`rounded-card` 是自定义工具类，经过 `cn()`（tailwind-merge）与组件自带圆角类合并时可能共存；见到这类冲突优先用语义令牌 `--card-radius` 或原有 `rounded-xl`。

## 6. 后端修改约定

- 先读 `AGENTS.md`：JSON 必须用 `common.Marshal` 等包装；数据库代码必须同时支持 SQLite、MySQL≥5.7.8、PostgreSQL≥9.6，优先用 GORM；计费代码不得产生负扣费，遵守 `common/quota_math.go` 与饱和审计规则；修改 `relaykit/` 必须 `GOWORK=off go build ./...`（在 CI 的 "Build relaykit independently" 步骤里验证）。
- 版本门禁在 `middleware/edition.go`；封锁路径清单是 `isLANRestrictedPath`；`capabilities` 描述界面入口，后端封锁范围以该函数为准。
- 后端测试在 CI 的 "Backend vet, build, and test" 作业与数据库回归作业里运行。

## 7. 部署脚本修改约定

- `cli/` 是零依赖 Node 脚本，测试 `cli/test/*.test.mjs` 在 CI 的 "Distribution and release contracts" 作业里运行；`deploy/install.sh` 是 bash（macOS 自带 bash 3.2 与 Linux bash 的差异要注意）。
- 不改镜像坐标与包名；不发布；不接线签名、下载、安装、更新（决定未落地前只做纯合同与合成测试，见 `docs/RELEASE_MANIFEST.md` 第 9 节与 `PLAN_PHASE2.md`）。

## 8. 起子代理（如果环境支持）

- 一个角色一个子代理；委派前写清：目标、成功标准、涉及文件、禁止事项、汇报格式；并发时要求路径限定提交，避免互相带入文件。
- 子代理同样不在本机构建；让它们逐文件核对类型与导入，并在汇报里列出"最担心的 5 处"供 CI 红时优先排查。
- 子代理汇报不是已验证事实：关键结论要核对原文、读 diff，最终以 CI 为准。

## 9. 完成一步后要做的事

1. 推送并读 CI；通过则在 `docs/evidence/redesign-20261024/TASKS.md` 记录提交号、CI 运行号与结论，再提交推送。
2. 向维护者按六段格式汇报。
3. 回到 `handoff/README.md` 第四节，取下一项；需要方案的先给方案等确认。

## 10. 禁止事项清单

- 合入 `main`、发布、打 tag、推镜像、部署生产、改生产配置、调用真实供应商、重新压测。
- 在本机安装依赖、构建、跑浏览器或 Docker。
- 放宽或删除用于证明可用性、金额与用量规则的断言；删除数据库表字段。
- 强推、reset、stash、清理未提交内容、`git add -A`。
- 删除不是自己创建的文件，或在没有授权时对外发送内容。
