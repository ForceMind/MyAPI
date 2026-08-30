# MyAPI 品牌与归属审计清单

这份清单把运行时品牌、发行元数据、法律归属和兼容字段分开，避免为了视觉替换误删必须保留的许可证信息。

## 每次发布前检查

在仓库根目录执行（仅用于人工快速定位）：

```bash
git grep -n -I -i -e 'new api' -e 'quantumnous' -e 'new_api' -e 'new-api' -- \
  ':(exclude)docs/**' ':(exclude)patches/**' ':(exclude)NOTICE' ':(exclude)LICENSE*'
```

`git grep` 只检查已跟踪文件；它不能代替下面的分类审计。

命中项必须逐项归类：

- **必须替换**：页面标题、默认站点名称、About 归属、公开产品链接、容器/镜像默认名和用户可见文案。
- **兼容保留**：`NEW_API_*` 环境变量、旧数据库字段、迁移注释和兼容路由。它们不得成为新安装的默认值、用户可见品牌或运行时主叙事；发行包可在有迁移说明的前提下保留必要兼容资产。
- **法律保留**：`LICENSE`、`NOTICE`、源码版权头及其链接。未经法务确认不得删除或改写；发行说明应明确这些内容的来源和适用范围。
- **审计标记**：内部错误码、历史迁移标识和补丁文件中的上游路径。它们不应出现在用户界面或默认日志中。

自动化审计（只读取 Git 已跟踪文件，不读取 `.env`、数据库、日志、依赖或构建产物）：

```bash
node tools/branding/check.mjs
node tools/branding/check.mjs --json > brand-audit.json
# 也可以通过 npm（使用 --silent 以保持 JSON 输出可解析）
npm run --silent brand:check -- --json > brand-audit.json
```

检查器只对公开发行面中的未解释旧品牌阻断；`LICENSE`、`NOTICE`、源码头、补丁、
兼容 wire header 和审计文档会输出分类信息但不会被误报为品牌泄漏。输出只包含
文件路径、分类和规则名称，不包含文件内容。

CI 应把 `blocking_count` 作为唯一失败条件；`findings` 中的 `legal`、
`compatibility`、`source-header`、`audit-document` 和 `audit-tool` 项是有意保留的
审计记录，不代表运行时品牌泄漏。发布前若新增 `public` 阻断项，应先检查其是否为
新安装默认值或用户可见文案，再决定替换或补充分类规则。

## 已实现边界

- 新部署默认使用 `MyAPI`、`my-api` 和 `ghcr.io/forcemind/myapi`；LAN Lite 使用独立的 `myapi-lan` 镜像。
- 自定义站点名称与 Logo 由管理员设置优先，构建参数只提供默认值。
- NPM 包、Docker 镜像、静态官网和 Electron 工作流均使用 MyAPI 发行标识。
- 原项目法律归属不在本阶段擅自删除；后续若要改变 `NOTICE` 或源码头部，必须先完成法律审查并单独提交。
- 根 Docker Compose 的新安装默认数据库名为 `myapi`、`myapi-log` 和
  `myapi_logs`，分别可通过 `MYAPI_DB_NAME`、`MYAPI_LOG_DB_NAME` 和
  `MYAPI_CLICKHOUSE_DB` 覆盖。已有旧数据卷迁移时必须显式设置当前库名；
  Compose 不会隐式重命名或迁移数据库。

## 验收标准

运行时页面、默认 HTML、镜像标签和 CLI 输出不得出现未解释的旧产品名称；兼容字段不得成为新默认值、用户可见品牌或运行时主叙事。为保障升级和旧环境恢复，发行包中允许保留经过审计的兼容资产（例如旧服务名/数据库迁移标识），但必须有迁移说明、不得作为新安装默认名称，并应在未来主版本评估移除。法律文件和兼容回退均有文档说明。该审计不等同于 NPM 发布或生产部署批准。

## 内部协议迁移边界

以下字段虽然含有旧标识，但已经是运行中的安全或兼容协议，不能通过一次
全局替换处理：

| 协议/字段 | 当前用途 | 安全迁移方式 |
| --- | --- | --- |
| JWT issuer/audience 与签名域 | 令牌验证和跨节点会话一致性 | 先支持 MyAPI 新值签发与旧值只读验证，完成会话轮换后再退役旧值 |
| `new_api_refresh` Cookie | 浏览器刷新会话 | 双 Cookie 读取、单一新 Cookie 签发，等待旧会话自然过期 |
| `new-api:*` Redis namespace | 渠道亲和性、缓存和锁 | 双读/单写或版本化 namespace，避免切换时丢锁和缓存污染 |
| `new_api_error`/`new_api_panic` 等错误类型 | 客户端错误解析与兼容测试 | 增加 provider-neutral MyAPI 错误类型并保留旧值兼容期，按 API 版本退役 |
| `X-New-Api-Other-Ratios` 等 wire header | 上游/客户端协议兼容 | 新旧 header 并读，明确弃用日期，不改变既有数值语义 |

迁移前必须补充：影响面清单、双读优先级、回滚条件、跨节点部署演练和
客户端兼容测试。法务保留项（`LICENSE`、`NOTICE`、源码版权头）不属于
上述运行时迁移，须单独取得法律意见。
