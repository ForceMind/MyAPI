# 历史兼容候选的云端验收

更新日期：2026-10-12。已验基线为 `1608f85f0b7b2882a91dd60c0525ccccc7e583df`，tree `be9453563816059afc753218840e41e7165f100d`；[CI 38142050913](https://github.com/ForceMind/MyAPI/actions/runs/38142050913)十项全部 success，独立终审通过。[草稿 PR #11](https://github.com/ForceMind/MyAPI/pull/11)保持 Draft。当前继续原金额方案的 Key 调整幂等和派生总额兼容，不以基线绿灯替本批记通过。历史账本仍未核清，生产保持不动。

## 当前开发：Key 额度重试与派生总额（待验证）

### 4fe12f4 的夹具回归与后继修正

`4fe12f46bf09706a5e1f3e978d166dd4bcc3d4c7` 的 [CI 38146609291](https://github.com/ForceMind/MyAPI/actions/runs/38146609291) 前端类型检查和 lint 成功，Test 实际为189文件/1145测试通过、1文件/2测试失败，后续构建和浏览器步骤 skipped。新增双视图测试的 `/api/status` 空响应会异步重置配额显示，造成组件文本与断言格式不一致；视口及移动端标签也须按真实组件契约设置和匹配。后继仅修正这些夹具，保留边界、双分项、总额不可表示和无派生进度断言，不修改生产组件来迎合测试。

集成复核另确认两处旧测试接线需要更新：batch/cache 测试使用独立 SQLite 库，必须在自己的 AutoMigrate 中创建 `TokenQuotaAdjustment`；只验证响应 Key 脱敏的旧请求移除 `remain_quota`，明确采用 metadata-only，额度编辑脱敏仍由新请求回归覆盖。后端原始日志随后确认六项 cache 回归因缺表失败、上述一项 controller 脱敏测试因缺额度身份失败，均与这两处定位一致。该轮实际终态为八项 success、前后端两项 failure；18项/五包386步骤已成功，完整根模块测试失败后 relaykit 测试、审计和竞态步骤未执行。这些修正不改变生产代码、收据身份、账务或缓存政策。旧运行与失败日志保留；后继须由准确提交重新验收，不能把上述静态定位或其他专项 success 写成整轮通过。

### bc60788 的真实浏览器失败与布局修正

`bc6078850bcfde460c62b207d44122807c21d0bd` 的 [CI 38147494629](https://github.com/ForceMind/MyAPI/actions/runs/38147494629) 前端190文件/1147测试全部通过，type/lint/build及独立 quota/routing/access 浏览器步骤成功。共享 UI 报告为32旅程中31通过、1失败，42个设置深链通过，237张截图；新 Key 场景在320px因 exact 选择器缺少真实标签冒号而超时。报告中的1280正式截图存在、320仅有 failure 图，且失败页面文本直接证明冒号差异。该 compound 步骤失败后 Playground 未执行，不能记完整前端通过。

主线程和独立审查实际查看两张截图，另确认1280px长额度覆盖 Access profile，预算按钮被表格边界裁切。后继同时修正真实标签选择器和当前 Key 的局部布局：超界额度标签、完整值分行并在本列换行，预算按钮受可用宽度约束；保留金额与动作语义、Tooltip、移动卡片及所有原行为断言，新增列边界和重叠的浏览器检查。旧截图、失败日志及整轮实际终态继续保留；修正须由后继准确提交的实际渲染验收，不以静态选择器修正代替视觉通过。

### bc60788 的迁移恢复竞态失败与夹具修正

该轮最终为八项 success、前后端两项 failure。后端 root 与独立 relaykit 的 vet/build/完整测试、18项五包386门禁、63项审计及前七组竞态检查成功；最后 durable submission model 竞态步骤失败。`TestAccountQuotaMigrationResumesPartialHeadAfterDatabaseReopen` 预期 `context canceled`，实际错误链为 `context deadline exceeded`，不是作业总时限取消，也不是整套后端通过。

该测试原先创建201个独立请求，仅在最后一个 head 已保存、obligation 创建前取消。生产迁移函数保留5秒执行预算，并在整页完成后保存 cursor；文件 SQLite 在竞态检测下尚未到指定取消点，内部 deadline 已先发生。后继仅调整合成夹具：先由真实迁移建立一个完整请求，再加入第二个旧 receipt，并将合成 cursor 置为未完成、保留第一条的非零 LastID，表示已完成前页而后项待处理。主验证调用直接处理第二项，在原取消点中断，保留取消身份、未完成 cursor、部分 head、缺失 obligation、关闭重开、重复恢复及原身份/金额/计数断言，也核对第一项身份不变。不改生产执行预算或接受 deadline 替代取消；450行分页、1001行预算和201条长链测试保留。修正后的完整测试与竞态结果仍待准确后继的 Actions 验证。

同一 bc60788 的 B2 与 Redis 作业已成功并通过独立只读结果审查：MySQL5.7.44、PostgreSQL9.6分别实际执行当前金额 schema 和新 Key 调整合同，零跳过；22表41列是当前源码范围，不能外推任意旧窄列迁移。Redis7.4.11共51项各一次 RUN/PASS、零 skip/fail。两个成功专项不覆盖上述前后端失败，也不证明历史账本或生产升级资格。

### e3bfbb1 的手机总额定位修正

`e3bfbb1a952098e9c790b1d5d71925c21eafade4`（tree `3ae692e06ce8fadeead79b67f0efca24a00cc8ac`）的 [CI 38150140466](https://github.com/ForceMind/MyAPI/actions/runs/38150140466) 前端仍为 failure：190文件1147测试、type/lint/build、独立 quota/routing/access 和 profile九场景成功；共享 UI 为31/32旅程、42设置深链、237截图，Playground未执行。此前修正遗漏了手机总额度的实际DOM结构：label子节点与 `Unavailable` 直接文本共用父span，页面完整文字为 `Total quota: Unavailable`，全页 exact `Unavailable` 无匹配。主线程和独立审查实际查看正式1280与failure320截图，确认双分项完整、额度不再覆盖相邻列、按钮不再裁切；1280先于320执行，原桌面几何断言已成功，不能记未执行。手机实际显示正确，但正式320截图仍缺。

后继仅修改浏览器断言、该预算按钮间距及本记录三个文件：绑定真实总额度label的父容器，规范空白后精确比较整条 label 与 `Unavailable`，保留双分项、无progress、所有桌面边界及双截图要求；预算按钮由默认 `px-3` 改为 `px-2`，避免宽屏中 `budgets` 最后一个字母单独换行。后者仍须新1280/320正式截图确认，不以静态尺寸推测记视觉通过。无金额、权限、API、生产迁移、依赖或workflow改动；后继保留该轮后端至终态后才提交，准确新提交的完整验收仍待结果。

该轮现已终态九项 success、前端一项 failure。后端 root/独立relaykit完整test、38618项五包、审计63及全部八组race步骤成功；最后durable race的准确selector包含新的两请求恢复fixture，model包实际成功。此命令未使用 `-v`，不声称日志含该单项RUN/PASS计数。MySQL/PostgreSQL共享B2合同129次唯一RUN/PASS、Redis51次唯一RUN/PASS均零skip/fail，独立专项结果审查通过。这些成功不覆盖前端失败或历史账务缺口。

### 3f69794 的抽屉重开测试时序

`3f69794c2541ffbd6c06fd48842f0a48e2f64da7`（tree `abaaadbaeeca237645ad639df9109970e90c66f5`，实际PR检出 `768825ad5adb296da6a16c4db4fde06a894f2675` 同tree）的 [CI 38152144218](https://github.com/ForceMind/MyAPI/actions/runs/38152144218) 终态九项success、前端一项failure。前端类型检查和lint成功，单测189/190文件、1146/1147测试通过；唯一失败为关闭重开抽屉后第二个PUT缺少 `request_id`。后续build和全部browser步骤未运行，artifact为0，不能复用上一轮截图称本轮布局验收通过。后端root/独立relaykit完整测试、38618项五包、审计63及八组race步骤成功；MySQL/PostgreSQL B2的129次唯一RUN/PASS和Redis7.4.11的51次唯一RUN/PASS均零skip/fail，独立专项审查通过。这些成功不抵消前端失败。

主线程与UI实现者核对到测试等待条件的竞争：关闭重开后同名旧输入仍在，`findByDisplayValue` 可在异步读取完成前返回。界面这时已通过 `aria-busy`、`inert` 和禁用保存阻止操作，测试的 `fireEvent.change` 却仍可修改旧输入；随后初始化恢复原100额度并清除dirty，第二次PUT因此成为省略额度身份的元数据请求。此解释须由受控用例及后继云验收检验，不声称已在生产复现。

本批只改该额度编辑测试和本记录：helper等待真正初始化后编辑；用受控Promise挂起重开时的详情读取，检查加载屏障，完成读取后核对100基线，再改为120并提交。两个请求保持相同Key、版本、元数据指纹和120意图，保留且加强第二次额度payload、两个ID存在且不同的断言，使关闭清理成为新ID的必要原因。不改生产表单、金额、权限、API、依赖、workflow或任何执行预算，不用sleep、弱化断言或重跑掩盖失败。本机仅静态检查；后继准确提交的完整Actions、正式1280/320截图和Playground结果仍待验证。

### 39d6cb0 的测试类型检查失败

`39d6cb0a67d7794a7ceffe4130ac5dc8f84c3e86` 的 [CI 38153794478](https://github.com/ForceMind/MyAPI/actions/runs/38153794478) 前端在类型检查即失败：新增完整payload比较直接展开 `unknown`，触发TS2698。单测、构建及浏览器均未执行；此前独立静态审查不等于类型检查通过，不能据此称受控重开用例已验证。

后继仅在这份测试增加非空、`typeof === 'object'` 的运行时检查，使异常请求体明确失败、合法请求体可安全展开；完整payload相等、金额及身份断言保持，不使用类型断言绕过检查。生产Go和全部后端测试与已验3f69794相同；正常后继推送可替代尚未结束的39运行，取消项按真实终态保留，不记通过。新准确提交的类型检查、受控用例和完整CI仍待Actions结果。

### a34816b 的自动检查与桌面视觉拒绝

`a34816be2f613835bc8bb7b449f49dbc3be2175e`（tree `999b5404811cf7111f61bf46ed35b5a857c88e94`，实际PR检出 `3585cd57480c5f2947d70d2e2b9849cc853065c8` 同tree）的 [CI 38154277053](https://github.com/ForceMind/MyAPI/actions/runs/38154277053) 前端作业成功：typecheck、lint、190文件1147测试、build、独立 quota/routing/access、共享 UI 32/32旅程、42/42深链、238正式截图、profile九场景及 Playground 8/8旅程均通过。已下载 artifact 并核对摘要、报告和截图绑定；主线程与独立审查者实际查看正式1280/320图，320通过，1280仍将 `Usage budgets` 显示为 `Usage` / `budget` / 孤立 `s`。额度完整、未覆盖相邻列、无progress、按钮边框未裁切，这些成功不能抵消该视觉失败。整体视觉审查为FAIL，未记候选验收完成。

水平按钮中的图标和间距继续占用窄单元格宽度，`px-2`未给 `budgets` 留足一行，现有scroll边界断言没有识别词内断行。后继只改该按钮、既有浏览器场景和本记录：桌面断点将图标与文字纵向排列，手机保留水平布局；label保留完整翻译和受限宽度，长文案仍可换行。浏览器在真实文字节点上以 DOM Range 读取各英文词的视觉行，要求 `Usage` 和 `budgets` 各只占一行，保留全部金额、总额不可表示、无进度、列边界和双截图断言；不拆分文字节点绕过验证。新正式截图和完整Actions仍待准确后继运行，静态检查不代替实际渲染。

该轮现已终态十项success，但人工视觉审查仍为FAIL。后端root/独立relaykit完整测试、38618项五包各一次RUN/PASS零skip/fail、审计63及全部八组race步骤成功；durable非verbose selector包含恢复fixture且model包成功，不声称有该单项原始RUN/PASS计数。B2合成MySQL5.7.44/PostgreSQL9.6共129次、Redis7.4.11共51次唯一RUN/PASS均零skip/fail，独立专项审查通过。39d6cb0前轮已终态八项success、前端failure、后端cancelled，原失败不追溯改为成功。

本机仅 `node --check` 与差异检查，无构建、服务、浏览器或数据库副本。实现者只写上述两份UI文件，主线程维护文档和同步，独立审查者只读。所有旧失败继续保留；本批不改Go、金额、API、翻译键、依赖、workflow或预算，不额外dispatch Docker/native。历史缺口、DO_NOT_RESTORE、ledger_reconciled=false和production_eligible=false保持，未合并、发布或部署。

原方案要求显式 Key 额度修改具有持久请求身份；1608 基线只有版本与元数据 CAS，响应丢失后的重试不能返回原调整结果。本批在现有 PUT `/api/token/` 保留一次表单的名称、权限及额度原子修改，增加规范小写 UUID `request_id` 和不可变成功收据。收据以 `(user_id, request_id_hash)` 唯一；哈希及原身份、Key ID、包含字段存在性的客户端白名单指纹一起核验，避免 MySQL 排序规则把不同输入视为同一身份。持久指纹具有明确版本和固定规范编码，未来缺席字段不得改变旧请求摘要。重试在当前 owner 验证后、写入权限及版本检查前查询原收据；相同请求返回原结果和当前持久数据库行的脱敏 Key，不再次修改余额或发布旧缓存。该 Key projection 沿用原API数据库读取语义，不代表尚未drain的有效缓存余额；缓存离线也不阻止确认成功收据。异参数冲突，任何失败事务不留下收据或部分表单修改。

仅元数据请求省略 `remain_quota`；显式金额即使等于当前值也要求身份、原版本和元数据指纹，`null` 不能被静默视为元数据。金额、累计 used、负余额、无限 Key、writer 和缓存屏障沿用现有政策，不产生历史恢复许可。新表 `token_quota_adjustments` 的三个金额列加入迁移前物理检查，当前 schema 合同扩至22表41列；保留原13字段宽值往返和七类回滚断言，再用实际 Key 调整验证新收据的宽 before/used。

`GetTokenUsage` 分别校验 remain/used，组件合法而派生和超过 `M=2^53−1` 时保留组件，返回 `total_granted=null`、`total_granted_representable=false`；可表示时仍返回 number 和 true。Key 列表、详情、搜索不新增派生 DTO 字段。前端在真实 API 边界校验整数，桌面与手机共用先检查后相加的逻辑，超界时显示已有翻译并保留组件、停止显示比例。完整失败提交保留同一 request_id，修改意图或关闭、切换、成功后清理；后台 refetch 不替换打开表单时的版本。

实现分工为 Sol/high 核心与 UI 两代理、主线程迁移/三库接线/CI/文档、独立 Sol/xhigh 只读审查。共享新请求契约接入既有 SQLite 生产迁移与 B2 MySQL5.7/PostgreSQL9.6 临时服务；后两者使用两连接验证竞争。原17项386门禁保留并加宽收据金额/字符串版本合同为18项/五包。测试先写，未执行本地基线红例；本机只做静态、gofmt和必要 i18n 元数据检查，编译、测试及浏览器交给 Actions。`bun run i18n:sync`成功且无tracked locale差异；本地`format:check`因未安装node_modules/oxfmt无法执行，不记格式验证通过，也未为此安装依赖。不新增服务、job、权限、artifact 或历史数据库副本。版本仍为未发布 beta.11 候选，准确后继 SHA/CI 结果在 PR Checks 与正文单独回读；未发布、未合并或部署。

1608 的实际结果可复用为基线：17项386顶层测试/五包各 RUN/PASS 一次、零 skip/fail；B2 MySQL/PostgreSQL 完整 schema 子项各一次 RUN/PASS；Redis7.4.11共51次 RUN/PASS；前端187文件/1126测试、profile九场景及既有UI验收完成。它们不证明本批新增收据或 UI 已通过。旧 c94 386编译失败、3feff 前端取消和早期全包386/mock限制继续保留。

## 早期09ef源码的验收结果

被测应用与测试源码为 `09ef63268b5c6742fe73e246f6c5e26e30559903`，tree 为 `622566fb1c345343d1f10d7489e1af881469b2ac`。PR 测试合并提交 `4d42b99a7e17a2cc3086275abe768b40c1e71b5a` 的 tree 与之相同。[草稿 PR #11](https://github.com/ForceMind/MyAPI/pull/11)保持 Draft。后继 `3feff8e` 仅更新五份文档；再后一批只调整前端CI总时限和相应记录，应用与测试源码保持不变，不将后继提交称为旧运行实际检出的源码。

| 运行 | 实际结果与范围 |
| --- | --- |
| [CI 38128779978](https://github.com/ForceMind/MyAPI/actions/runs/38128779978) | 十项 success；根模块与独立 relaykit 的 vet/build/test、linux/386、竞态组、审计工具63项；前端187个测试文件/1126测试、个人资料9个Chromium合成场景；runtime三文件41项、零失败/跳过 |
| [Docker 38128779248](https://github.com/ForceMind/MyAPI/actions/runs/38128779248) | 五项 success：bundle、LAN源码安装、Full/LAN fresh legacy及Full handoff legacy；未发布镜像 |
| [原生专项 38132921489](https://github.com/ForceMind/MyAPI/actions/runs/38132921489) | `c8e7bbc` 上三个native job串行success，原十个job全部skipped；MySQL和PostgreSQL的父测试及指定engine子测试各实际PASS，零skip |

个人资料场景包括1280/320px正常读取、null设置、失败重试，以及1280px请求挂起、认证刷新挂起与启动读取挂起后的恢复。它们验证候选和合成API；生产个人资料故障尚未经过升级后验证。

Docker handoff固定旧源 `71277bf6055ce68b8cd1d11f1d10f1907d7696fd`：无结算历史旧库正向迁移与原镜像恢复通过；单次15额度请求后的历史副本被当前镜像以execution gate固定原因拒绝，退出1且非OOM，主DB/WAL存在状态及存在时摘要不变；再从未升级原备份恢复旧镜像，精确额度、有限Key和日志身份一致，上游总计仅1次。Legacy applied fact必需，intent缺失合法、存在时严格核验。本轮不证明快照一定含WAL，也不证明任意生产旧库的回滚资格。

### 原生专项的来源、执行与边界

用户已授权正常开发及本次临时合成服务。独立分支 `codex/historical-accounting-native-20261011` 的提交 `c8e7bbc04676e46f2ad0c806a9ac78562923a4ef`（tree `65e509daa2c9b03c4dffa9bcbf077c73602b1101`）唯一parent为上述源码，只改变 `.github/workflows/ci.yml`，不建立PR或合并main。排除该CI文件后，两提交的3067项路径/mode/blob清单摘要均为 `b48098d81d57f71c957a07c4cc1e3b046803dc5d498d5c102bcd05e6034b0acc`。native状态归于 `c8e7bbc`，不追溯写到 `09ef6326` 的Checks。

GitHub实际接受branch-ref新增inputs。guard核验event/ref/完整SHA、单亲关系、tree、唯一CI差异、洁净checkout与上述摘要，随后安全目标测试实际PASS。guard成功后MySQL启动，MySQL成功后PostgreSQL启动。MySQL使用 `mysql:5.7`，日志确认服务器5.7.44；PostgreSQL使用 `postgres:9.6`，未单独读取其patch版本。两者在不同runner分别动态绑定 `127.0.0.1:32768`，使用本次新空数据库或新schema；各自1 CPU、768MiB、256 PID、512MiB数据tmpfs及run/attempt标签的实态强断言通过。容器和专属网络均已回收。

镜像精确tag、image ID格式及限定仓库RepoDigest检查成功，具体ID/RepoDigest写入远端Job Summary；REST检查接口未返回summary文本，因此本地没有保存这些具体值。日志独立保存的pull digest分别为MySQL `sha256:4bc6bc963e6d8443453676cae56536f4b8156d78bae03c0145cbe47c2aad73bb`、PostgreSQL `sha256:caddd35b05cdd56c614ab1f674e63be778e0abdf54e71a7507ff3e28d4902698`，不将pull digest与image ID混写。

两库共享合同验证事务回滚无intent/fact/permit残留、宽余额保留、不可精确表示值及浮点物理列只读拒绝、未知fact冻结、许可绑定、资金侧成功/Key侧失败后的重开恢复及重放零追加。Go/tee/JSON解析失败、空选择或skip均导致步骤失败；两个成功步骤证明父项和指定子项各一次run/pass。完整JSON仅在runner临时目录，不上传artifact。成功步骤回显的脚本文本不单独作为PASS输出证据。独立只读审查通过。

同源码的既有CI还验证了Redis 7.4.11四个quota Lua脚本共44个子项PASS、零skip，本轮不重复创建Redis service。该旧作业只证明客户端loopback目标，host publish实际为all-address；宽余额、冷发布和其他缓存生命周期仍有仅miniredis/Go覆盖的部分。原生数据库专项同样不能补齐真实资金、预扣、缓存、全部writer、beta.3来源、504未知事件、旧终态凭证来源或实际回滚证据。

本机本轮没有Go/Bun/Docker编译、服务或新历史数据库副本。原225文件候选和全部旧失败证据保留。以下早期计划及执行记录保留当时状态；其中“待授权”“未验”“未提交”不覆盖本节最新结果。后继文档提交触发的常规CI须按自己的SHA另行回读，不额外手动重跑Docker或native。

### 文档后继CI：前端总预算不足，已保留失败并修正预算

`3feff8e0878a83e5f8e055cd5c198f67f9c2e1df` 只改变五份文档，其PR合并提交 `1d9190f29a43cc256b09c49cdefb91be29bbe5d4` 的tree与之相同。[CI 38133755286](https://github.com/ForceMind/MyAPI/actions/runs/38133755286)最终为9项success、前端cancelled；前端被平台15分钟总时限取消，annotation明确为`The job has exceeded the maximum execution time of 15m0s`，不能计为整轮通过。前端1126测试、profile九场景与31条通用UI旅程已通过；最后Playground命令尚未完成就被取消，artifact上传不代表未完成命令通过。原始日志、annotation及取消结果保留。

旧准确源码前端成功耗时899秒，已接近900秒上限；新旧profile约51秒、通用UI约426/432秒，尾段分别约74/75秒，没有新的断言失败或实质挂起证据。本批仅把frontend job总时限15→20分钟，给依赖安装、runner差异和完整末段留余量；全部步骤、脚本期限、断言和上传范围不变，不将加时本身称为修复通过。新准确提交的自动CI结果另行回读；不再手动运行Docker/native。应用与测试源码仍与09ef一致，原生专项继续按自身c8e7执行身份引用。

### 真实Redis金额与缓存定向验收（af18853，已完成）

后继 `3a1d50a59d7caeaf98e149151842d4c58c4973e2`（tree `32376205ded6a2e1182c01b55b031dde8c8a657d`）的[CI 38135269862](https://github.com/ForceMind/MyAPI/actions/runs/38135269862)十项全部success。前端187文件/1126测试、profile九场景、31条UI旅程及最后Playground命令完整通过，耗时13分18秒；后端完整测试、63项审计工具及竞态检查通过。预算修正已完成独立审查与复验，旧3feff取消仍按原事实保留。

完成审计发现[金额兼容方案](HISTORICAL_AMOUNT_COMPATIBILITY_PROPOSAL.md)要求的必要原生Redis证据尚未由既有44个quota-script子项覆盖。本批只新增共享测试合同与相应文档，复用现有configured Redis及miniredis两个父入口，原44断言不变：核验宽余额精确加减及边界零写入、2^53相邻/MaxInt64版本排序、无owner冷缓存发布限制及owner重建、只读信任检查、旧schema局部失效。原始十进制值和只读/拒绝分支的PEXPIRETIME绝对值直接核对；合法hydrate刷新期限的原语义保留。

原生入口继续要求literal loopback、非默认动态端口、DB15和全实例为空，并记录实际Redis7版本。仅使用传入client与runner临时SQLite合成库，保存还原全局状态；不使用FastForward或等待时序，也不FLUSH。主线程维护文档与集成，实现者只写两个测试文件，独立审查者只读核验。构建及服务仍在GitHub-hosted runner，本机只做轻量静态检查。准确提交af18853的[CI 38138227403](https://github.com/ForceMind/MyAPI/actions/runs/38138227403)十项success；原生Redis7.4.11原44子项、新3组及3个拒绝子项、父项共51次RUN/PASS，各一次、零skip/fail，原始日志与步骤终态经独立复核，容器及网络回收成功。

本批应用源码与原生数据库合同文件保持不变，原Docker/native结果继续按09ef/c8e7的实际执行身份及范围复用，不将新增测试后的整棵tree称为旧运行检出对象。MySQL/PostgreSQL专项只覆盖共享合同内创建的表，不扩称所有可选金额表均已原生验收。Redis服务的host publish仍为all-address，镜像仍是floating redis:7-alpine；这些合同不证明TLS、集群、故障转移或历史缓存同点。历史缺口及DO_NOT_RESTORE、ledger_reconciled=false、production_eligible=false均保持。

### 386订阅通知格式与定向行为验收（c94e5ed，失败保留）

前一准确提交 `af18853416eb744ad246e36b883babbd3f962ce6`（tree `4383d934d27e1aab463bdb83c2f464a679838f98`）的CI十项全部success，实际PR检出 `9da59b249ee16e8bb86d826c835992fb3bf50c4f` 同tree，parents为main92fa与af188。后端完整模块/独立relaykit测试、63项审计工具及竞态组通过；前端187文件/1126测试、profile九场景、31条UI旅程及最后Playground命令通过，前端14分34秒、后端21分47秒。该结果只属于af188，不替后继提交记绿。

[金额方案](HISTORICAL_AMOUNT_COMPATIBILITY_PROPOSAL.md)要求386定向行为，而既有CI只有386 build，不会编译新增的_test.go。旧私有386记录支持部分宽状态、payload、receipt及缓存行为，但没有工件内源码SHA绑定，整轮曾因旧duration缺陷和五项miniredis宿主宽度限制失败，不能写成准确后继或386全包通过。

本批发现并修复订阅通知的三处 `logger.FormatQuota(int(remaining))`：在386下 `-4294967313` 会变为 `-17`。生产异步路径改用领域通知builder，直接格式化int64；原阈值、提前返回、提示/模板、类型及充值链接保持。纯DTO合成测试覆盖Bark/Gotify/默认/Webhook的精确宽负值和普通值，使用正式设置快照/发布/恢复接口，测试正文不发送通知、不调用数据库或网络；执行service包时，既有TestMain仍初始化内存SQLite。

现有backend job在386 build后增加一个有界步骤，五包14个精确顶层测试涵盖宽余额/used、relay DTO、receipt事务、V3/V4原身份、缓存版本/schema、Key元数据/CAS和通知。GOOS=linux、GOARCH=386、CGO_ENABLED=0，p1/count1/timeout240s；Go JSON与tee经pipefail保留失败，stdlib门禁要求14项各RUN/PASS一次、五包各PASS一次、所选树零skip/fail。不新增服务、job、权限或artifact，不预先加大30分钟后端预算。旧五项mock限制不改断言或加架构skip；新结果只能称定向386合同，不能称386 model全包通过。本批准确SHA及终态以[PR Checks](https://github.com/ForceMind/MyAPI/pull/11/checks)和对应原始日志回读。

分工沿用原Goal：实现者仅写service通知及其测试，主线程维护CI/五份说明和集成，独立审查者只读；本机仅静态检查，构建/行为在Actions。新增应用差异限通知组装，不再称整个业务tree等于09ef；common应用逻辑与既有历史原生专项合同不变；model的B2测试补验范围见下一节。旧Docker/native专项只按各自执行身份及未变路径复用，未重新验新整棵应用。元数据继续未发布beta.11，历史证据、DO_NOT_RESTORE、ledger_reconciled=false及production_eligible=false保持。

### 完整金额物理schema补验（c94e5ed，原生通过）

完成矩阵复核发现，既有c8e7原生历史合同只创建部分金额表，生产只读探针允许可选表缺失，因此不能由该成功结果推导全部当前金额schema已验。复用现有B2 MySQL5.7/PostgreSQL9.6服务和SQLite旧日志夹具，不增加job、服务、权限、上传或时限；原日志maintenance拒绝、backfill、dedup、quota及credit业务断言保持。

B2在显式日志维护后成功执行标准生产migrateDB，走到SubscriptionPlan末尾迁移，再核对旧日志原值；SQLite使用绑定全局的既有迁移夹具完成同一路径。共享合同要求quotaAmountTableSpecs的21表38列全部存在，再执行现有物理类型/范围只读检查。可选表缺失在本合同中不能计通过，不从spec生成DDL自证。

缺少原生宽值证据的13个聚合、收据、订阅和管理员字段，以int64(2^31+100)及合法组合通过生产model精确读回；单次费用字段保持int32。样本仅在一个合成事务内，固定sentinel回滚后按独有标识核验所有触碰表零残留，操作失败也先核验回滚。原生只读probe在写事务前关闭，避免单连接池嵌套Begin。实现者仅写两份既有model测试，主线程维护CI/文档，独立审查者只读复核；本机仍仅静态检查。

准确提交c94e5ed6e85724d89bbb995f5096507cf254fe6c（tree6015dd3275098368b3dc91d1c5dbeecb778334e9，实际PR检出9816d3bf9c148a3678058b240ac2a06067bf97e6同tree）的[CI38140987775](https://github.com/ForceMind/MyAPI/actions/runs/38140987775)中，B2 job成功；MySQL/PostgreSQL的historical-amount-schema各一次RUN/PASS、零skip/fail，后续旧业务矩阵及容器/网络清理成功，原始日志经独立复核。该CI的后端386步骤失败，不能称整轮通过。这只验证生产迁移创建的当前schema和宽值往返：B2旧输入是旧日志，不证明任意旧金额窄列可升级，也不替代历史资金、缓存或执行资格。原c8e7仍按其子集范围引用，历史账务未核清，副本DO_NOT_RESTORE、ledger_reconciled=false和production_eligible=false保持。

### 386旧测试夹具编译兼容（本批，后继待验）

c94e5ed的新增386步骤实际发现三处既有夹具编译错误：service/openai_text_source_cost_test.go把MaxQuota+1传给int，service/token_counter_test.go两处把MaxInt64传给int。其余13个顶层测试、四个package实际PASS；service测试未运行，后续backend完整测试/竞态等步骤未执行，不能记14项或整轮成功。失败原始日志保留，前端与其余job分别按自己的终态记录，不外推。

本批只修上述两份测试的架构输入：图片夹具使用math.MaxInt，64位仍为原MaxInt64，386为可表示的MaxInt32；原精确3742和quota conversion错误断言保持。文本费用测试在两架构都检查可表示的负边界，64位保留原正MaxQuota+1拒绝，用int64变量计算后在架构分支内安全转换。386的int不能表达正超界值，不使用溢出cast、t.Skip、build tag或放宽生产验证。

同一386步骤保留原14项，再加入这三项改动回归，共五包17个精确顶层；严格JSON门禁仍要求各一次RUN/PASS、五包各PASS且零skip/fail。全部job、服务、权限、预算、上传及其他断言不变。本机仅gofmt/静态检查；生产Go、schema、Redis合同不变，不新建手动Docker/native运行。后继准确SHA/终态以[PR Checks](https://github.com/ForceMind/MyAPI/pull/11/checks)及原始日志为准，当前未记PASS。c94的原生Redis7.4.11仍为51次RUN/PASS各一次、零skip/fail，B2和Redis两个job的容器与网络清理成功并独立审查通过；这些成功不抵消386失败。

现有Goal及正常开发授权继续：实现者仅两份service测试，主线程CI/五文档，独立审查者只读，按审查后继正常提交推送。旧386全包失败和五项mock限制不改，17项后继仍仅定向合同。历史缺口、DO_NOT_RESTORE、ledger_reconciled=false、production_eligible=false和未发布/未部署保持。

## 范围与来源

继续既有修复目标：个人资料持续加载、新请求结算保护、历史金额与账务兼容。不开新功能，不合并 main，不发布版本或镜像，不部署生产。

主线基线为 `92fa1182e265924312d70917a0a637965aebc617`，修复分支为 `codex/historical-accounting-compat-20261010`。接入前的 225 个候选文件已逐文件匹配私有封存摘要；原 v10 候选、旧失败记录、原始账务副本和恢复证据继续保留。新增 CI 文件和本批交接更新属于后续验收批次，不能继续称整个提交只有 225 个文件。

缓存清理已解除上一轮磁盘不足条件，恢复时可用空间约 26 GiB。历史来源、缓存、预扣与事件证据的缺口仍存在。Goal 工具恢复时没有现存记录，已按原批准范围重建主目标；旧停止点作为历史记录保留。

## 执行计划（首次提交前准备快照）

下表保留首次提交前的准备状态，不追溯改写历史记录。当前状态以草稿 PR 中对应提交的 Checks、本页“云端执行记录”和验收说明为准；不以准备阶段状态代替实际结果。

| 阶段 | 状态 | 产物和完成条件 |
| --- | --- | --- |
| 回读基线与候选 | 已完成 | 远端 main 仍为上述基线；225 文件内容匹配，修复分支此前不存在于远端 |
| 接入云端验收 | 已完成，运行待验证 | 仅修改 `.github/workflows/ci.yml`，新增 386、审计工具合成测试、lint 和 profile 异常恢复场景 |
| 公开边界与独立审查 | 进行中 | 核验明确文件清单、私有数据排除、workflow 触发器和上传范围 |
| 提交及分支同步 | 未开始 | 只提交已审代码与文档；推送后回读远端 SHA，保持 main 和历史标签不变 |
| 准确提交的 CI | 未开始 | 建立草稿 PR；核对检查对应的 head SHA、每个作业和新增步骤结果 |
| 镜像合成验收 | 未开始 | 必要时手动运行现有 Docker smoke，构建及 runner 内加载；Full/LAN bundle 按既有规则上传 Actions artifacts 保留 7 天，不推送 GHCR 等发布仓库 |
| 历史实证与生产资格 | 未完成 | 仍依赖历史证据、专项原生数据库验收授权及真实回滚合同 |

## 本批实现与检查

CI 补充如下，复用 GitHub-hosted runner 和现有依赖：

- `GOOS=linux GOARCH=386 CGO_ENABLED=0 go build ./...`，验证本次金额表示在 32 位构建上的兼容性。
- 在 `tools/accounting-audit` 运行 `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -v`，只使用测试生成的合成临时数据库。
- 前端执行 `bun run lint`，保留现有类型检查、测试及构建。
- 已有 Chromium 步骤执行 `tools/ui/profile-browser.mjs`，覆盖正常读取、空设置、失败重试、请求挂起、认证刷新及启动读取挂起。脚本只访问本机合成 API。

YAML 解析、补丁应用检查和独立静态复核已通过。触发器、权限、job/service 列表及 artifact 路径保持既有定义；新增浏览器结果仅进入 runner 临时目录中的 PNG/JSON 上传范围。现有 Docker smoke 手动运行还会上传构建 bundle，来源是远端 clean checkout，不包含私有生产副本。这些结果不是云端作业运行通过。本机不重复全量编译、不重新创建账务数据库副本。

## 云端执行记录

首个源码提交为 `5ff2e228bdb6bb18c567e1dd97635a93a058d955`，已正常推送至上述修复分支并建立[草稿 PR #11](https://github.com/ForceMind/MyAPI/pull/11)。独立审查确认 227 个提交文件为代码、测试、CI 和文档，未包含生产数据库、凭据或原始敏感日志。

[首轮 CI 38124726661](https://github.com/ForceMind/MyAPI/actions/runs/38124726661)的 PR 测试合并提交为 `cd112d6e0ae4cf78bc842cb926f57a6ed97fc48d`，tree `c353b61066837a3ad4a66eb61b58b952769eefd0` 与源码提交一致。该轮 MySQL/PostgreSQL B2 回归均发现签到额度断言仍使用 `int(25)`，实际字段已为 `int64(25)`，数值相同但严格类型比较失败。本批将此断言改为精确 `int64`，并将充值回执的额度断言从 `EqualValues` 改为同样严格的 `int64` 比较；原金额、并发及回执数量断言保持，生产金额代码不变。

此处记录首轮发现及对应修复，不能称首轮全绿。后续准确提交、CI 每个步骤及必要 Docker smoke 的结果统一在 PR 的 Checks 和验收说明回读，失败原始日志保留为私有证据。PR Check 通过不等于业务 CI 通过。历史原生专项仍未新增或启用。

独立审查另发现同一 B2 fixture 的资金设置和数据库 schema capability 未完整恢复，本批补齐测试 cleanup，避免跨 engine 留下临时状态；恢复失败也须明确报告。主线 CI `38022171765` 的后端作业实际耗时 18分36秒，首轮新增 386 构建另用 2分28秒，两者合计21分04秒，超过原20分钟总预算；将后端总上限调整为30分钟，全部测试及各命令期限保留，不以超时调整掩盖业务失败。

### 准确候选与 Docker handoff 修复计划

PR 运行记录显示，准确源码提交 `35b4c7bf234a386bec48574fda25d4967ff58ed2` 的[常规 CI 38125528583](https://github.com/ForceMind/MyAPI/actions/runs/38125528583)十项均为 success，PR 测试合并 tree 与源码 tree 一致。[Docker smoke 38125659791](https://github.com/ForceMind/MyAPI/actions/runs/38125659791) 的 handoff 场景仍失败：固定旧源码 `71277bf6055ce68b8cd1d11f1d10f1907d7696fd` 在升级前执行了一次扣 15 额度的请求。代码路径表明该请求会产生历史 intent/fact，与当前必须先具备 execution gate 的预检相冲突；原日志未保留 clone 的具体启动错误，此原因尚待新检查直接确认。不能据此放宽生产门禁，也不能把本轮 Docker 验收写成通过。

修复保持正向迁移与拒绝/恢复两条路径互不混用：

1. 在旧镜像完成 Root、普通用户、有限 Key 和渠道配置后、发出任何请求前，保留无结算历史的旧库种子；从两个独立副本分别验证当前镜像的正向迁移和原镜像恢复。
2. 继续原运行中的固定旧镜像夹具，执行且只执行一次 15 额度请求，核对已完成 fact 的日志证据；intent 缺失在 legacy 下合法，存在时必须为正整数且恢复前后身份一致。随后保留原始升级前备份。当前容器读取这一历史快照时必须因缺少 execution gate 而拒绝，退出码固定为 1、`OOMKilled=false`，主数据库文件及 WAL 摘要保持不变。
3. 再由固定旧镜像恢复同一份原始升级前备份，精确核对用户额度、有限 Key 额度和日志身份；上游请求计数仍为 1，证明恢复及就绪探测没有重放请求。

修复不创建 gate 或 permit，不改写历史记录，不降低启动预检，也不改变结算与许可政策。先完成上述 Docker handoff 修复并形成新的准确候选，再将原生 MySQL/PostgreSQL/Redis 云验收提案绑定该最终 SHA 重新生成和复核；原生提案当前仍为 pending，不创建验收分支、不 dispatch、不启动专项 service。

2026-10-11 本批脚本已完成独立只读复审，未发现阻断问题。定向 Node 验证通过 20 项：SQLite 恢复 11 项、交接 6 项、relay 前置验证 3 项；覆盖错误退出码、OOM、错误拒绝原因、数据库或 WAL 变化，以及拒绝证据不足时不得启动旧镜像恢复。另有两份既有 workflow 测试文件执行成功，语法检查和 `git diff --check` 通过。上述均是脚本与合成单元验证，新的实际 Docker 结果仍待云端运行。

复审者执行完整 `node tools/runtime/docker-smoke.test.mjs` 时，三个既有合成上游监听用例因沙箱禁止绑定 `127.0.0.1` 而报 `listen EPERM`：非敏感摘要、Playground 流式合同、缺少 usage 或畸形 stream 拒绝。这轮完整测试未通过；原始输出保留在工具调用记录，未生成本地日志文件。不申请本机端口、不放宽断言。

后续覆盖核对发现，既有 CI 不执行这三份 runtime 单元测试，Docker smoke 只执行实际集成入口。为补齐上述受限用例及错误/OOM/摘要变化分支，本批在现有 distribution 作业加入一个 Node 步骤，仅执行 `docker-smoke.test.mjs`、`sqlite-restore-smoke.test.mjs` 和 `handoff-upgrade-smoke.test.mjs`；不新增 service、job、依赖、权限或 artifact。提交 `28f4f609914f7f64d0e7e4a206a3bee8eaf82c69` 已推送且 tree 与 PR 测试合并树一致，其 CI `38127825760` 和 Docker `38127843731` 为补验前的运行，不能据此声称新增 Node 步骤通过；新步骤结果以随后的准确提交运行回读为准。

### Legacy 结算日志合同修正

[CI 38128072273](https://github.com/ForceMind/MyAPI/actions/runs/38128072273)对应 `c27658c4279f3b715a8aa2f14d335829d9836d91`，PR 合并 tree 与源码一致；新增 Node 步骤实际通过 39 项、失败和跳过均为 0，包括本地 EPERM 的三项监听用例。`28f4f609` 的 CI 被该提交替代并取消，8 个作业成功，前后端取消，不能称全绿。

[Docker 38127843731](https://github.com/ForceMind/MyAPI/actions/runs/38127843731)对应前一 `28f4f609`，镜像 bundle 和 LAN 源码安装成功，Full fresh、LAN fresh、Full handoff 三项均在 `SMOKE_FIXTURE_CONSUME_LOG_MISMATCH` 处失败。全部失败日志与终态私有保留。两版源码独立核对确认：legacy session 不设置 `settlementIntentReady`，会直接创建并恢复 fact，审计合法输出 `state=applied` 和正 `fact_id` 而不含 `intent_id`；Root 日志路径保留该字段。新增脚本与 fake 单测此前错误要求同时存在正 intent，39 项单元通过没有识别此错误合同，不能替代实际镜像验收。

本批仅修正两处 smoke 断言和对应夹具：fact 始终必须为已完成且正整数身份，intent 缺失允许，存在的零值、null、非整数或错误身份继续拒绝。恢复前后仍比较 fact 及 intent 的存在和值，保留 15 额度请求、精确账务与隐私断言、当前镜像拒绝原因/退出码/OOM/DB及WAL摘要验证。不改 Go 结算、门禁、许可或历史数据。该修正仍须新准确提交的云端单元及 Docker 验收；任何原生专项提案只能在最终候选通过后重新绑定并独立复审，不执行旧提案。

本批无监听的 relay/proof 定向检查及 handoff 7 项通过，额外输入明确保护“只有 intent 而没有 fact 必须拒绝”和“恢复后新增 intent 也属于身份变化”。四份变更脚本语法及差异检查通过；未改的 SQLite 恢复单元沿用前轮 11 项结果。本地未启动服务或构建，完整三文件 Node 步骤和实际双镜像仍由新提交的 GitHub-hosted 运行验证。

独立复审已从固定旧版和当前 Go 真实路径确认 fact-only 合同，当前七文件差异通过复审，无阻断项；这项静态与定向结果不代替新镜像运行。

## 分工与资源

主线程负责目标、文档、workflow 步骤命名、集成、提交同步和云端结果核对。`resume_ci_plan` 使用 gpt-5.6-sol/ultra 只读规划，后仅更新本页计划；`ci_offload_implementation` 使用 gpt-5.6-sol/high，依次负责 CI、上述 B2 测试及 `tools/runtime` 的三份 smoke 脚本和三份对应测试；`candidate_publish_boundary_review` 使用 gpt-5.6-sol/xhigh，只读核验公开、运行边界及修复。禁止递归派工；共享文件由主线程串行集成。

不改 Git 用户配置。若环境没有提交身份，使用明确的 Codex 机器人标识并在 PR 声明 AI 协助；不得冒用维护者身份。草稿 PR 保留仓库模板，人工确认项如实保留未确认状态。

CI 失败只修复本范围内有证据的缺陷。候选摘要漂移、私有数据进入提交、意外发布/生产连接、账务语义或许可政策需要改变时停止对应步骤并保留证据。原生历史 MySQL/PostgreSQL 专项服务未在本批新增或启用。

## 未闭合边界

历史 beta.3 一致性来源、完整资金流、历史预扣/缓存及全部 writer、未知事件、旧终态凭证适用来源和实际回滚仍未闭合。未知项继续冻结，演练副本继续 `DO_NOT_RESTORE`，`ledger_reconciled=false`、`production_eligible=false`。

云端合成结果只证明对应代码和合成输入下的行为，不能证明用户历史数据库可安全升级。发布、实际部署及生产恢复继续另行取得明确批准。更多事实及历史验证范围见[历史账务兼容核查记录](HISTORICAL_ACCOUNTING_COMPATIBILITY.md)。
