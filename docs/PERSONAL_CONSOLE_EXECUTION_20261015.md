# 个人控制台第一批实施记录

已批准依据：`PERSONAL_CONSOLE_IMPLEMENTATION_HANDOFF_20261015.md`、prototype-v0.1、visual-v0.2。七张最终 PNG 已由主智能体实际打开检查。2026-10-15 开工基线：`codex/subscription-key-share-20261014` / `5d8cb5ac25942aeb9b27b6d4460e22a7c40a8dc5` / beta.14，远端同 SHA；既有 CI 38570694456 success。保留开工时两份已有文档差异及未跟踪设计/原型。

## 范围与实施顺序

1. 主代理：共享个人外壳、原 M logo、224px 深色侧栏、四项移动底栏及设置次入口；仅由已确认服务端 disabled 模式启用，保留原商业外壳及权限。新增 `/upstream-accounts`，复用渠道管理与登录。
2. overview_upstream：概览和上游账号，复用窗口观测、本人日志、现有 Keys 与 subscription-share API。未知/过期/失败独立显示，真实窗口分别表达。只读池视图补 Root-only channel_id，避免同名渠道误关联；不新增数据库字段/迁移。
3. profile_logs：身份、仅本人只读范围、偏好和安全入口；个人日志列表/详情。成功结果与缺 usage 分开展示，Token 未知；失败可重试且导航可用。
4. keys：已有 Key 列表及分额表单/预览，沿用 CAS、操作幂等、未知保存保护、暂停/恢复。调整保留已用；20+50 可存、60+50 拒绝、0 停后续请求、100 合法，不预留容量。
5. 主代理集成翻译、候选 beta.15、测试和普通 Actions；非实现者独立审查代码及实际浏览器截图，修复范围内问题后同步同一功能分支和 Draft PR #17。

## 文件所有权与资源

最多四个活跃角色，无递归。当前代理继承主会话模型/强度；工具未返回实际模型标识，不声称切换至 Terra/Sol。主代理持有 components/layout、styles、routes、locales、VERSION、只读 Go 补齐及综合浏览器/CI；overview_upstream 独占 personal-workbench、personal-upstream、对应浏览器脚本；profile_logs 独占 profile、usage-logs 及 profile/personal-status 浏览器脚本；keys 独占 keys 及 subscription-share 浏览器脚本。同一文件单写者。

保留beta.14后端语义、渠道登录、Key身份、权限、商业模式及所有历史账务。禁止生产数据/凭据、历史副本、恢复worker、生产启动/迁移、main合并、标签、发布、部署。仅普通验证和已授权功能分支 Git 同步；不新建远程仓库。

## 验证与交付条件

先相关行为测试、TypeScript、涉及文件lint、七语言i18n，再复用普通CI的构建/Chromium/后端兼容回归。本机未安装前端依赖和浏览器，优先Actions，不重复安装重型缓存。浏览器以隔离合成API运行真实编译页面，覆盖1280/1440和320/390、长文、导航、焦点/键盘、资料失败重试、成功缺usage、分额边界；保存实图并人工查看。合成浏览器不能证明真实上游或生产状态。

交付包含可运行页面构建、真实桌面/手机截图、准确源码/版本、测试范围、独立审查、剩余项。beta.15远端标签查询尚未占用，本批只更新候选元数据，不打标签。风险：旧浏览器驱动绑定旧UI，需更新定位并保留行为合同；后端未提供的HTTP/usage字段必须显示未知，不能填设计示例。

## 阶段状态

- 基线与设计核对：已完成。
- 外壳、页面及只读关联字段：进行中。
- 翻译/集成/验证：未开始。
- 独立审查/截图/分支同步：未开始。

持久 Goal 已创建；阶段结果和CI/源码证据在此追加。只有全部交付条件满足才标记完成。

### 第一轮实现集成

外壳及五个页面已写入；新增131条英文源文案并经规定脚本填写七语言，i18n sync七语言missingCount=0。个人Keys移除钱包策略入口；旧商业界面的策略/预算回归改为明确commercialShell场景，新个人浏览器覆盖无金额/成功缺usage/分额及恢复。

独立静态审查已修P2：日志跳转Keys的filter未承接、手机版本不可见、未知基线误显示0、外壳缺旧维护页面所需上下文；头像与页面动作预留间距待实图核对。调用记录按user/role/session隔离并使用Intl合法locale，金额不展示，缺可靠HTTP字段显示未知。

本机已执行Node fixture测试和浏览器驱动语法检查、Bun轻量TSX解析、diff检查；本机Go定向测试因只读默认GOCACHE未执行，未额外建立重缓存，交CI。无node_modules，Vitest/tsgo/lint/生产构建/Chromium尚待CI。CI把同一生产构建提前至typecheck之前，由TanStack插件生成新增路由类型；不重复构建，保留构建+路由产物供运行预览和回读生成文件。

首轮源码 `80594a49216400afa215f468b9363ac2d9c24e20` 已同步 Draft PR #17，普通CI `38743485122` 启动。生产构建成功；typecheck发现新外壳固定URL未按TanStack动态路由声明、只读数组传入可变导航类型，以及3个fixture/测试API类型问题，均按错误逐项修正。保留失败日志 `/tmp/myapi-console-80594a4-frontend.log`，不以语法解析代替类型检查。分发合同亦失败，正在定位。历史数据与生产未操作。

修订验证：使用项目固定oxfmt 0.57.0和oxlint 1.72.0轻量工具整理涉及文件（保留原版权头），未安装完整前端/浏览器依赖。42个涉及TS/TSX文件lint通过；模板候选版本定向CLI测试在正常子进程权限下通过。第一次本机完整CLI测试在受限沙箱仅返回文件级失败、无具体用例证据，保留 `/tmp/myapi-console-cli-tests.tap`，不作通过结论。部署模板已统一beta.15且保留未发布说明；新增RELEASE_BETA_15.md。综合浏览器独审补强输入/输出Token各自Unknown、fixture时钟及准确CI SHA，分工明确覆盖五个页面。
