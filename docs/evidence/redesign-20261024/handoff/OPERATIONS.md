# 操作引导

面向接手者的逐步操作。所有构建、测试、浏览器验收都在 GitHub Actions 上进行，本机不安装依赖。

**2026-10-30 最新：UI首批正文/SSE分页与单日历范围已实现并通过完整CI**。源码 `3e31517e243ac92ab95bbdba3b17eb2c59e404a9` / [手动CI41013409921](https://github.com/ForceMind/MyAPI/actions/runs/41013409921)，run_browser=false、body_ui_baseline=true。固定旧源37450c2a的两项缺陷先按预期失败，恢复候选后分页/日期/返回状态专项78/78、前端全量1974/1974、同步/构建/typecheck/lint通过；完整CI首次success（10成功、1浏览器作业跳过），全部作业/步骤已回读，含根模块/独立relaykit/原生与386构建、两模块全量、既有竞态/三库/Redis/分发等。收尾只改交接，应用源码仍绑定3e31517e，接手读取最新文档CI。提供首尾/数字页/直接跳页、20/50/100条；按history entry恢复已应用筛选/page/pageSize，身份/权限失效清除，有界内存不写URL/storage；同一日历选起止、跨月/预设/秒级时间/取消应用/开放端点与非法时间校验。新增21键en/zh、其余5语言英文回填；必要补齐原Calendar焦点ref。既有线稿/方向3/Logo不重做，未改后端/计费/生产或诊断控制器，不打包部署新镜像；生产上次已核对源码3e6b8cca，不因dev新提交自动更新。浏览器未验证；准确进度见[TASKS](../TASKS.md)及[线稿实现记录](../ui-design-v2/wireframes/README.md)。

**2026-10-30 最新：3e6b8cca已部署；本轮只读取证结束，控制器修复待确认，随后回UI**。生产实际app为已验收源码3e6b8cca（目标config 38b61c5b…，服务器已校验等价的loaded ID c303b3e8…），最终容器healthy、缓存会话开关true、诊断false；本轮只读复核保持该状态。部署当时三阶段9479条消费/9680条日志及各自累计扣费一致、完整性/FK检查通过，使用最新数据、保留旧资源和备份，未恢复旧快照。单次窗口02:51:17—02:51:19约2.5秒、保存挂载日志327字节、自然样本0，不能称缓存命中验收通过。已证控制器以max(累计日志增长,进程自启动wchar)>=80MiB提前关窗；该口径混入非日志写入。退出2与初始化时收到停止信号的运行时路径吻合，但历史信号事件/调用栈及当时计数/PID未保存，仍属推断。详细证据、边界和待确认最小方案见[缓存专项第0节第六轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。本轮只改4份交接，不改产品/服务器文件/生产配置，不重开诊断、不发模型请求；推送后读取本次文档CI，镜像仍绑定3e6b8cca。下一批回到已有UI线稿的正文/SSE分页和单日历范围选择，不重画全站，不自动实施或跑浏览器。

**2026-10-30 最新：缓存修复新镜像已在Actions打包验收，生产方案待确认**。源码 `3e6b8cca333194644a8bb2902f662095b407e090` / [完整CI40911707379](https://github.com/ForceMind/MyAPI/actions/runs/40911707379)通过；[Full+SQLite 40924927031](https://github.com/ForceMind/MyAPI/actions/runs/40924927031) attempt1 success，构建/独立加载2作业成功、LAN安装跳过，20项检查全true，browser=not_run。新[镜像包](https://github.com/ForceMind/MyAPI/actions/runs/40924927031/artifacts/12919793136)与[报告](https://github.com/ForceMind/MyAPI/actions/runs/40924927031/artifacts/12919559223)至北京时间11月6日01:51/01:53；准确manifest/app身份在[缓存专项第0节第五轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。包内缓存会话开关默认false；将交接服务器任务先提出更新镜像及开启开关的最小方案，维护者针对该方案确认前不得动生产，只拟观察自然流量，不主动模型/采样/凭据刷新。本机未下载构件/安装依赖/构建测试，未发镜像注册表或tag，未部署。收尾只改交接，镜像仍绑定本段源码，接手读取最新文档CI。

**2026-10-30 最新：A/B/C最小修复已通过产品完整CI**。候选 `42a258dfecc8e584a231f6b81955440091a969d1` / [CI40905811752](https://github.com/ForceMind/MyAPI/actions/runs/40905811752) 首次success（10成功、1浏览器作业跳过）；已读全部作业/步骤，两模块测试、独立relaykit、原生/386构建、全部竞态、三库/Redis/分发等通过；前端24/24专项、1917/1917全量、同步/构建/typecheck/lint通过。收尾只改交接文档，应用仍绑定42a258df，接手读取最新文档CI。产品/探针源码 `9fbf640e` 的[对照40905060265](https://github.com/ForceMind/MyAPI/actions/runs/40905060265)成功，后续42a仅工作流/locale顺序/合成夹具修改，后端与对照工具等价。A补usage形状/四态投影及内部计费信封净化，B默认关闭 `MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED`，C只读保存窗口刷新并区分旧错误/WHAM失败/转发限额。全部验证在Actions；浏览器未验证，未部署/构建新Docker包/改生产/发真实供应商请求，不承诺真实缓存命中。准确证据与边界集中在[缓存专项第0节第四轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。

**2026-10-29 最新：仅测试的缓存合成对照已通过**。源码8f9e533c / 对照40888007112、完整CI40888007450首次success。工具在 `tools/cache-compat-compare/`，固定sub2api a3298b46仅在Actions检出；6请求/8usage观察与未知/零区分不等同产品修复（product_fixed=false）。[缓存专项第0节第三轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)链接矩阵与A/B/C候选最小方案，维护者确认前不改产品、采样/UI或生产，不发真实请求。生产仍cd35；f0ae4505存在性修复为另一会话成果，已保留不重做。后续收尾纯文档，不重复触发对照，接手读最新常规CI。

**2026-10-29 最新插单：空响应头修正代码/镜像已验证**。源码cd35f867 / [完整CI40846159247](https://github.com/ForceMind/MyAPI/actions/runs/40846159247)首次success（10成功、1浏览器混合作业跳过）；[Full+SQLite40846469827](https://github.com/ForceMind/MyAPI/actions/runs/40846469827)首次success、20项通用检查通过，空头专项真实路由回归8个四状态用例全PASS。新构件/manifest/app身份与开关条件集中在[缓存专项第0节第二轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)，新包至11月5日，旧d2b1f664包不含修正。默认关闭，不改请求/响应字节；未知stream不猜测，不把未采集当零。随后收尾只改4份文档，镜像仍绑定cd35f867，接手读最新CI。本会话未部署、未动生产、未开诊断、未发模型请求；后续生产动作另行确认，浏览器未验证。

**2026-10-29 当前插单已验证**：默认关闭的Codex上游诊断日志代码d2b1f664 / [CI40828494736](https://github.com/ForceMind/MyAPI/actions/runs/40828494736) 首次success（10成功、1浏览器混合作业跳过）；[Full+SQLite打包验证40828937555](https://github.com/ForceMind/MyAPI/actions/runs/40828937555) 首次success、20项全true、browser=not_run，构件保留至11月5日。新开关 `MYAPI_UPSTREAM_DIAGNOSTIC_ENABLED=false` 与正文日志双开关生效，Compose源模板已透传；新阶段在现有JSONL中，以request_id/attempt/channel关联，现有页面/API仍只呈现原入口/出口。源码SHA、manifest可信哈希、包/报告链接及限制集中在 [缓存专项第0节](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。随后收尾只改4份文档，接手仍须读最新CI；不把文档SHA当镜像源码。本会话不部署、不改生产环境、不主动发模型请求；接手按对应授权操作，不得沿用旧镜像或仅靠beta26版本字符串识别。

**2026-10-28 最新任务优先于下文临时部署记录**：UI先行，每个页面先画桌面/手机线稿，维护者确认后才改产品。[线稿目录](../ui-design-v2/wireframes/index.html)和[覆盖/审阅说明](../ui-design-v2/wireframes/README.md)为本批交付；CLI来源规则后移。线稿为纯设计源文件，不连接API，默认查看无需任何依赖。未运行浏览器，不把线稿或静态覆盖核对写成视觉/交互验收通过。

维护者已单独授权并完成main快进到c916051e，main CI40664697561首次通过。合并额外触发网站浏览器40664697462，取消时已结束，必须如实保留；`.github/workflows/website-browser.yml`仍需显式授权门禁，不能只看ci.yml的浏览器开关就宣称所有入口受控。本批只改docs，不修改该工作流或再次合main；推送后检查全部实际触发的工作流，发现未授权浏览器运行时立即请求取消并报告实际状态。

**2026-10-28 当前任务**：已批准 GitHub Actions 内的 Full + SQLite 临时测试部署。浏览器默认不运行，只有当前任务明确授权后才手动开启 `run_browser`；推送dev只执行非浏览器检查，混合Go/浏览器作业也按此规则跳过，报告须注明“浏览器未验证”。不要把跳过项写成已执行通过。本次Docker手动入口为既有 `docker-smoke.yml`、ref=dev，输入 `api_only=true, run_browser=false, personal=false`；只准备Full镜像并在另一Runner加载同一构件做HTTP/API、重启和同版本SQLite/WAL恢复验证。构件保留7天，不下载到本机，不发布GHCR、不打Git tag、不部署生产。SHA、镜像ID、包哈希和producer attempt校验继续保留，不回退到旧包。

## 1. 准备环境

1. 工具：`git`、`gh`（GitHub CLI，已登录对 `ForceMind/MyAPI` 有推送权限的账号；`gh auth status` 检查）。读写代码的编辑器即可，不需要 Node、Bun、Go、Docker。
2. 工作区：项目**只放在维护者的移动硬盘上**（exFAT 格式），不要在内置磁盘另建副本或克隆。**2026-10-27 已整理成功，`/Volumes/Work/Prive/MyAPI` 本身为 dev，两个嵌套工作区已移除。不要重跑仓库旧 consolidate-workspace.sh。** 先只读核对当前分支；若不是 dev，停止并问维护者，不自行切换或重建工作区。不要删除、移动、重命名仓库或 `.git`，其中有本地独有历史提交。内置磁盘两个 Codex 工作区未动，删旧项目后的 prune 由维护者处理。
3. exFAT 注意事项：每条 git 命令加 `-c core.fileMode=false`（否则每个已跟踪文件显示权限位变化）；输出会有大量 `non-monotonic index ... ._pack-*.idx` 报错噪音，用 `2>&1 | grep -v 'non-monotonic'` 过滤，不影响结果；不要 `git add -A`，只添加自己的文件；Go 与 Node 会忽略 `._*` 文件，但统计代码行数或 grep 时要排除它们（用 `git ls-files`、`git grep` 更可靠）。
4. 确认 Git 身份是仓库维护者既有身份，不要修改 Git 用户配置。
5. 不要创建新分支；所有工作在 `dev`。

## 2. 了解当前状态

```bash
git -c core.fileMode=false fetch origin
git -c core.fileMode=false log --oneline -15 origin/dev
gh run list --repo ForceMind/MyAPI --branch dev --limit 5
```

- 先读 `handoff/README.md` 零节和 `CACHE_REVIEW_20261027.md`，再按阅读顺序了解原任务。
- 修复前最近全绿：`e7eef352` / CI 40447186866（11/11 成功），含保留策略修复 `6392caf9`。5b、5c 已通过 CI，不能再当作未验证构建。当前最新状态以README零节为准。
- `e8652185` / CI 40462005121 与仅文档的 `50bad4db` / CI40474459858：仅会话头取证的5个子场景失败，其余10作业成功。维护者随后已明确批准最小产品修复；`069f67d7` / [CI40500636411](https://github.com/ForceMind/MyAPI/actions/runs/40500636411) 与显式值映射补充 `9795c0f5` / [CI40504125721](https://github.com/ForceMind/MyAPI/actions/runs/40504125721) 均已11/11成功。其后的纯文档提交仍读取自己的完整CI；不重做修复，不放宽测试或门禁。

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

- 若整轮仍运行，`gh run view --log-failed` 会拒绝。确认目标 job 已 completed 后，可用 `gh api repos/ForceMind/MyAPI/actions/jobs/<JOB_ID>/logs --allow-escape-sequences > <自己创建的临时日志>`；该选项仅允许 ANSI 原样写文件，不执行日志正文。日志查完清理，长期引用 Actions 与缓存专项摘要。
- exFAT 上新文件的原子硬链接可能报 ENOTSUP；只在确认不是 sandbox 拒绝、目标不存在后，用支持独占创建的方式写新文件，禁止覆盖其他文件。既有文件先读再定向编辑。
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
   git commit -m "type(scope): 说明" -- <同样的路径...>
   git show --stat HEAD        # 核对只含自己的文件
   ```
   提交信息沿用既有风格：`feat(web): ...`、`fix(web): ...`、`test(tools): ...`、`docs: ...`、`feat(cli): ...`。按维护者最新约定，新的提交标题、正文与附注不添加代理生成标记、代理署名或相关Co-authored-by；不重写历史提交。
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

### 2026-10-30 UI第二批验证入口

已批准三个日志列表；CI前端专项现包含 `src/features/usage-logs`、`src/features/full-content-logs`、`src/components/date-time-range-picker` 和 `src/lib/__tests__/log-pagination.test.ts`。所有命令仅runner执行，浏览器保持未授权/跳过；本机不安装依赖/运行工具链/下载构件。此前7ab47a82的CI41021525656后端首次超过35分钟作业上限，未放宽断言或超时，仅该作业重跑attempt2成功；本批以自己的CI为准。

第二批源码b9a5d855的[完整CI41041830295](https://github.com/ForceMind/MyAPI/actions/runs/41041830295) attempt1 success，10成功/1浏览器跳过；专项315/315、全量2046/2046、同步/构建/类型/lint及完整后端/数据库等通过，全部作业/步骤已读。前两轮41040228906（专项失败）/41040724991（测试全通过但lint失败）被补修替代，终态cancelled，原失败记录保留。收尾只改4份交接，仍需读其自己的CI。
