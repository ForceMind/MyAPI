# 接手提示词（可直接复制给接手 AI）

以下内容整体复制。方括号里的是需要接手者自己填写或确认的位置。

---

你要接手 MyAPI 项目（GitHub：`ForceMind/MyAPI`），延续已有成果，**不要重新搭建或重做已完成部分**。

## 工作约束

1. 与维护者的全部对话使用简体中文，结论先行，不用华丽措辞。
2. 方案经维护者明确确认后才执行。动手前先写出一行"成功标准：……；验证方法：……"。
3. 未验证的内容不写成无标记的断言：亲自运行过的直接陈述，只读了代码的写"从代码看"，猜测的写"猜测"。未经验证不称"完成"。
4. **构建、测试、浏览器验收全部在 GitHub Actions 上进行，不要占用本机空间。** 不在本机安装依赖、构建、跑 Playwright、跑 Docker。本机只读写代码和做不占磁盘的轻量检查。
5. 只在 `dev` 分支工作，不新建分支，不合入 `main`，不发布 NPM / GHCR / tag，不部署生产，不改生产配置，不调用真实供应商。这些都需要维护者单独授权。
6. 每一步完成后提交并推送到 `origin/dev`，推送后读取该提交的 CI 结果，通过才算该步完成。提交只含自己的文件（路径限定提交），提交信息末尾按你所在环境的要求附署名行。
7. 界面语言暂时只要求中文与英文：新增文案经 `t('English key')`，补 en 与 zh 译文，其余五种语言用英文回填。
8. 不放宽测试断言；"遮挡 / 裁切 / 不可读 / 功能不可达 / 用量未知不补零 / 不重放写入"类断言绝不放宽。数据库表与字段不删除，三库（SQLite / MySQL / PostgreSQL）兼容，`relaykit` 保持独立可构建，计费安全不变量不放宽。
9. 删除或覆盖不是你创建的文件、对外发送内容、发布、花钱、改生产，都先问维护者。
10. 汇报格式：实质性的一轮结束后按"当前完成 / 验证结果 / 审查发现 / 已修复问题 / 剩余任务 / 下一步"六段；小事一段话。

## 先读这些，按顺序

1. `docs/evidence/redesign-20261024/handoff/README.md`（接手文档：已完成、未完成、风险、约定）
2. `docs/evidence/redesign-20261024/handoff/OPERATIONS.md`（操作引导：CI 循环、命令、提交规则）
3. `docs/evidence/redesign-20261024/handoff/DIRECTORY_GUIDE.md`（目录引导）
4. `docs/evidence/redesign-20261024/TASKS.md`（任务清单与已确认决定）
5. `docs/evidence/redesign-20261024/PLAN_PHASE2.md`（第二阶段计划）
6. 仓库根 `AGENTS.md`、`web/AGENTS.md`（项目硬规则）
7. 需要时：`docs/evidence/redesign-20261024/architecture-review.md`、`ui-design-v2/DESIGN_DIRECTION_V2.md`、`ui-design-v2/step1` 到 `step4` 的 README 与截图、`docs/MYAPI_MASTER_PLAN.md`、`docs/RELEASE_MANIFEST.md`。

注意：`docs/` 里大量文档带历史阶段描述（"未提交""CI 失败"等），当前状态以上面的接手文档、`git log` 和 GitHub Actions 为准。

## 你的第一步（不需要问维护者的部分）

1. 准备工作区。项目**只放在维护者的移动硬盘上**（exFAT 格式），维护者已决定本机内置磁盘上的旧版本项目会删除，所以**不要在本机内置磁盘另建副本或克隆**。
   - 先在 `/Volumes/Work/Prive/MyAPI` 里运行 `git -c core.fileMode=false branch --show-current`：
     - 如果显示 `dev`：维护者已经运行过整理脚本，这个目录本身就是工作区，直接用。
     - 如果显示别的分支（例如 `codex/r1-usage-review-20261002`）：整理脚本还没运行，工作区是 `/Volumes/Work/Prive/MyAPI/.claude/worktrees/dev-work`（`dev` 分支）。请维护者在终端运行 `docs/evidence/redesign-20261024/handoff/consolidate-workspace.sh`（用法写在脚本开头注释里），**你自己不要运行它**（它会删除你正在使用的 dev-work 目录）。运行前先用 dev-work 继续工作。
   - 进入工作区后执行 `git -c core.fileMode=false fetch origin` 和 `git -c core.fileMode=false pull --ff-only`。
   - 工作区依赖硬盘上 `/Volumes/Work/Prive/MyAPI/.git`，**不要删除、移动或重命名 `/Volumes/Work/Prive/MyAPI`**；该 `.git` 里还保存着几条远端已不存在的历史分支的本地独有提交。
   - exFAT 上每条 git 命令都要加 `-c core.fileMode=false`（否则每个文件显示权限位变化），并用 `2>&1 | grep -v 'non-monotonic'` 过滤 `._*.idx` 报错噪音；不要 `git add -A`，只添加自己的文件。
   - 确认 `dev` 的最新提交包含接手文档（`docs/evidence/redesign-20261024/handoff/`），`git log` 里应能看到提交 `2f5b7420` 或更新的提交。
2. 用 `gh run list --repo ForceMind/MyAPI --branch dev --limit 5` 看最近 CI。最后一个已确认全绿的提交是 `a53973d4`（运行 40271665685，经重跑通过）。其后的第 5b、5c 步代码**从未经过 CI**，推送后期望会红。
3. 读最新一次 CI 的失败步骤和日志，按接手文档第六节的优先排查点逐个修，修一批、推一次（同一分支新推送会取消未跑完的旧运行）。
4. CI 全绿后，用 CI 上传的浏览器截图检查 5b、5c 各页面的实际效果，对照 `ui-design-v2/mockups/3-flow/` 样稿与 `step1`–`step4` 截图，不一致的返工。

## 之后的工作（先向维护者给出方案并等确认）

按接手文档第四节的优先级：P1 UI 收尾 → P2 第二阶段（结构审查与重构、部署脚本 B3–B7）→ 收口（七语言补译与验收、独立质量审查、全量验收）。每一项先给"成功标准 / 验证方法 / 涉及文件 / 风险"，维护者确认后再做，做完推送并读 CI。

## 子代理（如果你的环境支持）

默认单智能体。复杂任务可按需起子代理：实现类用中等模型，涉及计费、对外契约、存储结构或跨模块的改动用最强的非 Fable 模型；Fable 只用于生成方案、咨询、审查方案。委派时写清目标、成功标准、涉及文件、禁止事项和汇报格式；子代理汇报不是已验证事实，要亲自核对关键命令的输出或 CI 结果。子代理同样不得在本机构建。

## 已确认的决定（不要再问）

- Docker 的 `docker-build.yml`、`docker-smoke.yml` 暂不改用 `lite`；命令行与安装脚本对齐到安装脚本更严格的来源规则（尚未实现，见接手文档 P1-9）；分析图表第 6 名起用中性灰；第二阶段计划里的 5 项决定（Lite 镜像坐标不变、签名信任根用 GitHub OIDC / Sigstore 但不接线、原生安装路径、Desktop 先 macOS 再 Windows、自动更新默认关闭）。
- 维护者已选定 UI 方向 3「流向图」，已批准第二阶段计划，不要重开设计方向的讨论。

## 开工前向维护者确认一件事

本次接手的优先级是否与接手文档第四节一致。如果维护者给出新的需求，以维护者最新一句话为准，但不要因此丢掉已商定的剩余任务。除此之外不需要再问，直接按"你的第一步"开始。
