# 接手文档（2026-10-26）

本文件是接手入口。同目录另有：[PROMPT.md](PROMPT.md)（可直接复制给接手 AI 的提示词）、[OPERATIONS.md](OPERATIONS.md)（操作引导）、[DIRECTORY_GUIDE.md](DIRECTORY_GUIDE.md)（目录引导）。

证据标记：**[CI]** 在 GitHub Actions 上实际通过或失败过；**[读码]** 只读了代码或报告，未运行；**[未验证]** 已写入但没有任何运行结果；**[维护者]** 维护者在对话中明确说过。

## 一、项目与目标

- 仓库：`ForceMind/MyAPI`，开发分支 `dev`，稳定分支 `main`（本批工作没有动 `main`，仍是 `92fa1182`）。只用这两个分支，不新建分支，不合入 `main`，不发布，不部署生产。
- 项目：Go（Gin + GORM）与 React 19（Bun、Rsbuild、Base UI、Tailwind）的 AI API 网关。三个版本：Full 完整版、Lite 个人版、Desktop 本地版（Lite 的桌面形态，默认只开放本机）。定义见 `docs/MYAPI_MASTER_PLAN.md` 第 1、8 节与 `docs/RELEASE_MANIFEST.md` 第 2 节。
- 维护者本批需求原话 **[维护者]**：遵循第一性原理评估整个项目；审查代码结构，精简代码；按项目计划制作完整版、个人版、本地版；保留现有 Logo，整个 UI 重新设计，要易用，PC 与手机都要适配度高；使用设计 skill；部署脚本也要完善。
- 本批起点：维护者给定的已验证基线 `46a39080`（`0.2.0-beta.26` 候选，CI 运行 39943576911 全部通过）。

## 二、维护者的硬性要求（接手者必须遵守）

1. 与维护者的全部对话用简体中文；不用华丽措辞；结论先行。
2. 方案经维护者明确确认后才执行；每一步先说明成功标准与验证方法。
3. 未验证的内容不得写成无标记的断言；未经验证不称"完成"。
4. **构建、测试、浏览器验收全部在 GitHub Actions 上进行，不要占用本机空间** **[维护者]**：不在本机安装依赖、构建、跑浏览器、跑 Docker。本机只读写代码和做不占磁盘的轻量检查。
5. 每一步完成后提交并推送到 `origin/dev` **[维护者]**，推送后读取该提交的 CI 结果。
6. 不合入 `main`；不发布 NPM / GHCR / tag；不部署生产；不改生产配置；不调用真实供应商；不重新压测。这些都需要维护者单独授权。
7. 界面语言暂时只要求中文与英文 **[维护者]**。新增文案提供 en 与 zh 译文，其余五种语言（zh-TW、fr、ru、ja、vi）用英文回填并记入待办。这是对 `AGENTS.md` 七语言要求的临时放宽。
8. 子代理默认不用 Fable 模型，只有"生成方案、咨询、审查方案"可以用。一个角色最多一个子代理。子代理的汇报不是已验证事实，要亲自核对。
9. 用户可见的结论格式：每次实质性汇报按"当前完成 / 验证结果 / 审查发现 / 已修复问题 / 剩余任务 / 下一步"六段，小事一段话。

## 三、已完成的工作（按时间顺序，均已推送，除非标注）

### 3.1 评估与精简
- 架构评估报告：`docs/evidence/redesign-20261024/architecture-review.md`（`61e0f877`）。**[读码]** 由 Opus 子代理只读完成，主会话抽查了关键结论；它自己列出的覆盖缺口见第五节。
- 删除 74 个未引用的前端文件与 5 个依赖（`f4d0026c`）、158 个不可达 Go 函数（`ba6a3c41`）**[CI]**。计费相邻的 37 个不可达函数按约定保留。

