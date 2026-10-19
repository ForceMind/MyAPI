# OAuth前置核查与隔离验证证据

对应[核查与候选适配方案](../../PERSONAL_CONSOLE_OAUTH_PREFLIGHT_20261019.md)。日期2026-10-19，应用仍为`0.2.0-beta.20`。本目录包含合成测试输出，不含真实账号或凭据。Token/JWT/账户名及额度值均为fixture，不能用于真实授权或账务。

## 来源和结果

- 当前分支`codex/subscription-key-share-20261014`；准确被测来源`3b5f5b26431a51b018252a2b61b51dbd8f5fe90d`、tree `a4268033b5abe8cc0eeccdefaba865e14a44795e`。从git archive建立临时副本，只加本目录两个测试覆盖文件；未修改产品实现。后续docs归档提交不是新的被测应用源码。
- [source-baseline.json](source-baseline.json)：准确来源、overlay位置、范围及受保护用户文件摘要。
- [source-integrity.json](source-integrity.json)：逐个核对临时副本中1499个已跟踪Go/module/VERSION文件的Git blob摘要，全部与准确基线相同；两个新探针为额外覆盖文件。
- [commands.json](commands.json)：最终执行的真实argv、cwd、环境覆盖、选定测试名与来源文件；不是展示用伪命令。
- [execution-results.json](execution-results.json)：最终三个进程实际退出码/墙钟时长；[run-tests.py](run-tests.py)为本次驱动原件。报告中的Go包耗时和该墙钟时长分别包含不同的编译/进程开销。
- [test-summary.json](test-summary.json)：从原始日志生成，最终**68顶层/118叶子通过**、0个失败条目。只计final，父/子不重复相加；早期重试不计入。
- [independent-review.md](independent-review.md)：非测试实现者对协议、输出、范围和结论的审查。
- [manifest.json](manifest.json)：除自身外全部归档文件的字节数和SHA256，含原始stdout/exit/测试源码；摘要不代替执行结果。

## 原始文件

| 范围 | stdout/退出码 | 本轮结果 |
| --- | --- | --- |
| service最终 | [stdout](logs/service-final.stdout) / [exit](logs/service-final.exit) | 15顶层/57叶子通过；含一个自动刷新探针 |
| controller最终 | [stdout](logs/controller-final.stdout) / [exit](logs/controller-final.exit) | 38顶层/45叶子通过；含三个顶层/四叶子探针 |
| model最终 | [stdout](logs/model-final.stdout) / [exit](logs/model-final.exit) | 15顶层/16叶子通过 |
| 首次controller沙箱 | [stdout](logs/controller-initial-sandbox.stdout) / [exit](logs/controller-initial-sandbox.exit) | 退出1；httptest创建loopback socket被沙箱拒绝 |
| 首批controller获准loopback重试 | [stdout](logs/controller-initial-loopback.stdout) / [exit](logs/controller-initial-loopback.exit) | 同源码9顶层通过，退出0；与final中的重复案例不另计 |

早期两条controller日志由探针代理生成，精确原始regex未单独保存，故只作环境失败/恢复的辅助证据，不作为最终可复验案例集。最终三个命令和原始退出码完整保留；早期service/model回归被最终精确集覆盖，不额外叠加计数。

原始stdout保留Go/logger的CRLF、ANSI和行末空格；本目录`.gitattributes`仅对这些stdout设置`-text`，防止Git换行规范化破坏摘要。编辑文档通过whitespace检查，原始日志不做格式修整；最终另核对Git blob与manifest字节。

临时测试源码保留原字节，使用`.go.txt`后缀，避免文档目录意外成为Go包：

- [自动刷新探针](probes/codex_credential_refresh_task_probe_test.go.txt)：25h不刷；1h连续两个显式单轮都刷新，不用真实10分钟等待或改时钟。
- [查询/取消/关闭调度探针](probes/codex_oauth_isolation_probe_test.go.txt)：401和403分别观察实际HTTP发送边界的三请求；手动deadline续期；取消后下一个channel零查询且旋转落库；关闭scheduler开关仍能运行batch。

每个探针只描述现有行为，不提供新SIWC、统一计数或停止功能。既有测试含fake transport、httptest recorder和loopback server；所有数据库为新建临时SQLite。未启动MyAPI主进程或读取真实配置/数据库。SQL错误行包含故意模拟的持久化失败等情况，以测试断言和实际退出码判定，不能把日志中单个ERROR词当作测试失败。

## 复验方法和限制

1. 在新的隔离临时目录对准确来源执行`git archive`，不要复制带`.env`/私有目录的工作区。将两个`.go.txt`按source-baseline的overlay路径复制为`.go`。
2. 按commands.json的每条env/argv串行执行，仅替换cwd和输出目录；可在副本调整run-tests.py中的本次绝对路径。需要本机loopback权限，仍用`GOPROXY=off`和已有缓存，不安装依赖或调用供应商。
3. 核对每包退出0、选中顶层15/38/15及stdout里的精确观测。manifest可用Python hashlib逐项校验；manifest本身是归档索引，不是供应商证明。

`HTTP派发次数`为fake transport观察到的MyAPI出站边界调用，不是浏览器IdP导航或真实供应商收到的次数。取消测试只证明下一候选不查询，不能撤销已派发的操作。未验证真实OAuth、JWKS轮换、原生窗口/重置、真实429、实体Passkey、MySQL/PostgreSQL新探针或生产恢复。本轮没有页面行为改动，未生成新页面/截图；既有beta.20实际页面和44张原图沿用第一阶段交付，不能冒称本轮新协议视觉验收。

本轮未操作真实凭据/生产数据/历史账务，未合并main、打标签、发布或部署。beta.21/beta.22仅为核查文档中的待审候选范围。
