# 当前 Go 可达性取证

本迭代只为**指定提交、指定工具、指定目标**收集静态可达性证据，不删除、重命名或重构产品代码。报告固定包含 `analysis_only: true` 和 `deletion_authorized: false`；`manual_review_candidate` 也只表示可以人工复核，不能作为自动删除清单。

## Actions 入口

使用已注册的 [CI 入口](../../.github/workflows/ci.yml)，手动设置 `run_reachability=true`，调用可复用的 [Go reachability evidence](../../.github/workflows/go-reachability.yml)。该开关默认 `false`，不自动开展 SSA 分析。当前只接受手动 `dev` 请求：调用方 ref 必须为 `refs/heads/dev`，传给分析工作流的 `source_ref` 固定为 `dev`；不支持任意 ref。checkout 固定该 run 的实际 SHA 并再次核对。分析和构建全部在 Actions；本机不安装或运行 deadcode、SSA、Go 构建、Node 合同测试、Docker 或浏览器。

维护者可执行下面的一次性手动请求；它不是后台或定时任务，也不需要为了注册新工作流合入 `main`。所有既有完整 CI 门禁仍会执行，不是跳过原有测试。示例明确关闭浏览器；可达性分析自身不启动业务应用或执行 Go 测试二进制：

```text
gh workflow run ci.yml --repo ForceMind/MyAPI --ref dev -f run_browser=false -f run_reachability=true
```

工具固定为 `golang.org/x/tools/cmd/deadcode@v0.38.0`。每个目标都必须独立完成以下三个输入，不能用一个目标的结果补另一个目标：

| 目标 | 根模块生产入口 | 根模块含测试入口 | 独立 relaykit 含测试入口 |
| --- | --- | --- | --- |
| linux/amd64 | 必需 | 必需 | 必需 |
| darwin/arm64 | 必需 | 必需 | 必需 |
| windows/amd64 | 必需 | 必需 | 必需 |

每个 job 在 Ubuntu runner 上运行，按目标设置 `GOOS/GOARCH`，固定 `CGO_ENABLED=0`、默认构建标签与 `GOWORK=off`，先执行 Node 报告合同、根模块构建及 `relaykit` 独立构建，再运行三次 SSA 分析。只为满足 Go embed 的编译输入在 runner 创建空 `web/dist/index.html`：这不是完整前端、可发布二进制或产品构建验收。交叉构建不是在 macOS/Windows 上运行应用，也不是 GUI、供应商、生产、动态调用或全部构建标签的验收。`relaykit` 必须以独立模块边界处理，不能因为根应用没有使用其某个入口就认定它没有外部消费者。

Node 合同入口为：

```text
node --test tools/go-reachability/summarize.test.mjs
```

这条命令也只在 Actions 执行。测试自建临时 Go 源码和工具 JSON，不调用 Go、deadcode、Docker 或产品进程。

## 输入和失败语义

`summarize.mjs` 导出 `createReachabilityReport`，接收仓库根目录、实际 checkout SHA、固定工具身份、`{goos, goarch}`、run ID/attempt，以及三个扫描的原始 JSON 文本和退出码：`root_production`、`root_with_tests`、`relaykit_with_tests`。

deadcode v0.38.0 的包记录形状为：

```json
[{"Name":"package-name","Path":"module/package","Funcs":[{"Name":"function","Position":{"File":"file.go","Line":1,"Col":6},"Generated":false}]}]
```

退出码为 0 且文本为 JSON `null` 或 `[]`，才是合法的零记录输入。空文件、损坏 JSON、工具失败、缺少任一输入、重复冲突、仓库外路径、符号链接逃逸或无效符号位置必须失败，不能解释为零候选，也不能产出一份看起来成功的部分报告。

每条记录保留模块归属、包、符号、仓库相对文件、行列、文件 SHA256、两类扫描的不可达证据、测试入口可达性、导出/方法属性、保护原因和人工复核标记。文件哈希绑定本次源码字节；行列与源码符号不一致时拒绝猜测函数范围。报告头的 source SHA、工具 pin、目标、run ID 和 attempt 必须同时匹配，旧报告不得重标为新提交。

原始 deadcode JSON 本身不带源码哈希或 run 签名，位置匹配也不能单独证明 SSA 来自当前 checkout。同一次 Actions 的源码记录、真实扫描退出码和完整日志是来源链的一部分；不能手工修改 metadata 或复制旧 JSON 冒充新的执行结果，无法核对时应记为证据未验证。

汇总命令由工作流调用，不在本机重跑：

```text
node tools/go-reachability/summarize.mjs --repository-root PATH --evidence-dir PATH --output PATH
```