### 3.2 版本能力与后端
- 后端识别 `lite`（`lan` 为 Legacy 别名），`/api/status` 新增 `product_edition` 与 `capabilities`，Electron 改传 `lite`（`e99d66a9`）**[CI]**。
- 维护者批准的三项门禁变更（`07207713`）**[CI]**：未知 `MYAPI_EDITION` 值拒绝启动（合法值为空、`full`、`lite`、`lan`）；Lite 下补封 `/api/redemption`、`/api/deployments`、Midjourney（`/api/mj`、`/mj`、`/:mode/mj`，后两者由主会话扩展，已告知维护者）；Lite 下只放开 `GET /api/subscription/self`。
- 日志新增可选参数 `result=failed`（`eb08c3f3`）**[CI]**，MySQL 与 PostgreSQL 的测试接入了 CI 的 S1 作业且通过。
- 维护者确认：管理员概览继续读取全部用户的记录（与调用记录页一致）；亮色青色对比度保持不变（遥测填充必须同时显示数值）。

### 3.3 UI 重新设计（方向 3「流向图」）
- 第一版设计（方向 A，工作台加检视器）被维护者否决，其外壳、令牌值、页面布局已被取代。第二版方案与三个可点击样稿：`docs/evidence/redesign-20261024/ui-design-v2/`，维护者选定**方向 3**（`00763936`）。
- 第 1 步 令牌与顶部导航外壳：`b28cf752`、`e59adcdd`、`d57a4d7b`、`f5770baa`，CI 运行 40166671819 **[CI]** 通过。
- 第 2 步 首页流向板：`edf7ae1d`、`cceb4aa5`、`ed45ec3e`，CI 运行 40175849784 **[CI]** 通过。数据口径：24 小时内最近 100 条日志在前端聚合，核心卡总数精确读取；未新增后端接口。
- 第 3 步 Key 列表、Key 对象页、请求页：`67be4186`、`b2607555`、`f3b29ffe`、`20ec63f1`，CI 运行 40187649824 **[CI]** 通过。新地址 `/keys/<id>`、`/keys/<id>/requests/<requestId>`、`/usage-logs/request/<requestId>`，旧 `?inspect=` 重定向。
- 第 4 步 上游卡片网格、连接对象页，加三个收尾项（概览"失败 N"用 `result=failed`、分页按钮手机 44px、Lite 个人资料页"我的订阅"只读卡片）：`1a025f6b`、`ae4a9f63`、`58be988e`，加两个 lint 修复 `bef61f33`、`ead5041f`。CI 运行 40268212418 **[CI]** 通过。
- 第 5a 步 请求区（调用记录、任务、绘图、正文与 SSE、分析）：`1e91109f`、`4605684d`、`8ee0ada6`、`045127e1`、`7cd12762`、`a53973d4`。CI 运行 40271665685：首次失败于"高级维护"浏览器步骤（`window-maintenance` 的"关闭订阅池"对话框 15 秒未隐藏），**只重跑失败作业后通过** **[CI]**。按现有证据判断为偶发，但没有找到根因，见第六节。

### 3.4 部署脚本（第二阶段 B1、B2）
- B1 版本值贯通（`80f9185b`）：`Dockerfile`、`cli/myapi.mjs`、`deploy/install.sh`、`tools/lan/check.mjs` 接受 `lite`（`lan` 为别名），镜像坐标 `ghcr.io/forcemind/myapi-lan` 不变 **[CI]**。
- B2 配置与引导（`cddc648d`）**[CI]**：刷新 `deploy/.env.example`；`myapi init --public-url` 一步生成 `.env`；`myapi doctor` 新增版本值、端口、目录权限、磁盘空间、Docker 与 Compose 版本、访问范围一致性检查。
- 第二阶段计划：`docs/evidence/redesign-20261024/PLAN_PHASE2.md`（`cc2e191d`）。

### 3.5 已写入本地但**尚未经过 CI**（接手后第一件事是推送并读 CI）
- 第 5b 步 设置、用户、系统信息、个人资料：`df2b96ba`、`acd53ee5`、`403cca15`、`470d0860`、`6e89abd3` **[未验证]**。
- 第 5c 步 商业（钱包、订阅、兑换码）、认证页、安装向导、公共页与错误页、Playground / 聊天 / 提示词学习、模型目录与账号窗口：`496dafc5`、`5844e5b8`、`107daf1b`、`58d8ce2c`、`5bbf6cb1`、`3ca80983`、`e44f4056`、`c531c0a8`、`e1aad933`、`ad33f977` **[未验证]**。
- 这两批代码是在本机没有依赖、不能编译的条件下写的，子代理只做了人工核对（5b 另外用 bun 在内存里转译过 22 个文件检查语法）。**期望 CI 会红**，要按日志逐个修。

## 四、未完成清单（按优先级）

### P0 立即
1. 确认 `dev` 上 5b、5c 已推送（本文件随同一次推送提交）；读 CI（约 30 分钟一轮），把红的步骤修绿。优先排查各子代理自己列出的风险点，见第六节。
2. 5b、5c 通过后，用 CI 上传的浏览器截图（工作流里的 "Upload ... screenshots" 步骤）检查视觉，不通过的页面返工。

### P1 UI 收尾
3. 剩余的 lucide 图标迁移到 Hugeicons（维护者已确认统一 Hugeicons，随页面批次迁移；此前统计 279 个文件用 lucide、32 个用 Hugeicons）。必须在 CI 上确认导出名存在。
4. 5b、5c 子代理记录的未做项：
   - `profile/components/personal-profile.tsx` 是生产代码里无引用的死组件，但有测试覆盖，需先迁移测试再删除。
   - 设置页没有共享"危险操作区"组件。
   - 模型目录与账号窗口页没有统计块。
   - 订阅与兑换码的手机记录没有单测；5c 没有写 README 与截图证据。
5. 已知但没修的界面问题：Lite 版本行在 1280 宽度下于"用量未提供"中间折行；英文计数无单复数；1024 宽度上游节点名被截断；`/channels` 视图切换按钮没有可访问名称；`models-390` 的"更多"按钮 40px 的修复未验证；Key 节点只显示 `#ID` 而非掩码尾号、上游节点没有"测试失败 401"（现有数据不提供）；首页没有"今天 / 7 天"切换；Full 首页核心卡没有"用户 / 分组"摘要；channels 的 toast 文案仍写 "Track progress in System Info"；设置里"Header navigation"的中文"顶部导航"易与控制台顶栏混淆；`personal-console.css` 的 `.pc-card` 依赖运行时可能不存在的 `--radius-card`。
6. 七语言：补 zh-TW、fr、ru、ja、vi 的译文；验证俄语、法语下顶栏导航（1024–1279 宽度需把一项收进"更多"，样稿未验证）；恢复七语言截图验收。`web/src/i18n/locales/_reports` 一直没有重新生成（子代理不能运行同步脚本），在能运行 `bun run i18n:sync` 的环境（注意 exFAT 上要过滤 `._*.json`）重新生成。
7. Desktop 专属界面：首页"本机接入"卡；托盘文案接 i18n（见部署 B5）。
8. `tools/` 下不在 CI 里的浏览器脚本（`console-layout-browser.mjs`、`input-policy-browser.mjs`、`tools/quota/token-budget-browser.mjs`）可能依赖旧外壳，需检查；`tools/runtime/personal-browser.test.mjs` 有 5 个基线就存在的失败，不在 CI 里。

### P2 第二阶段（`PLAN_PHASE2.md`，B1、B2 已完成，其余未开始）
9. A1 补全结构审查（账务与任务恢复逐文件读；支付回调在 Lite 下对未完成订单的处理；只用外部登录、没有密码的账户在 Lite 下能否登录；`tools/`、`website/`、`deploy/`；790 个单调用点函数；637 个疑似未用翻译键）。
10. A2 Lite 降级影响修复（依 A1）；A3 审查并删除 37 个计费相邻不可达函数；A4 分层整理（`controller/channel-billing.go` 约 1.2k 行业务逻辑下沉 `service/`）；A5 内联单调用点短函数；A6 清理前端未用导出（140 + 68）、未引用 UI 原语（约 1,448 行）、未用翻译键。
11. B3 Linux 原生服务（修正 `my-api.service`：行内注释从 systemd 语法看不被支持，未实测；加固；`deploy/install-native.sh`；user service）；B4 `myapi backup` / `myapi restore`；B5 Desktop（托盘 i18n、窗口关闭 / 退出 / 停止服务区分）；B6 容器加固评估（非 root 运行）；B7 中英文部署教程。
12. C1 七语言补译与验收；C2 独立质量审查（Fable，只审查）；C3 全量验收。