证据目录包含 `metadata.json`，以及 `root-production`、`root-with-tests`、`relaykit-with-tests` 各自的 `.json`、`.exit`、`.stderr`。原始退出码与 JSON 是汇总输入，stderr 和构建日志用于定位失败。报告通过后才打印 `reachability_metadata` 和逐条 `reachability_candidate` JSON 日志；后者包含受保护记录，必须看 `protections` 与 `manual_review_candidate`，不能仅看日志类型名称。

`summary.records` 是本次工具记录的去重并集；`summary.manual_review_candidates` 只是其中未被保护的人工复核项数量。后者为 0 可能表示记录全部受保护或不能可靠解释，不等于工具没有报告不可达符号。

## 如何解释记录

- 生产扫描不可达、含测试扫描不再不可达的根模块函数，标为测试入口保留，不能当作可删代码。
- 根模块的导出函数（含 Unicode `Lu` 大写首字母）、`init`、所有方法、生成代码及测试源码均受保护。GORM、JSON、数据库驱动等框架钩子可能经动态调用触发，不能从静态缺边推断安全删除。
- 匿名或无法可靠识别的符号只能保留人工保护标记，不能猜成普通私有函数。
- 所有 `relaykit` 记录保留独立模块/公共边界保护；根应用视角不能证明其 API 对其它程序无用。
- billing/quota 等相邻名称或路径只是复核提示，不证明函数参与计费，也不证明它与计费无关。金额、权限、身份、存储和恢复等语义仍需按具体代码审查。
- 即便某个私有普通函数在两个根模块扫描中都不可达，结论也只限本次入口、目标和默认构建条件。反射、插件、未知外部消费者、其它标签和部署组合不在此报告的完备性承诺中。

历史“37 项”原始候选名单不在本工具输入中。本次不会按数量或旧描述重建那份名单；本次结果与历史数量相同也不能证明是同一批符号。

`billing_adjacent` 始终附带 `billing_basis: "heuristic-only"`，不增加“已确认计费”或“已确认可删”含义。`source_range` 只有在保守源码识别可确认时存在；匿名/未知符号或复杂签名不能确认范围时保持人工保护，不据简单文本匹配放行。

## 不下载构件也能复核

在 Actions 页面逐个打开目标 job，读取 checkout 身份、工具版本、三个扫描退出码、构建步骤和报告打印步骤的完整日志。使用已登录的 GitHub CLI 也可以只读日志：

```text
gh run view RUN_ID --attempt ATTEMPT --log
gh api repos/ForceMind/MyAPI/actions/jobs/JOB_ID/logs
```

这些命令用于读取日志，不是 `gh run download`；不需要下载构件或在本机重跑分析。先核对实际 checkout SHA，再核对报告中的目标、pin 和 run/attempt；只有 summary 数字或构件已上传，不能代替逐项读取失败状态和完整记录。任一输入/构建未完成时，应报告缺失证据，不把它记为“无候选”或“全部支持”。

## A5 当前短函数 AST 清单

独立的 `short-functions.go` 只读 Git tracked Go 源码；两个 Go 文件均使用 `//go:build ignore`，需在 Actions 显式指定文件运行：

复用CI的`run_cleanup_inventory=true`手动dev入口，保持`run_browser=false`；该开关同时收集当前前端files/exports/types清单，不改变产品或自动删除。普通push不启动清单扫描，完整CI原门禁始终保留。历史数量不能替代本次来源与日志。

```text
GOWORK=off go test tools/go-reachability/short-functions.go tools/go-reachability/short-functions_test.go
GOWORK=off go run tools/go-reachability/short-functions.go --root "$GITHUB_WORKSPACE" --source-sha "$GITHUB_SHA"
```

本机不执行这些命令。CLI 在扫描前后核对完整 HEAD、tracked tree 和 tracked Go 文件列表；脏树、解析失败、缺失或越界源码、符号链接均失败，不补成零。输出是标准库 XML，根为 `short_functions_report`，固定保留 `files`、`records`、`references`、`protections` 容器；未知计数用 `unknown="true"` 空元素，不能当0。没有 JSON 序列化、业务导入、类型检查或产品初始化。

清单包含物理声明跨度不超过15行的函数及保护原因，并另列含花括号的 `body_lines`。引用只在同目录、同 package 的裸标识符范围观察，定义不算引用；方法引用不作类型解析。导出 API、方法、init、生成/测试/fixture、relaykit、构建约束或重复定义，以及非直接引用均保护。`manual_review_candidate` 仅表示一个私有普通函数在该词法范围恰有一个直接生产调用、没有直接测试引用且无保护项；动态引用、外部消费者、实际 build-tag/平台可达性和安全内联仍需人工核对，`analysis_only=true`、`deletion_authorized=false` 始终保留。它不复原或凑齐历史790/396清单，也不自动删除产品代码。