### 待维护者回复的小决定（均有推荐，不阻塞）
- Docker 的 `docker-build.yml`、`docker-smoke.yml` 是否改用 `lite` 构建：推荐暂不改，等 Lite 镜像坐标落地。
- 命令行接受 HTTPS 公网来源而安装脚本只接受本地或私网来源：推荐对齐到安装脚本的更严格规则。
- 分析页图表前 5 个系列用设计色、其余统一中性灰（色板只校验过 5 色）：维护者是否接受。
- 另有 5 项原文档遗留决定已按推荐确认（见 `PLAN_PHASE2.md` 第二节）：Lite 镜像坐标保持不变、签名信任根采用 GitHub OIDC / Sigstore 但本阶段不接线、原生安装路径、Desktop 先 macOS 再 Windows、自动更新全部默认关闭。

## 五、已知覆盖缺口与风险

- 结构评估的缺口（评估报告第 8 节）：账务子系统与任务恢复只读到文件级；三种数据库的实际迁移没有运行验证（CI 的数据库作业覆盖其中一部分）；`tools/` 约 4.2 万行脚本、`website/`、`deploy/` 未评估。
- 全量前端测试在本机（macOS、Node 26）有环境性失败：两个 `credential-*` 存储类用例基线就失败，高负载下 `strict-parameter-selection`、`account-switch`、`handoff-lifecycle` 偶发失败。CI（Linux）上这些都通过，以 CI 为准。
- 子代理的汇报都不是已验证事实。本批里主会话亲自重跑过的验证已在对应 CI 记录中。

## 六、接手后第一轮 CI 变红时的优先排查点

子代理自己列出的最担心处（**[读码]**，未验证）：
- 5b：吸顶保存栏的浏览器断言（"Qualify shared UI" 步骤）；`system-status.test` 里带参数的路由 Link 的 href 断言；设置卡片统一字号覆盖所有分区后的视觉；用户手机记录依赖用量单元格渲染，`personal-status` 的精确文本断言不能出现第二处匹配；个人资料卡片改用默认间距与 h2 后的测试依赖。
- 5c：公共顶栏从 fixed 改为 sticky 并去掉各页 `pt-16`/`pt-20`，320 宽度可能溢出或遮挡；`rounded-card` 与组件自带圆角类经过 `cn()`（tailwind-merge）合并时两者共存的冲突（已修 Skeleton 与 InputGroup 两处）；`--radius-card` 运行时是否存在；订阅与兑换码在 640–1023 宽度的分页与固定高度；充值卡在 320 宽度下标题与按钮并排；提示词学习页改用 `SectionPageLayout` 后的单测。
- 偶发项：`window-maintenance` 的"关闭订阅池"对话框隐藏超时一次，重跑通过。如果再出现，先判断是否为对话框退出动画（`data-ending-style`）未结束，再决定是否改脚本等待方式；不得放宽断言。

## 七、重要约定（避免重复踩坑）

- oxlint 规则 `curly: multi-line`：`if` 的语句体换行时必须加花括号（同一行单语句允许）。这条规则让 CI 红过两次。测试里用 `.querySelector()` 而不是 `.getElementById()`。
- GitHub CI 对同一分支的新推送会取消还没跑完的旧运行，所以攒好几个提交再一起推送；一轮约 30 分钟。
- 修复某个失败前先从 CI 日志找到具体断言和文件，不要凭推断改（本批里一次凭推断改错了位置）。
- 浏览器验收脚本里，"遮挡 / 裁切 / 不可读 / 功能不可达 / 用量未知不补零 / 不重放写入"类断言不得放宽；只有需求确实变了的断言才改，并写明依据。
- 每个提交只含自己的文件（路径限定提交），避免并发会话互相带入暂存内容。
