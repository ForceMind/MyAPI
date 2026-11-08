# 全项目评估、精简、三版本与 UI 重设计：任务清单（2026-10-24）

## 2026-11-08 C1第二批：beta.49源码候选，授权与模型40键待CI

本批沿同一C1主线更新40键/218处语种条目：18个实际授权UI键的en值原为中文，另外五语90个值也逐项确认等于该错误中文；恢复现有英文key原文并补五语。另22个模型目录/详情/元数据键补五语，范围覆盖本次目录返回/筛选、分组价格aria、限流数据未提供、说明查看/展开、未配置模型结果/复制/配置及分页。只已有键值，七语仍预期7890键，无增删；简中保持。付款/授权等副作用流程不变，localhost/URL/ChatGPT/Codex品牌、count/group/model/name变量、未知与未验证、模型未配置不等于不存在/不支持等含义保留。

通过规定临时add-missing-keys脚本仅写源码，实际en18/zh0/其余五语各40，共218处，临时自有脚本已删除。仅六字典、VERSION/根package/三个分发模板版本标识、Desktop/发行说明及本清单/交接共15文件；没有改组件逻辑、权限/确认、计算、真实配置或数据。默认浏览器仍不跑，母语/GUI/真实供应商/生产未验；不在本机同步、lint、测试或构建、不安装依赖/下载产物。Node语法、Compose YAML、JSON源数据及diff仅轻检查，不当验收。推送后本SHA的完整CI、七语清单、Full/Lite实际教程构建均须通过才验收，不拿beta.48绿灯替代。

整体Goal仍是Full/Lite/Desktop及A2–A6/B3–B7/C1–C3全部获准范围；当前只推进C1，不新开BUG专项。后继设置16键、钱包/订阅/用户36键仅有未应用机器草稿，不算完成或全量覆盖；其余版本能力、未授权删除、浏览器及生产边界继续保留。

## 2026-11-08 C1首批beta.48已验收：46eadb53 / CI42451649080

`46eadb5302380db480ade3c23b316fa87a763bb0` / [CI42451649080](https://github.com/ForceMind/MyAPI/actions/runs/42451649080) attempt1 success：14执行成功/5可选跳过、20712行全部实际日志已读，14 checkout与全部步骤已核、失败/race标记0。前端421文件/3452测试，Root/独立relaykit/386/三库/race/分发/Root合同均通过。清单job128641961812完整373行，8合同通过/0skip，七语各7890键、相对en缺键/额外键/占位符差异均0；已定位4静态缺键归零，词法动态未知仍388，英文同值只是候选，translation_accepted=false。教程job128641962097完整749行及安全回执已核，24原命令exit0/全部后置检查true；Full/Lite实际构建启停/status均beta.48/setup=false，自有清理全true，临时local alias使用但ghcr_published=false。Native仅预览、Desktop仅Node合同、TLS入口/跨机LAN/浏览器/母语仍未验证，不是全版本计划完成。

安全报告：翻译[13605353145](https://github.com/ForceMind/MyAPI/actions/runs/42451649080/artifacts/13605353145)，422460 bytes、SHA256 d12968c4c42c94cc724bea653e60287d208368179563415e0f465d5d5027d1f6、到期2026-11-15T05:40:09Z；教程[13605224239](https://github.com/ForceMind/MyAPI/actions/runs/42451649080/artifacts/13605224239)，1979 bytes、SHA256 3dc9ebb130053f3b39453bb4e4ebcc6c65ac2dafec6acb237ce5461693f2d96a、到期2026-11-15T05:46:07Z。均未下载，没有发布或生产动作。以下保留该候选提交当时范围，待验描述不覆盖上方实证。

沿已批准C1主线合并首批78键：连接余额/账号窗口/WHAM保存证据、路由读取与保存未知、模型来源及Key交付/客户端端点/模型限制/预算说明。源于当前en/zh值与实际组件，主会话已审稿，通过技能规定的临时`add-missing-keys.mjs`仅写字典源码，共382处语种条目更新：en4/zh4/zh-TW74/fr74/ja77/ru74/vi75；临时自有脚本已删除。70个已有键补五语、4个已登记能力/Range缺键补全七语，另修日语3处`__ PH_0 __`与越南语误插`{{LinuxDO}}`。既有en/zh值保持，预期7890键、原4占位符差异归零及4缺英文键归零仍须Actions证明；英文同值含合法品牌，不能据此称全七语完成。

本批只七字典、`VERSION`/根package及三个分发模板版本标识、既有Desktop/发行说明和本清单/交接，共16文件；beta.48未发布。未改应用逻辑、请求、费用计算、权限、确认、返回上下文或实际服务器配置。copy保留无模型限制vs全部模型禁止、失败vs不支持查询vs未知、历史恢复未确认、保存/缓存发布/未知后先读等含义及原变量。母语和浏览器未验。后续自身CI开启`run_i18n_inventory`和`run_deployment_tutorials`，在Actions同步/类型/lint/全部测试/构建并实跑Full与Lite教程；本机没有同步/测试/构建/依赖或产物下载，只有源码编辑及轻检查。

工具提交`313f2c0c5f5abe713bcd6d361b0d86d29a28447b` / [CI42449299756](https://github.com/ForceMind/MyAPI/actions/runs/42449299756) attempt1完整通过：13执行成功/6可选跳过，19573行全部实际日志已读，13 checkout/全部步骤已核、失败/race标记0；前端421文件/3452测试，Root/独立relaykit/386/三库/race/分发及Root合同通过。清单job128634883496完整381行、8合同/0skip、原始七语各7886键和真实副本sync/source保持receipt通过，`translation_accepted=false`。安全[报告13604654116](https://github.com/ForceMind/MyAPI/actions/runs/42449299756/artifacts/13604654116)为428277 bytes、SHA256 `74715d6601ed84bdcf9a0dccc2e68284de036b1a4a5ba049fb4a690747076de1`、到期2026-11-15T05:07:37Z，未下载。原字典/页面从ec至313未变化的Git差异已核，草稿基于该实际清单；本候选写入后不把313的结果当作beta.48验收。第二轮0ce/CI42448678659最终cancelled，12490行全部可用日志与13 checkout已读：10成功/1失败/2取消、6可选跳过，取消不当绿。其余C1、版本能力及最后统一质量阶段仍在同一Goal；本轮不另开BUG专项。从en文件阅读还见Codex提示值为中文，留C1后继原文语言复核，不在本批顺改。

## 2026-11-08 C1：当前七语言清单候选；不提前进行统一BUG审查

第二轮`0ceacc6539a51357c31c4e8ed6a881e3dbc7bd4a` / [CI42448678659](https://github.com/ForceMind/MyAPI/actions/runs/42448678659) 仍未完整通过：前端job128633935395完整333行与annotations已读、checkout匹配，原4个lint错误消除，新增字符码检查的`Array.from()`触发`unicorn/prefer-spread`，26警告/1错误。后继仅改为等价字符串迭代`[...value]`，不改字符范围、合同或规则，待自身完整门禁。清单job128633935386已success，具体回执仍须读取，不以API单项状态代替完整验收。首轮ec9156d4最终cancelled，13份执行日志共11113行已读、13 checkout匹配，7成功/1失败/5取消、6可选跳过，取消不当绿。

首轮`ec9156d4b35b38c3f407ebb195cd1b1978498b81` / [CI42448319640](https://github.com/ForceMind/MyAPI/actions/runs/42448319640) 不能验收为完整绿，已请求取消剩余重复作业。清单job128631936078完整382行已读，8合同全部通过/0skip，raw字典每语7886键、相对英文均0缺键/0extras；英文同值候选zh178/zh-TW1077/fr1191/ja1085/ru1085/vi1123，包含品牌与同形词，不作为漏译数量。词法1617文件/10381引用/388未知；静态登记4个缺英文键候选，日本语3/越南语1个占位符差异仍待上下文确认。canonical真实副本运行、源码保持及safe receipt均过，`translation_accepted=false`。前端job128631936031完整341行及4条准确lint annotations已读：新reporter两处嵌套三元、两处控制字符正则违反门禁，前端回归尚未执行。

后继最小候选改为原顺序的显式比较分支和字符码拒绝检查，控制范围保持C0/DEL及临时路径额外C1；对应拒绝边界加合同，预期未放宽。只清理本脚本冗余检查/const/字符串拼接，不改规则、字典或产品。修正后仅Node语法/diff已查，完整CI仍待新提交；首轮技术清单通过不替代完整门禁。

按已批准顺序继续C1，不重做页面、效果图或既有线稿。新增`web/scripts/report-i18n.mjs`和8组纯合同，`ci.yml`新增默认关闭的`run_i18n_inventory`。从代码看，入口仅允许Actions/manual/dev/本仓库精确SHA，在原始字典上先记录缺键/额外键/英文同值候选/占位符差异及词法源码位置；动态和绑定未确定的引用保留未知，不构成删键依据。canonical sync只在Runner自有副本实际执行，7字典、原同步脚本与扫描源码哈希及Git身份在前后重核，安全报告绑定run/attempt。`passed`仅代表清单执行成功，`translation_accepted=false`、`analysis_only=true`、`deletion_authorized=false`始终明确；不把旧报告或英文同值当本轮语言验收。

本候选尚待自身完整CI和清单作业，当前只有源码审阅、Node/YAML/Python语法与diff检查。没有修改locale、执行本机入口/测试/sync、下载产物或浏览器/生产/真实供应商动作。应用版本仍beta.47。连接/Key首批12键已有上下文补译草稿，尚未应用或验收；后续通过技能规定的脚本批量写入并由Actions同步验证，不直接编辑字典。历史BUG仍留版本能力后的统一阶段，完整Goal及所有剩余范围保留。

## 2026-11-08 B7：源码教程命令与完整CI已验收

`5b05eea42edc15ba6a9d87a1e704e0827efaa87a` / [CI42446080862](https://github.com/ForceMind/MyAPI/actions/runs/42446080862) attempt1 success：13执行成功/5可选跳过，19908行全部实际日志已读，13 checkout和全部步骤已核，失败/race标记0。前端421文件/3452测试通过；Root/独立relaykit/386/三库/race、分发及串行Root合同通过。教程job128625189803完整748行及原始安全回执已读：24个唯一marker原命令均exit0、executed/exit_zero/postcondition_verified全true，Full与Lite均实际构建、启停和读取状态，版本beta.47、setup=false；自有容器/网络/tag清理全true，fixture仅留一次性Runner。Lite私网临时别名实际使用，`ghcr_published=false`。私网仅Runner自有网络，Full只核secure cookie配置，`tls_ingress_verified=false`；Native仍仅help/plan/render，Desktop仅Node合同，不是实际安装、真机GUI或跨机/浏览器验收。

安全[报告artifact13603359096](https://github.com/ForceMind/MyAPI/actions/runs/42446080862/artifacts/13603359096)：1979 bytes、SHA256 `9783b07498983125430dc326be2cbb4abaebb7231dd54d8aabea382f422d57c2`、到期2026-11-15T04:23:44Z，未下载。首轮42444890601最终cancelled，13份可用执行日志共14571行已读：7成功/2失败/4取消，另5可选跳过，13 checkout匹配；取消不当绿。串行Root合同通过不能反推首轮unknown的确切原因，不降低未知时拒绝或原observed断言。本批没有改产品CLI、发布或生产动作，B3/B4/B5后续范围和完整Goal仍保留。

## 2026-11-08 B7：首轮镜像后置检查失败；后继最小修正（历史时点）

首轮 `3c8a8dfeced8a01b7512a235c77e4b82eb01293f` / [CI42444890601](https://github.com/ForceMind/MyAPI/actions/runs/42444890601) 未通过，不能作为教程验收。教程 job128621644090 完整746行已读：Full build 原命令 exit0，但随后的镜像读取未通过，前4条命令及自有资源清理已核。分发 job128621644137 完整3748行已读：Root归档合同 private-map 关闭占用后的扫描 status=unknown，期望仍为 observed；83项81通过/2失败（含父项），未知错误码原日志未带出，不能确定其原因。已请求停止该轮剩余重复作业，取消不算通过。

从 helper/CLI 调用链看，原 `docker build` 没有显式输出选项，而 docker-container builder 默认不自动导入 Engine；教程工作流增加 `default-load=true`，不改产品 CLI、命令文本或镜像身份断言。[Docker 官方加载说明](https://docs.docker.com/build/builders/drivers/#load-by-default)。另将全局 `/proc` Root合同的测试文件串行执行，减少不同文件同时起停fixture的干扰；原 private-map 关闭后 observed 断言只附安全错误码消息，期望、未知时拒绝、无重试及产品逻辑均不变。跨文件竞态只是猜测，不据此声称根因已确定；后续原命令与完整 CI 仍待本候选实跑。本机仅Node/YAML语法及diff检查，未执行测试或构建。

## 2026-11-08 当前：beta.47门禁已核；B7教程待CI

**最新beta.47实际门禁已核：81ab6c4867ad1247800928727f0aaf7324dc6df3 / [CI42442205783](https://github.com/ForceMind/MyAPI/actions/runs/42442205783) attempt1 terminal success。** 13执行作业success/4可选skip，21466行全部实际日志已读，13 checkout匹配、失败/race标记0；前端421文件/3452测试，Root/独立relaykit/386/三库/race、Root合同与真实backup/restore/prune作业均过。真实资格精确删1/保2、restore引用/latest保护、一次性确认和trigger、原source新增17单位调整保留、每个实际netns fake累计2且recovery0、清理均已核。步骤API曾滞后，不能写“步骤元数据全部同步”：后端全部原run命令与无continue-on-error、S1和A2实际PASS日志已逐项复核；A2三库20叶场景PASS/0skip，job终态success且日志已完成清理，API仍可能显示旧in_progress/pending，保留该异常。未重跑成功作业，不把浏览器/生产/真实供应商或发布算验收。

**本提交为B7教程候选，待自身Actions。** 沿现文档增量补Full/Lite/Native预览/个人电脑/Desktop中英任务教程，24个唯一marker绑定原Bash文本/source行/含LF哈希；GHA helper只manual/dev/精确SHA，真实CLI两次local image build和串行固定my-api启停/状态、临时私网绑定，保持镜像ID/项目labels/bind/secret内存一致性。私网参数当前会改写myapi-lan坐标，CI临时local alias仅本daemon，无pull/push/发布、不替换生产镜像。Native只有plan/render/help、Desktop只有Node合同，本批不宣称实际安装/GUI/HTTPS ingress/真实跨机LAN。仅safe报告7天artifact，构建/env/DB不上传，fixture只留一次性Runner。新入口run_deployment_tutorials默认false，未在本机执行命令，语法检查不是实际资格。完整Goal的其余范围与未授权删除/浏览器/生产边界仍保留。

## 2026-11-08 当前：B4a beta.46已验收，B4b beta.47候选

**最新已验收为B4a beta.46：`b9c4dcd4ce72fea5679b2da0a7d394039a003f23` / [完整CI42439207854](https://github.com/ForceMind/MyAPI/actions/runs/42439207854) attempt1 success。** 13执行成功/4可选跳过，主会话读取20680行全部日志，13 checkout与所有步骤已核、失败标记0；前端421文件/3452测试、Root/独立relaykit/386/三库/race和root合同通过。真实CLI资格job128604632216完整2094行及报告已核：plan/confirm备份、WAL与原目录/权限保全、隔离新目录restore、错摘要/重复确认/原及既有目标拒绝、账务精确复读、原fake2+恢复fake0=2、精确网桥规则/本机canary/host健康与清理全部通过；浏览器/生产/真实供应商未执行。

安全[资格报告](https://github.com/ForceMind/MyAPI/actions/runs/42439207854/artifacts/13600832096)为1585 bytes，SHA256 f90df8a037ead5503820e166aa32d1ed612c65c045c2820c9b9f2a500eccddea，到期2026-11-15T02:39:37Z，未下载。默认keep-all，完成范围仅Root Docker Full/SQLite停机备份与隔离恢复，不是整个B4或全部三版本；旧红灯/取消不当绿。

**beta.47首轮90cb0f6c / CI42441559549未验收。** Root合同job128611690844完整3804行已读：7个prune场景均在同一fixture建库Python内嵌引号处SyntaxError，尚未执行这些场景断言。候选仅将两条fixture SQL改为参数化，原断言及产品逻辑不变；Node和内嵌Python仅语法检查。真实资格job128611690523已success，2123行与报告已核：精确删1/保2、引用和latest保留、错摘要/重放确认/trigger复用拒绝，新增17单位调整与原Key/消费日志精确保全，所有实际netns fake计数2+0+0+0和recovery0，cleanup true。该单项成功不能覆盖Root合同失败，须后继SHA完整重跑。

**本提交为B4b beta.47候选，待自身Actions验收。** 数量/天数并集与最新/引用/占用保护、具体plan-confirm及一次性trigger、完整manifest/root/FD/mmap/cwd/全部容器挂载重验后精确清理；未知/部分失败留现场不重试，默认仍keep-all。新primitive/root合同和真实CLI清理资格源码已集成，实际仍待Actions；必须证明原source新增合成账务+17保留且所有fake累计2、不能靠断开的上游伪证明不重放。现仅源代码/语法检查；未来仅Actions运行，不本机依赖/构建测试/分析/Docker/产物下载，不动生产/浏览器/真实供应商/main/发布。以下历史候选状态按原时点理解，完整Goal与未授权10文件删除仍保留。

## 2026-11-08 当前：B6完整CI已验收；B4a beta.46候选

**B6已验收，完整CI通过。** dev/`0f7d98260010c0d88bbcdf58c16603a47507aece` / [完整CI42433123893](https://github.com/ForceMind/MyAPI/actions/runs/42433123893)。job128586211061完整2662行与结构化报告已核：0755/0700均为实际UID/GID10001应用拒绝、exit1，无OOM；拒绝阶段全树bytes/owner/mode保持，root业务精确复读、各fake2→2/0新增、停止源不变、cleanup true。镜像image.Size=92005247 bytes；未创建传输tar，registry压缩量未测，两者null。默认USER未改，也无切换授权。attempt1 success，13执行成功/3可选跳过；主会话读取20674行全部日志、13个checkout和所有步骤，失败标记0，前端421文件/3452测试及原Go/独立relaykit/386/三库/race门禁通过。浏览器未运行。安全[报告构件](https://github.com/ForceMind/MyAPI/actions/runs/42433123893/artifacts/13598572847)为2942 bytes、SHA256 eb83621d7cc575589fff954259ddc8235fd805b0bc6b3dacf84a2e3108c5f181，到期2026-11-15T01:15:00Z，未下载。

**B4a首轮 `e18822b8` / CI42436085982未验收。** 主会话已读取21157行完整实际日志，13 checkout均绑定e188，终态9成功/2失败/2取消/4跳过。root合同job128596131466的唯一错误是symlink先被owner/mode归类而未到type拒绝；候选只将type/link检查移到owner/mode之前，原全部断言与权限门禁不变。实际CLI资格job128596131567在backup-plan以HEALTH_BINDING_MISMATCH拒绝，未进入停机/复制；不改现有validateContainer门禁，当前修正候选仅为CI自有normal bridge+固定接口精确DROP规则，新增规则添加/读回/删除、gateway自有canary的异步负向探针及host published健康检查；未知或清理缺证仍非零，未探测外网。此候选还未运行，须后继SHA重验。两项静态网站42436083531/42436083619分别381/430行已核且成功，不能代替B4产品资格。原自动42436083629为12取消/5跳过，metadata证明未checkout；Native作业日志API不可用，未称全部日志已读或已测试。

**B4a修正轮 `487ce8ac` / CI42438501082尚未验收。** Distribution/root合同job128602513332已success；资格job128602513409完整2096行已读，在network-isolation以B4_GATEWAY_INTERFACE_MISMATCH退出、cli_failure和cleanup_failure均null，尚未创建source。依据[libuv接口过滤源码](https://github.com/libuv/libuv/blob/v1.50.0/src/unix/linux.c#L1832-L1844)，从代码看新bridge未有carrier时Node枚举会过滤；候选改用固定iproute2内核地址JSON，精确核iface/index/IPv4与owned gateway，仍在所有容器启动前完成阻断规则，不放宽任何产品/账务断言。待后继SHA实际资格与完整CI。

**本提交为B4a beta.46候选，待自身Actions验收。** 首批交付本地root Docker Full/SQLite的真实`myapi backup/restore`计划与确认：同注册/全局flock/实例锁，确证正常停机、root本地FS和可见FD/shared-writable mmap停写；whole-tree manifest、两阶段COMMITTED、DB/WAL/SHM同盘私有副本完整性校验，保全部bytes/UID/GID/mode。默认keep-all，restore只新隔离目录，先验源自备份后未变，不覆盖、不自动启动、不重放；未知结果保null，失败冻结并留现场。原重启策略另存计划/journal，关闭自动重启后保持停机，恢复服务须另行人工决定；不回灌旧账务。

涉及CLI、tools/operations注册/锁/journal/copy/guard及独立backup引擎、两份新资格workflow/helper、原五处版本与[说明](../../BACKUP_RESTORE.md)。主会话已亲读关键源码、精确对照legacy三个Python action正文不变；Node/Python/YAML/Bash仅轻语法检查，未运行项目入口或测试。新合同与实际产品CLI Docker往返全部待Actions：source假上游2次/恢复假上游0次、辅助未checkpoint WAL、原始与既有目标拒绝、原数据/备份元数据和账务复读及完整CI；不能用unit或源码代替实际资格。无本机依赖/构建/项目测试/分析/Docker/产物下载，无浏览器/生产/真实模型供应商/发布/main。

完整主Goal仍保B3余项、B4后续（数量/天数保留、恢复/安装形态扩展）、B5b后续、B7及C1/C2/C3；A5/A6原已验片段不重做，145未知保留、10整文件删除仍待明确授权。R4保持关闭，不另开BUG专项，历史问题最后统一质量阶段处理。

B6红灯过程已保留证据：7a3eb2ff / CI42431111672为就绪失败（16182行实际日志已读）；3f38e0a4 / CI42431898229为同一宿主端口路径差异（14819行已读）；776d33d0 / CI42432457165的0755通过、0700阶段预判不符（15459行已读）。这三轮均因后继候选替代而最终cancelled，各10成功/1失败/2取消/3跳过，不当全绿；自动42431094719/42431895654/42432454726/42433120963也已回读，分别3167/0/0/0行，后三次未执行任何作业，不能称已测试。b7自动42425102725补读4199行，2成功/10取消/3跳过，仍不当绿。

## 2026-11-08 当前：A6a与重绑清单已验收，B6容器评估候选待CI（仍beta.45）

`b7abb2bf88e2d2bb39e732c9f4e4f1d06563c436` / [手动完整CI42425121214](https://github.com/ForceMind/MyAPI/actions/runs/42425121214) attempt1 success：13个执行作业成功、2个可选作业跳过。主会话已亲读19568行全部日志，核对13个checkout及所有步骤，0 error；前端3452与根Go、独立relaykit、386、三库等原门禁通过。自动CI42425102725 cancelled，但主会话尚未全读其实际日志，不能写成已亲读完或计入成功证据。本节优先于下文A6a“候选/待自身CI”等历史时点记录。

最新清单job128561699305共1346行已读：Go 1576 files / 5872 short functions / 422 manual；前端configured为2 files / 372 exports / 174 types，ui-inclusive为13 / 417 / 176。相对6200959c恰少1个Go私有wrapper和4个type导出修饰符，与本批范围一致；其余计数不表示全部已使用、可删或已逐项验收。[cleanup-review-6200959c.csv](cleanup-review-6200959c.csv)仍是6200959c旧基线的1033项复核记录，不重标为b7abb2bf；145个未知项继续保留，10个UI整文件删除仍在等待具体授权，尚无回复，禁止删除。

当前新source batch仅做已批准B6的容器用户/体积兼容性评估：既有`ci.yml`增加默认false的`run_container_assessment`，调用新`.github/workflows/container-hardening.yml`及`tools/runtime/container-hardening.mjs` / `.test.mjs`。callee仅manual/dev并绑定精确SHA；两个source-bound镜像只在Runner build/load，`push=false`，不打包发布。主会话已亲读helper/callee/test，node --check、YAML与内嵌Bash轻检查完成；10组Node合同及真实Docker评估尚未CI运行，应用保持beta.45，不升版本。

成功标准是取得可分类、可保全的评估证据，不是将默认USER改成非root。独立Runner仅用Full/SQLite与fake上游：分别从0755和0700的root旧卷克隆真实合成账务及metadata，不修clone的owner/mode；在预期权限拒绝之后，要求拒绝阶段字节完整保全，再以root精确复读原业务身份/账务，fake调用计数维持2→2，不能重放请求。0755必须出现真实UID marker再exec；0700若在OCI预执行阶段已拒绝，则UID应为null，不能补0或冒充已执行应用。缺少必要metadata、误分拒绝阶段或保全失败均非零，不能用任意失败制造评估成功。

source/root和Recovery保留实际/proc UID证据；拒绝阶段按已执行UID marker或OCI预执行拒绝如实分类，未产生应用进程时保留null。拒绝阶段要求全字节保全；后续root业务复读可能合法写会话，必须分阶段说明，不把整个流程说成字节完全不变。有效评估完成后明确报告`compatibility=false`、`default_user_changed=false`，仍不能据此宣布可安全切换USER或完成权限迁移；当前仅候选，尚无本批运行结论。

镜像体积仅将image.Size记为精确bytes；history大小只作human显示，diffids是标识符而非大小，tar/registry压缩体积未测时为null，不补0、不据此比较发布包。验证全部在Actions：10组Node合同、两个镜像来源/加载身份、实际非root拒绝与root业务复读、metadata/数据/调用计数保全，再按本source batch自身SHA亲读完整CI。轻语法检查不替代这些运行证明。

完整Goal继续保留B3余项、B4、B5b后续、B6/B7及C1/C2/C3，A5/A6的未知和未授权项不因计数减少自动关闭。B4 writer边界只读方案已保存，不等于backup/restore已接线或获准操作现有实例。不新开BUG专项，不做本机测试、依赖安装、构建、Docker或artifact下载；生产、真实模型/供应商、浏览器、发布和main保持不动。下文旧记录完整保留，按各自源码和时点解释。

## 2026-11-08 当前：A5b common已验，A6a内部类型修饰符候选（仍beta.45）

common小批`05088cbbd70b2fc13c6b8c74342bbed381d8551a` / [完整CI42422805505](https://github.com/ForceMind/MyAPI/actions/runs/42422805505) attempt1 success：12执行作业成功/3可选作业跳过。主会话读取18111行全部日志，核对12个checkout与所有步骤，失败/race标记0。`make test`的真实common包输出`ok .../common 0.524s`，新公共Render表驱动6场景包含于该包全量执行；该命令不逐例打印，不能写成逐例日志PASS。根main/根与独立relaykit/386/三库/原有race及前端3452等原门禁均成功。清单与复核基线仍绑定6200959c，不重标为当前源码；只删除一个已核对的私有转交，SSE帧算法与MIT版权头完整保留，没有扩展BUG专项。

A6a只在三个内部工具文件移除四处export修饰符：full-content-logs/lib/saved-sse.ts的SavedSseMarker/SavedSseEvent、performance-metrics/lib/format.ts的SuccessRateLevel、system-settings/utils/json-parser.ts的JsonParseResult。主会话已文本对照确认除四个修饰符外所有字节不变；定义/字段/价格与成功率判定/JSON和SSE行为/权限/返回上下文不改。源码复核未发现这些类型的外部或barrel/namespace消费，不据此宣称未知外部源码消费者不存在；本批只收内部模块边界，不处理其它145未闭合项。

成功标准：调用方可编译，所有原功能与精确断言保持。验证方法：Actions的typecheck、lint、既有完整测试与构建，加manual/dev两模式清单重新绑定最终SHA；本机只源码/差异检查，不装依赖、执行类型检查或项目测试/构建/Knip，不跑浏览器或下载产物。低影响类型修饰符变化不新增镜像实现的类型测试，原测试不改。候选待自身完整CI，beta.45不重升，没有新发布包。

本批提交仅三前端源及两交接文档。独立B6评估的ci.yml/callee/helper/test仍为另一WIP，不含本提交；B6只测当前镜像用户/体积/root旧卷兼容性，不改默认USER或生产，预期不兼容须严格保全后报告compatibility=false，资料未知/保全失败非零。10个UI整文件仍等具体授权，没有回复不删除；原生B3执行器、B4、B5b后续、B6/B7、七语言与统一质量/全量验收仍在主Goal，不将本批称A5/A6或全项目完成。生产/真实供应商模型/发布/main与浏览器均未操作。

## 2026-11-08 当前：清单证据已验收，A5b common 候选待自身CI（仍beta.45）

`6200959c5ec5148c74b3b629c589245db97515bf` / [手动完整CI42419812845](https://github.com/ForceMind/MyAPI/actions/runs/42419812845) attempt1 success：13个执行作业成功、2个可选作业跳过。主会话已亲读20142行全部日志，核对13个checkout及所有步骤，0 error。自动CI42419777489 cancelled的全部实际日志也已读，仅部分执行，不计为完整绿灯。本节优先于下文“工具尚未执行”“待Actions”等历史时点记录。

清单作业128545672516共1347行已亲读：Go collector 10组、前端reporter Node 15组通过；当前Go扫描1575文件、5873个短函数、423个manual候选。前端configured的files/exports/types为2/372/178，ui-inclusive为13/417/180，实际工具退出码均0。报告仍为`analysis_only=true`、`deletion_authorized=false`；AST词法和Knip结果不是动态可达性、无间接覆盖或安全删除证明，工具验收不等于整个A5/A6验收。

[清单证据artifact13592982829](https://github.com/ForceMind/MyAPI/actions/runs/42419812845/artifacts/13592982829)：584252 bytes，SHA256 `4400708028dd5f549d2cecd3090f92f0d9894da3c44ea9ecbaa5cda166557677`，到期`2026-11-14T22:17:08Z`。仅记录服务端证据，没有下载到本机。

逐项复核集中保存在[cleanup-review-6200959c.csv](cleanup-review-6200959c.csv)，共1033行数据，保留每项身份、文件hash、理由和实际阅读范围：Go 423项为422 keep / 1 inline；前端610项为451 keep / 145 unknown / 4 modifier / 10 wholefile待授权。保留项部分仅读定义名字/签名，不能把未读调用链称已验证；未知项保留，不按旧数量凑精简。该CSV是集中复核入口，不另建状态体系。

当前A5b common小批仅在`common/custom-event.go`删除私有`encode`并按原顺序内联到公开`Render`，新增`common/custom_event_render_test.go`保护6个公共SSE场景；writer适配、帧字节、原协议及完整MIT版权头保持。源码已准备，gofmt/文本核对完成，6个场景尚未CI运行；将与本交接合批提交，再亲读该提交自身完整CI。应用保持beta.45，不升版本；不能借6200959c的清单绿灯称本common候选已验收。

前端3文件4个type导出修饰符移除另有WIP，本次common提交不包含它们。10个UI整文件删除已按旧约束9异步请求具体授权，尚无回复，禁止删除；不将修饰符候选或整文件候选称为已交付。

完整Goal继续保留A5/A6后续、B3余项、B4、B5b后续、B6/B7与C1/C2/C3；B4只读定位不等于用户备份恢复已实现，Release Manifest阶段边界不变。不新开BUG专项，不接生产、真实模型/供应商、浏览器或发布；所有项目测试与构建只在CI执行。下文旧候选和失败记录保留，按各自源码/时点理解。

## 2026-11-08 A5/A6：当前精简候选清单（工具候选，应用仍beta.45）

B3a已通过源码及非浏览器合同验收：dev/`34ad42acf14ce72aecacc7c96657528d6c0eabba` / [完整CI42415882062](https://github.com/ForceMind/MyAPI/actions/runs/42415882062) attempt1 success，12执行作业成功/2可选作业跳过（A3分析、浏览器真实API）。主会话读取18695行全部日志，核对所有步骤与12个checkout一致，失败/race标记0；Linux预览/argv合同6/6/0skip、systemd255.4两scope及复杂路径真实解析通过（native作业282行），Desktop合同105/105/0skip、前端421文件3452/3452、额度历史三库12场景、根main/根与独立relaykit/386/原有race均过。前端lint26 warnings/0 errors未扩展处理。本批不等于完整B3：真实EnvFile读取、MyAPI/manager生命周期、安装/升级/回退/卸载、浏览器及生产仍未实现或未验证。

过程证据保留：d3b89d16/CI42414107174及7dc5eeee/CI42414561122均最终cancelled，各9成功/1native失败/2取消/2跳过，所有实际作业日志、步骤与checkout已读取；前者lint阻塞、后者user路径字段解析阻塞，没有把跳过/取消算绿，最终只认34ad42ac自己的完整CI。旧d3两静态网站成功仍只绑定原SHA，不重标34ad42ac或后继源码。

沿PLAN_PHASE2 A5/A6生成当前清单，不重建过期790/396或140/68/637数量，不开新BUG专项。成功标准：清单绑定精确SHA、源码哈希、实际工具状态，合法空与缺失/损坏分开，未知位置/方法引用保持未知，不自动删代码。验证方法：Go collector10组及前端reporter15组合同、当前Go AST/Knip扫描和完整CI全部Actions，亲读最终SHA所有步骤、日志和报告；本机只源码、gofmt、node --check、YAML/内嵌脚本轻语法，不执行工具/测试/分析、安装依赖、构建、Docker、浏览器或下载产物。

两名复用sol/high代理分别唯一负责Go只读collector/test/既有README、前端只读reporter/test；主会话集成既有ci.yml开关、新cleanup-inventory.yml及本交接，无新代理/递归。Go两文件build ignore，Actions显式命名文件运行；仅标准库AST和XML，不导入业务、不typecheck或执行产品init，不改Go模块与根JSON wrapper门禁，也不另加JSON例外。普通函数同目录/包词法引用，API/方法/init/生成/测试/fixture/relaykit/build或平台特定/cgo/重复/函数值/编译器指令等保守保护；引用未知输出XML unknown而非0，Go/token列为字节坐标。SHA256和Git blobSHA1绑定实际读取字节与index，HEAD/整tracked树前后核对，缺源/变更/越界/链接/parse错误非零，不伪零候选。AST计数不是动态/外部或目标构建可达性证明。

前端冻结锁实际为Knip6.27.0（package范围^6.24.0，不改依赖）；[官方reporter合同](https://knip.dev/features/reporters)和现成6.27源码已只读核对。configured保持原ignore，ui-inclusive仅Runner在web内创建临时config去掉UI ignore、保routeTree ignore及同ignoreDependencies；每次收files/exports/types。6.27根为issues数组，files在各row内，不盲兼容旧顶层files。合法空合法零，缺/坏/未知schema拒绝；缺位置null、pos0合法、line/col1-based UTF16，整文件位置not_applicable，不补0。每项tracked安全路径/hash/位置核对，整tracked树前后检查；source_files只代表报告涉及并校验的文件组数，不是Knip全扫描文件总数。工具版本/exit/config选择元数据明确caller-reported、invocation_verified=false；外围CI真实执行命令/退出码和日志才补调用来源链，不能拿一个report JSON当验签。

入口复用已注册CI：`gh workflow run ci.yml --repo ForceMind/MyAPI --ref dev -f run_browser=false -f run_cleanup_inventory=true`，新开关默认false、仅manual/dev可运行reusable工作流，checkout固定run SHA，原完整门禁保留。报告仅analysis_only=true/deletion_authorized=false/manual_review_required，候选/保护计数日志与完整原始证据上传7天；不下载到本机。当前工具和清单尚未执行，不报候选为0或称A5/A6完成；拿到当前证据后再逐项审阅，同包精简另提交，整文件删除仍需具体范围授权。

本批不改产品/界面/转发/计费/数据库/权限/版本，不发布/部署/main或调用真实供应商模型；R4继续关闭，历史BUG统一质量阶段处理。完整三版本Goal仍保A5/A6后续、B3剩余安装信任/生命周期、B4一致性备份恢复/保留策略、B5b后续、B6/B7及七语言/独立统一质量/全量验收。B4已复用现有copier/CLI/operations只读定位，不把停写整树copy或合成smoke伪称在线backup/用户restore，不因此顺带接线T1或运维生产。

## 2026-11-08 B3a：Linux原生路径预览与服务模板（beta.45候选，非安装器/生命周期验收）

第二次候选`7dc5eeee7c26d926a8b1f83c2fee66685f5070af` / CI42414561122的native作业128529798642完整268行已读：lint成功、6/6 Node合同PASS/0skip；真实systemd255.4在user WorkingDirectory外层引号处报非绝对路径，故后续复杂路径解析skipped，不能称unit通过。按[systemd-stable v255.4字段解析](https://github.com/systemd/systemd-stable/blob/v255.4/src/core/load-fragment.c#L2400-L2494)改为field-specific路径编码：WorkingDirectory raw仅%%；EnvFile raw再转义glob层的反斜杠和*?[]，不套word引号，Environment/Exec保原编码。原6组、PID/环境/退出码/数据保全和完整路径期望不减；字段decoder现在拒绝非法外引号/裸glob，并补config literal glob路径。依据[EnvFile加载源码](https://github.com/systemd/systemd-stable/blob/v255.4/src/core/execute.c#L676-L739)，此glob结论标“从代码看”；真实EnvFile读取仍未验证，analyze仅证明解析，不外推配置加载/生命周期。beta.45不重升，后继源仍须完整CI。

首推`d3b89d16a5d90bfc049827cf9634c25bb54e857e` / CI42414107174的native作业128528355160失败，183行完整日志/步骤已读：bash语法过，shellcheck报CDPATH空赋值SC1007，以及case匹配/printf输出literal systemd dollar时的两处SC2016。Node6组合同和两类systemd解析全部skipped，不能算功能已验。仅将CDPATH写成显式空串，并在仅匹配/输出固定模板的case前标注`# shellcheck disable=SC2016`：这里必须原样输出$$，不能展开为当前shell的PID/参数；完整argv等全部功能断言和shellcheck命令保留，不改其它lint配置。beta.45不重升。此SHA静态检查42414107228与打包42414107204成功，381/433行及步骤/checkout已读，仅绑定d3b89d16，不外推后继源；最终候选仍需自身完整CI。

前一小批A5a已验收：dev/`679ceee0f5e91162b1cffbc7becbbd0632955306` / [完整CI42411676577](https://github.com/ForceMind/MyAPI/actions/runs/42411676577) attempt1 success，11执行作业成功/2可选作业跳过；18494行全部日志、步骤和11个checkout已核对，失败/race标记0。原三库12场景、Native105/105/0skip、前端421文件3452/3452及根main/根与独立relaykit/386/原有race均通过。仅coalesce机械展开和原进度文档，未改原断言或发布；这不等于整个A5已逐项清理。B3a是在等待该CI期间并行准备，未抢先推送，实际结果仍待本批自身CI。

沿已批准PLAN_PHASE2 B3及已确认system/XDG路径，先交付可审阅的只读入口与两种unit，不重做现有CLI、operations或Desktop。维护者确认的Release Manifest阶段仍只做合同/合成，不接线下载、验签、安装；本批没有新增管理执行器。`bash deploy/install-native.sh plan --scope system|user`只显示目标目录，`render-unit --scope system|user`只向stdout输出服务文件；帮助和预览有中英说明，明确尚未检查现有安装、配置、健康/端口/edition，不可据此接管或替换服务。安装/升级/回退/卸载及未知命令均非零拒绝，不创建目录或控制服务。

成功标准：固定system专用账户与路径、解析user XDG路径，特殊字符保持字面值，预览不读密钥/正文/配置或改变既有数据；system/user模板各自适用，未确认停机不假称可继续。验证方法：6组Node合同的路径/拒写/字节权限保全及合成exec转交、shellcheck、真实systemd-analyze解析system/user及复杂路径unit，均只在Actions；随后按最终SHA读取完整CI。当前为源码候选，轻检查不代替这些运行证明，不提前称B3a验收或完整B3完成。

文件与分工：复用2名gpt-6.1-sol/high，模板/入口owner仅my-api.service、deploy/my-api-user.service、deploy/install-native.sh；测试owner仅tools/runtime/native-service.test.mjs；主会话负责既有CI新增独立native合同作业、NPM源码files/check-pack required资产接线、既有五处beta.45值和本交接。主会话集成后亲读全部候选源码，未在本机执行入口/test/shellcheck/systemd，只有bash -n、node --check、YAML及内嵌Bash/Python轻语法/差异。没有安装依赖/项目编译测试/Go分析/Docker/浏览器或下载产物，NPM只验证源码包，不发布。

system固定/opt/my-api/{current,releases}、/etc/my-api/my-api.env、/var/lib/my-api/{data,state,backups}、/var/log/my-api、/var/cache/my-api，由my-api账户运行。user从已校验的HOME和显式/默认XDG data/config/state/cache解析绝对路径；data/release与state/backups分开，不改HOME、不扫描或自动接管旧目录。空/相对/控制字符/点段路径拒绝；plan不把目标路径说成存在或健康。SQLite缺省与必需EnvironmentFile并存，显式配置仍可覆盖，unit只写此SQLite缺省而不硬编码密钥/功能开关。system限制后端仅写data/log/cache，不赋予安装state/backups写权；user不依赖usernamespace文件系统隔离，均UMask0077/NoNewPrivileges/on-failure。

停机：从main代码看默认HTTP Shutdown为120秒且随后可能保存看板数据，故unit用TimeoutStopSec=infinity避免manager默认截止后强杀；未确认停止时不得备份/换程序/重启，不把等待说成已停。依据[systemd官方停机合同](https://www.freedesktop.org/software/systemd/man/256/systemd.service.html)；本批不发任何stop或manager控制命令，不证明实际停机过程。

特殊路径：从[systemd v256官方源码](https://github.com/systemd/systemd/blob/v256/src/core/load-fragment.c)看ExecutablePath与argv展开不同，直接给可执行路径加$$或转义quote不能支持本批已锁的literal域；env也将含=的operand视为赋值。user因此采用固定/bin/sh与固定`exec "$1" --log-dir "$2"`字面脚本，目录只通过位置参数传入，不拼shell源码、不eval；`exec`转交语义见[Ubuntu官方dash手册](https://manpages.ubuntu.com/manpages/noble/man1/dash.1.html)。完整6项argv严格断言，合成程序校验相同PID、原binary/log路径、继承环境、原非零退出码以及无额外命令/文件修改；中文、空格、引号、反斜杠、%/$/=及命令样文本不缩减。真实systemd解析另校验，不能只用自写词法decoder当外部验证。

Actions的system/user verify只使用一次性Runner的inert executable/临时配置与目录，拒绝覆盖预存/opt等路径；不会安装/enable unit、启动manager或真实MyAPI，也没有真实模型/供应商请求。Node的合成exec不是服务生命周期、真实配置加载或生产账务验收。剩余B3的可信制品接线/安装升级/兼容回退/保数据卸载及manager生命周期仍未实现或验收，不能以本批替代；B4/B5b后续/B6/B7、A5后续/A6及七语言/统一质量/全量验收继续同一Goal。R4保持关闭，历史BUG到版本功能之后；无生产/服务器/main/tag/GHCR/NPM发布或部署，浏览器未验证。

## 2026-11-08 A5a：controller单调用机械辅助函数（沿用beta.44，完整CI待验）

A4已通过源码与非浏览器验收：dev/`3df6dd05ef16098a2bf6741f1f271829d0eca9a6` / [完整CI42408737566](https://github.com/ForceMind/MyAPI/actions/runs/42408737566) attempt1 success，11执行作业成功，A3可选分析和浏览器真实API两作业跳过。主会话读取18578行全部执行日志，核对全部步骤与11个checkout一致，失败/race标记0。A4原23条相关test在race专项逐名PASS，独立的分析窗口解析test保留于普通controller全量；15条纯测试、分桶三断言与10个保留test/helper的文本映射一致。三库12叶场景全PASS/0skip（B2完整2203行）；根main、根/独立relaykit、386及原有race成功；前端421文件3452/3452、Native105/105/0skip通过。前端lint26 warnings/0 errors，本轮未扩展处理。相同源码的静态网站检查42408737654和打包42408737578成功，382/431行日志、步骤与checkout已核对；未下载产物。浏览器、真实供应商/模型、生产及平台GUI未验证或操作。

本轮dev/3df6dd05干净，fetch后0/0。沿已批准A5按包做小批，不按旧790/396数量删代码，不新开BUG专项。复用两代理只读核对controller/service/common/setting/relay/model的近似机械项；这是窄查，不是全包调用图或旧清单复测。主会话本批只写controller/model_sync.go及两份既有进度文档，不删除文件，不改公开API、JSON、权限、价格、数据库或供应商协议；应用沿用beta.44，无新发布包。

成功标准：只内联唯一生产调用的6行私有coalesce函数，空串与纯空白仍回退为空，带首尾空白的非空图标保留原值；vendor缓存、已有vendor读取、插入及状态回退不变。验证方法：调用/定义与差异文本核对、gofmt；controller与既定根main/全量/独立relaykit/三库/386/race全在Actions，推送后按精确SHA亲读完整CI。此可逆机械展开不新增镜像实现的字符串单测，原测试不改；本机不测试、编译、装依赖、运行分析/Docker/浏览器或下载产物。

保留service参数覆盖审计/最终协议标记、common认证/writer接口/资源初始化、setting快照复制/代际构造、model缓存条件/GORM logger、relay重定向回调/事件类型/请求ID等有实际边界的短函数。relay/helper的单行newStreamResult工厂另记下一同包候选，本批不改。其它方法、多行签名和全部adapter未逐一复核，不能称A5全量完成。A5后续/A6、B3/B4/B5b后续/B6/B7及七语言/统一质量/全量验收仍在同一Goal；R4继续关闭。原生B3沿固定system/XDG路径，Release Manifest本阶段不接线下载/验签/安装的已确认边界保持，不把调用方verified当实际信任证明。


## 2026-11-08 A4：额度历史聚合下沉 service（beta.44候选，三库/完整CI待验）

上一轮A3b已通过：dev/`1378d4ac1557aeb36cfd97718db45de8937c68d8` / [manual完整CI42404468288](https://github.com/ForceMind/MyAPI/actions/runs/42404468288) attempt1 success，14执行作业成功/1浏览器真实API跳过；主会话亲读22194行全部日志、步骤和14个checkout，失败/race标记0。前端421文件3452/3452、Native105/105/0skip、根main/根模块/独立relaykit/386/三库及原有race全部通过。三目标root/relaykit交叉构建与9次固定工具分析actual_exit=0，合并473条记录（root production377/with-tests45、relaykit115），private人工候选归零；这只证明本批17个private精简及当前分析边界，不声称公开API或其它标签都可删除。同SHA静态网站工作流42404467033/42404467039成功，382/431行日志已读；自动42404467089取消且未执行步骤，不计绿。没有浏览器/部署/发布或CI构件下载。

A4开工dev/1378d4ac干净，fetch后0/0，ff-only最新。复用PLAN_PHASE2与既有报告，不重做页面、不另开BUG专项。成功标准：额度历史聚合下沉到service，HTTP鉴权/参数/JSON字段与省略规则、完整范围读取、原计算和未知状态保持。验证方法：保留并搬移原断言，service/controller/router相邻消费者专项及SQLite/MySQL5.7/PostgreSQL9.6回归、根main/全量/独立relaykit/386/原有race全部Actions；推送后亲读最终SHA完整CI全部作业、步骤与日志。本机仅源码、gofmt和轻语法/差异，不装依赖/编译/测试/分析/Docker/浏览器或下载产物。

复用2名gpt-6.1-sol/high代理：service owner仅新service/channel_quota_history.go与_test.go；controller owner仅channel-billing.go、channel_quota_changes.go、codex_usage.go及原history_test；主会话负责三库回归、既有CI接线、版本/交接和集成。29个业务函数从原controller机械迁移；主会话文本对照确认仅名称导出/命名空间和gin.H→普通map映射。15条完整纯测试、混合首test的3条分桶断言及ptrFloat helper按映射逐字保留；保留的10个controller测试/helper内容同基线，HTTP解析/错误/ID→渠道校验顺序及history区外采样不改。

service只接收已规范化的范围、显示预算、粒度、时区、分析窗口和series筛选，无Gin/controller依赖。复用原Count/List完整范围与消费/分析函数，原150000条源预算、5000显示点上限、先读全范围再压缩、并发Count/List不完整返回空points、不补零和失败外显保留；不改SQL、表/字段、转发、计价、路由权限或通知。共享changes/codex仅切service导出符号，不复制算法或留旧wrapper。

新增三库专项复用B2临时数据库服务，DSN限127.0.0.1与myapi_b2_test；每场景自建独立channel，仅清理自己的fixture记录。覆盖完整范围先于显示上限、分析与粒度无关、读取不改行、最新失败不补零/历史摘要仍在、已解析空身份精确隔离、空范围与数据库错误。普通全量只跑SQLite、跳过关系库；B2显式门禁开启时两DSN缺一即失败，不能把skip算三库已验。原断言不减、不把任何模糊结果改成零；生产未改。

候选版本仅既有五处统一beta.44，无新包或发布；最终CI尚待，不能提前称A4完成。后续A5/A6、B3/B4/B5b后续/B6/B7与七语言/统一质量/全量验收仍在同一Goal；R4保持关闭，历史BUG留到版本功能之后。无main/tag/GHCR/NPM、生产/服务器或真实供应商/模型操作；浏览器未验证。

## 2026-11-08 A3b：17个私有未使用函数精简（beta.43候选，重新分析/完整CI为门禁）

上一轮为实质进展：A3a当前可达性证据已交付，dev/`7e6d3fa452aa89462da83903ee690f2dbcb5c9c4` / [manual完整CI42401193288](https://github.com/ForceMind/MyAPI/actions/runs/42401193288) attempt1 success，14执行作业成功/1浏览器真实API跳过；主会话亲读21759行全部日志、步骤和14个checkout一致，失败/race标记0。被手动运行替代的自动CI42401192003真实cancelled，仅31行setup已执行，其余未执行/无日志，不计绿。前端3452/3452、Native105/105、A2三库20场景与原有根/独立relaykit/386/race通过。

三目标linux/amd64、darwin/arm64、windows/amd64分析job分别128489088279/128489088167/128489088187成功，完整1301/1300/1301行及每个步骤已读。各Node分类合同17/17、0skip；每目标root production/root with-tests/独立relaykit with-tests三扫描actual_exit均0，报告函数数394/62/115、合并490条、仅17条可进入人工复核。三目标这17项名字/位置/文件hash集合完全相同；导出API/方法/relaykit全部manual=false且deletion_authorized始终false（证据不替代用户范围授权）。不宣称其它标签、外部消费者、平台GUI或真实服务已验。

三份原始证据保留7天，本机未下载：Linux构件13585980053（ZIP SHA256 `0750926e4d1e56d5ea909483ab8d864292345afdb89d022d70343c9f22559b7b`，到2026-11-14 18:27:35 UTC）；darwin13585681915（`71a5d880a5fdbf57a3da5963b5feb07e948ceec4f3b2874212520245cc1ac6d0`，到18:28:11 UTC）；windows13585821199（`080f4271e6ccc07114a41fd812a3f5493a4c37d25c2f2bde2a708b770fdb4d37`，到18:28:19 UTC）。全部绑定run42401193288/attempt1/7e6d3fa4，不重标后继源。

本轮dev/7e6d3fa4干净，fetch后0/0、ff-only最新，基线CI仍success；主会话已亲核17项当前文件hash仍等于分析报告。完整读过pkg/billingexpr/expr.md，保原表达式、token归一化、计价/结算和饱和审计；本轮不改它们，不新找BUG。仅执行已批准A3内私有函数级精简，**不删除整文件或对外API**。不重建旧37清单或按过期数量制造完成。

成功标准：只删除已逐项复核且三目标prod/test均不可达的17个private完整定义及唯一失效注释/确实无用import，所有活跃功能/字段/价格/权限/付款/账务/未知/不重放契约与原测试断言保持。验证方法：相关既有测试、根main/根模块/独立relaykit/三库/386/race与三目标重新构建/分析全部Actions，推送后按最终源码SHA亲读完整CI。本机仅源码、gofmt、轻语法/差异，不装依赖、编译、测试、运行分析/Docker/浏览器或下载产物。

本批文件与唯一owner：复用2名gpt-6.1-sol/high分别负责controller三个文件中的14函数、model三个文件中的3函数；主会话负责版本/交接、集成、提交/CI，无新代理或递归。函数边界：

- controller/channel-billing.go：updateChannelCloseAIBalance、updateChannelOpenAISBBalance，另移除后者已失效注释调用；保活动余额分派、超时/响应上限/采样持久化和两个导出response类型，不宣称同供应商已验替代。
- controller/topup.go：getPayMoney、getMinTopup、getMaxTopUpAmount；保captureEpayQuoteContext、quote.payMoney/minimum、maxTopUpAmountForSnapshot、normalize、严格额度转换和容量检查。现有decimal等import仍有活动用途，不连带删。
- controller/payment_webhook_availability.go：isStripeWebhookConfigured、isCreemWebhookConfigured、isWaffoTopUpEnabled、isWaffoWebhookConfigured、isWaffoPancakeTopUpEnabled、isWaffoPancakeWebhookConfiguredFromPaymentConfig、isWaffoPancakeWebhookConfigured、isEpayTopUpEnabled、isEpayWebhookConfigured；Pancake两项内部相互引用成对处理。所有活动FromPaymentConfig与provider/签名/环境/funding decision/epoch/状态/幂等结算及路由保留，不删文件或连带import。
- model/account_quota_refund_fact.go：nowUnix，保活动DB时钟和time超时用途；model/channel_quota_alert.go：channelQuotaAlertNow及此文件唯一time import，保显式now/deliveredAt/failedAt API；model/quota_batch_drain.go：quotaBalanceGenerationTouchesSubject，保主体关联表JOIN/预算/游标及所有类型/字段/constants。

导出PostConsumeQuota、DeltaUpdateUserQuota及其活动下层/真实GORM hooks继续保留，relaykit API不动；非默认标签/外部私有补丁不当作已知无调用。代码删除不是把函数改成空壳或放宽检查，也不新增“函数不存在”测试；必要旧回归按原语义执行。当前只是候选，实际运行须看新提交完整CI及三目标重新分析，应用候选统一beta.43但不发布/部署/main/tag/GHCR/NPM或改实际生产配置，不发真实模型请求。剩余A3外部API边界、A4–A6/B3/B4/B5b后续/B6/B7与统一质量/七语言/全量验收仍在同一Goal。

## 2026-11-08 A3a：当前Go可达性清单（仅分析，非删除授权）

上一轮为实质进展：A2配置兼容检查已源码/非浏览器交付，最终dev/`77fed01ef38081cdbab1d22a9506f892b2f4e40c` / [完整CI42396095331](https://github.com/ForceMind/MyAPI/actions/runs/42396095331) attempt1 success，11执行作业成功/1浏览器真实API跳过。主会话亲读17688行全部日志、步骤与11个checkout一致，失败/race标记0；新增根main定向5个Test全部PASS（含3个状态子场景及实际main子进程/迁移前拒绝），SQLite5/MySQL8/PostgreSQL7共20叶场景通过、CLI181/181、前端421文件3452/3452、Native105/105/0skip，根/独立relaykit/386/全部原有race通过。首次产品源f308bfcd的整体绿但漏root main及后继覆盖修正保留，不把旧make test排除根包当已验。真实登录/认证器、历史大库、浏览器和生产均未验证/操作。

本轮开工dev/77fed01e干净，fetch后0/0、ff-only最新，基线CI仍success。沿PLAN_PHASE2 A3，不重做A1或新开BUG专项。旧报告的`37`只是历史计费相邻数量；主检出scratchpad/out/deadcode.txt、deadspan.txt不存在，旧c916051e Git树也无这些文件。原始逐项名单未归档，不能据此编造剩余函数或删除代码。历史具名的controller.getPayMoney、model.DeltaUpdateUserQuota、service.PostConsumeQuota仍存在；其中导出wrapper/底层活跃实现/真实GORM hooks/relaykit公开API各有保护边界，不能连带清除。旧“7个relay转换wrapper均不可达”已与当前引用不符，不能沿用过期概括。

成功标准：在Actions生成绑定准确源码SHA、工具版本、目标平台和实际命令的当前候选清单，明确prod/test及独立relaykit边界；失败/缺输入/无法解释不得伪报零候选。验证方法：固定工具安装、Go分析/跨目标构建、独立relaykit构建及整理工具合同全部在Actions；本机仅源码/轻语法与差异检查，不装依赖、编译、运行分析/测试/Docker/浏览器或下载CI构件。先清单，后逐项按现有契约处理，不把这批标为全部A3清理完成。

本批新增manual-only dev工作流和`tools/go-reachability`整理工具/合同，复用2名gpt-6.1-sol/high代理分别唯一负责工作流+报告工具、测试+工具说明；主会话负责既有交接、集成和CI日志亲读，没有新代理或递归。工具固定`golang.org/x/tools/cmd/deadcode@v0.38.0`，主会话已读官方tag源码及go.mod（工具要求Go1.24，项目两模块Go1.25.1）；安装/包下载只Runner，关闭Go遥测，不改根模块go.mod/sum。

候选分析范围：Linux/amd64、macOS对应darwin/arm64、Windows/amd64的Go构建目标，均用Ubuntu Runner交叉分析，不运行GUI或宣称平台安装验收。每目标比较根模块production与含测试两次分析，relaykit在自身目录GOWORK=off单独构建/分析；根模块视角绝不能证明库公开API无外部消费者。原始JSON/工具与构建元数据只存Runner并上传7天构件，日志输出安全结构化条目供主会话不下载产物也能核对。

触发接线：新workflow仅dev文件，不能假定已在默认分支注册的独立workflow_dispatch。为不合main，既有ci.yml增加run_reachability布尔输入（默认false），只在manual dispatch且ref=dev时调用同一提交的reusable go-reachability.yml；callee仍强制事件/分支/source_ref为manual/dev/dev。实际入口为`gh workflow run ci.yml --repo ForceMind/MyAPI --ref dev -f run_browser=false -f run_reachability=true`，同时执行全部原完整门禁；默认push的分析job跳过，不加push/cron分析或后台自动化。依据[GitHub手动工作流说明](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow)。

报告固定analysis_only=true、deletion_authorized=false；候选记录源码文件hash、声明位置、包/符号、prod/test可达性及保护理由。仅测试可达、导出函数、所有方法（含GORM/JSON/driver接口）、init、生成文件、测试源与relaykit全部先保护/人工复核；不承诺外部消费者、反射或其它构建标签都已覆盖。计费相邻标记只是启发式，不替代逐项业务判断。工具合法null/[]与损坏/空输出区分，未知不补零；源码越界/位置/输入冲突不悄悄猜测。

本批不删除或移动产品文件，不改转发/计费/权限/认证/数据库/版本配置，不发布/部署/main/tag/GHCR/NPM或发模型请求；应用保持beta.42。后续A3实际精简、A4–A6/B3/B4/B5b后续/B6/B7及统一质量/七语言/全量验收仍在同一Goal。

## 2026-11-08 A2：Lite配置兼容预检与实际部署来源（beta.42候选，三库/完整CI为门禁）

上一轮为实质进展：B5b1源码及非浏览器验证已交付，dev/`65325d7926d706fe66aaac3c5e1cdd52c686a40f` / [完整CI42385209689](https://github.com/ForceMind/MyAPI/actions/runs/42385209689) attempt1 success，10执行作业成功/1浏览器真实API跳过；主会话亲读17305行完整日志、全部步骤与10个checkout一致，失败/race标记0。原生105/105且0skip，前端421文件3452/3452，根/独立relaykit/386/三库/全部既有race通过。同SHA额外[静态打包42385209664](https://github.com/ForceMind/MyAPI/actions/runs/42385209664)和[静态检查42385209663](https://github.com/ForceMind/MyAPI/actions/runs/42385209663)成功，全部431/382行与步骤/checkout已读，没有浏览器或部署。beta.41仅源码交付；beta.40旧安装包仍绑定13eedad9，不重标新源。完整B5b公网外部验证仍在原Goal，不以入口记录向导代替。

本轮开工dev/65325d79干净，fetch后0/0、ff-only最新，基线CI仍success。复用A1报告及既有支付、密码、Passkey、来源/身份/batch保护，只实施PLAN_PHASE2 A2；R4保持关闭，不重开R1–R3/R5–R8或其它BUG专项。不新增页面或重画UI，已有方向3/效果图/线稿保持；本批交付是后端启动门禁、独立只读检查及CLI/安装接线。

成功标准：Lite启动迁移前可识别未完成充值/订阅购买、无本地登录配置及无法确认的数据；未知不放行，检查不改订单/账务/账号/绑定/身份；CLI核对实际受管容器而不是把已编辑.env当成当前版本，拒绝把普通升级当未确认切版。验证方法：专属SQLite/MySQL5.7/PostgreSQL9.6矩阵与只读/行和文件保全、服务判定及实际main子进程、CLI/真实Bash配fake Docker合同，加完整CI全部在Actions；推送后按最终精确SHA亲读所有作业/步骤/日志。Go格式、JSON/YAML/Node/Bash语法与差异只作本机源码轻检查，不编译/测试/装依赖/跑Docker或浏览器，不下载产物。

已锁定文件与分工：2名gpt-6.1-sol/high代理分别唯一负责新model只读facts与三库/SQLite回归、CLI/installer与合同；主会话负责service判定、main首flag/启动接线与回归、CI新A2数据库作业、既有五处beta.42值和本交接。无新代理或递归。候选范围：

- 原生只读连接解析同一SQL_DSN primary，复用现有SQLite/WAL保全和三库只读事务工具，不改旧金额/writer/execution规则。检查仅5表：top_ups、subscription_orders、users、options、passkey_credentials；user_subscriptions活动权益不作为pending购买订单。
- 区分真实新库、已有库、部分schema/外部表/视图及不可确认；失败的三个facts段为null，不补0。订单精确识别pending/success/failed/expired，NULL/未知状态不自动改为终态。MySQL必需表均InnoDB才声称跨表一致读，不对生产ALTER引擎；三库自有fixture对照失败/终态/未知及行不变。
- 本地认证仅检查配置兼容：精确已保存/启动默认/非法的PasswordLoginEnabled与passkey.enabled，启用且未软删账号的受支持bcrypt/结构有效Passkey候选及缺失/未知，含Root；不返回hash、credential ID、公钥或OAuth标识。Passkey保留为本地方式，候选不等于实际登录，报告固定scope=configuration、login_continuity=not-verified、runtime_login_verified=false；建议切换前用原合法登录流程确认，不自动设密码/开关/绑解绑。
- 新首flag `my-api --edition-check=<full|lite|lan> [--json]` 在普通InitEnv/DB/日志/Redis/后台/HTTP初始化前处理，匹配.env读取优先级和SQLite原位选择；不以旧二进制可能忽略的子命令探测。0/1/2分别配置兼容/已阻止/无法确认，JSON只用common包装。Full目标保原功能面，不为Lite缩减检查打开DB；Full启动原路径保持。
- 匿名裸:memory:在每个连接独立，不能把新进程空库当运行实例的旧数据，独立检查报告不可确认；仅明确的全新Lite匿名内存初始化可沿原启动行为(scope=initialization)，不开放用户bypass旗标。命名/shared内存仍不可确认，不打开另库造旧数据证据。依据[SQLite官方内存库说明](https://www.sqlite.org/inmemorydb.html)，对应真实行为仍由Actions测试证明。
- `myapi doctor --target-edition lite [--json]` 为独立只读模式，不跑普通doctor端口bind探测。投影读取同一现有容器ID/immutable image ID/实际edition/状态/data与log绑定/工作目录/非secret存储来源标记，不dump Env、DSN或原stderr；只在该running容器exec原环境的明确后端flag，不pull/up/新建探测容器。报告是时点检查，不是切换授权或后续数据不变证明。
- 普通up/upgrade/installer在首个备份/写配置/mkdir/pull/up前核对来源。实际版与目标不符或未知时拒绝并给出检查/独立切换方案提示；正常受管Full→Full、Lite→Lite、lan↔lite不依赖新bin旗标，已停容器同版可继续；旧bin不支持检查则unknown，不猜成功。原fresh-data/identity/batch/回滚保护及拒绝前零写入断言保留。
- 当前deploy模板未转发SQL_DSN/SQLITE_PATH，不能拿带额外存储配置的运行容器冒充模板默认同库；此情形拒绝且不自动迁移/补变量。根docker-compose手工MySQL/PG形态和原生后端仍保三库，不宣称由该CLI模板覆盖。

候选beta.42只在dev源码与Actions验证，不发布/部署/合main、不改实际生产配置或发模型/供应商请求。新增专属A2三库作业，缺任一DSN必须失败，普通全量跳过关系库专属入口不当作三库已验。浏览器、真实供应商、实际登录/认证器、真实主机/大库/公网尚未验证；A3–A6/B3/B4/B5b后续/B6/B7及统一质量/七语言/全量验收仍按同一Goal继续。

### A2首次CI与主程序覆盖补齐

首个产品提交`f308bfcd7ed35e7c4c140186443fba7a8e0a991d` / [CI42392877229](https://github.com/ForceMind/MyAPI/actions/runs/42392877229) attempt1 success，11执行作业成功/1浏览器真实API跳过；全部18414行日志、步骤和11个checkout已亲读，失败/race标记0。新增A2三库job128463957725完整834行已读：SQLite5/MySQL8/PostgreSQL7共20叶场景全部PASS、无skip，含MyISAM拒绝与原行保全；CLI主合同181/181，完整分发job2869行及全部步骤已读。前端3452/3452、Native105/105/0skip与既有门禁通过；两静态网站工作流42392877216/42392877332完整387/431行及源码/步骤已读成功。

**本轮验收尚未结束。** 主会话在核对实际测试命令时确认`Makefile:49–54`的`make test`明确排除了根目录main包，因此新增`edition_compatibility_command_test.go`里的独立旗标/实际main子进程/迁移前启动拒绝5个Test函数尚未执行；不能把上述整体绿冒充这部分已验。只在CI backend增加`go test -p 1 . -run '^TestEditionCompatibility' -count=1 -timeout=120s -v`（已有embed占位），不改变Makefile、产品逻辑或测试断言，不重升beta.42。与本段交接合并一次提交，按最后SHA重读完整CI；只有新主程序专项与全部原门禁均通过才交付A2。本机未编译或运行测试，继续只Actions。

## 2026-11-07 B5b1：已有HTTPS反代接入向导（beta.41候选，完整CI为门禁）

上一轮为实质进展：B5a源码与双平台非浏览器打包已交付。dev/`13eedad93b365f1a00f45da27faf05e18409af9d` / [完整CI42379188640](https://github.com/ForceMind/MyAPI/actions/runs/42379188640) attempt1 success，10作业成功/1浏览器真实API跳过；主会话亲读16195行完整执行日志、全部步骤和10个checkout一致，失败/race标记0。前端421文件3452/3452，Native59/59且0skip，根/独立relaykit/386/三库/Redis/ClickHouse/全部既有race通过。原生owned-child合成进程实际close、无关进程不受影响通过，不冒充GUI/安装验收。

同一13eedad9的[macOS42379249054](https://github.com/ForceMind/MyAPI/actions/runs/42379249054)成功，完整1381行/全部步骤及实际checkout已读：59/59/0skip，arm64 DMG+ZIP，minimal、VITE_BUILD_ID及Go/Electron版本均为该源码/beta.40；release跳过。[macos-build13576574439](https://github.com/ForceMind/MyAPI/actions/runs/42379249054/artifacts/13576574439) 276358755 bytes，构件ZIP SHA256 `56e9c2f3c897ea53b735d4c8277e9e69e10397aae9df10f0aa4d6e9df9c925d6`，有效至2027-02-05 13:56:38 UTC。

随后同SHA的[Windows42379624002](https://github.com/ForceMind/MyAPI/actions/runs/42379624002)成功，完整1400行/全部步骤已读；Windows git.exe实际checkout原始行另行精确核对13eedad9，59/59/0skip，win32 x64 NSIS+portable、minimal与各版本身份一致；release跳过。[windows-build13577680791](https://github.com/ForceMind/MyAPI/actions/runs/42379624002/artifacts/13577680791) 227981950 bytes，构件ZIP SHA256 `2d9a48f130a9c72b4ecb0e0fa110477ec4997dc3f46583bcd5d8fb1289c8ccac`，有效至2027-02-05 14:01:30 UTC。两个包始终绑定beta.40/13eedad9，不重标为本批beta.41；本机未下载，未发布/签名或安装运行。

本轮开工dev/13eedad9干净，fetch后0/0、ff-only最新，CI元数据仍success。继续完整Goal，R4已关闭，不开展新BUG专项。B5b1先交付“接入已有HTTPS反代”的本地四步向导，**不以本批代替完整B5b外部验证目标**。实际DNS、HTTPS/证书、反代/鉴权和外部网络可达性仍未验证，格式通过、本机健康和LAN启动均不能证明公网可用；后续版本工作保留这些明确边界。

成功标准：小白能分清当前真实监听与自己提供的公网入口，按反代位置→外部条件→HTTPS根地址→客户端配置完成交付；明确保存/清除仅操作此电脑固定私有入口声明，不修改监听、后台配置、数据/Key、反代/隧道/防火墙或外部端口。验证方法：共享URL校验、真实main/IPC/HTML/renderer的Node/VM回归、原生命周期/public-bind/LAN和全部既有门禁仅Actions，提交推送后按最终SHA亲读完整CI。本机仅源码、格式、JSON/XML/AST/node --check等轻检查，不运行项目tests/lint/typecheck/build/Docker/浏览器或下载CI产物。

设计与文件边界：先用内置imagegen参考原Desktop效果图生成[公网接入向导效果图](ui-design-v2/screenshots/desktop-public-access-concept-20261107.png)，已看图，明确概念/示例/非实机；再只更新既有desktop-tray的两SVG、单catalog对象/article及作者源，仍329视图，其余328保持。复用2名gpt-6.1-sol/high代理，main/module/IPC与其直接测试、panel/renderer与其直接测试各有唯一owner；主会话负责线稿、规范locale脚本、资源白名单/checker、beta.41五处版本值与本交接/CI，没有递归或新建代理。

候选行为与必要保护：

- 原本机服务/托盘、关窗/停止/退出、配置/secret/显式SQLite路径及LAN/private/public-bind限制保留；公网向导默认折叠，分步只展当前任务，反代异机只提供原LAN明确入口，不因填写地址自动重启。
- 共享纯校验模块只接HTTPS根origin及可选端口，明确拒绝路径前缀/已附v1、凭据、query/fragment含空参数、控制符/反斜杠/超长输入；已知本机/LAN入口回对应分支。域名/IP格式不作DNS、TLS或可达性证据，不请求或打开输入URL。
- 固定私有声明、可信本地file主frame、专用有界IPC、默认取消原生确认，返回后复核来源/服务身份/声明revision与忙状态。新公网地址与旧本机复制集合分开，明确点击只复制所见且仍准确匹配的/v1，旧回包/撤销后旧值不可复制。
- 未配置、无法读取、当前不支持、外部状态未知和已存未验证分开；读取坏文件不冒充空配置。合法DTO但不可读时先专门确认清除本应用记录，成功变为空记录后才允许保存新地址；失败保旧字节与状态，不自动修权限/删除外部文件。保存失败保旧记录和草稿，刷新不重置步骤/输入或抢焦点。
- 主窗不读Key、不改客户端自动导入；保存后打开原本机控制台选择Key。清除只解除本应用声明，外部反代须自行撤销；个人电脑的开机/休眠/断网/退出/地址变化、上行带宽与资源/额度影响通过折叠帮助可达，不替用户选厂商、账号、费用或NAT设置。

本批交付文案新增85条原生English key（69界面/16确认与结果），按i18n-translate规范脚本写en/zh，各196键，临时脚本已移除；JSON与占位符轻检查通过。原生仍仅中英文，Web七语言无新调用或改动，frontend i18n:sync由Actions执行。对本批候选的自查补护包括：规范化后的IDN长度仍受2048上限；实际listenerScope使异机分支在未开LAN时不能继续/保存；保存/清除直接回复固定机器结果与origin/expectedRevision/recordRevision/unchanged，普通snapshot/推送不携上次结果。界面不从状态变化猜本次保存成功；rename后读回无法确认保未知、禁copy且不恢复旧值，只有明确成功的匹配回执才进客户端配置。取消、外部同值变化、窗口/身份失效和旧回包均保留草稿，严格回归仍待本提交Actions，不以代码审读称运行通过。

候选beta.41先做源码与完整CI交付，安装包在Desktop整体收口再按精确源码重打，不以beta.40包冒充本批。没有生产/服务器/模型请求、main/tag/GHCR/NPM或发布；浏览器、真实安装/托盘和公网仍未验证。B5b后续、A2–A6/B3/B4/B6/B7与统一质量/七语言/全量验收仍留在主Goal，不缩减为本批即可完成。

## 2026-11-07 B5a：Desktop本机服务、双语托盘与构建对齐（beta.40候选，完整CI/平台构建为门禁）

上一轮为实质进展：UI-M2源码交付完成。dev/`4601139a48fc875cbd975428f8b1a545b18f4947` / [CI42367969543](https://github.com/ForceMind/MyAPI/actions/runs/42367969543) attempt1 success，10执行作业成功/1浏览器真实API跳过；主会话亲读15918行完整日志、步骤与10个checkout一致，失败/race标记0。前端421文件3452/3452，新增节点5/5与路由13/13在专项和全量均通过；精确0、ID8/旧ID消失、无抢焦点/scroll0等断言保留。首轮2d242099前端4221行/3450过2失败及对应严格测试修正保留；不把失败当绿，不重开R4或其它历史BUG。

本轮基线dev/4601139a干净，fetch后0/0且ff-only最新，基线CI元数据仍success。沿完整Goal进入B5第一个可交付迭代；**B5a不是全部B5**，独立公网向导B5b及其可达性验证继续保留，不为了小范围过测缩减完整计划。旧public-bind拒绝、LAN明确同意及私网约束不放宽。本机服务不等于公网可达，公网第三方/端口/账号/费用仍由用户选择。

成功标准：小白能分清关闭窗口留托盘、仅停止本应用内置服务、确认退出应用；后端失败/停止后本地面板仍可用，控制动作只作用于本应用spawn的child，配置/数据/身份密钥路径保留，不复制旧库、不重放请求。验证方法：实际controller、真实main/IPC/preload/renderer的Node VM合同、GHA临时合成子进程close握手、既有runtime/public/健康探测合同，以及完整CI全部在Actions；再按macOS先Windows后执行无发布的Electron构建，验证源码/版本/minimal/本地资源打包。不会启动Electron GUI或浏览器，不以Node mock合同声称真实托盘/OS交互通过。

本批设计→实现顺序与范围：

- 先制作[Desktop服务效果预览](ui-design-v2/screenshots/desktop-service-concept-20261107.png)，标明示例状态/非实机截图；再仅更新既有desktop-tray一对象/一个article与两SVG。仍329视图，其余328保持；窄窗口SVG只表达排列，不宣称移动App支持。
- 复用2名gpt-6.1-sol/high代理：运行UI owner负责main、独立file面板/独立preload、固定IPC/native i18n模块；controller/测试 owner负责无Electron依赖的owned-child状态机、Node/VM/合成child及build helper/tools/workflow；主会话集成、资源白名单、规范locale脚本、版本/交接和CI亲读。没有新代理或递归派工。
- 保持旧网关HTTP `preload.js`只读能力；新本机面板只用固定窗口+主frame+精确本地file URL，确认返回再复核lease/child/generation/身份。仅固定动作，不给任意invoke、命令、PID、路径或URL；DTO不含环境/secret值/正文/原始stderr，DOM按文本渲染，面板禁任意导航和新窗。
- controller独占child/operation/generation/timer；状态分开stopped/starting/running/stopping/failed/stop-unconfirmed/external。kill返回true不算已停；旧probe/close/timeout不能操作下一generation，停止未确认保child、禁第二个spawn，不假装安全退出。缺ready探测不能默认running；dev外部服务不开始/停止，仍保既有开发前端探测。
- main先建托盘和本地面板再启动，状态不依赖网关HTTP页面；关窗不退出，停止只停owned child，退出/已确认LAN relaunch等真实关闭后才继续。确认默认取消，控制串行/去重；启动探测可取消，退出不被未完成probe拖住。不自动重启/重发模型、恢复快照或动其它进程。
- `userData/data`、日志目录、显式MYAPI_SQLITE_PATH/SQLITE_PATH优先级、旧DB原位采用、SESSION_SECRET与Lite后端环境保持；相关备份提示只说准确数据/身份/显式路径范围，不再承诺复制单一目录等于完整备份，不在本批新增备份工具。
- 托盘/本地面板/相关确认双语，111个en/zh原生键由规范脚本写入，非中文OS语言回退English；语言偏好仅固定本应用userData配置，写失败明确仅本次有效。Web已有Desktop引导新增1键(七locale齐全)同步为不依赖旧Quit英文标签的语义，不往HTTP页面加控制。
- `electron/build.sh`与`electron-build.yml`对齐root VERSION、实际checkout HEAD、Lite/minimal品牌、Go ldflags和Electron有效extraMetadata.version；私有包1.0与依赖树不另建版本体系。AppId/userData/发布坐标、publish:null、签名/发布门禁及自动更新关闭保持；manual平台选择默认both兼容旧用法，允许先mac后win。Linux旧开发分支保留，不称首发支持。

必要验证资产包括新controller/main/IPC/renderer/合成child Node测试与固定fixture、tools/desktop合同、原runtime-config及desktop-probe合同；生产探测仅多了本次AbortSignal第6参时，静态签名严格要求signal，原status/env/logs/dev断言全部保留，并以真实VM补更强取消/JSON/非2xx行为。Root package白名单追加controller、安全诊断模块、面板/IPC/preload/CSS/i18n及en/zh资源；测试fixture不打包。

候选统一beta.40，仅dev源码与GHA构建产物；本机未安装依赖、运行项目tests/lint/typecheck/build/Docker/浏览器或下载CI产物，只有源码/既有格式器/JSON/XML/语法轻检查。无生产/服务器/真实供应商模型/main/tag/GHCR/NPM/发布或新签名证书。首次候选`b54b0cd191cae5d7965783fb360be8bb21a6ff33` / CI42374604372的Desktop job128409516612完整434行日志已读：仅“packaged UI follows the effective LAN bind host”静态check失败，Node行为尚未执行；checker先前同步为局部别名window.loadURL，最终main仍调用mainWindow.loadURL，实际loadHost继续getHealthCheckAddress(BIND_ADDRESS)，URL仍固定http://${loadHost}:${loadPort}。只将checker精确引用同步回实际同一console窗口调用；host/port/旧public/LAN及VM真实行为断言不减，main运行逻辑不改。修正后继续完整Actions，不把此失败当功能测试或最终绿。

第二候选`225d6b76d9488c9fae08c1a9ce71498eb7df340e` / CI42374795810的Desktop job128410139447完整808行已读：静态合同过，Node59项57过/2失败/0skip；仅renderer前两case在mount返回时DOM尚未完成(state空、第二地址节点不存在)，主进程/控制器/IPC与GHA合成child已有实际PASS。测试helper只await外层Node的initialResponse，不能代表真实VM async续体已完成渲染；后续仅让helper等待真实最后locations.replaceChildren，非法初始状态等真实validator读过revision再核拒绝、缺bridge保同步禁用。原Running/地址/版本/revision/无副作用等断言不改，不加sleep或任意微任务次数，不修改产品来同步化测试。实际结果继续以新提交完整Actions为准。

结果须按最终提交精确SHA亲读自己的完整CI与平台构建，不提前称完成；公网B5b、A2–A6/B3/B4/B6/B7及统一质量/七语言/全量验收仍在主Goal。


### B5a平台构建续办（测试路径修正，结果按最终SHA核对）

第三候选`f7220ea9f985fd3693938b22b4d59880200ba391`的完整CI42375347710已成功：10执行作业成功，前端3452/3452、Native59/59且0skip；主会话亲读16293行完整日志和10个checkout一致，失败/race标记0。macOS构建42378099362成功，同SHA Native59/59/0skip，实际arm64 DMG与ZIP版本0.2.0-beta.40、minimal与VITE_BUILD_ID=f722一致；全部1428行、步骤/源码已亲读，release跳过。构件13575844254（macos-build）276358764bytes，ZIP SHA256 `70a659128c47950cf1b97f93a508c8afa2cd4489f1f6100c563d393d3496015c`，有效至2027-02-05 13:41:19 UTC。本机未下载，该包始终绑定f722，不重标为后继源码。

随后Windows构建42378665848在Native门禁失败，完整1073行已读：59项55过4失败、0skip，未进入打包，release跳过。四处均为Main VM夹具混用硬编码POSIX fake-fs键与Windows原生node:path：canonical假查无文件误选legacy，语言JSON与diagnostic键查得undefined；真实合成child及其它55项过。仅在`electron/test/main-lifecycle.test.mjs`以host path.normalize/join建立准确synthetic资源、userData、日志、DB、prefs和diagnostic常量，fake-fs只认明确canonical集合；不改产品，不接受canonical/legacy二选一，不减JSON、0600、秘密及字段断言，也不normalize用户显式数据库环境原字符串。测试修正已提交`a6c485b2`；该轮交接脚本因UTF-8输入错误未写文档，随后仅补本段，不重做产品。新最终SHA须重新完整CI、macOS后Windows构建；旧mac包保留原身份，GUI/真实安装仍未验证。

## 2026-11-07 UI主线M2：Full首页保存模型与路由快捷入口（beta.39候选，完整CI为交付门禁）

上一轮为实质进展：UI-M1的Lite三步接入、有限保存窗口摘要、Full中性核心和准确请求返回已通过非浏览器验证。dev/`de4ee4d2571ce6634796e59d3c57dbcc1718da6c` / [CI42362321291](https://github.com/ForceMind/MyAPI/actions/runs/42362321291) attempt1 success，10作业成功/1浏览器真实API跳过，主会话亲读16542行完整执行日志/步骤，10个checkout一致，失败/race标记0；前端419文件3434/3434、首页44/44、平台流程16/16（新增桌面/手机返回与modified click三场景在专项和全量均明确PASS）。根/独立relaykit、386/race/三库/Redis/ClickHouse/分发/Desktop门禁通过。旧599ee3d2 lint21错误、420da142测试夹具漏capabilities及主会话误判步骤成功已留证并更正，不把取消当完整绿。M1对应源码收口，浏览器未验证，不代表三版本全量完成。

本轮dev/de4ee4d2开工干净，fetch后0/0、ff-only已最新，基线CI元数据仍success。沿[同一三版本效果图](ui-design-v2/screenshots/three-editions-home-concept-20261107.png)，只细化既有overview桌面/手机2SVG及一个catalog对象/article；329视图与其它328对象保持。复用2名gpt-6.1-sol/high代理，分别唯一负责FlowNode/FlowBoard与父层返回/真实路由回归，主会话负责线稿/i18n/版本/集成/CI；没有新代理或递归派工。

成功标准：Full首页连接节点可展开完整已保存模型ID，显示优先级/权重且区分明确0与缺失，准确进入同ID的原routing页签；节点筛选、模型展开、路由跳转各自可键盘操作且互不误触、不嵌套button。返回保原节点/activity/flow与准确入口焦点、滚动；换actor/role/SID不套旧恢复，加载/失败不借另一行。验证方法：实际组件/真实路由回归、types/lint/i18n/构建及全部既有门禁仅Actions，推送后按最终提交精确SHA亲读完整CI；本机仅源码/格式/JSON/XML/差异轻检查，浏览器未验证。

本批候选范围：

- 使用现有ConnectionSummary.models/priority/weight与connectionValues，去空白去重；模型缺失为未知、明确空配置可为0，表示保存配置而非供应商实时支持集合。展开不发现模型、不查询供应商、不读取密钥。
- 原整卡筛选button保留为独立控件；模型展开与路由Link在同一非交互容器中并列，保手机管道/44px目标及完整长名称。Full仅在状态和读取权限/导航可用时启用工具；Lite沿M1行为，不新加编辑器或后台edition。
- routing Link精确`/connections/$connectionId?tab=routing`，仅跳转只读入口；原页的新鲜权限、字段编辑、确认与写入未知恢复继续复用。首页无新增API或写入，模型工具不更改节点筛选。
- 入口恢复只保有界的数值ID/坐标和actor/SID归属，不保存正文/凭据；只捕获同tab无修饰点击，等待真实相同ID可见，用户主动操作可取消等待恢复。

统一候选beta.39（既有五个版本/分发示例），新增2键通过t(English key)及规范脚本补en/zh，其他五语言英文回填；同步/构建测试全在Actions。本机无依赖安装、项目测试/typecheck/lint/build/Docker/浏览器或CI产物下载，既有oxfmt仅格式化本批源文件。无生产/服务器/真实供应商模型/main/tag/GHCR/NPM/部署。实际结果须读本提交完整CI，未验证不称完成；后续Lite/Desktop真实版本能力、A2–A6/B3–B7仍按主Goal，历史BUG不单独重开。

首次M2候选`2d2420991bb32d4a3449e882546b5adfea87c4cc` / CI42366293337的前端job128384970267完整4221行日志已亲读：源码/i18n/构建检查通过，全量421文件中419通过、2失败，3450例通过/2例失败，仅本批两个新增测试。Node5中旧Lite的Priority文本保留“· ”分隔符，原精确匹配漏分隔符；routing13中removed分支释放请求后立即取得缓存ID7而非等待新ID8。后续只修这两份新测试：严格匹配原可见“· Priority 0”，等待真实DOM精确ID8出现且7消失，再保留不获焦点/scroll0原负断言；其余16个新场景、全部旧断言与产品逻辑未改，未放宽为任意ID或可接受Unknown。候选仍beta.39，修正提交重新完整Actions，失败/被替代取消都不算绿。


## 2026-11-07 UI主线M1：三版本接入入口与首页信息层级（beta.38候选，交付以完整CI为准）

上一轮为实质进展：R4最小产品修复已交付。dev/`09be9f7a8c238657b82171df9f3aea5c74bfbcb3` / [完整CI42355744255](https://github.com/ForceMind/MyAPI/actions/runs/42355744255) attempt1 success，主会话已亲读10份执行日志共16517行、全部步骤与10个实际checkout一致，失败/race标记0；前端419文件3417/3417，根/独立relaykit、386/race/三库等门禁通过，浏览器与真实API按边界跳过。[R4三库专项42354605593](https://github.com/ForceMind/MyAPI/actions/runs/42354605593)原始Go exit0、12case全部pass，原断言未弱化。此BUG关闭，本轮不再追R1–R3/R5–R8或开新BUG审查；持续主Goal保留完整Full/Lite/Desktop版本计划，不以本批代替全部交付。

本轮开工dev/09be9f7a干净，fetch后0/0、ff-only已最新，基线CI元数据仍success。沿既有方向3与已入仓[三版本效果预览](ui-design-v2/screenshots/three-editions-home-concept-20261107.png)，复用2名gpt-6.1-sol/high代理：一名唯一负责首页/网关核心与组件回归，另一名仅细化既有3视图/6SVG及精确元数据；主会话负责最近请求返回、i18n、版本/交接、集成与Actions亲读，无新代理/递归派工。

成功标准：Full保持流向任务与全部信息，核心卡紧凑中性；Lite/现有Desktop壳的首页以“连接→Key→客户端配置”入口和真实BaseURL为主，用户知道链接实际落点，不假造已创建或接入成功。全部字段、权限、未知/失败/风险、高级统计仍可达；从首页请求回来保持原节点/结果筛选、展开状态、滚动和准确请求焦点。验证方法：本地仅代码/格式与JSON/XML/差异轻检查，实际组件/URL/真实路由返回回归、类型/lint/i18n/构建/全部既有门禁只在Actions，提交后读取本提交完整CI与checkout。浏览器未验证，不以静态SVG/DOM合同证明实际视觉。

本批可查看/操作的候选交付：

- 线稿只改overview、connections、key-delivery三对象及6SVG；catalog/index只改同3对象/article，329视图/658归属SVG不变，其余326对象保持。作者源的历史目录差异不扩大为清理/删除；没有运行全量生成或删除输出目录。
- 首页沿真实full/lite能力；Desktop仍是Lite原生壳，不新增第三个后端edition。Full保原图、计数、P50、小时图、样本覆盖、地址复制、meta、funding/site panels条件，核心卡改中性紧凑表面。
- Lite三任务复用当前连接/Key管理页与Key交付，不伪造直接创建query、不自动选Key/读密钥或探测供应商。无可用入口时给联系管理员的下一步；已知正常加载/空/未知与真正读取失败分开表达。完整流向/统计通过键盘可展开，原GET重读保留，待处理和保存观测状态在外侧；保存窗口按连接/账户/周期/单位展示，有界首页摘要与完整明细分开，不把来源不完整当全量正常。
- 首页请求的activity与Lite flow展开写入已校验URL，原Key/连接筛选保留；同tab无修饰点击复用既有actor+SID绑定返回缓存，实际数据就绪后恢复准确ID的焦点/滚动。修改键/独立打开不写回原位置，不保存正文或凭据；请求页原鉴权/关闭回退不改。
- Desktop仅依据现有electron flag显示已有关窗留托盘/退出停后端及访问范围引导；没有新服务状态、停止按钮、LAN控制或IPC，原生能力仍在B5。

候选统一beta.38（VERSION、根package和三个分发示例）；新增45条产品文案均t(English key)，经规范脚本写en/zh、其他五语言英文回填，i18n同步由Actions执行。所有项目依赖/测试/构建仅GitHub；本机未安装依赖、跑项目测试/typecheck/lint/构建/Docker/浏览器或下载CI产物，既有oxfmt仅格式化本批源文件。没有服务器/生产/真实上游模型、main、tag、GHCR/NPM或发布操作。当前是候选，实际CI结果必须按最终提交精确SHA回读，未验证不称完成。

首次候选`599ee3d234eab8c4872232afd5801833feed355b` / CI42360650724的前端job128368128834在早期源码检查失败；主会话已亲读353行完整日志和精确annotations：26个既有warnings、21个新增lint errors（多行if大括号与新测试非空断言，包含本批格式化引出的原单行分支）。实际组件回归尚未执行，不称红绿回归已成立。后续仅在本批4个TS文件补大括号、把非空断言换成保留原expect的明确fatal setup校验；逻辑、数值、权限和所有行为断言未放宽。候选仍beta.38，修正提交继续完整Actions；被推送替代的运行取消不算完整绿。

第二候选`420da142159ac7f8f4ea39d4ae481bc5a7f52a2d` / CI42361075864的前端job128369455644通过源码检查，首页44个组件场景通过；完整5588行前端日志显示仅本批3个新首页返回场景失败，3431通过/3失败，旧行为断言未删。首个差异是新测试夹具只报product_edition=lite而漏了真实后端会返回的capabilities；默认站点内容能力因此仍开启，未配置的uptime响应{}导致groups.map异常，阻塞请求页/返回页。仅补测试的完整Lite能力画像，并新增等待真实接入区域出现的断言；没有为夹具修改UptimePanel或放宽返回断言。主会话此前按“进入下一步”误报返回专项通过，已明确更正：该工作流会在失败后继续执行，必须核对步骤conclusion和真实日志。后续仍按最终提交读取完整CI，不计取消为绿。

剩余主线不缩减：本批不是三版本全量实现或视觉验收；Full首页模型计数展开/权重等快捷入口与完整视觉对应继续沿既有组件细化，Lite闭环的后续状态与Desktop B5真实控制分批交付；原A2–A6/B3–B7保留（删除旧文件仍须精确范围授权），最后再做统一质量与翻译/全量验收。既有连接/Key/请求详情不重做，浏览器/真实平台/生产另有边界。


## 2026-11-07 R4最小产品修复与主线恢复（beta.37候选，交付以最终完整CI为准）

维护者已明确同意最小修复；本轮不再停留在取证授权。基线dev/`b946e40098f766e85544ce4654dd5a49585d64da`干净，fetch后0/0、ff-only已最新，基线CI42346687092仍success。本轮复用2名gpt-6.1-sol/high代理，分别唯一负责产品预检与对应回归，主会话复核关键差异、版本和交接；不新增代理、不递归派工。

成功标准：真实合法direct_token增减能通过公开启动金额预检；普通负收费、越界和身份不明负记录继续拒绝，预检不改任何数据。验证方法：复用已保存三库红证据，在Actions执行原R4三库专项和完整CI（含金额回归、三库及独立relaykit门禁），原NoError和只读/幂等断言不改；浏览器未验证。

从代码看，改动仅在`model/quota_amount_preflight.go`为回执的两个金额最低值检查添加可识别例外：phase/billing_source/billing_preference字节精确为direct_token/token/direct，requested与applied相等、非零、均在原signed单笔界内。NULL身份明确拒绝；缺识别列、隐藏列、固定CHAR及未知存储类型回原非负规则，旧表合法非负值继续兼容。不改变通用收费下限、单笔上限、余额/回执写入、schema、计费公式或转发。对应测试补在现有`model/quota_amount_preflight_test.go`；未修改R4原目标断言或报告checker。

候选版本按既有体系同步为beta.37（VERSION、根package与三个分发示例），不表示已发布或可部署。实际构建测试全部Actions，本机只代码/文档/Git/格式和差异轻量检查，无依赖安装、构建测试、Docker、浏览器或产物下载；不操作生产、服务器、真实模型/供应商、main、tag、NPM或GHCR。三库专项已亲读；本批最终交付仍要求收尾提交的完整CI success、全部执行日志与checkout SHA回读。源码首推主CI42354593006由最终收尾门禁接续，不单独拿其进行中状态或取消当完整绿。既有三库红证据及过程错误保留于下节。

三库绿证据：源码`2795b70c672613cebd5e8f68be29474d98b578c1` / [专项42354605593](https://github.com/ForceMind/MyAPI/actions/runs/42354605593) attempt1 success。主会话亲读唯一job128349820837全部1447行日志、步骤/checkout与完整JSON；SQLite3.50.4、MySQL5.7.44、PostgreSQL9.6.24共12case全部真实pass、原Go exit0、actual_failed_tests为空、evidence_valid=true/status=not_reproduced。每库合法扣100后Key500→400/used100/version1、钱包1000不变、direct_token -100回执仅1条且同key不再扣；公开预检现已接受该记录。合法加额、普通预扣/结算和非法负收费拒绝对照仍过，预检前后三库行与SQLite文件/目录不变。报告固定product_fixed=false表示合成报告不作产品/生产整体证明，本次修复交付另须完整CI；不是专项断言失败。未执行完整服务/容器重启或生产更新。

[证据构件13566957572](https://github.com/ForceMind/MyAPI/actions/runs/42354605593/artifacts/13566957572)：5397bytes，ZIP SHA256 `1839ad51b067295167ab6205c3221a224ef591488334213919430aa281a274db`，有效至2026-11-14 08:17:41 UTC；从日志读取报告、本机未下载。

维护者最新主线决定：本BUG验证交付后关闭本次专项，不再主动逐项追查R1–R3/R5–R8或开展新一轮找BUG；历史发现保留到版本计划完成后的统一质量阶段。继续原三版本计划与方向3“流向图”，已有页面/功能/线稿及B1/B2/P1-9/受限Ops成果复用。后续设计按“效果图→线稿→实现→验收”逐步推进，Full、Lite、Desktop先给各自任务入口、共用与差异，再落实当前迭代；不把新设计当重新搭建授权，不删尚未迁移的功能/权限。小白交互规则继续有效，浏览器仍需单独授权。

下一主线先沿[三版本首页效果预览](ui-design-v2/screenshots/three-editions-home-concept-20261107.png)细化，概念数据非实机截图，当前未改产品UI。执行顺序与原计划对应如下，当前迭代只展开首页/任务入口的设计差异，不重做已收口页面：

| 顺序 | 可查看或操作的交付 | 复用与后续范围 |
| --- | --- | --- |
| 1 | 三版本首页效果图→对应桌面/手机线稿：Full调用链、Lite三步接入、Desktop本机服务；标明主操作/状态/进入与返回 | 共用Logo/顶部导航/卡片与按钮；既有329视图线稿及已实现上游/Key/请求详情优先复用，原权限/字段/高级操作不删 |
| 2 | Lite个人接入闭环：添加连接、按连接获取模型、创建Key、复制/导入客户端、看保存窗口与请求结果 | 复用既有闭环；A2的版本切换条件按原范围落实，Full团队/商业能力与Lite可达范围说清，不隐式更改edition |
| 3 | Desktop本机体验（B5）：首次接入、托盘中英文、关闭窗口/退出/停止职责、默认本机与显式LAN访问 | 先macOS后Windows，共用Lite核心；已有LAN/目录/服务能力保留，不重搭；真实平台和浏览器另分验收 |
| 4 | 剩余安装/数据保护/分发（B3/B4/B6/B7），以及原A3–A6结构整理 | 原生安装/升级/停止/卸载保数据，SQLite一致性备份/校验恢复，容器加固评估与真实教程；来源/身份/配置/最新账务门禁保留，删除旧文件仍须精确范围确认 |
| 5 | 版本计划完成后的统一质量收口（C1–C3） | 再集中处理历史待验证项、翻译与完整验收；浏览器、真实服务、发布/生产不借本轮授权 |







## 2026-11-07 R4：Actions三库合成取证与交互验收约束

维护者对“先做Actions三库合成取证，再定最小修复范围”明确回复“确认”，并要求交互操作让小白能看懂。本轮授权是测试/取证/报告，不包含产品金额校验修复或再做一批UI。基线dev/2b6c19a7干净、远端0/0，[完整CI42339945718](https://github.com/ForceMind/MyAPI/actions/runs/42339945718) attempt1仍success；主会话与2名gpt-6.1-sol/high代理分别负责集成/反例校验、Go三库fixture、专项workflow与报告，不递归派工。

成功标准：真实authoritative正常扣减、精确余额/used/version/回执与同key单次效果先成立；关闭临时DB连接后调用InitDB实际使用的公开金额预检，区分预检是否拒绝；SQLite/MySQL5.7/PostgreSQL9.6与3项对照均有结果，预检不改数据。合法扣减的目标断言保留require.NoError，不改成期待拒绝；只有精确目标红可计复现，编译/初始化/缺库/额外断言不算。验证方法：仅Actions临时服务，SQLite=t.TempDir、关系库literal127.0.0.1+myapi_r4_probe门禁及每case新库，拒借生产/default SQL_DSN；没有完整服务重启、生产访问或模型请求。主CI可明确跳过opt-in专项，另有手动专项执行；报告的evidence_valid不等于product_fixed，product_fixed始终false。

### 面向初学者的后续交互验收规则（本轮只记录，不称UI已实现）

- 先解释“发生了什么、现在是否成功、下一步做什么”，技术ID/原始JSON/账务阶段等放可展开详情，不能仅给code或笼统“操作失败”。
- 按任务命名按钮并交代影响，例如读取保存记录与查询上游是不同操作；不能用一个“刷新”让用户误判是否会联系供应商、改配置或扣费。
- 未获取、不支持查询、读取失败、已读取要分清；用量未知不补0，只有可比较观测才显示带时间/依据的估算，不把估算写成真实账单或缓存命中证明。
- 写入结果不明时先“查看结果/核对记录”，不能诱导再次提交、自动重放或刷新登录重试写入。敏感确认与原权限保留，常见操作的结果说明不能被高级详情完全藏掉。
- 合并重复展示和入口时保留全部字段/高级能力/返回上下文；常用的权重、优先级、启停等必须可达。折叠是信息层级，不代替功能迁移；小白能完成任务与原断言同时满足，浏览器验收仍须单独授权。

### 实际取证结果（产品尚未修复）

- 专项真实源码`5455efdfdad872e154149f47ebeb4822fbfb3c36` / [R4 Actions42346062834](https://github.com/ForceMind/MyAPI/actions/runs/42346062834) attempt1 success。主会话已亲读唯一作业1445行完整日志、步骤、实际checkout SHA及完整安全JSON报告。evidence_valid=true、status=reproduced、product_fixed=false，Go真实exit=1保留；不是将所有红都吞成绿。
- 三库真实版本：SQLite3.50.4、MySQL5.7.44、PostgreSQL9.6.24。12个case：3个合法扣减的真实NoError目标断言红、9个对照绿。每个case干净基线预检先通过，所有扣额/回执/幂等/只读不变前置均通过；不存在编译、初始化、连接或额外断言错误。报告checker的5个反例测试在Actions通过。
- 直白结论：Key原有500额度，真实扣100后剩400、已用100、version=1；用户钱包仍1000；signed direct_token回执requested/applied=-100且仅1条，同key重放不再扣。关闭连接后真实启动金额预检把这条正常记录拒绝。合法增加额度、普通预扣后结算、非法负收费仍被拒绝三个对照在每库都通过。SQLite文件字节/目录、三库全部fixture行在预检前后不变。
- 只模拟“关闭连接→再次启动金额检查”，未启动完整应用/重启容器、未查询生产writer配置，不据此认定线上已经触发或所有部署均受影响。产品代码与beta.36镜像未改，R1–R3及R5–R8未顺带取证/修复。
- [证据构件13563123017](https://github.com/ForceMind/MyAPI/actions/runs/42346062834/artifacts/13563123017)：5917bytes，ZIP SHA256 `967dd0db70d1b978ed5fe2e423d24d84bbeaaefe41f18573ef20acbf9bcd7f6b`，有效至2026-11-14 06:16:57 UTC；含原始Go JSON/原始exit/stderr和report.json。报告完整内容已从日志读取，本机未下载构件。
- 过程错误保留：首次`6f4bd971`中job级env误用了job.services/runner.temp，GitHub422拒绝解析，运行42345893786 failure/0 jobs，未执行数据库。主会话承认漏审并只在5455efdf把DSN移step env、目录用RUNNER_TEMP写GITHUB_ENV；不是R4红证据。6f对应主CI42345894902被后续推送取消，不计完整通过；最终文档提交会替代5455efdf主CI42346036650，取消同样不计绿，最终仍读自己完整CI。

### 待维护者确认的最小产品修复方案

成功标准：真实合法direct_token增减记录能通过启动金额预检，普通reserve/settle等收费记录仍非负且在原单笔上限内；未知类型、缺类型而无法证明合法的负记录仍拒绝。预检全程只读，不改余额/回执，不重放写入，不改schema/计费公式/供应商转发/生产配置。

拟涉及`model/quota_amount_preflight.go`（仅按可识别回执类型细分amount规则与三库查询）、`model/quota_amount_preflight_test.go`（合法正负direct_token、普通负charge/越界/未知或缺类型拒绝及只读控制）及必要专项测试合同保留；现有R4合法扣减NoError不得放宽，修复后应真实转绿。无类型字段的历史表继续兼容原合法非负记录，不因缺字段统一报错；负记录不能凭空推断direct_token。精确实现以获批最小diff为准，不把所有receipt列一律改为signed，不将金额预检扩成全账本修复器。

验证方法：先用已保存本轮三库红证据，再在Actions执行针对性金额/单次入账/只读回归、R4三库专项（目标NoError全部真实通过）、完整CI及既有relaykit/三库门禁。构建测试全部Actions；浏览器/生产/真实模型/供应商不运行。修改产品逻辑尚未获本轮授权，当前只提交测试取证与报告。

## 2026-11-07 P2-A1第三批：剩余账务、工具与部署资产收口

维护者明确要求补完A1，沿用本报告/总账，不重做前两批及已完成UI/P1/部署。基线dev/`eea2634e0d84de2a912e8105c50421fb03cbdd15`干净；fetch后0/0且ff-only最新，审查后再次核对同样成立。[基线完整CI42327110195](https://github.com/ForceMind/MyAPI/actions/runs/42327110195) attempt1仍success，沿用上轮已亲读10份日志/实际checkout一致证据，不冒充本轮新反例运行。

成功标准：PLAN_PHASE2 1.2第1–4项剩余源码结构、调用链/权限/账务/三库/恢复及tools/website/deploy都有逐文件或分级覆盖，重要结论附路径/行号，原A1-01..28/R1..3保留；验证方法：实际构建测试仅Actions，本机只源码/文档/引用/差异轻量检查，提交推送后读本提交完整CI。无浏览器、Docker、依赖安装、项目构建/测试或产物下载，无服务器/生产/供应商/真实模型，无main/tag/GHCR/NPM/网站发布。

本轮依原计划分工启用2名gpt-6.1-sol/high只读代理，无递归派工、无代理文件写入：持久账务代理完整读account/quota/usage_review等生产实现并复用补订阅/topup/funding/share；资产代理对183 tracked资产F/K/T/I/G逐路径归档并复用补12个预算/dispatch准入生产文件。主会话补计费service/任务内部/根9个资产，最终资产192项；关键结论亲读抽查超过10项原文，代理报告不自动算运行事实。i18n技能仅用于Desktop文案边界审查，没有改locale或额外跑本机同步。

[现有architecture-review-v2第三批](architecture-review-v2.md)已补A1-29..51共23条账务/任务结论、A1-D1..D16共16条资产结论、完整覆盖索引与A1关闭/后续条件。四个结构审查缺口均有交付：账务/任务持久事实与恢复、首批Lite支付回调、首批仅外部登录账户、全部目标工具/网站/部署入口。F/K/T/I/G不混为“全部源码每行已验”，测试全文/运行故障/真实平台不在只读审查证明中；A1只读覆盖收口，文档交付以本提交自身完整CI为门禁，不等于产品修复或全量真实验收。

新增优先候选R4：authoritative direct_token允许负增量回执，而启动amount preflight对相同两列一律非负；主会话已核创建hook、真实扣额调用和InitDB前置链。代码条件闭合但未运行重启反例、未查生产writer值；建议下一项单列Actions三库反例→最小phase-aware修复方案，**不普遍放宽receipt为signed**。R5首次退款fact确实未落盘时的恢复身份边界、R6邀请聚合int32跨库边界、R7桌面两构建入口参数、R8复制目录备份提示另列待证据/后续B项。旧R1排队gate、R2批次success语义、R3审核并发回查仍未复现/未修复；CI绿不能替代这些反例。

B1/B2/P1-9和既有Ops/kit保持完成；CLI .env备份、CI SQLite夹具、Ops保全和历史审计模拟分清，不写成“没有备份/运维能力”或“用户通用backup/restore已完成”。A2–A6、B3–B7、C1–C3仍按原计划分项实施，历史790/637/37候选未重测，不产生删除授权。只更新本TASKS、handoff/README和现有审查报告，不改产品/配置/版本；已部署beta.36仍绑定f46f19c7/42256473767，不重标为文档提交。

## 2026-11-07 P2-A1第二批：异步恢复与任务账务（只读审查）

维护者回复“好的，继续按照计划”，本轮沿已批准A1继续，不重做UI/P1-9或beta.36部署；只更新本TASKS、handoff/README和既有architecture-review-v2。基线dev/`4e42f4939422e8a7f04de4f6f50ce102dd77ed21`干净，fetch后0/0、ff-only为最新；[完整CI42259867386](https://github.com/ForceMind/MyAPI/actions/runs/42259867386) attempt1仍success，沿用上轮已亲读10作业/全部日志与checkout一致证据，本轮只复核元数据。继续复用现有总账和报告；主代理独立审读，没有新子代理。

成功标准：真实入口→T0意图→T1预扣→T2发送授权→T3结果→终态→回执/投影/日志的责任、状态、幂等、失败和恢复条件有源码位置，明确读过的函数/文件及未读范围。验证方法：本批只读产品/测试代码，不新增或运行行为测试；本机只Git/文本/文档轻量检查，无依赖安装、构建测试、Docker、浏览器或产物下载；唯一提交后亲读自己的完整Actions作业/步骤/日志。文档CI通过不等于新发现已复现，不据此修产品。

[审查记录第二批](architecture-review-v2.md)新增A1-12至A1-28共17条代码结论和实际覆盖清单：已找到controller系统任务handler的真实注册与Master调度，不能把service内无调用误判为未接线；durable入口并非全部任务默认开启；新提交/已有义务/schema gate分开，部署secret与DB身份不自动重绑；发送前原子CAS、实际发送在事务外，发送不明保留预扣且不重放；未发送reserved只按原回执安全释放；终态缺证据/clamp进入人工核对；余额/事件/回执/outbox和Task在事务内衔接，缓存与日志各自持久化恢复，不能凭日志状态重扣或重发。

三个**待验证点**均未实施产品修复或新反例：R1排新行时的Enabled与已有pending行执行时未复核gate的差异；R2单条日志交付失败可持久化retry/隔离但系统扫描批次仍报success的语义；R3人工审核事务失败后的幂等回查与首次读校验不一致，已读顺序冲突测试不能覆盖并发回退。报告给出最小Actions合成取证方法；没有认定生产已触发、重复扣/退、日志丢失或权限漏洞，不改门禁/幂等/退款策略，不把未知补0。

A1整体仍未完成：普通文本/用户/账户账务其余模块、legacy polling/provider全分支、所有请求指纹形式、projection/account事实内部及tools/website/deploy其余覆盖继续保留。3个候选先合成验证/明确契约再给精确修复范围，不在审查阶段顺手实施。产品版本与已部署镜像仍为beta.36/f46f19c7/42256473767，未重新打包、合main、发布或操作生产/服务器/真实模型。

## 2026-11-07 P2-A1首批只读审查与beta.36部署候选

维护者要求“继续按计划开发，可以部署的版本部署一个”。本轮沿原P2计划先补A1两个明确缺口（Lite支付回调、仅外部登录账户），不扩为支付/认证修复；同时为已完整CI通过的beta.36源码打包Full+SQLite。主代理独立执行，无新增子代理。基线dev/f46f19c7干净，fetch后0/0，ff-only已是最新；源[完整CI42207405306](https://github.com/ForceMind/MyAPI/actions/runs/42207405306) attempt1仍success，沿用上轮已亲读10作业全部日志/checkout匹配证据，不因纯文档重复升级应用。

成功标准：增量审查给文件/行号、区分静态与运行证据；可部署候选通过精确来源/镜像身份与Full+SQLite隔离验证，后续生产更新保配置/开关/新增账务。验证方法：所有实际构建测试在Actions，不跑浏览器、不主动调用真实模型/供应商；本机只代码/文档读写、Git与日志元数据轻量检查，不安装依赖、运行构建测试/Docker或下载产物。

### beta.36镜像与资格证据（Actions已验证，生产结果另见下节）

- 真实构建source/head：`f46f19c71dbf6954620feb43c01b8d8aa87bb1d5`；tree：`9f12fcbf1b544736f973dc96f265644eeaf6ac07`；版本`0.2.0-beta.36`，Full、linux/amd64。
- [Docker smoke42256473767](https://github.com/ForceMind/MyAPI/actions/runs/42256473767) attempt1 success，输入api_only=true/run_browser=false/personal=false，producer与独立consumer共2作业成功、LAN安装1作业跳过；已亲读全部步骤、2份执行日志与实际checkout SHA。消费方按manifest摘要验证并加载同一包，不从registry补取应用镜像。
- 日志内完整qualification已读：20/20全true，鉴权/权限/合成用量及精确单次扣费、正文脱敏、7个静态HTML/JS/CSS资产HTTP、原容器重启、同镜像独立SQLite/WAL恢复通过；browser=not_run。这些不是浏览器/真实供应商/生产数据迁移验收。
- app config/image ID：`sha256:45328cb9158b6c85e31131caf4507428f003856683eb470adbac969a8ac03152`；manifest.json SHA256：`7d4c00ecf574013393b40cb156b5c47b70a51a2ddad75f9c7c32bc9d90c87d56`。
- [镜像包13523545793](https://github.com/ForceMind/MyAPI/actions/runs/42256473767/artifacts/13523545793)：`myapi-smoke-images-42256473767-1-f46f19c71dbf6954620feb43c01b8d8aa87bb1d5-full`，315187419 bytes，ZIP SHA256 `aea07352ae8957063166ea7b4caaed1c4a4a5b660c3319dd86c5e6bd20c8e1d1`，有效至2026-11-13 16:55:50 UTC（北京时间11月14日00:55:50）。
- [资格报告13521909250](https://github.com/ForceMind/MyAPI/actions/runs/42256473767/artifacts/13521909250)：ZIP SHA256 `68206dcf07a28809084570a19663f4df9e2614631894078696031ceddac82d7c`，2072 bytes，有效至2026-11-13 16:57:00 UTC。tar摘要与服务器OCI descriptor摘要须从上述可信绑定的manifest/tar读取并逐项核对，本机未下载产物，不编造未读字段。
- 原工作流同时产出的operations kit不构成安装授权。本轮不发布GHCR/NPM/tag、不合main，不改变生产edition/配置/开关，不开诊断，不安装运维服务，不更新Docker。

### A1本次已读结论与边界

见[增量结构审查](architecture-review-v2.md)：11条结论逐项有源位置，均为“从代码看”。Lite多数支付回调在业务签名处理器前被路径门禁拒绝；Pancake的同级路径不匹配原waffo斜杠前缀，但仍有签名/资金/订单门禁，不能称其可无签名充值。funding retirement与edition不是同一开关，不应互换；OAuth可创建空密码账户，Passkey/邮箱恢复分别有条件，不能假定都有本地登录方式。完整A1账务/异步恢复及tools/website/deploy其余审查仍待，不因这11条或旧CI绿标整体完成；本轮没有新增行为测试或产品修复。后续A2先明确预检/订单/账户范围，不能直接开放所有回调或删资金门禁。

### 部署执行与回读（2026-11-07）

维护者已直接回复“授权发送并执行部署”，主聊天向既有服务器任务「查看状态并更新部署版本」（01a181f9-e239-7543-a32a-ddd24edfe1b9）发送了上述精确制品及最小边界。该任务本轮turn `01a1b1d2-2025-7d42-8a2a-7a6d8c102957` 已completed/error=null。主聊天已亲读该轮11条命令的status/exitCode，全部completed/0，其中包含一次 `deploy.py` 与最终 `readback.py`；源码、tree、config、ZIP和manifest常量与本节绑定相同。部分长命令在跨聊天记录中被截断，主聊天没有独立读取服务器所有JSON文件，以下运行数据明确来源于服务器任务最终回报，不能包装成开发机现场测量。

服务器任务最终回报：已由beta.34更新至beta.36 Full/f46f19c7，容器 `my-api-beta36-f46f19c7-live` healthy；版本、首页和7静态资源回读通过，双摘要、全部5层、RootFS及完整配置核验通过，11个离线错误输入均被拒绝。切换约26秒；SQLite完整性、外键、账务/历史日志连续性通过，原配置与开关不变（缓存会话/正文日志开启，诊断关闭），原容器/镜像/备份保留，未回灌旧快照或重放。未改Docker、未装运维服务、未开诊断、未发真实模型/供应商请求，浏览器未验。

服务器本地完整记录：`/root/myapi/docs/evidence/beta36-deployment-20261107/README.md`，同目录包含authenticated-binding、preflight、image-identity、qualification-summary、deployment-result、readback及command-summary。它是服务器任务负责的记录，不冒充已推送到本仓库的文件；主聊天本轮本机未下载任何构建产物。镜像始终绑定f46f19c7/42256473767，不重标成文档SHA。首次交接提交9f003130/CI42257544099当时仍运行；最终部署结果收尾提交会替代它，取消运行不计完整绿，接手读最终提交自身完整CI。

## 2026-11-06 P1-9：CLI与安装器的受限版本来源规则（源码与完整CI通过）

维护者对上一答的具体范围回复“好”，本轮只做cli/myapi.mjs、cli/test/edition-contract.test.mjs和必要tools/lan/check.mjs合同同步；安装器只读作更严格基准，不改现有部署文件/配置。基线dev/ee7c5a8f工作区干净，fetch后无分歧。既有UI/正文分段源码已收口，不重做、不因此运行浏览器、部署或重新选择Lite坐标/签名等已确认决定。

成功标准：lan/lite拒绝公网HTTP和HTTPS，合法本地/私网来源保留；Full原HTTPS规则不变；拒绝前没有任何Docker命令或配置写入。验证只在GitHub Actions，先兼容旧源的真实CLI/安装器合同取红，再做最小预检修正并读本提交完整CI。使用原隔离项目和假的Docker边界，新增命令日志记录包括container ls，保证零调用断言不被原fixture跳过；精确核对.env原字节，覆盖up、升级dry-run和原CLI默认实际升级入口。不能用不支持的选项或后续Docker假环境失败伪造来源拒绝。该批次窄而串行，主代理独立完成，不增子代理；不本机依赖/构建测试/浏览器/Docker/产物下载，不动生产/真实模型/main/发布。

红证据已亲读：9f806f1359be103ca9cc3003af71f7d9ac110242 / CI42201604018的分发job127837970166日志1388行，147例135过12红。12项均为lan/lite公网HTTPS（域名与公网IPv4）的up、dry-run和默认实际升级入口旧CLI返回0；up同源安装器拒绝前置已成立，其他原测试/HTTP拒绝/合法本地私网/Full对照通过，不是参数、假Docker后续失败或初始化造红。现最小产品修正只将受限版本来源限定为原placeholder或validateLanOrigin，Full继续validatePublicOrigin，不改原validator允许集合、绑定、额度/缓存模式和配置归一化。tools/lan/check仅在既有临时假Docker边界同步相同HTTP/HTTPS预检、零命令和原.env字节合同。本批作为未发布beta.36候选的P1来源门禁收口，不增加发布动作或反复升调试版本。当时尚待产品提交自身完整CI；最终结果见下段。

产品代码`e0699bd9730212d3219881c621845d60661a6811` / [完整CI42202296214](https://github.com/ForceMind/MyAPI/actions/runs/42202296214) attempt1 success：10作业成功、1浏览器/真实API作业跳过。全部步骤、10份执行日志与实际checkout SHA已亲读核对，失败/race标记0。CLI 147/147，包括18项公网来源拒绝回归，LAN合同新增24条来源拒绝/零Docker命令/.env字节不变检查通过；合法本地/私网及Full控制通过。前端419文件3417/3417，同步/构建/types/lint通过（26警告0错误）；根模块/独立relaykit、原生/386、两模块全量与既有race、三库/Redis/ClickHouse/分发/Desktop门禁通过。红测运行42201604018在后续推送后整体取消，不计完整绿；产品完整绿以42202296214为准。

本项已完成批准范围内的源码与非浏览器验证。沿用未发布beta.36候选，没有新增镜像包、tag或NPM发布，没有修改现有部署配置；浏览器、真实部署与生产未验证。收尾仅更新本总账与handoff/README，接手仍读取最新文档提交自身完整CI，不能把源码门禁通过当作已有可部署镜像。

从代码看，validateRuntimeConfiguration在受限来源不合法后仍回退validatePublicOrigin，公网HTTPS可被放行；已有用例只拒绝公网HTTP。新红测同一来源先证明安装器拒绝，再要求CLI也拒绝，并保合法本地/私网及Full对照。此段是改动前的源码发现；后续红绿与产品结果以上文为准。


## 2026-11-06 UI 整体衔接与精简：源码非浏览器收口（未整体验收）

**2026-11-06续办3.6（源码与非浏览器CI通过，视觉/真实日志性能未验）：** 维护者在上一答给出管理员只读分段接口、原路由及full-content-logs组件/测试的最小方案后回复“继续按原来的计划进行”，本轮按原3.6补齐，不重做其他页面。基线dev/HEAD=525696504625bcb534f2ce41da334218c101ba9a，工作区干净；fetch后无分歧，ff-only为Already up to date。基线CI42104485709仍attempt1完整success，日志证据沿用已读记录；继续复用本总账，不另建计划体系。

成功标准：已保存内容逐段完整可达，缺失/过期/损坏/未结束明确，不串请求或重放写入。验证全在Actions：先提交旧源可编译的真实HTTP/UI缺失能力回归读取红证据，再实现并读本提交完整CI；本机只代码读写/JSON/XML/Git/gofmt，不安装依赖、构建测试、浏览器、Docker或下载产物；不动生产/真实模型/发布/main。两名既有gpt-6.1-sol/high代理分别负责后端controller/原router与测试、前端full-content-logs模块与测试，主代理负责契约、线稿/翻译/CI/版本和集成，不递归派工。

从代码看，旧collector先裁response_chunk后，join可能无法再判定截断（原总量大于已读但flag为false）。这是3.6完整性提示的直接必要前置：单独追加现loader的小型UTF8/cap回归，与缺segments路由的回归分开，在Actions取红后修最小flag传播；不得删截断断言或改成空值。新入口还需用总量>已读/本地预览受限兜底旧服务。

红证据已亲读：5cb71edef51622e5db096c72f84c67c83ec27f17 / CI42154060719 attempt1 failure，全部作业/步骤及10份执行日志/checkout SHA已核对。后端精确UTF8前缀/总量成立后truncated=true失败；真实Admin旧详情200后新segments返回404，匿名/普通目标401/403亦因缺route404失败，均无未定义符号或初始化错误。前端同步/types/lint过，旧3363例通过，新增1例在缺Read saved response in segments按钮失败；原读取/零写前置已通过。其余8作业success，浏览器/真实API skipped。已据此进入产品实现；候选版本仅在本交付批次统一为beta.36，不发布镜像/tag或部署。

产品候选已落源码，仍未通过CI：后端独立只读GET、JSONL顶层body位置投影及common完整JSON校验/小转义解码、页范围复制和二次目标版本复核，不整段拼接响应；原collector真实截断标记取OR。新回归包含真实17MiB尾部/UTF8跳转、元数据缺失/null/0、base64、同inode同长度两遍间改写不得返body、普通/匿名及actor/SID隔离、参数/范围和取消。前端保持有界单页、decimal字符串/BigInt，前后/首段/直接位置、明确重读、同part预览切换、仅20条位置/版本元数据，401/403清同scope旧body，当前只读保续期。最大审查风险是新的流式body定位/转义处理；common现有元数据64层/5万节点校验边界仍适用，重复扫描大日志的性能未实测，不宣称性能或浏览器验收。已新增31键由脚本写en/zh+五英文，待Actions sync/类型/lint/构建及全部门禁。主代理已读关键源和红日志，子代理报告不代替CI。

首产品候选a529d61e / CI42160115033：前端类型已过，lint26警告/1错误仅单子节点Fragment（saved-content-segments.tsx:25），主代理已将fallback等价改为preview??null，不改权限或预览行为。后端新分段/真实截断门禁已success，完整旧门禁尚执行；后续最小修正会替代本候选，取消不记完整通过。官网静态42160115031/静态打包42160115101均success，不能替代产品主CI；未运行浏览器。

3.6最终产品代码31960fd412b81ff5c4a071a72323a7a21382bdac / [完整CI42160794199](https://github.com/ForceMind/MyAPI/actions/runs/42160794199) attempt1 success：10作业成功、1浏览器/真实API跳过。全部步骤、10份执行日志与实际checkout SHA已亲读核对，失败/race标记0。前端419文件3417/3417，分段专项4文件65/65、日志/返回52文件483/483、原Key461/连接292等均过；i18n同步/构建/types/lint通过（26警告0错误）。后端真实Admin的17MiB尾部、UTF8/base64、缺失/null/0、参数/权限、actor/SID隔离、取消及两遍间同inode同长度改写门禁通过；根/独立relaykit、原生/386、两模块全量、全部既有race、三库/Redis/ClickHouse/分发/Desktop通过。红证据与单个lint修正保留，取消候选不记完整绿。

本轮交付仍仅beta.36源码：保留旧1MiB预览、16MiB扩展与SSE结构化局部搜索；分段原文按明确动作只读当前页，支持前后/首段/字节跳转，完整表示不被旧预览二次裁切；同部分预览显隐切换，关闭恢复状态/焦点，只保有限位置/版本元数据，不全量累加/下载。完整性只描述保存记录，不推定生成成功、缓存命中、用量或账务。31新键经脚本及Actions同步（en/zh+五英文）；7视图14SVG及目录更新，329视图不变。common严格JSON元数据64层/5万节点边界保留，真实大日志性能、浏览器和生产未验。无本机安装依赖/构建测试/Docker/产物下载，无生产/真实模型/main/tag/GHCR/NPM操作。收尾仅现有交接，产品代码截至31960fd4，接手仍读最新文档提交自身完整CI；未来镜像另核对真实构建身份。

最小读取契约：新增同AdminAuth+DisableCache组的GET `/:request_id/segments`，part=request/response，单段解码后4B–1MiB、默认1MiB（base64/JSON表示有编码开销）；offset/next/已存及预期字节数用十进制字符串，缺预期元数据为null不补0。版本标识绑定同请求/part/actor/session的保存记录，其他请求自然追加不使它失效；目标变化/轮转丢记录/两遍读取不一致明确409，不返回混合正文。UTF8边界完整、base64保精确字节，完整性只描述保存证据，缺终止/预期元数据为null，不表示业务成功/用量/结算。不得调用旧详情max_response_bytes=0或整段拼接；保旧预览、16MiB扩展、SSE结构化预览/搜索和权限。分段原始查看仅明确动作GET，支持前后段及起始字节跳转、明确重读；不累加全量、不静默下载、不改日志写入/数据库/计费/供应商。

维护者已确认第0–8步总计划，授权在dev连续实施和路径限定提交推送，不逐小项等待。唯一主线为：连接上游 → 获取模型 → 调整路由 → 创建Key → 配置客户端 → 请求/用量 → 故障定位。沿既有方向3及329个线稿视图，已有功能先复用和核对，不因新清单重新开发。基线cc60be87/CI41982100994完整success，初始工作区干净、fetch后与origin/dev无分歧。

**当前结果：第0–8步已批准UI源码与非浏览器验证已收口，包括3.6完整逐段阅读；浏览器与真实环境仍未验。** 第1步匹配SHA的完整CI已通过；两名既有代理已转为请求返回/公共页、Key/钱包生命周期核对，主代理负责其余源码、总账、翻译、集成与亲读Actions。所有构建、类型/lint/测试在Actions，不在本机安装依赖、构建测试、Docker或下载产物。浏览器需单独授权；当前源码/CI通过不等于视觉/全站体验验收。生产、镜像身份适配、运维安装、真实模型、main/tag/GHCR/NPM不在本次执行范围。

2026-11-06首候选beta.35：共用路由字段编辑器已接新连接卡片、详情与真实表格；保原800ms/blur/cleanup提交与标签批量确认。未知字段不补零，500/408/无响应锁定，明确配置GET成功才解锁；待提交/保存/失败草稿与已确认快照优先，写前仅取消列表及本连接在途读，避免旧GET覆盖新值。/channels固定真实表格并保手机横向访问、Tag/批选/敏感控制/列偏好，删除唯一旧channel-card展示和内层切换；列预设不再叫第二套视图。卡片→表格→详情保focus与upstreamReturn，未打开任务的无意义状态说明移除，当前失败仍外露。旧卡片职责已映射到表格原字段/选择/动作，引用迁移后才删文件。既有线稿源/catalog及相关SVG更新，全部329视图源引用存在；只做JSON/XML/Git轻量核对，尚待Actions，浏览器未验证。12补键含1个原缓存待发布说明缺键，en/zh、其他五语言英文回填；版本及三分发模板一次对齐beta.35，不代表发布或生产升级。

第一阶段Actions循环（仍未整体验收）：a57ada36 / CI42071795405 source失败于测试不支持的ByRole exact参数，lint0错误；25c29741 / CI42072194416类型过、定向58例仅旧blur/cleanup两项过早检查PUT，改为await async act并保准确一次/换号撤权零PUT；0f0e6759 / CI42073419506定向58/58、原Key420及其余既有专项通过，连接292例仅两项新route尾斜杠期待不符。89ddcfd6 / CI42075961328修正命中旧用例，原/new路径期待互换导致3项失败；46d786c2已按各自原入口精确恢复（不放宽regex或产品路由），完整CI42077445265已success，10作业成功/1浏览器与真实API跳过，全部作业/步骤与执行日志已亲读；前端401文件3154/3154、连接292/292及同步/构建/types/lint（24警告0错误）通过，根模块/独立relaykit/386/全部既有race/三库/Redis/ClickHouse/分发/Desktop通过，失败和race标记0。所有已完成失败作业原始日志已读，失效候选被下一推送取消，不记完整绿；Key等后续源保持独立本地WIP，未混入第一阶段提交。

第二批改动（现已通过上述源码CI，浏览器未验）：第2步修Key创建精确ID交接（后端成功仅增data.id、不返凭据）、复制去重/手输模型主入口/期限常驻/窄屏输入，价格缺失不伪0、显式零倍率不变1、真实API页面不再显示seed生成限流；第3步补请求→连接/Key返回、stream未知不伪Error、正文默认1MiB和明确16MiB扩展，history-entry+actor有界数字滚动与all/self范围恢复；第4步补账号选择被过滤/返回缺失时不借其他账号、同筛选刷新不因排序变化换账号，以及性能读取失败/缺成功率不伪0；第5步普通实例/任务查询按Root身份分区，钱包及购买仅修生命周期/未知状态/报价代次，原请求载荷/金额/幂等/不重放不改。钱包共享API仅增可选错误呈现配置，旧调用默认不变，受控UI自行显示当前scope错误，写skipAuthRefresh:true不可被覆盖。

第3步历史边界（现有独立分段契约已补齐，旧预览上限保留）：max_response_bytes=0会取消服务端响应收集上限，本轮UI不使用0。默认1MiB保护保持，用户明确动作扩展到现接口最大正值16MiB；超过仍截断须保警示。现请求详情没有filename映射且API无offset，不能猜原文件或静默读取整个日志；>16MiB的精确完整原文访问仍待最小后端契约方案确认，不能标3.6整体完成。浏览器仍待单独授权。

2026-11-06第二批最终产品代码提交`63ac3acb4600b77c8e4a557508b216265eb59d98` / [完整CI42099102382](https://github.com/ForceMind/MyAPI/actions/runs/42099102382) attempt1 success。10作业success、1浏览器/真实API skipped；全部作业/步骤和10份原始执行日志已亲读，10个实际checkout SHA均精确匹配，失败/race标记0。前端416文件3363/3363；新定向130、Key交付65、连接定向69、认证251、设置/分析/公共848、资金权限404、模型详情61、账号88、Key全量461、连接292、日志429均通过；i18n同步/构建/types/lint通过（24警告0错误）。后端新Key创建两父项明确PASS，根/独立relaykit、原生/386、两模块全量、全部既有race、三库/Redis/ClickHouse/分发/Desktop均通过；386金额18父项/5包0跳过0失败。所有失败候选/必要修正过程保留在下文，不把fixture红写成产品问题已复现。

本次只补22个新文案键，en/zh、其余五语言按维护者要求英文回填，脚本写入及Actions同步通过；本批候选版本沿用beta.35，没有为调试或文档再升级。最后收尾仅本TASKS和handoff/README，产品代码截至63ac3acb；接手仍需读取最新收尾提交自身完整CI，后续若打包应绑定该次真实构建SHA/CI及镜像身份，不能混用旧包。本机未安装依赖/运行构建、测试或Docker/下载产物，浏览器与生产未验证；本批未动生产、未发真实模型、未合main或发布镜像/tag/NPM。

beta.35历史边界（3.6已由上文beta.36补齐）：正文页面仍是明确标记的1MiB预览；原始模式复制含当前已读取内容。明确同request读取最多16MiB，扩展SSE解析/搜索同样有限；这不等于任意正文已完整显示。完整逐段阅读及>16MiB准确定位缺后端契约，3.6继续阻塞，不能以复制或“取消上限”参数0冒充全量页面验收。浏览器（中英文/明暗/各宽度/全部页面弹窗）、真实账号/供应商/支付与生产、七语言母语及后续P2仍是各自未验/待办，不因本次CI通过抹除。

### 规则与页面索引

- 每项状态只用：待核对、实施中、复用并有证据、源码CI通过/浏览器待验、验收完成、阻塞。不得把页面数量或CI测试数当作整体体验已完成。
- 页面/页签/弹层的逐项设计索引继续使用[既有catalog](ui-design-v2/wireframes/catalog.json)，其中已有URL、源文件、字段、动作、权限及桌面/手机线稿。以下功能ID是验收单位，catalog视图是页面索引，不新建第二套状态文档。
- 删除仅限重复展示/入口、替代职责已完整迁移的旧组件；业务能力、字段、数据库、权限和敏感确认保留。删除前核对所有引用、替代入口和回归，禁止只折叠后宣称已删除。
- 每批记录实际删除/合并/保留映射、源码SHA和自己的完整CI。失败/未知、不重放、功能可达和裁切断言不放宽；不能验证的维度明确保留待验。
- 服务器最新执行记录表明beta.34因config与OCI loaded ID不符退出2、未切换；本UI主线不处理该部署阻塞，也不沿用历史生产授权。

### 功能验收表

| ID | 页面/功能 | 保留、删除或合并目标与验收 | 当前状态 |
|---|---|---|---|
| 0.1 | 页面盘点 | 对照catalog逐页核对实际入口、角色、任务、操作和返回位置 | 复用并有证据 |
| 0.2 | 操作盘点 | 创建/编辑/启停/测试/复制/删除/批量逐项核对，字段显示不替代操作迁移 | 源码CI通过/浏览器待验 |
| 0.3 | 删除清单 | 每个旧入口指明唯一替代位置与权限，不删未迁移业务 | 源码CI通过/浏览器待验 |
| 0.4 | 页面状态 | 加载/空/失败/未知/未授权/保存中/结果未知各有准确表现 | 源码CI通过/浏览器待验 |
| 0.5 | 完成口径 | 源码、CI、浏览器与真实业务证据分列；未验证不写完成 | 规则已登记 |
| 1.1 | 连接/渠道入口 | 同一对象工作区，旧/channels地址、搜索与深链接兼容 | 源码CI通过/浏览器待验 |
| 1.2 | 唯一视图切换 | /connections是真卡片、/channels是真表格；只保留一套切换，刷新/Back/宽度不擅改模式 | 源码CI通过/浏览器待验 |
| 1.3 | 权重 | 新卡片/表格/详情直接编辑同一对象，共用原提交逻辑，不切旧UI | 源码CI通过/浏览器待验 |
| 1.4 | 优先级 | 共用编辑器、保存反馈一致，优先级与权重选路语义不变 | 源码CI通过/浏览器待验 |
| 1.5 | 数值与保存 | 原范围/合法零值保留，未知不伪零或隐式写回，保存中/失败/未知准确 | 源码CI通过/浏览器待验 |
| 1.6 | 编辑权限 | channel.write、fresh access、actor/session守卫、撤权和401不重放保持 | 源码CI通过/浏览器待验 |
| 1.7 | 标签批量 | 显示对象/数量/字段并保确认，不把单连接调整扩大到整标签 | 源码CI通过/浏览器待验 |
| 1.8 | 启停 | 保原权限和失败处理，未知结果不伪成功 | 源码CI通过/浏览器待验 |
| 1.9 | 测试 | 仅明确操作触发，保对象/时间/耗时；CI只用合成上游 | 源码CI通过/浏览器待验 |
| 1.10 | 创建/编辑连接 | 全部基础/高级字段、校验定位和保存/取消保留 | 源码CI通过/浏览器待验 |
| 1.11 | 获取模型 | 当前连接明确获取，参数/对象/身份变化隔离旧目录 | 源码CI通过/浏览器待验 |
| 1.12 | 模型/分组摘要 | 数量展开完整内容，原手填ID、映射与配置保留 | 源码CI通过/浏览器待验 |
| 1.13 | 列表上下文 | 筛选/排序/分页/滚动/焦点在视图、编辑和返回间准确 | 源码CI通过/浏览器待验 |
| 1.14 | 用量证据 | 重证据明确读取，不支持/未配置/过期/失败/未知及身份不混 | 源码CI通过/浏览器待验 |
| 1.15 | 旧卡片职责迁移 | Tag/勾选批量/敏感标识/字段/动作在统一表格完整可达后删除旧channel-card及内层切换 | 源码CI通过/浏览器待验 |
| 2.1 | 模型目录筛选 | 名称/供应商/现有能力筛选和返回位置一致 | 复用并有证据 |
| 2.2 | 模型可用性 | 目录、配置、真实请求证据分开，不把目录存在当调用成功 | 源码CI通过/浏览器待验 |
| 2.3 | 模型详情 | 能力/价格/接入固定位置，列表和详情不反复堆相同信息 | 复用并有证据 |
| 2.4 | 模型价格 | 原算法/单位/分组差异保留，缺价格不补零 | 源码CI通过/浏览器待验 |
| 2.5 | 管理元数据 | 创建/编辑/描述查看复用，删重复说明、保字段与权限 | 复用并有证据 |
| 2.6 | 公开/管理模型 | 展示复用，权限/URL分开，公开页不露管理操作及内部连接 | 复用并有证据 |
| 2.7 | Key列表 | 状态/期限/预算/主要操作固定，长名单摘要、删重复展示 | 源码CI通过/浏览器待验 |
| 2.8 | 创建/编辑Key | 保字段，成功后有进入该Key配置的明确入口 | 源码CI通过/浏览器待验 |
| 2.9 | Key模型范围 | 唯一编辑位置，其余摘要跳转；未限制/空限制/未知分清 | 复用并有证据 |
| 2.10 | Key预算/分额 | 预算、窗口份额、观察估算分开，原计算与确认不变 | 复用并有证据 |
| 2.11 | 地址/凭据 | 来源明确，展示/复制/导出同地址，明确操作才读完整Key | 源码CI通过/浏览器待验 |
| 2.12 | 客户端配置 | 复用CC等原入口，保不同协议地址与高级字段 | 源码CI通过/浏览器待验 |
| 2.13 | Key模型选择 | 按显式限制筛候选，保允许的手填ID，未实测不称可用 | 源码CI通过/浏览器待验 |
| 2.14 | Key动作去重 | 同一区域复制/导入/编辑只留一处，原能力仍可达 | 源码CI通过/浏览器待验 |
| 2.15 | Key失败/切换 | 换对象/账号清凭据，目录失败保草稿、错误和重试 | 源码CI通过/浏览器待验 |
| 3.1 | 请求列表 | 结果/模型/连接/Key/耗时/用量信息顺序稳定 | 复用并有证据 |
| 3.2 | 单日历范围 | 复用已实现预设/起止/应用/取消，同一任务链不暗改日期 | 复用并有证据 |
| 3.3 | 分页 | 复用跳页/首尾/条数，筛选和返回页码一致 | 复用并有证据 |
| 3.4 | 请求返回 | 列表到详情保筛选/页码/滚动/原请求焦点 | 源码CI通过/浏览器待验 |
| 3.5 | 请求结果 | HTTP、流完成、中断、未结束与未知分开 | 源码CI通过/浏览器待验 |
| 3.6 | 正文/SSE | 入口固定，内容完整可达，翻页保对象与位置 | 源码CI通过/浏览器待验 |
| 3.7 | 用量/缓存 | 缺失/null/0/正数保区别，未采集不称零命中，估算不称原始值 | 复用并有证据 |
| 3.8 | 关联对象 | 请求→连接/Key→返回保上下文和原权限 | 源码CI通过/浏览器待验 |
| 3.9 | 错误/敏感信息 | 当前失败不藏，普通用户不见管理元数据，正文/脱敏权限保留 | 源码CI通过/浏览器待验 |
| 3.10 | 请求信息去重 | 基本信息一份权威展示，诊断引用、不反复复制 | 复用并有证据 |
| 4.1 | 账号窗口 | 每账号/窗口保来源/时间/身份，不混成伪总额 | 源码CI通过/浏览器待验 |
| 4.2 | 查询能力 | 未配置/不支持/无记录/过期/真失败分开，不按类型猜 | 复用并有证据 |
| 4.3 | 历史错误 | 其他窗口成功不等于旧错恢复，证据及恢复未确认保留 | 复用并有证据 |
| 4.4 | 反推估算 | 保估算标识/区间/说明，不变成精确请求用量 | 复用并有证据 |
| 4.5 | 分额配置 | 对象/窗口/影响范围、暂停/超额/未知与确认保留 | 复用并有证据 |
| 4.6 | 概览流向板 | 方向3复用，核心状态/对象入口保留，删重复统计和装饰说明 | 源码CI通过/浏览器待验 |
| 4.7 | 模型/用户/流量分析 | 时间/单位/筛选一致，第6名起中性灰等已确认决定保留 | 复用并有证据 |
| 4.8 | 图表到列表 | 对象/区间不丢，读取失败不伪零图表 | 源码CI通过/浏览器待验 |
| 5.1 | 用户管理 | 查询/编辑/状态/额度/绑定/安全重置的权限、金额和确认保留 | 复用并有证据 |
| 5.2 | 钱包 | 当前资金/历史/选购分清，未知/失败/购买确认外露 | 源码CI通过/浏览器待验 |
| 5.3 | 订阅方案 | 列表/编辑/状态/购买入口去重，价格与资格原计算保留 | 源码CI通过/浏览器待验 |
| 5.4 | 兑换码 | 创建/查询/批量/删除统一，明确操作对象 | 复用并有证据 |
| 5.5 | 个人资料 | 资料/安全/绑定/订阅固定位置，删重复介绍/跳转 | 复用并有证据 |
| 5.6 | 登录安全 | 密码/OAuth/2FA/Passkey/管理Key保持安全边界和恢复路径 | 复用并有证据 |
| 5.7 | 站点设置 | 品牌/通知/公共导航/控制台模块整理，与实际导航不混 | 复用并有证据 |
| 5.8 | 认证设置 | 注册/外部登录/自定义OAuth/Passkey/人机验证保原提供方配置 | 复用并有证据 |
| 5.9 | 计费设置 | 额度/货币/模型与分组价格/支付/写入模式/签到保算法与单位 | 复用并有证据 |
| 5.10 | 模型路由设置 | 全局/提供方/亲和/可靠性/价格来源/部署，基础与高级整理 | 复用并有证据 |
| 5.11 | 运维设置 | 通知/邮件/日志/性能/Worker/更新/行为，保危险操作确认 | 复用并有证据 |
| 5.12 | 安全设置 | 速率/Token/敏感词/SSRF保校验、原配置和未知扩展 | 复用并有证据 |
| 5.13 | 内容设置 | 公告/FAQ/API/聊天/绘图/仪表盘/状态预览不覆盖草稿 | 复用并有证据 |
| 5.14 | 系统信息 | 版本/实例/任务/运维状态分清，未知不补零、运维执行受限 | 源码CI通过/浏览器待验 |
| 6.1 | 登录注册找回 | 统一反馈/返回，原认证流程不变 | 源码CI通过/浏览器待验 |
| 6.2 | 安装向导 | 前进/返回/结果未知/恢复明确，不自动重复初始化 | 复用并有证据 |
| 6.3 | Playground/Chat | 保现有引擎/预设及Key交接，打开不自动发模型请求 | 复用并有证据 |
| 6.4 | 提示词学习 | 策略/版本/运行各有任务，保授权和分页 | 复用并有证据 |
| 6.5 | 公共页面 | 首页/定价/榜单/关于/法律统一品牌导航状态，保法定声明 | 源码CI通过/浏览器待验 |
| 6.6 | 错误页 | 401/403/404/500/503按原因恢复，不一律跳无关页 | 复用并有证据 |
| 6.7 | 编辑弹窗 | 标题/保存取消/滚动/错误定位/回焦一致，完整字段可达 | 复用并有证据 |
| 6.8 | 敏感确认 | 明确对象/数量/影响，处理中及未知不重复提交 | 复用并有证据 |
| 6.9 | 长内容 | 名单/说明/历史按需展开，当前故障及状态外露 | 复用并有证据 |
| 6.10 | 旧组件删除 | 功能替代通过、引用迁移后真删除，不长期两套并行 | 源码CI通过/浏览器待验 |

第二批首候选`4c6e79f0` / CI42090860956前端source失败，未执行回归：测试ByRole不支持exact参数、参数化数组被Vitest展开、3处form非空断言、账号选择effect依赖和删去推导事件数后的残留引用。按原始执行日志及check annotations逐项修正，字符串名称仍精确、原载荷/次数断言不变；本机未测试。其余尚在执行的候选不当完整通过。

`9c61761a` / CI42091426438同步、types/lint通过，定向131例123过8失败，另49例敏感任务通过；7项榜单失败来自测试初始化未沿生产nsSeparator=false，冒号键被当namespace，修fixture后保Total/Unknown/零原准确期待；钱包1项是Close同时匹配页脚和右上角，改为明确Actions组的Close，不改付款锁/POST次数断言。原始失败日志已读，不把fixture失败称为产品行为已复现。

`d5ee6acb` / CI42092022493前述131/131、Key64/64、连接69/69、敏感任务49/49通过；认证251例250过1失败，原始日志显示微信按钮SVG title把可访问名称拼成“WeChatContinue with WeChat”，测试原准确label无法定位。为OAuth按钮显式绑定原翻译label（视觉/提供方/载荷不改），保准确名称断言，继续原微信状态失效流程验证。

`aa32ed57` / CI42092800286微信按钮准确名称已通过，原流程推进到失效状态检查；250/251认证通过。测试在query Promise结算后立即读alert，改为等待真实错误可见、重试后等待Confirm确实可用，保原3次GET/0认证POST及全部草稿断言；没有固定sleep或放宽等待结果。为避免首个定向失败遮住后续全量证据，既有build只要source门禁通过且未取消就执行；既有全量Test随后仍按build成功运行，任何失败仍使作业失败，未开启浏览器或continue-on-error。

收口核对另见`model/token.go`：仅-1永不过期，非-1且小于当前时间就过期，因此明确0必须按Unix epoch显示，不能将其作为Unknown。已在Key到期helper、桌面/手机及详情补准确日期，Unknown继续由不可表示日期覆盖；TimestampCell显式zeroIsEpoch只用于到期字段，创建/上次使用0的原语义不变。此7文件修正已随63ac3acb通过完整CI；明确0有准确epoch日期和过期样式，未知不可表示日期继续Unknown。

前一完整失败候选`64b76246839a78ba732f36f70a9e9311dee09df6` / CI42093719533已failure：9作业success、前端failure、浏览器/真实API skipped。后端新Key创建两父项明确PASS，原生/386、两模块全量与全部既有race通过。前端9项失败已按原始DOM/代码逐项修正，账号/Key/请求入口断言改为实际唯一入口并加强精确导航、价格按实际K单位，保全部功能/金额/载荷/次数/列偏好和返回。钱包仅补任务错误与具体原因同时可见。所有较早候选完整作业状态及可用原始日志已读，取消不算绿。

CI42093719533前端完整执行日志7981行已读：同步/types/lint（24警告0错误）/构建通过；新定向131、Key64、连接69、认证251等通过；全量416文件3352过9失败。失败精确分布为新概览错误文案期待2、Key唯一导入/关联对象原生Link角色/列偏好3、账号入口role2、动态价格K单位期待1、钱包确认任务错误语义1。按实际代码和日志修正，不恢复重复入口、不改金额公式、不删未知/可达/返回/精确载荷及次数断言。后端随后完整success，全部既有race、根/独立relaykit、386、三库/Redis/ClickHouse/分发/Desktop均过；10份执行作业日志与全部步骤已读，整体仍因前端失败，不算完成。

### 第2–6步源码审查与实际合并映射（源码CI通过，浏览器未验）

- 2.1–2.6：目录和管理元数据沿用原筛选/权限/详情。价格缺失显示Unknown、零倍率保持零；详情移除按模型名生成的限流伪数据，仅展示真实可知字段。证据：`pricing/components/__tests__/model-catalog-price.test.tsx`、`model-details-states.test.tsx`及已有`features/models`回归，价格计算模块不改。
- 2.7–2.15：Key期限常驻，详情交付区仅保一处完整Key复制/客户端导入；More的独有连接信息JSON保留。创建成功只返回精确data.id并给明确配置入口；目录失败仍可在原Key限制内手填模型。概览、模型API示例、展示、复制、CC导入共用`lib/gateway-address.ts`及当前配置，避免重复/v1和旧storage地址。证据：`keys/__tests__/key-creation-handoff.test.tsx`、`key-client-delivery.test.tsx`、`api-create-identity.test.ts`、原`cc-switch-task.test.tsx`与新增`controller/token_creation_identity_test.go`；不返完整Key、不按同名猜ID、不自动读取或重试写入。
- 3.1–3.10：原列表/分页/单日历/唯一请求身份与诊断分组复用；新增`usage-logs/lib/request-return-state.ts`只保20条history-entry+actor下的数字滚动/请求ID/枚举，不存正文或筛选隐私。请求→Key/连接→Back保原tab、OnlyMine、焦点与原权限；缺流状态为Unknown。证据：`usage-logs/__tests__/platform-workflow.test.tsx`、`request-page.test.tsx`、`request-diagnosis-layout.test.tsx`及原`log-columns`、`list-pagination`、`common-logs-stats`、单日历`selection`回归。正文16MiB边界及剩余阻碍见上文，扩展失败保预览和错误、拒绝权限不露旧扩展内容。
- 4.1–4.8：账号显式选择没有观察时不借其他账号；首次默认选择固定到原返回entry，刷新排序变化不换对象。性能响应失败/畸形不能作空成功；缺成功率不补零，删掉没有后端依据的“No incidents”，分组摘要保后端真实值。方向3及原金额/窗口/估算算法保留。证据：`personal-upstream/__tests__/upstream-workspace.test.tsx`、`pricing/components/__tests__/model-details-performance-states.test.tsx`和原账号/分析回归。
- 5.1–5.14：用户、兑换码、资料/安全、各设置原任务分区、权限、完整高级字段和确认复用（`users`、`redemption-codes`、`profile`、`system-settings`既有回归）。钱包金额各自Unknown，私有数据按actor/SID隔离；报价按代次，关闭、换用户和旧响应不外跳/通知/回写。系统实例/任务只读按Root身份隔离。证据：`wallet/hooks/__tests__`、`wallet/lib/__tests__`、`wallet-session-workspace.test.tsx`、`subscriptions/components/dialogs/__tests__/subscription-payment-session.test.tsx`、`system-info/__tests__/system-status.test.tsx`，旧金额/载荷/确认/幂等测试不删。
- 普通新付款/订阅未知结果在同actor+SID的SPA内存锁定，关闭重开/换方式/同计划返回不建第二笔，历史GET不自动解锁；Affiliate保持原quota+request_id明确恢复，兑换码保持原同code单次业务事件恢复。锁不持久化跨硬刷新、新tab或新登录，原API没有对应新订单恢复证据，不宣称永久锁或强恢复。未操作真实资金。
- 为上述私有状态补最小HTTP opt-in `isCurrentScope`：仅显式请求在dispatch、响应、401续期后检查原UI/身份；旧请求不刷新/重放/通知新身份，当前只读保持原续期，写入仍强制skipAuthRefresh。取消和明确自管错误不再被全局500导航打断。证据：`lib/__tests__/http-client-scope.test.ts`、真实Axios adapter的`wallet-http-session.test.tsx`及既有认证回归。
- 6.1–6.10：安装向导、Playground/Chat、提示词学习、公共/法律、错误页、编辑/敏感确认壳和SummaryDisclosure复用既有任务测试，不为了“全替换”重写稳定能力。榜单缺失/null/非法计数及合计溢出显示Unknown，明确0保留（`rankings/__tests__/models-section-totals.test.tsx`）；登录公开状态未confirmed时锁定密码/Passkey/微信/OAuth入口及native提交；法务同意按已确认配置保留，不从pending状态继承隐式同意，错误只读重试并保草稿（`auth/__tests__/sign-in-status-gate.test.tsx`）。浏览器/真实认证设备、提供方与线上均未验。
- 实际删除：旧`channels/components/channel-card.tsx`已在首批删除，职责迁入唯一真实表格；本批删除无运行引用的`usage-logs/components/compact-date-time-range-picker.tsx`，实际筛选已用共享单日历，替代`date-time-range-picker/__tests__/selection.test.tsx`包含于首批3154例全绿。没有删除业务能力、字段、数据库或仍被消费的通用renderer。

### 跨页与交付门禁

第7步六条链分别验收：连接→权重调整→返回；连接→模型获取→保存；Key→配置/导入；请求→正文/SSE→关联对象→返回；账号窗口→分额→返回；设置编辑→失败/取消/保存。均保对象、角色、URL/滚动/焦点、未知、草稿及准确载荷；非浏览器回归在Actions，浏览器需另行授权，未获批保持待验而非整站验收完成。

第8步按总账输出实际删除/合并/保留清单，确认替代后再删旧文件、补七locale（en/zh+五语言英文）、路径限定提交推送和读完整CI。后续P2结构审查/CLI来源/原生安装/备份/Desktop/容器加固/教程及七语言母语、独立质量、全量视觉验收沿原清单核对，不因本次UI确认自动获得生产或扩大实现授权。

## 2026-11-05 beta.34 Actions 打包验证与服务器方案交接（不动生产）

- 维护者授权在Actions打包验证beta.34并交接服务器任务先给更新镜像最小方案，确认前不执行。本机不安装依赖、构建测试或下载产物；不跑浏览器、不发真实模型请求、不发布GHCR/NPM/tag或合main。本批不安装/启动独立运维服务，不配置安全接入。
- 新镜像真实构建源码/head为 `08e44520a0958711c96321bf8804d062880783e1`、tree `90a0d51d54b3d602ab062a3edbeb39d33f22bfbc`，版本0.2.0-beta.34、Full、linux/amd64。同源码[完整CI41975976001](https://github.com/ForceMind/MyAPI/actions/runs/41975976001) attempt1 success（10成功/1浏览器与真实API跳过）已回读，前端3130/3130、后端/独立relaykit/三库/全部既有race通过。产品变更收口于a3ad44ac，但镜像实际绑定08e44520，不得重标。
- [Docker smoke41980838279](https://github.com/ForceMind/MyAPI/actions/runs/41980838279) attempt1 success，api_only=true/run_browser=false/personal=false。构建、独立加载2作业成功，LAN安装跳过，全部步骤和原始日志已读。Full+fresh SQLite报告20/20全true：鉴权、合成上游精确用量及权限、7个前端静态资产HTTP、原容器重启、暂停SQLite/WAL后在另一个同镜像容器恢复均通过；browser=not_run，隔离容器清理成功。不是生产兼容性、真实供应商、跨版本回滚或浏览器验收。
- 目标app image ID必须精确为 `sha256:03474e78120630f20aaf9ba3ec5522858633d02be3bfc0bf471a115c6f6dba5a`；manifest.json SHA256为 `d74fdeb02c45e59b8593b1499b3d80aac1deeaa6e11e7c40159249f60699dec8`。
- [镜像包artifact13394106401](https://github.com/ForceMind/MyAPI/actions/runs/41980838279/artifacts/13394106401)：`myapi-smoke-images-41980838279-1-08e44520a0958711c96321bf8804d062880783e1-full`，315056347 bytes，ZIP SHA256 `0f25e0ef9baa695ec98a170796052200d17f8f57aafe7eeab1a9feb90b6af23e`，有效至2026-11-12 06:33:25 UTC。
- [验证报告artifact13393727830](https://github.com/ForceMind/MyAPI/actions/runs/41980838279/artifacts/13393727830)：ZIP SHA256 `2da227e580376736b86029a9c930a58b6f0075dc5866978477444039ab83a0a2`，有效至2026-11-12 06:34:46 UTC。含source/manifest/qualification；tar内部摘要须从已校验manifest读取，本机未下载产物，不编造未读字段。GitHub artifact元数据、ZIP摘要与过期时间已读。
- 工作流原有运维kit同时产出并在runner独立解包28/28通过、servicesStarted=false/productionTouched=false；[kit artifact13394235519](https://github.com/ForceMind/MyAPI/actions/runs/41980838279/artifacts/13394235519)不构成本次安装/启动授权。
- 已向现有服务器任务「查看状态并更新部署版本」发送精确制品身份、证据与本轮边界，请其仅做必要只读预检并提交更新方案，等维护者确认。明确禁止在方案阶段下载/加载镜像、写文件/备份、改配置/容器/重启策略、安装kit、改Docker或开启诊断。上次beta.33因config ID与containerd/OCI loaded ID不完全相等退出2，未部署；这次仍严格相等，不沿用beta.31等价授权。未证明兼容就停，不通过重标tag、换产物或弱化门禁继续。
- 服务器任务已返回只读预检及最小方案，等待维护者明确确认：其回报现网beta.31/98dcd374仍healthy、HTTP200，缓存会话和正文日志开启、诊断关闭，当前未下载/加载制品或修改生产。确认后先保持旧服务运行下载并严格核验ZIP/manifest/tar及加载后ID；身份通过才排空、备份当时最新SQLite/WAL/配置/日志并以最新数据副本单次切换，再只读检查健康/版本/静态HTTP、DB完整性/外键及历史账务连续性。服务器估计切换30–90秒，排空/迁移可能延长；备份不完整不启动，新增账务出现后不自动恢复旧快照。containerd镜像ID兼容尚未验证，若不符停止，适配身份方案须另行审批，历史执行授权不延用。
- 未来经确认的方案必须保留全部现有配置、开关与当时最新SQLite/WAL/日志，排空后一致备份；切换后新增账务不得被旧快照覆盖，故障保全、不重放请求。仅健康/版本/静态HTTP及数据连续性验收，不主动模型请求。收尾只更新本任务记录和handoff/README；镜像继续绑定08e44520/41980838279，读取收尾文档提交自己的完整CI，不把后续文档SHA当新镜像。

## 2026-11-05 连接能力、用量状态与 Key 客户端配置（beta.34，源码完整 CI 已通过）

- 收口应用源码 `a3ad44acaf843e40f84493730c23ab2e0d0c6268` / [完整CI41972253884](https://github.com/ForceMind/MyAPI/actions/runs/41972253884) attempt1 success：10作业成功、1浏览器与真实API作业跳过；全部作业/步骤及10个执行作业的原始日志已亲读。Key交付定向56/56、保存证据定向38/38、原Key420/420、连接268/268、日志410/410、前端全量399文件3130/3130通过；同步/构建/构建前后types/lint通过（24警告0错误）。根模块/独立relaykit vet/build、原生/386、两模块全量、全部既有race、三库/Redis/ClickHouse/分发/Desktop通过；新增余额能力3父项/含子项8条全PASS，38618父项/5包0skip/fail，原始失败与race标记0。
- 收尾仅更新本任务记录与handoff/README；应用源码继续绑定a3ad44ac，未生成新服务器镜像或将文档SHA重标成应用。收尾推送后仍须读取其自身完整CI，不能沿用本段应用CI替代。33个新增文案键en/zh，其余五语言英文回填，未称七语言母语验收。

- 维护者已明确批准本批实现、路径限定提交推送 origin/dev 并读取完整 CI。沿既有 connections / connection-windows / key-scope / key-delivery 线稿继续，不新建路由或重做已有模型折叠、复制和导出。基线5acf6fbb；读取远端后无分歧、工作区初始干净。
- 连接卡片、渠道表格和详情复用保存证据状态。余额查询与原生账号窗口分别说明，未支持、未配置、无记录、过期和真实失败不混用；每个系列保留来源、时间、身份和历史失败，未知不补零。打开和刷新只读保存数据，不主动请求供应商。
- 后端最小修正限定有效 AdvancedCustom 配置缺少 BalanceRoute：归入 unsupported / quota_not_configured；坏 JSON、无效配置和已配置端点的真实失败仍为错误。新增最新 error_code 可选投影，供页面区分缺配置与不支持；原历史、计数契约、数据库结构、转发、计费和采样锁/身份/预算不改。
- Key 接入信息按地址、候选模型、复制/客户端导入整理，复用已有 CC Switch 和 Key 访问范围编辑入口；统一地址处理并结合本 Key 限制筛选候选，不默认选择、不将目录当实测能力。凭据只在明确操作时读取，保留权限、确认、返回上下文、会话隔离和不重放。
- 新回归与原全量测试均待对应 Actions 执行，不在本机安装依赖、构建、运行测试/浏览器/Docker或下载产物。新增文案经脚本补 en/zh，其余五语言英文回填，i18n 同步在 Actions；VERSION、package 和三个分发模板一次同步候选 beta.34，不代表已发布镜像。
- 首候选当时未验证，最终结果以上述收口记录为准；浏览器始终未验证。本批不动生产、实际配置或独立运维服务，不主动发模型请求，不自动打包发布、合 main 或部署。
- 首候选259087a3 / [CI41969848904](https://github.com/ForceMind/MyAPI/actions/runs/41969848904) 前端source检查失败，原始日志已读：两处model_limits可为null、渠道测试重复导入；尚未进入前端测试，不能记通过。修正只处理nullable空集合和合并导入，新增null/空限制行为回归，不改类型及断言强度。
- 两个实现子任务交叉只读审查发现并在范围内修正：CC导入只在打开边缘初始化，目录刷新失败或移除候选不清手工草稿；保存证据读取核验四项完整性元数据、使用后端现有2000条上限，截断时保已读失败并明确不完整，不把遗漏系列或连接写成无记录。新增101系列、截断/空截断、actor隔离和目录变化保稿回归；超过2000系列仍显示不完整，不声称全量。修正后的完整CI仍待读取。
- 第二轮afb18d40 / CI41970330722类型通过、仅测试多行if缺括号导致lint失败，3345ce0b补括号；第三轮[CI41970789742](https://github.com/ForceMind/MyAPI/actions/runs/41970789742) source/翻译同步通过，认证247、摘要32、敏感说明49通过，渠道41例仅原“库存不读取重证据”1例失败。按原断言修产品：表格新增明确读取保存用量状态按钮，首次进入、换actor/IDs、切任务和返回均不自动读取；manual模式不定时刷新，卡片原只读保存刷新仍保留，绝不调用供应商。原工作区测试未改，新增按钮、ID变化/返回不重读回归。
- Actions增加本批Key交付/原导出及余额能力的定向门禁，提前定位直接受影响流程；全部原有全量、构建、三库与race检查保留，浏览器仍只有显式授权才运行。后续读取修正提交自己的完整结果，不拿此前局部绿替代。
- 第四轮6cc2f9eb / [CI41971521636](https://github.com/ForceMind/MyAPI/actions/runs/41971521636) Key定向56例54过2失败，原始日志已读；失败仅新“目录刷新保稿”用例。代码对照表明测试Escape实际关闭弹层，随后查询已卸载portal（无alert / root.getElementById异常）。仅将结束输入改Tab、刷新前后断言活跃弹窗并重取控件；原alert/Retry、完整草稿、单次密钥读取、零写入与零外部打开断言全部保留，不改产品Escape合同。定向CI将Key与保存证据两组均执行并汇总任一失败，再继续原全量门禁；修正仍待对应CI验证。
- 第五轮3a2ca94a / [CI41971932783](https://github.com/ForceMind/MyAPI/actions/runs/41971932783) Key定向56/56已绿，保存证据38例37过1失败；库存原不自动读取合同已过。日志显示详情摘要及窗口详情各有一处“No reliable usage sample”，旧全页getByText匹配两处。仅将详情分支限定到Saved account windows区域，仍要求真实可见，并保留无百分比环、无100%伪值、仅一次已保存GET和精确params断言，不改产品或用量语义。最终仍需完整CI通过。

## 2026-11-05 beta.33 打包与服务器最小方案交接（本轮不动生产）

- 维护者授权Actions打包验证，并交接现有服务器任务先提出镜像更新、独立运维服务安装与安全接入最小方案，确认前不执行。保配置/开关/新增账务，无browser/真实模型请求/本机产物下载。
- 源码基线5173eefbf28267800fea698c9f25447cc053693e / 完整CI41901918734与真实Ops41901918686均已success；本轮仅补必要离线kit打包和验证，不重做服务或页面，不改VERSION、main、GHCR/NPM/tag或生产配置。
- 镜像包仍沿docker-smoke api_only=true/run_browser=false/personal=false；新增独立运维kit，同生产者source/tree/run/attempt绑定，固定白名单文件与manifest/tar摘要、独立解包/模块检查，排除真实凭据/实例/数据库/journal/测试内容。
- 已向服务器任务「查看状态并更新部署版本」发只读预检边界；其beta.31镜像表示差异的历史追加授权不通用于beta.33，须在新方案说明严格loaded ID门禁与现存镜像存储模式兼容性。最终制品与验证证据完成后补交接。


## 2026-11-05 第二批：独立受限运维与升级批准（候选，待对应 Actions 验收）

- 真实Ops [41900763817](https://github.com/ForceMind/MyAPI/actions/runs/41900763817) / test bc669df65b56f2b83058ea57893b6cf35c812f1e attempt1 success，全部步骤及安全日志已读。真实GitHub资格与制品身份、TLS、旧555→目标0489切换、配置/开关、DB/WAL/SHM备份、新增17 units与daemon重启/桥接、错误身份及匿名卷/动态端口拒绝、竞争daemon与SIGKILL冻结、启动后故障保17账务、不重放均已严格断言。故障明确为合成注入，未触生产/model/browser。
- 安全报告artifact13357454715，SHA256 0f18b61b50e0e954f53e5f3f7550ef72f14251000159050c4a84eacb2a404b4e；本机未下载。后续revision4仅按固定21项白名单在Actions日志打印安全报告，便于直接读回；目标image仍0489，服务实现绑定bc669df6，后续文档/日志提交不是镜像SHA，自身完整CI仍须回读。

- 已明确授权：Docker + Full + SQLite 独立服务、升级计划/面板确认，提交dev并读取完整CI；全部构建/测试仅Actions，无本机依赖/构建/测试/Docker/浏览器/产物下载，生产与真实模型请求禁止。
- 方案和实现合同集中在[REMOTE_MANAGEMENT](../../REMOTE_MANAGEMENT.md)，配置/离线安装在[deploy/operations](../../../deploy/operations/README.md)。根应用三库行为不改；运维功能只支持登记SQLite实例，默认未配置。
- 从代码看：HTTPS只读/准备/执行与私有Unix批准分离，同一管理Key显式Root运维scope、SQLite fresh只读校验，原Key不扩权；2FA/Passkey proof绑定plan ID+digest。固定GitHub仓库/workflow/dev/source/tree/VERSION/ZIP digest、manifest/tar与完全相等imageID，门禁失败前不修改旧容器。
- 审查必要修正：旧容器always重启会双写，禁用旧restart且新容器保原策略；匿名卷/动态端口preStop拒绝；运行时网络元数据不误当配置；OS flock防并发启动；协议缺失/null身份拒绝、不补0。源变更意图先fsync，任何未知结果冻结，不回滚DB/重启旧镜像；WAL/SHM备份与同data_dir新写保持。
- 首源码 `91fed69f13796f9367a828aa12a235372ec27489` / CI41886621709：已读完整作业/步骤，8成功/2失败/1浏览器跳过。Node服务26/26、MCP11/11通过；后端新增Ops证明/DTO/缺配置通过，原资源范围用例误用扩展后的“全部scope”夹具触发400，已固定原5scope、保原隔离断言；前端type通过、lint缺props effect依赖1错误已最小修正。该源码不是合格升级目标；新增运行时同源码完整CI成功门禁，不能用其Docker smoke绿绕过主CI红。
- 第二轮 `2a0570aa` / CI41888886679：前端type/lint已通过，资料/认证213通过1失败，新增默认scope测试误依赖Base UI自动DOM ID，已改为准确勾选数量+四个可访问名称，ops默认未勾断言不变。审查补下载后完整CI资格重验；启动后观测改为SQLite只读事务，忙/诊断失败明确null/unknown、不把正常新账务写入误判冻结；停稳备份强一致断言保留。
- 额外真实验收入口 `.github/workflows/operations-smoke.yml`，只手动dev触发、固定55531d1e旧Full与动态精确合格目标制品，真实TLS/SQLite/Docker/GitHub身份与SIGKILL/故障注入；只上传安全报告，测试不在本机运行。91的Docker41886760519成功仅为包基线，其主CI红，新增门禁禁止作为正向目标，必须用后续全绿源码新制品。
- 已验证应用/服务基线 `0489f1c21659253d65361d9b205553c0904bf7a0` / [完整CI41892349765](https://github.com/ForceMind/MyAPI/actions/runs/41892349765) attempt1 success，10成功/1浏览器跳过，全部作业/步骤与前后端原始日志已亲读；前端397文件3084/3084，运维面板17/API16、认证资料+运维早期247，type/lint/build/i18n通过(24既有warnings/0error)；Node服务31/31、MCP11/11、Go绑定proof/DTO/权限/未知与全部既有后端/独立relaykit/386/race/三库通过，后端FAIL/race标记0。
- [Full SQLite41895639097](https://github.com/ForceMind/MyAPI/actions/runs/41895639097) success，source同0489、tree5c9a5170340d1531a541e4b09ed36101ed9d23b2，20/20通用HTTP/API/同镜像恢复检查，browser=not_run；artifact13356191175 ZIP64aa729f4831cbdb16e599a03ab36866337747185984a5fd7c6d7d0b42d59da0、manifest984b9c92319726ca3db364b3b35e8d3f046850754a61812cfb50abee14ed96a7、app sha256:9150578c6fef5d462af9012b47dbca691b4590cdee0351f1660e0f854c45620a。此包只作当前真实Ops升级测试目标，不能重标后续测试提交SHA；通用20项不代替尚待的独立Ops真实验收。
- 后续小修仅独立服务/测试编排：源restart禁用请求结果未知时标null，不输出虚假的false；最后网络读取后再查批准TTL；构建器只移除本轮setup生成者，关闭自动dockerbuild记录上传，保Runner容量与仅safe报告边界。新提交须回读自身CI，真实Ops结果继续单列。
- GitHub实际拒绝新workflow_dispatch（03d02d79后调用404，工作流未在默认分支）。按官方默认分支限制，改为仅本仓库dev的受控qualification-target.json更新触发新验收，不改main。目标内容为0489/41895639097的既定身份，revision只控制CI重跑，不改变制品；未来默认分支具备工作流后可用手动入口。该触发机制不是生产部署授权。
- 首真实Ops41898016352 / test8ac59728：实际TLS/555旧Full/SQLite初始化与真实GitHub身份已运行，首次在TARGET_CANDIDATE_CONFIG_MISMATCH停，候选未启动；清理只本轮拥有容器，报告保留。猜测Docker继承目标默认Labels导致原配置比对失败；改为明确desired Labels=已验证image defaults+原用户labels（原值优先），其他完整比较不放宽，并加仅字段名诊断，尚待新真实证据。新Env/Volumes/ExposedPorts不支持变更preStop拒绝。fixture/revision2继续绑定同一0489镜像，不重标identity。
- 第二真实Ops41899971275 / test4ff6c52a：安全差异证据config_fields=[]、Mounts无变化，唯一HostConfig.OomKillDisable不同；Labels问题已消除但候选仍未启动。按Docker官方默认OOM启用语义，只规范该字段missing/null/false为未禁用、true严格保留，其他字段不改；补4×4矩阵与true漂移拒绝，新增仅该布尔字段类型诊断，revision3待新真实验收。不是放行任意HostConfig差异。
- 验证门禁：既有完整CI、Go真实路由/绑定proof/越权/无配置、Node鉴权/WAL/ZIP/持久锁/故障与MCP；额外Actions真实Docker Full SQLite升级、TLS、daemon kill/restart、源配置/开关与新增账务连续性。候选beta.33，不把源码或单测当生产验收，不发布镜像/tag或改main。


## 2026-11-05 第一批：管理 Key 与远程工具（验证记录见对应源码 Actions）

- 维护者已批准独立管理 Key、复用现有 API 的远程工具及面板入口；全部构建测试仅 GitHub Actions，浏览器/生产/真实模型请求禁止。第二批升级仅给方案。
- 从代码看：新增仅摘要存储的管理 Key 与事务审计表，现有表字段不删；登录管理员会话创建/列出/撤销，默认7天、最长90天。每次请求实时核对密钥和创建者；固定操作目录、权限相交、渠道/Key对象范围隔离，不能使用旧PAT、敏感凭据接口或模型转发路径扩大权限。
- 远程工具复用现有完整路由鉴权链，目录包含渠道、Key元数据、保存的用量与实例状态，以及明确确认的单渠道启停。未加入容器升级、任意Shell、删除、账务、诊断或上游测试。数据按接口白名单投影，缓存缺失/null/0/正数和大整数保留；写入结果未知不自动重放。
- 新面板沿个人资料安全区接入，不替换既有 Access Token。一次性凭据仅在当前身份局部状态显示，不进入持久存储；创建和撤销有对象确认、忙态、未知结果及会话切换隔离。七locale各补48键按脚本生成，en/zh实现，另外五种语言英文回填；不是七语言母语验收。
- 后端/工具首提交 `cd14e6d8b963a9a807ede0e7e2a9051a229a79ae` / [CI41867247766](https://github.com/ForceMind/MyAPI/actions/runs/41867247766) 为首轮证据，不能拿它代替后续面板或修正源码的CI。首轮已读全部作业/步骤：9成功、1后端失败、1浏览器作业跳过；三库管理Key测试全PASS，工具8/8、原前端3017/3017，后端仅实例大整数断言失败。独立只读审查发现实例Info在投影前经any解码丢大整数、stdio按累计缓冲区误限两条合法帧；最小修正保留原精度断言并新增分帧回归；后续UI审查补中文名Unicode长度、创建响应未知时GET恢复与旧空列表隔离、长ID默认折叠。最终以包含这些修正的源码完整Actions为准。
- 第二轮候选 `d89f0fb00471b6dfd07953561e038b4faedae680` / [CI41870152764](https://github.com/ForceMind/MyAPI/actions/runs/41870152764)：前端早期type/lint与同步通过，认证/资料专项209通过、2失败，均为两处Close按钮使新增测试定位歧义。仅把查找限定到弹窗内可见文本按钮，密钥清除、未知状态和POST单次断言不变；后续以新提交完整CI为准，不把失败运行记为通过。
- 验证要求：真实SetApiRouter鉴权/对象隔离/确认/权限撤销/脱敏、三库重复迁移与凭据生命周期、Node stdio与受限HTTP工具、既有完整CI；本机仅代码/Git/JSON/XML/gofmt轻量工作。浏览器和生产尚不在本批验收内。
- 入口与第二批最小方案：[REMOTE_MANAGEMENT](../../REMOTE_MANAGEMENT.md)；增量[桌面线稿](ui-design-v2/wireframes/svg/management-keys-desktop.svg)/[手机线稿](ui-design-v2/wireframes/svg/management-keys-mobile.svg)。沿用旧profile-security，非新路由，不重做既有UI。


## 2026-11-03 后续UI：本轮源码与完整CI收口（beta.31）

**2026-11-03 本轮UI源码与完整CI收口，最新应用0c51b733 / beta.31。** 应用 `0c51b733a1e10db49329ac2117624a39c0d6e707` / [CI41655135543](https://github.com/ForceMind/MyAPI/actions/runs/41655135543) attempt1 success，10作业成功/1浏览器与真实API作业跳过；全部作业/步骤及前后端原始日志已亲读。模型来源/弹窗23、钱包/权限/OAuth42、凭据/预算/分组49、设置795、资金315、模型175、公开35、账号85、Key391、连接251、日志410，全量392文件3017/3017通过；同步/构建/早期及构建后types/lint通过（24警告0错误）。根模块/独立relaykit/原生386/全部既有race/三库/Redis/ClickHouse/分发/Desktop通过，38618父项/55条/5包0skip/fail，后端失败/race标记0。

模型来源已先在b5cf0c6a / CI41642659748完整绿：去掉全局目录/前缀Fill，按当前渠道明确获取，目录绑定ID/参数/会话/权限，保手输、旧ID、预设原合并和映射；普通编辑改连接需先保存/恢复，不暗用旧连接或读Key。真实供应商当前返回哪些最新模型没有请求验证，不承诺真实缓存命中。本轮用户订阅/部署详情和后续8处钱包、权限/绑定、分配访问、OAuth、凭据/预算/用量说明、分组关系均沿既有线稿收口；12个既有视图24SVG更新，仍329视图/658SVG/66路由，不把视图数当新增路由或视觉验收。具体源/测试、所有红绿与必要修正在TASKS最新节。

自审和交叉只读审查补护包括：换渠道同地址掩码不混目录、外部关闭不自动获取、同actor撤权禁旧结果；确认关闭生命周期和准确对象回焦；未知额度/价格/日期不伪0，已知字段不因另一字段未知丢失；坏JSON可见且原文/保存准入不变；未知目录或非Root资料保存不发空权限矩阵，原Root资格/normalize/元数据准入保持。原转发/计费计算/数据库/后端API未改，全部关键断言与nonce/proof/phase/retry边界保留。新增文案共57键en/zh、其余五语言英文回填，不称七语言母语验收。成熟的轮换阶段/CC Switch/默认Bound Only/冲突确认表等保留，未为凑覆盖重做。

浏览器/真实尺寸与滚动尚未验证，全部浏览器依赖/Chromium/真实API作业跳过；官网静态41649572014与静态打包41649572292全部步骤成功，绑定83af4ada，相关官网源码/工作流/package至本应用无差异，未重标记构件。无本机依赖安装/构建测试/工具链/浏览器/Docker/产物下载；本轮未操作生产、实际.env/实例、真实模型或供应商，未发布NPM/GHCR/tag或合main，没有新服务器镜像。后续仅4份最终交接提交，应用源码仍绑定0c51b733；接手读取交接提交自己的完整CI，不能拿文档SHA当镜像。当前UI源码/非浏览器验收清单已收口；浏览器、P2、发布部署与后台无查询能力的采样策略不自动执行。下文“候选待验/最近b5”等为历史取证，不覆盖本节。

**2026-11-03 渠道模型来源已完整验证，剩余8处UI合并beta.31候选待验。** 最近已验证应用 `b5cf0c6a56b1d799e6c93851fa538691b8d4696f` / [CI41642659748](https://github.com/ForceMind/MyAPI/actions/runs/41642659748) attempt1 success，10作业成功/1浏览器与真实API作业跳过；全部作业步骤及前后端原始日志已亲读。新增模型/弹窗专项23、原连接251、资金300、后台模型175、日志410、全量387文件2994/2994通过；同步/构建/早期及构建后types/lint通过（24警告0错误）。两模块/独立relaykit/386/全部既有race/三库/Redis/ClickHouse/分发/Desktop通过，38618父项/55条/5包0skip/fail、失败/race标记0。

渠道编辑去掉全局目录与前缀Fill，按当前渠道或现协议可表达的草稿明确获取；目录绑定ID/参数/会话/权限，A/B同地址隐藏凭据不混、外部关闭不自动获取，保手输/旧ID/预设原合并与映射。普通编辑改连接需先保存/恢复，真实上游当前返回了哪些最新模型未经请求验证。用户订阅记录/授予与部署技术摘要同批收口，全部失败/夹具/类型/回焦修正见TASKS，原关键断言保留。

当前beta.31合并候选为钱包三任务、用户权限/绑定、分配访问模型、OAuth分组，以及凭据idle迁移、预算/策略说明、分组关系8处，先更新8视图/16SVG，仍329视图/658SVG/66路由；不是8个新增路由或视觉验收。产品源/21新增合成用例及必要目录错误透出/购买回焦依赖集中在TASKS；候选尚未Actions通过不能当可部署版本。40新键en/zh、另五英回填；版本仅仓库五模板beta.31，不是发布。所有构建测试只Actions，本机仅源码/JSON/XML/Git轻量检查，无本机依赖/工具链/浏览器/Docker/产物下载；生产/实际.env/真实供应商/模型/发布/main均未动。浏览器未验证；后台无查询能力的采样策略仍在本轮UI范围外。下文候选/最近绿均为历史取证，以本节及真实CI为准。

beta.31首轮 `83af4ada8b548ca385f0aec3346ea7ea07cad629` / [CI41649572171](https://github.com/ForceMind/MyAPI/actions/runs/41649572171)前端125963839564原始日志已亲读：早期types/lint通过（24警告0错误），认证180/摘要28/渠道37/模型来源23通过；钱包/权限/OAuth6文件36例27通过/9失败。后面的凭据/预算/分组专项尚未运行，取消失效候选，不声称完整绿。钱包5项首差异均是实际成功字幕前有“· ”而新测试少此前缀（真实DOM已有25/0/4对应数）；只同步7处精确完整字幕，不改owner/读取或后续断言。权限3项实为已展开且可达的checkbox名称由两个span黏连，实际Synthetic readRead description；只补标签间文本空格，原精确name、false/继承/载荷断言保持。OAuth1项是新测试错期待合法JSON的aria-invalid=false，真实JsonCodeEditor合法时移除属性；改精确absence断言，非法true/原文/强制展开/0请求不变，不改共享编辑器或校验准入。仍需下一轮验证更深断言。

beta.31第二轮 `102d3a82056752f216ed5b94e45a3b8b22cf2c24` / [CI41650856078](https://github.com/ForceMind/MyAPI/actions/runs/41650856078)前端125968293956原始日志已读：上述早期专项36例33通过/3失败，权限/OAuth及钱包回焦/SID已通过；仅钱包新测试两处Unlimited漏同一元素中的Total Quota前缀，以及BaseUI禁用Tab为aria-disabled而非native disabled。日志确知total0显示Total Quota: Unlimited，改精确完整文本；保禁购断言并实际fireEvent点击验证aria-disabled=true、aria-selected=false、当前页不变、无选购/Subscribe、GET/POST不增，不改控件/金融逻辑/超时。失败候选取消，未跑到的后续不记成功。独立的凭据/预算/分组专项只依早期source检查成功而运行，使前组失败也能读取其真实结果；原全部门禁和断言不减少，源码types失败仍快速失败。

beta.31第三轮 `08fe3ce016da3cce5001c313008d20ef8dc1b375` / [CI41651611795](https://github.com/ForceMind/MyAPI/actions/runs/41651611795)前端125970952916原始日志已读：钱包/权限/OAuth36/36通过；凭据/预算/分组5文件49例46通过/3失败，分组2新case及预算原/新case通过。仅凭据2处同样误用native disabled，真实aria-disabled=true且强制展开；补精确ARIA与fireEvent点击后关键目标/错误/unknown仍可见、GET不增/原preview一次POST/unknown0POST和回执URL全保。策略新case中模拟PUT成功后GET仍回initial.no_balance=false，真实资金enabled、保存条件回到stored allowance；修正仅成功PUT同步后续GET为同一saved revision1/no_balance=true，不改条件文案/服务mode/原nonce或confirmed载荷。产品源未改，失败候选取消，未跑完整回归不记绿。

beta.31第四轮 `e96e9941fbe354b59b2dcbf8c4e128005c9e3b30` / [CI41652586077](https://github.com/ForceMind/MyAPI/actions/runs/41652586077)前端125974368984原始日志已读：两个新增专项36/36、49/49均过，设置795/资金313/模型175/公开35/账号85通过；原Key46文件388通过/3失败，全为api-keys-action-menu旧用例直接要求折起的owner-public-model可见。仅两处加明确展开Available models(1)，保原toBeVisible、完整Key零读取、显式Copy/失败与键盘焦点全部断言；此文件加入早期专项，产品不预读秘密或改权限。后端/完整回归未结束即取消失效候选，不算全绿。

Root追加自审的必要权限载荷边界：user-form转换器原约定目录缺省就省略admin_permissions，但Drawer过去总传data??EMPTY，占位对象truthy可生成空矩阵；现在只在原Root编辑资格且query.data确有resources时传真实已读目录，初次失败/合法空目录或非Root传undefined，已有成功非空缓存可保原草稿。未新增提交门禁、未改normalize/后端/Role规则。新增2精确HTTPcase保护目录失败和非Root普通资料更新不含admin_permissions/role/quota，其他元数据与0POST保持；此为从代码发现的错误payload风险，不声称生产已发生。合成本批新增现23例，仍待下一Actions验证。

本次最终合并范围为原两个隔离UI组，未新增金融/鉴权/供应商目标。11个呈现/读取源与11个测试路径（含后补旧Key菜单合同）：权限3源/2新test/owner-session旧4处展开；钱包1源/1新test和PurchaseDialog仅可选returnFocus UI透传；OAuth1源/原test新增3例；凭据/预算/用量3源+原3test+独立真实HTTP凭据新test；分组详情1源/1新test；另users/api.ts仅既有catalog getter验证success及两个数组，失败抛消息，成功返回shape/URL/config和所有写方法不变。原权限/金额/JSON/CAS/nonce/proof/phase handlers/API载荷与重试门禁不改。

钱包保当前/记录/选购及分页、读取失败不伪真空，未知报价/次数不伪0；自审补已知total+未知used分列、Canceled/Expired与未知日期独立。权限raw true/false/继承明示，目录失败/缺data Unknown+Retry保草稿不显示伪0；原checkbox normalize及保存准入保持。OAuth先基本/凭据/回调，后端点/映射/高级分组，原字段keepMounted；独立只读审查发现坏JSON不属RHF错误可被新折叠隐藏，Root复用既有JSON校验，错误外露强制展开、无RHF错误的aria-invalid为undefined，不改schema/原JSON/保存准入；新增真实input坏JSON/修复保持展开与Secret不入摘要用例，错误焦点复用已有FormValidationFocus。

凭据仅idle配置收起，所有preview/error/unknown/恢复/nonce状态强制可达；预算仅说明收起，严格Chat1050000警告/单位/预算主表单常驻，Root把静态记账帮助移到读取失败时也可达的位置并补零写入断言；用量策略生效条件/inactive仍常驻可保存；分组详情仅长且有效覆盖/visible数组摘要，invalid/nonfinite/负值、Hidden/danger/不可选/Overview常驻，原方向/比率/顺序/唯一滚动与回焦保持。新增21例保护上述核心行为，不以代理静态汇报称已通过；原mock安全用例只加展开步骤，凭据2条新增另置真实HTTP文件不mock核心验证。

本批8视图16SVG已先补且XML有效、未浏览器渲染；author/catalog/目录同步，未重生全站或覆盖他人成果。40键脚本追加七语言并自删、旧值/顺序保；JSON/t字面轻量检查不替代runner i18n:sync。两个新早期专项及原完整检查均保留；所有构建测试只Actions，候选未绿不部署。此批合并验证后再更新最终交接并读取其自身CI，不重复创建状态文档。

维护者授权持续清查未完整替换、信息堆叠的页面/弹窗/组件，沿现有线稿精简合并，保全部功能/权限，不逐批停等；新增明确修复渠道模型填写混用全局旧目录。成功标准：模型候选来自当前连接明确获取的结果，已有配置/手填/映射/自定义预设保留，参数与身份变化隔离旧结果；其他surface依具体源码问题与现有线稿逐项收口。验证方法：全部定向/全量测试、构建/types/lint/i18n与原完整后端门禁只在Actions，browser/production/真实供应商不执行。

首轮候选 `8d51a1146b3d75e832a87fdcc3f540a42fb024ba` / [CI41634483602](https://github.com/ForceMind/MyAPI/actions/runs/41634483602)前端125912082427原始日志已读：认证180/摘要28/渠道权限37通过；新增早期门禁5文件21例17通过/4失败。部署摘要与原相关回归、订阅未知授予锁跨任务通过。模型预设用例期望保存+手输+预设，实得仅预设，须核对原明确替换行为，不为错误夹具改变产品；撤权两例在Fetch子弹窗仍打开、父层inert时查询父chips失败，修正为明确取消回父再保完整ID和0写入断言。订阅准确删除末页后预期标题回焦，实际body，须修复且不放宽焦点断言。失败候选取消请求已发出，后端与其余矩阵未完整结束，不称全绿；候选不能部署。下一批只读计划保留，先闭环实际失败。

模型3项红的最小修正只涉及新增测试：Root与代理核源码确认updateModels(items,true)为合并，未改产品合并逻辑。所读BaseUI ComboboxInput代码在popup未挂载的multiple模式按Escape会清空选择；原日志只证明最终剩preset，结合源码推断Escape清空，不声称本机/浏览器复现。用例保留手输Enter，明确收起列表后逐步断言完整chips，先精确保存saved+manual，再加预设精确保存saved+manual+preset；两撤权用例明确关闭Fetch后才查父层，原无旧结果/不可应用/完整旧ID/0写入断言保留。不mockModal、不延长超时、不改第三方键盘行为。订阅回焦从代码看与确认框仍open就条件卸载有关；最小修正在原订阅源保两个ConfirmDialog根挂载，以受控open关闭并守卫空载荷/禁Confirm，原金融handler和精确焦点断言不变，不改共享组件。上述修正均仍待下一轮Actions验证。

第二轮 `e13bb4bc9f013fec34758b4fc140702afd096c28` / [CI41635893249](https://github.com/ForceMind/MyAPI/actions/runs/41635893249)前端125916872480原始日志已读：新门禁21/21通过，含原精确合并/撤权/末页回焦；认证180/摘要28/渠道权限37/设置790/资金300/模型175/公开35/账号85/Key388通过。原连接专项249通过/2失败，均是connections-editor桌面/手机旧测试期待GET全局/api/channel/models。按维护者明确移除全局来源的要求，同步为该请求为0并补当前渠道原生详情GET必须存在，原全部GET限定、取消、0写入、密钥/测试/余额/fetch_models禁止断言和原15秒预算不变。产品不恢复全局目录，不用旧行为制造绿；失败候选取消请求已发，仍未完整绿。下一组已先补wallet/user-edit/assigned-access/OAuth四视图八SVG及隔离WIP，不包含在本次beta.30验证提交；未验证，不冒充实现验收。

第三轮 `b13cb48de0a87e9fa14ba098f4b6ad2147f5fe30` / [CI41638043905](https://github.com/ForceMind/MyAPI/actions/runs/41638043905)前端125924131733已读归档原始日志：定向门禁已走完，types4错、lint18错/25警告，前端全量尚未结束时取消失效候选以尽快取得错误，不记作全量通过。4个类型问题：Plus未用、deployment新测试ByRoleOptions两处无exact属性、订阅成功数组在setPage闭包内丢类型收窄；修正为删未用导入、保默认精确role name查询、捕获已验证数组const，不改断言或数据。lint为新模型来源useMemo读取getValues却把17个watch值列成未使用依赖，以及订阅新fixture一个多行if缺大括号；改显式raw字段对象与准确依赖、fixture加括号，不关规则、不删监听或延长超时。

Root静态再审补同渠道ID边界：旧modelCatalog/sourceScope只比auth与连接表单值，同实例A→B若URL/type/掩码key相同可错用A已完成目录。目录显式存channelId，视图与Apply核对ID，scope包括ID/open/identity；外部关闭或换对象会清Fetch状态，防重开编辑器自动探测。新增2真实组件case保护同配置不同保存凭据A/B、精确原模型载荷、明确Fetch才取B、A/B迟到拒绝及关闭后不自动获取。原snapshot/signal不改，无新API/Go/文案。本项是静态风险与防回归候选，未称旧CI已运行复现；新回归须下一轮Actions取证。下一组隔离WIP仍未提交，不混入本次beta.30源码验证。

第四轮 `20e35f1a6c9d6808537dcdfbec651f5a22e0d9ea` / [CI41640774155](https://github.com/ForceMind/MyAPI/actions/runs/41640774155)前端125933423132原始日志/注释已读：types通过，全部定向回归含渠道ID新case与原连接合同已走过；lint24警告/1错误，仅新SwitchingEditor夹具props.onReady作为method调用被要求props依赖。改为局部onReady引用并精确依赖，不改产品/测试断言。已取消失效候选取日志，前端全量未结束，不记作通过。增加早期源码types/lint检查，同时保留构建后原检查、全部回归/后端门禁与断言，失败候选尽早暴露语法/类型问题，不加超时、不本机运行。

本批候选范围：模型5文件移除getAllModels/前缀Fill，仅按当前已保存连接或现协议可表达的草稿明确Fetch，源字段/同actor权限变化、关闭/失败/迟到均不能应用旧目录；旧模型ID通过选中项fallback完整可见，手输/预设/映射保留但不作为支持证据。无Go/API协议改动，不读取真实Key预填，普通编辑变更需先保存/恢复，Advanced仅原协议准确支持字段可预览。新增Drawer6例、原Dialog加1例，原GET/POST401不重放合同继续跑。

用户订阅仅源/新workspace测试/原recovery交互步骤3文件：记录默认、Grant keepMounted保草稿，20条本地分页，Unknown/未确认锁与原全部金融handler/载荷/独立确认保持；新3例保护切换/页大小不读写、末页精准删除回焦、未确认结果跨任务恢复，原state/layout断言不改。原SelectItem缺价显示同步已有finite/Unknown判断，合法0仍原toFixed，不改金额算法。部署详情仅源及原state测试2文件：位置/容器完整列表复用SummaryDisclosure，原状态分布与读取错误外露，新2例保键盘展开/完整长ID、明确Open与网络次数、确知0/未知区别。合计12个新增合成用例，未在本机执行，待实际Actions取证。

17新文案键已通过临时add-missing-keys脚本追加七语言，en/zh、余五英回填，保旧值/顺序，脚本已自删；JSON解析及当前变更字面t键缺失扫描0仅轻量检查，i18n:sync只CI。五版本文件对齐beta.30，仅仓库模板不涉及生产。新增早期“channel model sources and compact management dialogs”门禁，原全部测试/构建/types/lint/后端/三库/relaykit/race保留，浏览器仍跳过。当前候选不能称可部署或真实最新模型支持已验证。

当前迭代 beta.30：①渠道模型来源与旧结果隔离；②用户订阅弹窗记录默认/显式授予与有界本地分页；③部署详情位置/容器摘要，原状态分布与读取失败外露。已补4个既有视图/8张SVG，无新增路由或视图。Root独占部署详情及原deployment-tools-state回归，新增2合成case待Actions；代理独占渠道模型与用户订阅，Root统一翻译/版本/CI，新增早期门禁但原全部门禁不删。后续钱包订阅/历史/选购的信息任务拆分另小批继续，不顺手改金额算法或支付协议。完成本迭代不是全站浏览器验收或主Goal完成。

后续只读清单（不是已实现或浏览器验收）：另一轮检查12个高密度surface，7项有具体收口空间、5项成熟保留。下一组优先user-edit（users-mutate-drawer资源权限矩阵/只读绑定）、自定义OAuth provider-form-dialog（端点/映射/高级表单分组）、assigned-access（模型可用性大卡/重复拒绝原因/协议说明），连同钱包subscription-plans-card任务入口按小批执行。后续单列credential-maintenance（轮换/迁移任务）、token-budget-dialog（预算/协议说明）、usage-policy（偏好/全局模式/生效条件）、group-ratio-visual-editor-3（入/出覆盖与可见性关系数组）。必须保持Root资格/准确对象、所有blocker/unknown/失败、资金边界/预算上限/原命令ID与精确scope载荷常驻可达，不改金融/认证状态机或计算。成熟保留：credential-rotation分阶段秘密区、CC Switch按应用字段、user-binding默认Bound Only、计费冲突确认表；channel-affinity rule-editor主条件与来源顺序保留，只在后续确有必要时整理Common Keys帮助文案，不为覆盖数量重做。确切viewID与源已由代理按catalog核对，后续执行先更新对应既有线稿；不新增路由，不因没有Disclosure机械判缺陷。

当前干净dev基线bac76563 / CI41626054346完整绿已由上一轮核对，应用eeb5f0de / CI41621188497。fetch/pull ff-only已核对0/0、无分歧。从代码看，编辑器getAllModels生成全局候选，type1只按gpt-/text-前缀筛基础项，其余类型全收；Fill related/all会直接用该名单更新表单。已有渠道FetchModels/FetchUpstreamModels可按连接取模型，Codex现有service走/backend-api/codex/models，并非需新建协议。此轮不改provider/凭据刷新/计费/数据库。

Root先补connection-edit与既有fetch-models弹窗四张桌面/手机SVG及catalog/author/目录说明；仍329视图、无新路由，XML有效但未渲染。复用两名Sol/high：一名独占渠道编辑/模型获取相关组件及合成回归，一名只读清点其他surface并提出具体候选；Root管线稿、翻译、CI、版本与同一交接。不得递归、不得本机工具链、不得替换他人改动。当前产品尚未修复/验证，不以设计或代理汇报称完成。

## 2026-11-03 渠道整页重整：源码与完整CI通过（beta.29）

**2026-11-03 渠道整页源码收口，最新完整绿 eeb5f0de / beta.29。** 应用 `eeb5f0de0a5ebb8de934834fe0dea44b6bc8479e` / [CI41621188497](https://github.com/ForceMind/MyAPI/actions/runs/41621188497) attempt1 success，10作业成功/1浏览器与真实API作业跳过，全部作业/步骤与前后端原始日志已亲读。渠道/权限37、原连接251、日志410、全量385文件2980/2980通过；同步/构建/types/lint通过（24警告0错误）。根模块/独立relaykit/原生386/全部既有race/三库/Redis/ClickHouse/分发/Desktop通过，38618父项/55条/5包、0skip/fail，后端失败/race标记0。

渠道按库存、额度证据、全部渠道事件、显式路由预览四任务整理；监控/选路/自定义列隔离保存，筛选、手机卡片、已选ID/整标签/全库操作、四区编辑与高级错误定位同步收口。保原字段、权限、敏感确认、URL搜索/返回位置/焦点、未知值及失败可见性；目标渠道无证据不借用其他账户，路由切换不自动提交。32项新增回归及三轮失败/最小修正集中在TASKS，未放宽原断言。线稿329视图/658 SVG/66路由/57模式/205页面源为归属覆盖，非视觉验收。

浏览器/实际尺寸与滚动尚未验证，所有浏览器依赖与Chromium步骤跳过。官网静态检查41615395919/静态打包41615395980全步骤成功，绑定eb06d098；官网源码/工作流/package到本应用无差异，复用证据，未运行官网浏览器。本机仅代码/JSON/XML/Git轻量检查，无依赖安装、构建测试、工具链/浏览器/Docker或产物下载；未发布NPM/GHCR/tag、合main、部署、改实际.env/实例或发真实模型请求。采样能力策略不在本批，不把历史失败改成功。后续仅4份交接提交，应用源码仍绑定eeb5f0de，接手须读取最新交接提交自身完整CI；不得把文档SHA当镜像身份。下文“候选未验”和此前最近绿均为历史取证。

第四轮 `eeb5f0de0a5ebb8de934834fe0dea44b6bc8479e` / [CI41621188497](https://github.com/ForceMind/MyAPI/actions/runs/41621188497)前端125867665255已success，原始日志已读：渠道/权限37/37、连接251/251、全量385文件2980/2980，认证180/摘要28/设置790/资金297/模型173/公开35/账号85/Key388/日志410均通过；翻译同步、构建、types通过，lint24警告0错误，Unhandled Rejection标记0。其余8作业成功、浏览器/真实API作业跳过，后端完整作业仍在运行；最终以其完成后全部步骤与原始日志为准，尚不能称完整绿。

第三轮 `afa96e44727b74ce62680ba3c1cc7687cc88872e` / [CI41617940108](https://github.com/ForceMind/MyAPI/actions/runs/41617940108)前端125856324989原始日志已读：渠道含原权限37/37、原连接251/251、所有既有专项与全量385文件2980/2980通过，翻译同步与构建通过。仍有两个类型错误：新证据fixture closest返回Element不能传within，以及父Link search reducer被推断为全路由联合；改HTMLElement精确泛型、Link使用已验证当前route search对象，完整URL断言不变。五处Hook依赖lint错误改为引用稳定的reportReadState/refetchPreview局部变量，不关闭lint规则或增不稳定对象依赖。24警告保留，错误待新CI确认；本轮不能称完整绿。后端仍在运行，后续推送取消旧运行，不能拿未完成结果当成功。

第二轮 `4ced80e84021218494746252b9272c6b040d8f0c` / [CI41616929886](https://github.com/ForceMind/MyAPI/actions/runs/41616929886)前端125852594585已读原始日志：新增门禁35/35通过，原连接专项249通过/2失败，均为旧权限fixture未挂Router而useLocation报null，未执行到原权限断言。修正为真实路由/父子组件+仅外部HTTP模拟，不再mock页面壳/provider/table/API业务封装，保原禁止挂载/查询和root挂载断言，并补active时撤read即卸载0写入。产品在无read时移除重型任务入口及section，旧权限边界不放宽。早期门禁加入此原权限文件，其原连接门禁继续保留。其他认证180/摘要28/设置790/资金297/模型173/公开35/账号85/Key388均通过；候选仍未完整绿。

首轮候选 `eb06d09895c2a909ccd6c242f8eb176ef7e28b48` / [CI41615395924](https://github.com/ForceMind/MyAPI/actions/runs/41615395924) 前端125847472833已读原始日志：认证180、摘要28通过；新增渠道专项35例22通过/13失败。11个证据任务用例全通过，父导航4例因BaseUI将链接赋button角色失败；修复为原生Link与hash-aware active，不更改link断言。库存两例分别匹配重复摘要/完整正文及标签选择，5个延迟写用例虚拟时钟超时；编辑2例高级区意外展开，按首个差异继续修正。父fixture补真实挂载会读取的2FA/Passkey能力GET，未mock内部状态机。未增加超时或放宽安全断言。官网静态41615395919、静态打包41615395980成功；主CI其余当时8成功、1跳过、后端与durable仍运行，失败候选取消请求已发出，不称完整绿。 修正根因：滚动观察器不再隐式打开高级区，显式导航/偏好/错误自动定位继续保留；Tag批量取flatRows内真实数字ID，私有调用向共享批量壳传可选selectedCount，其余调用者默认不变。hidden摘要/全文匹配分开且完整正文仍须可见；虚拟时钟后的交互改focus/fireEvent/同步act，保真实scheduler、0PUT与精确一次PUT载荷断言，不延长超时。旧CI最终cancelled，不能作为完整后端通过证明。

维护者要求渠道页整页重整，继续当前已批准 UI 迭代。成功标准：库存查询和操作优先，额度证据/全部渠道事件/显式路由预览各有清晰入口，全部字段、权限、确认、原 URL 搜索与返回位置保留；验证方法：GitHub Actions 定向回归、全量前端、构建/types/lint/i18n 及原完整后端/分发门禁，浏览器不执行。

设计提交 `f8473a19e5787f765f7c0fdab5cc67e94588e3e5` / [CI41605873115](https://github.com/ForceMind/MyAPI/actions/runs/41605873115) attempt1 success，10成功/1浏览器与真实API作业跳过，全部作业步骤及前后端日志已读，前端382文件2948/2948，38618父项/55条/5包全通过。本候选在同一设计源继续细化10张SVG的真实工具栏、六列库存、目录/正文、事件分页和路由表单；329视图/658 SVG/66路由/57模式/205页面源是归属覆盖，不是浏览器验收。

从代码看，库存、额度证据、全渠道事件、路由预览按任务切换；库存保筛选/选择/自定义列与滚动，隐藏时停止读取及分页/批量 Portal。监控/选路/自定义三种列布局隔离保存，不覆盖旧自定义偏好。筛选保旧 all 优先语义，手机卡片完整字段可展开；创建与选择常驻，显示设置/全库维护、行内诊断/模型/凭据/危险操作分组，已选ID、整标签、全库范围明确。编辑四主区保错误定位和全部高级字段。

额度目录按原窗口身份选择，指定渠道无证据不替换为其他账号；历史对象不匹配拒绝显示。缺失/null/0/正数和失败观察保留，不把HTTP成功等同健康；失败关闭任务后仍可见。事件明确全部渠道，未知总数不补零。路由只显式提交读取本地快照，切回/失焦/缓存失效不自动提交。身份/权限改变清理草稿及读状态；新增行内数值延迟写入的派发前身份/权限检查，保原同身份 debounce/blur/cleanup 时机与载荷，避免本批更多卸载时机跨会话写入。未称生产已复现此静态风险。

候选新增三组实际组件回归及原测试精确 signal 参数兼容，未放宽原断言；新增68英文键，中文翻译、另五语言英文回填，版本五文件同步beta.29。所有构建/测试/同步只在Actions；本机只源码/JSON/XML/Git轻量检查，不安装依赖、不跑浏览器/Docker、不下载产物。转发/计费/数据库/采样能力策略不改，生产/发布/main/真实模型不动。产品候选尚未通过完整CI，最近已验证应用仍e3db228e / CI41594615317；前批交接8aef9a7e / CI41598455431全绿。以下历史状态不覆盖本节。

## 2026-11-03 渠道页整页重整：追加授权，持续实施

维护者指出渠道页仍有大量原版堆叠交互，要求全面替换并自主持续推进。本轮不以“组件已改过”代表任务路径已设计：库存管理为默认主任务，额度证据、全部渠道事件、路由预览为同一路由内三个显式工作区；不把四块内容简单全部折叠。监控/路由/自定义信息视图与显示设置、已选渠道批量、整标签操作、全库维护分层，所有原字段/条件入口/权限/确认/参数/NoReplay保留。编辑抽屉保成熟四主区与错误定位，修正普通priority/weight使整个高级区自动铺开的问题。

成功标准：默认库存围绕查找、状态判断和明确操作；已配置/低频信息可达、手机桌面同任务层级，当前故障/Unknown/已知失败及不完整状态不被任务切换藏掉。四旧section锚点、quotaChannelId与page/pageSize/filter/status/type/group/model/focusConnectionId和返回焦点/滚动保持；目标无保存证据不自动展示另一渠道。默认不读重额度证据，事件API仍全渠道不伪造channel过滤，路由仍明确提交才读取；切任务保草稿/已知快照，hidden实际门控query/轮询而非只CSS。所有业务API/body/计费/数据库/后台采样策略与原确认不改。

已先写线稿：更新channels与connection-edit四张SVG，新增同路由额度/事件/路由任务六张SVG与目录/路由映射，无新产品路由。当前329视图/658SVG、66路由、57模式、205页面源；原326视图/652SVG记录是此前基线。Root负责index任务框架、URL/身份/状态/返回适配和小范围API signal兼容、线稿/i18n/CI/交接；复用两名gpt-6.1-sol/high负责库存/动作/抽屉和四证据组件，禁止递归/本机工具链/提交。

验证方法：全部GitHub Actions中真实UI与外部HTTP边界合成，覆盖默认无重证据请求、显式切任务、不自动预览/测试、旧URL精确目标、隐藏停止轮询、未知/失败仍可见、全部高级入口/完整字段、视图/偏好/返回保留与原载荷/确认/不重放；原完整构建/types/lint/前后端/三库/独立relaykit/race门禁不删、不加timeout。浏览器先不跑，不动生产/真实供应商/发布/main，本机仅源码与设计文档轻量检查，不安装依赖或下载产物。方案与线稿已备齐，产品候选未验证，最近完整绿交接仍8aef9a7e/CI41598455431（应用e3db228e）；本轮不重复修前批已验证Disclosure，也不冒称采样能力或全站视觉验收完成。

## 2026-11-03 信息层级收口：源码与完整非浏览器CI通过

已验证应用源码 `e3db228e31b62fb0a187ace71d46571ff7baccf9`，候选版本 `0.2.0-beta.28`；[完整CI41594615317](https://github.com/ForceMind/MyAPI/actions/runs/41594615317) attempt1 success，10作业成功/1浏览器与真实API混合作业跳过。全部作业/步骤、前后端原始日志已亲读：认证180/180、新摘要/展开7文件28/28、设置/分析/公共89文件790/790、用户/资金297/297、模型173/173、公开35/35、账号85/85、Key388/388、连接21文件221/221、日志410/410、全量382文件2948/2948通过；同步/构建/typecheck/lint成功，24警告0错误，无未处理错误。新增3条警告均为测试字符串拼接风格，未关闭规则或藏失败。

根模块/独立relaykit vet/build、原生与386、两模块全量、全部既有race、三库SQLite/MySQL/PostgreSQL、Redis/ClickHouse、分发/Desktop门禁通过；38618父项/55含子项/5包通过、0skip/fail，原始后端失败/race标记0。全部浏览器依赖安装与Chromium跳过；本机无依赖安装/产品构建测试/lint/formatter/浏览器/Docker或产物下载，未生产、真实模型、发布或main。

范围内收口：连接主卡模型/分组数量默认收起、优先级/权重紧凑行、启用移入状态行，已用/剩余明确，窗口时间与说明进明细，错误/未确认恢复仍外露；连接与模型父网格items-start使展开不强撑邻卡。连接详情/模型发现/Key保存名单和窗口份额/分配复用共享BaseUI Disclosure，保原草稿/旧空ID与请求载荷，空名单不设无用开关；超分额、暂停、Unknown及待归因保持首屏。Flow全选标签移入原数量Popover，FAQ/公告只读摘要可展开全文，模型长说明与榜单按需展开，金额/权限/当前故障/主要操作不藏。17个组件/呈现源、14测试文件，源码新增19例；全部原字段/长名/未知/请求/撤权/abort/确认/不重放断言保留。

已先更新12视图/24 SVG并以69e7dd2f / CI41588511701完整验证，再推产品；全部326视图/652 SVG与原路由目录保留。20新键en/zh，其他五语言英文回填，经临时脚本追加保旧值/顺序、CI同步，无新依赖/路由/数据库/后端采样策略/计费或转发算法。VERSION/package和三仓库模板一次同步beta.28，不反复升级。官网静态41592724407、打包41592724528全部步骤已读成功；从b32d9dc5到e3db228e相关官网/工作流/package/Logo源无差异，复用该证据，无官网浏览器运行。

本批余下仅更新现有4份交接并读取该交接提交自身完整CI，应用源码仍绑定e3db228e。浏览器真实呈现/高度/滚动未验证，不从语义单测宣称截图已验收或生产已更新。无额度查询能力的后台策略不在这次已批准UI批次，不把现有错误改成成功/不支持，也不声称采样已修；P2/发布/生产仍单列。下文“实施中/候选待验”是过程记录，以本节、git log和实际CI为准。

设计步骤69e7dd2fbd26168a789401f6e32c6f498d082101 / [CI41588511701](https://github.com/ForceMind/MyAPI/actions/runs/41588511701) attempt1 success已完整回读，10作业成功/1浏览器与真实API作业跳过，全部步骤及前后端原始日志已读。设计提交仅28个线稿/记录文件，应用仍为既有基线；前端全量378文件2929/2929、构建/types/lint（21警告0错误）及全部既有后端/独立relaykit/三库/race/分发/Desktop通过，38618父项/55条/5包、0skip/fail，原始失败/race标记0。产品候选另行提交验证，不把设计CI当新交互通过。

候选b32d9dc5 / CI41592724437已进入回归：新摘要/展开门禁、认证及后续设置/用户/模型阶段已执行通过，尚未完整CI。官网静态41592724407和打包41592724528成功；未触发官网浏览器。Root后续定向检查发现pricing/model-card-grid.tsx的父网格仍是默认stretch，虽然连接列表已加items-start，模型长说明展开仍可能撑高邻卡。这是源码静态发现、不是已做浏览器测量，也不是已在CI失败；仅补父网格items-start，不改卡片数据/分页/价格/顺序/布局列数，不新增镜像实现的CSS单测。原行为回归与完整CI照常保留。旧候选为此必要布局修正取消，不能标完整绿；修正候选继续完整验证，版本仍beta.28不反复升级。

d2c334b9 / CI41593555903前端125775938497原始日志已读：认证180、新摘要7文件28/28、设置/分析/公共790、用户/资金297、模型173、公开35、账号85、Key388通过；连接21文件220/221，唯一旧saved-windows只读刷新用例的全文正则同时匹配首屏与keepMounted隐藏明细。仅将该断言在/connections定位到Current account windows可见摘要，详情路由仍按原定位，原No reliable usage sample/无100%伪值/精确单GET与params断言不动，不取getAll第一个或改弱匹配。产品不改；已红主CI取消重复后端后，下一提交仍跑全部完整门禁。

维护者依据连接卡片截图要求“模型只显示数量、点击展开，同类交互都检查”，已确认本批方案并授权持续完成，不逐页再问。沿既有方向3与326视图，先更新12个实际父视图/弹层的桌面与手机线稿，共24张SVG；不新增页面/路由、不重画其它线稿。入口仍为wireframes/index.html；connections先画默认摘要和明确点击后的完整展开态。

成功标准：连接卡片默认紧凑，模型/分组数量展开可达、窗口/历史说明按需展开，同排不被展开卡片撑高；当前额度、真实故障、权限、确认、操作与未知值始终保留。同步收口连接Routing/模型发现、Key模型范围与窗口分配/份额、Flow重复已选标签、FAQ/公告只读全文预览、模型长说明及公开模型榜单；已有合理折叠不重做，必填编辑字段/金额/关键确认不折叠。展开只改变呈现，不调用供应商、刷新/探测、读取密钥或写数据，不丢草稿/旧ID/空ID/返回上下文。

验证方法：Actions内相关真实组件/HTTP边界合成回归、原全部字段/未知/不重放/确认断言、构建/typecheck/lint和完整既有CI；原浏览器开关保持关闭，浏览器未验证。本机仅源码/线稿作者文本与Git/JSON/XML轻量检查，不安装依赖、执行工具链或下载产物。Root管连接主卡、共享Disclosure、线稿/i18n/CI/交接；复用两名gpt-6.1-sol/high代理分别拥有渠道/Key/窗口和其它摘要预览组件，均不递归派工/提交/本机测试。

当前只完成线稿源更新，产品候选未验证。已有应用基线fffb2ef0/CI41539150203与交接29693229/CI41543179850完整绿保留。本批不更改后台采样能力策略、不把现有失败凭空改为“不支持”或“成功”，该方案仍单列待确认；不动生产、真实模型、发布或main。风险是折叠吞掉字段、误触整卡跳转、旧身份/草稿残留及隐藏故障，将保完整字段可达与相应回归，不能靠固定高度裁切制造紧凑。

产品候选已备齐、待CI：16个组件/呈现源（含新SummaryDisclosure）+14个测试文件，源码新增19例，不能据此写成通过。共享控件复用现有BaseUI Collapsible/Button，默认闭、keepMounted保草稿、明确type=button及键盘/ARIA；主卡模型/分组数量默认收起，优先级/权重紧凑行，启用移到状态行，已用环与剩余窗口明确，移除名称Link的整卡覆盖层，同排items-start；所有当前/未确认错误仍在首屏，每个完整系列的来源状态说明和观察时间留在明细。其它子批按上文12视图执行，既有合理details/分页保留。

交叉只读审查发现“按同channel较新成功判断历史”原语义不能用来过滤另一窗口/身份的故障，因此主卡compact不滤掉任何error行：旧分类不改，但摘要标恢复未确认，完整证据仍明示not resolved。新增7d失败账号A+较新5h成功账号B的真实组件回归，未跑。分额列表补外层超分额、pending/unsampled/uncertain待归因Key数，原逐行计算/精度与所有明细数值保持，不以折叠隐藏关键告警。

原可见性合同仅增加明确展开步骤，保长名/0与Unknown/旧空模型ID/完整保存body/返回上下文/撤权隐藏/abort/单次派发断言，不换宽匹配或提高时限。Actions新增7文件的早期摘要交互门禁，后面全部既有门禁不删。20新键经临时源码脚本追加，en/zh及另五语言英文回填，旧译文和顺序保留；同步仅CI。沿既有体系一次更新候选beta.28，同时同步VERSION/package与三分发模板，不改任何实际.env/实例、不发布/打tag。下一步在设计CI完整结束后推此候选并读其所有CI。

## 2026-11-03 UI14–18：剩余UI源码与非浏览器CI收口

已验证应用源码 `fffb2ef0ae3b46c9721896d5c47641e31c2be959`，候选版本 `0.2.0-beta.27`；[完整CI41539150203](https://github.com/ForceMind/MyAPI/actions/runs/41539150203) attempt1 success，10作业成功、1浏览器/真实API混合作业跳过。全部作业/步骤及前后端原始日志已亲读：认证27文件180/180、剩余设置/分析/公共88文件786/786、用户/资金29文件297/297、模型23文件173/173、公开8文件33/33、账号11文件85/85、Key45文件388/388、原连接19文件209/209、日志47文件410/410、全量378文件2929/2929通过。同步/构建/typecheck/lint通过，21警告0错误，无未处理错误。

根模块/独立relaykit vet/build、原生/386、两模块全量、全部既有race、SQLite/MySQL/PostgreSQL、Redis/ClickHouse、分发/Desktop门禁通过；386定向18父项/含子项55条/5包通过、0skip/fail，后端原始失败/race标记0。分发示例默认版本已与VERSION/package一致，原CLI版本/初始化/升级/签名断言保留。所有浏览器依赖安装与Chromium步骤跳过。本机未安装依赖、构建测试、跑浏览器/Docker或下载产物，未生产/真实模型/发布/main。

UI1–18按同一清单收口，保留既有成果和方向3线稿，没有重新搭建。326视图对应201个页面源、54模式、138弹层，66路由中含13别名/3布局；这些是归属覆盖，不能当326个独立页面或像素验收。UI15站点/认证/内容/安全保草稿与CAS显式Reload、原始JSON和未知字段；UI16模型/价格/集成/运维及SystemInfo保金额算法、Root权限和未知值；UI17分析单日历、学习双分页、稳定会话与Chat凭据读取，保Playground引擎；UI18公共失败/重试、长名/展开、Setup/错误/导航与官网，保原路由、第三方iframe和原生Desktop边界。局部NoReplay保原body/凭据/幂等或CAS标识，纯读取正常认证刷新保留。46补键为32新增与14既有缺键，en/zh、其余五语言英文回填，旧译文/顺序保留。

官网源码/工作流/package与fc6a4984相比无差异，复用已回读的静态检查41530134226、静态打包41530134298成功，以及41530134251两浏览器作业整项跳过（0步骤）；main CI的浏览器也未执行。官网浏览器以后必须另获授权，手动dispatch且run_browser=true，security_evidence分支同样受控。历史“仍需门禁”已由此源码解决，不把skip写成浏览器通过。

本次后续提交只更新现有4份交接，应用源码仍绑定fffb2ef0；推送后读取交接提交自己的完整CI，不为文档再升版本。当前UI源码和非浏览器CI实施范围已结束，浏览器视觉/真实交互、七语言母语补译、独立全站质量审查、P2后端/部署脚本及发布/生产不是本轮已验收范围。既有高级配置顶层未知扩展和RateLimit特殊组名边界仍列待办，不冒称修复。下文候选/失败/待实施均为历史取证记录，以本节、最新git log和真实CI为准。

## 2026-11-03 UI14：个人资料与认证（历史取证，后续完整CI通过）

旧源测试2e02c062 / [CI41523236626](https://github.com/ForceMind/MyAPI/actions/runs/41523236626)前端125544280381已读原始日志：27文件152通过/28失败。18组认证/邮件/挑战/凭据操作401后认证自动重放，403与成功原参数/Proof/轮换合同保留；Reset假口令确实进入success toast；验证和单个/其它会话pending时Escape实际调用close；绑定读取错误无alert、密码可见性无pressed、其余新region/field标签处首差异保留，不声称全部后续断言已独立复现。无真实重置/泄露/邮件/凭据操作。

候选仅25个UI源、4个API源与19新键：实际Profile卡片单页与原AuthLayout/OAuth44按钮保留，不生成假路由；敏感操作原security scope/2FA-Passkey选择/proof/flow_token/redirect/步骤/关闭清secret/30秒倒计时不改。自定义绑定失败Unknown+显式Retry，成功真空才Not bound；内置绑定来自实际profile，不伪未知。Session确认准确设备/IP/方法/最后活动/current，不展示完整SID，pending禁误关、失败保对象。密码Reset只去3条通知秘密，自动copy/单次明文/手工copy及原email/token body保留，inline POST补NoReplay。

Root对本轮相关18个原缺口与原已保护3个对照，局部API skipAuthRefresh，不改proof headers、signal、captcha params、原flow token、合法session rotation或全局默认重试。21×3真实Axios合同+10实际组件用例全部保留，候选仍待Actions；19键脚本追加七语言（en/zh，另五英文回填），旧译文/顺序不改。UI15–18后续候选及55静态补护仍隔离，不入UI14验收声明。浏览器/真实认证/生产/模型/发布/main继续不执行。


## 2026-11-03 UI13：用户、访问策略、钱包、订阅与兑换（源码/完整CI通过）

源码 `bed703aa3dd037ea01fc3057e1f72c04b722b51c` / [完整CI41518426080](https://github.com/ForceMind/MyAPI/actions/runs/41518426080) attempt1 success：10作业成功、1浏览器/真实API混合作业跳过，全部作业/步骤及前后端原始日志已读。用户/访问策略/资金专项29文件297/297、模型164/164、公开33/33、账号85/85、Key388/388、原连接209/209、日志410/410、全量347文件2585/2585通过；同步/构建/typecheck/lint通过（18警告0错误）。根模块/独立relaykit vet/build、原生/386、两模块全量、既有race、三库/Redis/ClickHouse/分发/Desktop门禁通过；386定向18父项/55条/5包全通过、0跳过，原始后端失败/race标记0。浏览器依赖与全部Chromium跳过；本机未安装依赖、构建测试或下载产物，未改生产/发真实模型与资金请求/发布/main。

沿用实际users/assigned-access/wallet/subscriptions/redemptions及其surface线稿，不新建路由。旧源码测试aceab899 / [CI41505306749](https://github.com/ForceMind/MyAPI/actions/runs/41505306749)前端125482970293：29文件215通过/82失败，原始日志已读。37种用户/套餐/支付/转账/兑换写封装的401合成用例均自动刷新后重放并返回成功；403/成功精确方法与载荷、四个只读金额估算认证刷新保留。用户同表单双submit实际2次PUT、只读刷新覆盖姓名草稿、确认配额POST成功后读回GET失败的Unhandled Rejection已有红证据；其它用例包括失败确认缺alert、准确对象/布局标签缺失，逐项以首差异为准。

夹具更正：兑换workspace8条原mock只接受基路径，未接住真实`/api/redemption/?p=...&page_size=...`，其中读取链条没正确渲染，不能计作产品红。只把夹具和精确出站断言按原query-string合同修正，API仍原路径/参数；未通过放宽页码、总数、裁切/可达性或未知值断言造绿。新方案全部候选仍待Actions验证。

候选范围：用户列表共享有界分页/未知total/稳定ID/手机全部原cell；用户、订阅和兑换provider按id/role/SID隔离，关闭按真实动作/ID或列表标题回焦。访问策略保成熟session/revision/409恢复，只收壳/尺寸。用户编辑失败保草稿；已确认配额调整的读回失败只显示Unknown并禁第二笔调整，普通metadata仍按原transform写入、无quota/quota_dollars字段，原数学与request_id复用构造不改。订阅Grant无原请求标识时只做UI未知结果保护：先显式刷新核对记录，再人工新命令，不加operation_id或幂等承诺。

钱包支付/转账/套餐购买保所有资格、金额、汇率、折扣、单位、渠道/handler和载荷；账单完整trade_no、固定分页/动作、读取失败/未知total不补0，独立完成订单确认失败保留。套餐/兑换编辑保原表单算法与生成/导出，确认准确对象；套餐列表的名称/状态/分组是对已返回完整数组的本地筛选，兑换保实际后端名称/状态契约，不伪造未提供的日期过滤。手机复用原列，未知金额/额度不冒充0或Unlimited，显式0保持原语义。

37个本轮相关写封装仅加skipAuthRefresh:true，原key/value、请求标识、方法和body不变；读取和四个POST金额估算保持原认证刷新。共享ConfirmDialog统一title/正文/footer与returnFocus，触控尺寸只作用Button/Select/combobox，保Switch/Checkbox原glyph和点击标签。26新键脚本追加七语言，en/zh、其他五英文回填，旧译文与顺序保留。UI14/15后续WIP不入本轮提交与验证。最终仍须完整CI成功；浏览器、真实认证/资金/供应商、生产与发布均未执行。

后续收口保留：在UI18质量审查统一核对其它既有写封装的认证自动重放覆盖，不把本轮37项范围写成全站所有API的形式化保证；不改变只读POST默认语义。源码全部UI可交付时按既有VERSION体系收口一次，不在中间调试反复改版本或发布。


UI13候选2d1cedb1 / CI41510304200前端125500133503：专项29文件289通过/8失败，原始日志已读。37写封装的401/403/成功与四个只读POST认证刷新全部通过；失败集中在布局/测试上下文。用户手机More details重复摘要中的ID/层级/角色，产品去重且保留旧Standard account单一字段断言。既有owner-session回归仍要求旧整Popup85dvh滚动，按已批手机全高/固定头尾更新为h-dvh/max-h-dvh/overflow-hidden、唯一正文与footer分区，并完整保留Refresh→Tab→Close→Enter仅关闭一次的键盘保护，不删除裁切/可达性断言。

其它为新测试误判：DataTablePage遇errorState本来隐藏整页分页，错误态断言改为无分页/无0 records，保Alert、旧行消失/非成功空及成功未知total的严格pager断言；套餐Status排序与筛选同名，查询限定表格外唯一筛选按钮；编辑器原关闭文案不是Cancel，改用原Escape路径并严格等待关闭/焦点/搜索页码/零写。$12.34在原购买弹层本来分别表示Amount Due与Required余额需求，不能为消除重复查询而删金额；给两行增加具名group，分别精确金额断言，原CTA/资格锁不变，测试显式固定USD/500000配额并恢复。所有修改仍待下一候选完整CI。


39902856 / CI41512130656前端125506961889：29文件295通过/2失败，原始日志已读。旧窄屏访问策略断言确实暴露model-availability原ul残留max-h-64/overflow-y-auto，产品仅删除内层高度/纵滚，唯一正文/全部模型原因/viewport与原键盘关闭断言保持。另新套餐筛选夹具误按menuitemcheckbox查询，实际Command控件公开角色是listbox Suggestions下option Disable；按该真实角色精确查询，不改筛选/排序/值或请求次数。其它UI13安全与金额测试通过，类型/lint/完整回归仍须后续候选。


3554b180 / CI41515115895前端125516933231：专项29文件297/297、模型164/164、公开33/33、账号85/85、Key388/388、原连接209/209、日志410/410及全量347文件2585/2585全部通过，原始日志已读；构建与同步通过。typecheck仅套餐列表未用import失败；lint19警告5错误为两个UI互斥状态的嵌套ternary、hook文件内新增局部组件fast-refresh限制及重复query import。修正只移除未用import、等价if/互斥渲染、column.cell直接稳定引用原DataTableRowActions、合并import，业务/断言/载荷/超时不改；emptyContent确认放在UserSubscriptionsTask同scope，未把错误变量放入StatusBadge。仍须读取本次最小修正的完整CI，不把2585测试绿代替类型/lint或后端验收。


## 2026-11-03 UI10–12：模型工具、部署、Key与连接剩余任务（源码/完整CI通过）

源码 `321f05510d00af45d82da165f4f03475c9bda976` / [完整CI41494046963](https://github.com/ForceMind/MyAPI/actions/runs/41494046963) attempt1 success：10作业成功、1浏览器/真实API混合作业按授权跳过，全部作业/步骤及前后端原始日志已读。模型23文件164/164、公开8文件33/33、账号11文件85/85、Key45文件388/388、原连接19文件209/209、日志47文件410/410、全量334文件2423/2423通过；新增渠道合同在全量中通过，不把原连接专项209误写成新增73项也在专项执行。同步/构建/typecheck/lint通过（17警告0错误）；根模块/独立relaykit vet/build、原生/386、两模块全量、既有race、三库/Redis/ClickHouse/分发/Desktop全部通过。386定向18父项/含子项55条/5包通过、0跳过，原始后端失败/数据竞争标记0。浏览器依赖安装与所有Chromium步骤跳过，未部署、未调用真实模型/供应商，本机无依赖安装/构建测试/产物下载。

沿用既有线稿完成上述实际入口与弹层，保留字段、权限、确认、金额与未知值语义、返回上下文；修复前红、夹具修正与直接凭据绑定证据保留在本节后续记录。321f是当前已验证应用，不把下一批WIP或测试红提交当可部署源码。主Goal仍进行中：UI13用户/钱包/订阅/兑换与UI14个人资料/认证源码候选、UI15设置20分区在后续分批验证；未授权浏览器/生产/真实服务/P2后端改造/发布/main。最后交接文档与下一批测试会读取其自身CI，不能仅引用本次绿灯。


## 2026-11-03 剩余全部 UI 批次：持续实施授权与主清单

维护者已明确授权“直接设置目标完成所有的批次”，覆盖既有逐页线稿对应的剩余 UI 实施、必要回归、路径提交推送 origin/dev 与完整 CI 回读。此授权替代历史“每批先等确认”；正常实现和范围内修正连续推进。新业务规则、浏览器测试、生产、真实供应商、main、发布及部署仍未授权。

成功标准：所有可达页面、页签与业务弹层按现有线稿逐批收口，完整字段、权限、确认、价格计费、未知值语义、返回上下文与不重放保持；验证方法：每批 GitHub Actions 非浏览器专项、构建、类型、lint 和完整既有 CI；浏览器未验证单列，不以静态/合成验证替代视觉验收。

清单口径：只读索引含 326 个视图条目、201 个源码路径、138 个弹层和 54 个内部模式；这些不是 326 个独立页面。66 个路由注册包含布局/别名；设置注册表有 42 个分区。已有 UI1–9 和已验证 5b/5c 保留，不重新搭建。下表是同一主目标的阶段记录，不引入另一套计划。

| 批次 | 页面与组件 / 必须保留的边界 | 状态 |
| --- | --- | --- |
| UI10 | 缺失模型、同步向导、冲突选择；完整值、跨页选择、取消/身份隔离、返回列表 | 源码321f0551 / 完整CI41494046963通过 |
| UI11 | 部署列表、创建、详情、日志、配置、重命名、续期、删除确认；只改 UI 和写入不重放，不改报价算法或准入 | 源码321f0551 / 完整CI41494046963通过 |
| UI12 | 连接与 Key 剩余编辑、高级配置和敏感操作弹层；复用已完成主页面，保留凭据/轮换/路由/预算 | 全部已列子批源码321f0551 / 完整CI41494046963通过 |
| UI13 | 用户、访问策略、订阅、兑换与钱包；金额、订单、退款和独立确认保持 | 源码bed703aa / 完整CI41518426080通过 |
| UI14 | 个人资料、登录/恢复/OAuth；保留二步验证、Passkey、会话和账号删除流程 | 源码a0854607 / 完整CI41523884159通过 |
| UI15 | 设置的站点/认证/内容/安全等分区及编辑模式；共享导航、权限与草稿保护 | 源码fffb2ef0 / 完整CI41539150203通过 |
| UI16 | 设置的计费/模型/运维/集成/维护、配额写入与系统信息；保留价格、Root 权限、未知值及确认 | 源码fffb2ef0 / 完整CI41539150203通过 |
| UI17 | 仪表盘分析、Playground、聊天和提示学习；只做模拟验证，不启动真实生成或学习任务 | 源码fffb2ef0 / 完整CI41539150203通过 |
| UI18 | 首页/关于/法律/榜单/安装向导/错误页及剩余布局和辅助弹层；保留实际路由与 Desktop 上下文 | 源码fffb2ef0 / 完整CI41539150203通过；Desktop原生来源只读保留 |

下一批UI12迭代：Key仅预算/模型范围/维护/轮换的固定头尾与单正文、CC Switch标签/读取错误及高级存储ID的嵌套滚动；渠道先共享任务身份/返回焦点、原编辑抽屉忙状态、Codex OAuth和多Key敏感任务。主线程补映射编辑器可访问标签/手机分区和已有动作标记。已完成卡片/表格/详情、主抽屉导航、Key安全状态机与分额不重做。渠道模型获取、上游更新、Ollama及高级可视/JSON编辑器留作后续子批。成功标准为完整字段/44px/单滚动与权限、金额、确认、未知结果恢复及不重放保持；验证为既有Key/连接全部断言加真实组件外部边界合成回归，只在Actions。下一批源码仅WIP，不计当前模型候选CI的覆盖，也不为它提前取消当前完整CI。

主智能体负责 API、共享组件、集成、翻译、CI 和交接；并行代理分别负责模型同步/缺失与部署 UI，不递归派工。本机仅代码读写和轻量文本/Git 检查，不安装依赖、构建测试、运行浏览器/Docker 或下载产物。部署相关 GET/报价也可能访问供应商，CI 全部使用外部边界模拟，不调用真实服务。

当前红证据：e2521323 前端 81 通过/13 失败。401 后三种模型写入被自动重放；缺失模型两种读取失败显示假空；语言重渲染重置、关闭/身份切换后的迟到预览继续写入或重开已由 Actions 复现。冲突弹层四条失败包含新标签/区域缺失，迟到覆盖用例在旧标题处提前失败，尚不能据此单独声称该运行证明迟到覆盖问题。后续候选必须保留这些断言。模型价格编辑复用的 updateSystemOption 与部署写入也纳入真实 Axios 401/403/成功载荷合成合同，只加本次相关封装的不重放配置，不改全局客户端/价格算法/后端。

部署扩展红基线：3c5b68b8 / CI41480666648 的前端作业125398070196，19文件93通过/30失败，原始日志已读。9种相关写入的401用例均因自动刷新后重放而返回success；403与成功方法/精确body保留。部署图片/名称草稿覆盖由实际值断言复现；删除失败无可读alert、pending确认已关闭、容器/日志/硬件读取失败无alert。未知总数/手机展开/报价用例先停在新增导航或标签缺失，不能单独据此宣称更深数据语义已复现。原测试与时限不放宽。该红基线不是完整CI通过，不作为可部署版本。

候选305bea6c / CI41482442222：模型专项23文件145通过/6失败，原始前端日志已读。产品问题为日志错误分支在logsContent声明前赋值、全选的label和aria-labelledby组合造成重复可访问名称、Missing模型名插值把斜杠显示成字面HTML实体。最小修正分别移动错误分支、独立关联全选label、仅React文本/aria插值局部escapeValue:false；精确名称、全字段选择和原始复制值断言保留。另外两处为新测试夹具误判：scrollIntoView实际定义在HTMLElement.prototype而非Element.prototype；Base UI radio使用span+aria-disabled，toBeDisabled仅识别原生控件。改为实际原型spy和精确aria-disabled=true，并增加点击后仍official、config未选中且零GET/POST；不放宽实际禁用行为或时限。该候选尚非完整CI绿。

f7d1335e / CI41483542685：前端作业125408636030原始日志及失败注解已读。模型151/151、公开33/33、账号85/85、Key377/377、连接209/209、日志410/410和全量323文件2326/2326通过，构建与i18n同步通过；typecheck因getHardwareTypes响应未声明message失败，lint因新增报价显示嵌套三元表达式失败（18警告1错误）。只补可选message类型、等价if显示分支和固定导航ID的querySelector，不改报价函数、准入或测试断言。尚未完整CI通过，不作为可部署应用。

为连续实施且避免空等，UI10/11与已备齐UI12作为本轮联合可交付候选收口；不提前宣称其中任何组完成。先仅提交下一批测试文件，CI使用旧渠道/Key产品取得修复前证据，随后再提交各自WIP及精确接口保护，最后回读联合候选完整CI。分批主清单保持，模型和下一批WIP的证据范围不混写。



UI12旧产品测试提交6df65b63 / CI41485859365，前端作业125416763239已读原始日志：模型151/151、公开33/33、账号85/85通过；Key原377通过、新11失败（45文件377通过/11失败）。读取失败无alert、任务region/尺寸和关联label缺失已复现；三应用精确URL用例在旧label处停止，不能写成旧URL参数错误已被证明。渠道专项/36项写入401/403/成功载荷加只读认证刷新用例因前置Key失败尚未执行，渠道异步与写入问题目前仅从代码看，不声称已取运行红证据。联合候选保留所有新增与原安全断言：Key六个组件与渠道敏感任务按上述范围收口，主线程仅给create/update、OAuth start/complete和多Key写动作局部不重放；get_key_status保持原读取认证刷新、全局客户端不改。模型映射只改每行关联label/手机归组/44px与图标，原转换/去重/错误函数不改。七新键en/zh、其他五英文回填，旧译文/顺序保留；四个只读任务正文补键盘焦点入口。当前联合候选尚未完整CI通过。


4fc48763 / CI41486931953：前端125420324611原始日志已读，模型151/151、公开33/33、账号85/85通过，Key383通过/5失败，均为CC Switch实际可访问名称Primary Model*与精确要求Primary Model *不一致；只把必填星号前的空格置于Label直接文本中，不改精确断言或任何URL参数。此前6个任务区/读取失败等新用例已通过，渠道尚被前置Key失败阻断。

ddffc5c0 / CI41489254195：前端125428308027原始日志已读，模型151通过/新增3失败；三个真实Axios用例在同一同步事件内发起写入后切换用户、角色、SID，原凭据绑定断言失败（日志凭据自动打码，不输出头值）。从代码看，原认证头拦截器异步读取当前store；必要最小依赖修正仅把该既有拦截器设为synchronous:true，使派发时捕获凭据，不改请求头内容、全局重试策略、API正文、后端或生产。该新证据使本轮必须触及前端http-client，覆盖前面阶段“全局客户端不改”的计划边界；它不是更换认证架构。两个同步弹层另加直接读取当前id/role/SID/Admin的派发与返回守卫，保留generation/abort并补10个同事件边界用例；不声称能撤销已发写入。

UI12后续源码已备齐：获取模型保自动获取、alias分类和三类列表；上游更新保add/remove和部分提交二次确认；余额保显式0、拒绝null伪0和旧对象迟到；Ollama保live/fallback、原pull事件及Abort取消、独立删除确认。高级路由保四模式、原模板替换确认与全部路由字段；参数覆盖保完整规则/条件、Visual/JSON和legacy字段、原Fill Template直接替换行为，外层正文单滚动，Yace/Textarea自身编辑滚动保留。新8+8个真实组件合成用例待Actions，浏览器未验证。Ollama删除及选定上游更新写入仅补局部skipAuthRefresh并扩展真实Axios合同，读操作配置保持；不改全局重试。新增10键按脚本追加七语言，en/zh、其余五英文回填。

审查范围说明：从代码看，Advanced原normalizeAdvancedCustomConfig本来只保留advanced_routes、会丢顶层未知扩展；本轮不改lib或协议，不声称已修此既有业务问题。UI13用户/访问策略与钱包/订阅/兑换已按全批授权并行进入下一轮WIP，保留金额、权限、订单/确认和未知恢复语义；它们不在当前UI12候选CI验证范围。最终只有联合候选完整CI成功才计本轮完成。


04e5c7e6 / CI41490720666：前端125433411816全部步骤/原始日志已读，模型164/164、公开33/33、账号85/85、Key388/388、旧连接专项209/209、日志410/410通过；全量334文件2420通过/3失败，唯一失败文件为新映射测试：带list属性的原生输入实际角色combobox，误按textbox查询。只改测试角色，精确字段名、完整值、duplicate-source错误和禁用断言保持，原datalist不删除。typecheck仅Ollama early-return后冗余abort被缩窄为never；移除不可达冗余调用，其余Close/cleanup取消全部保留。lint17警告7错误，修正渠道任务对象useMemo稳定及三个callback依赖、余额显示等价if分支；不改载荷/计算/重试/断言时限。该运行未完整绿，后端被后续必要修正替代时如实保留取消事实。



## 2026-11-02 UI第九批：供应商创建与预填分组维护（源码/完整CI通过）

源码 `2dfa732d1e945d71769838e107e808d8fda00bb1` / [完整CI41452879262](https://github.com/ForceMind/MyAPI/actions/runs/41452879262) attempt1 success：10作业成功、1浏览器混合作业跳过，全部作业/步骤及前后端原始日志已读。13文件75/75维护工具、8文件33/33公开定价、11文件85/85账号、43文件377/377 Key、19文件209/209连接、47文件410/410日志、313文件2250/2250前端全量及同步/构建/typecheck/lint通过（17警告、0错误）；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有race、三库/Redis/ClickHouse/分发/Desktop合同通过。386定向18父项、含子项55条Test记录、5包全通过且0跳过，后端失败/数据竞争标记0。RUN_BROWSER=0，浏览器安装与全部Chromium步骤跳过。收尾仅4份交接，应用源码仍绑定2dfa732d；接手读取最新文档提交自身完整CI，不把文档SHA当镜像源码。

- 供应商仅已有创建入口；菜单改为准确的Add vendor，不新增管理列表、编辑或删除入口。保留name/icon/description及隐藏status:1、原未可达编辑能力和两类缓存失效；短表单固定标题/动作、单正文滚动、可读错误、44px和提交忙状态。保留精确body与失败草稿。
- 预填保留协调器；桌面/手机共用摘要与操作，所有items通过明确展开可达，重复值仍显示且稳定key。保留名称/ID/说明/type/items、搜索、类型、本地页码、独立删除确认；保存/取消回原筛选页及编辑焦点，关闭整流程回More actions。不新增user groups/应用范围，不改既有渠道或承诺其已打开选择器自动同步。
- 编辑仅model/tag/endpoint三类，各类型本次临时items草稿独立，切换保存/恢复、不做自动互转。model/tag仍提交数组，endpoint仍提交原JSON字符串，未知扩展false/0不丢；取消不写，失败保留，处理中有同步admission锁与禁用关闭/字段。
- 四个实际写封装仅加skipAuthRefresh:true：createVendor、createPrefillGroup、updatePrefillGroup、deletePrefillGroup。全局HTTP客户端/旧模型价格提交和同步封装不改；401/403拒绝后不自动重放，成功方法/body不变。写入结果未知不自动补发或声称撤销已发请求。
- 共享入口使用既有ModelsProvider身份隔离和tools动作标记，菜单子项消失后返回真实More actions触发器；身份切换清旧草稿，旧写入迟到结果不关闭新身份任务。原模型/价格编辑器、同步/冲突/缺失流程、Go/三库结构与转发均未改。
- 27个新键脚本追加七语言，en/zh译文、其他五种英文回填，旧译文/顺序不改；临时脚本已清理。现有整个models专项与其他完整门禁保留。

红绿/修正记录：
- 24649084 / CI41447512342：52通过7失败，四个真实Axios双实例/认证拦截器合成用例401后返回重放成功；供应商连续form submit发2次POST；model→endpoint返回丢条目、model→tag混用条目。403/成功载荷和其它已有契约通过。不是夹具导入错误。此取证先于修复，后被候选替代，不计完整绿。
- 3e684a64 / CI41449536486：74通过1失败，剩余预填Saving...精确按钮名称找不到；原Spinner默认Loading标签参与按钮名称。328b64ee仅加aria-hidden，不改原断言和时限。
- 328b64ee / CI41450636734元数据专项步骤通过，后因静态复核新增供应商测试Array.from(NodeList)不符已知prefer-spread规则，以1cd32810作等价集合写法修正替代；不能当完整CI绿。1cd32810 / CI41451396901专项75/75、全量313文件2250/2250、构建同步通过，但新增测试使用ByRoleOptions不支持的exact导致类型失败，夹具button缺type导致lint失败（17警告、1错误）。2dfa732d仅将8处查询改为/^Create$/并补type=button，不改行为/断言含义/载荷/时限，2dfa732d前端作业success，全部步骤和原始日志已读：13文件75/75维护工具、8文件33/33公开定价、11文件85/85账号、43文件377/377 Key、19文件209/209连接、47文件410/410日志、313文件2250/2250全量，同步/构建/typecheck/lint通过（17警告、0错误）。RUN_BROWSER=0，浏览器安装及全部Chromium跳过；后端与全部作业已完整success，最终只以本节2dfa732d与完整CI41452879262为准；前述被替代的运行保留红/取消事实，不计完整绿。



## 2026-11-02 UI第八批：后台模型元数据列表、编辑抽屉与描述（源码/完整CI通过）

源码 `6773332e6964a62d66961fe45eb0d7ef0514923d` / [完整CI41413213608](https://github.com/ForceMind/MyAPI/actions/runs/41413213608) attempt1 success：10作业成功、1浏览器混合作业跳过，全部作业/步骤及前后端原始日志已读。7文件38/38后台模型、8文件33/33公开定价、11文件85/85账号、43文件377/377 Key、19文件209/209连接、47文件410/410日志、307文件2213/2213前端全量，以及同步/构建/typecheck/lint通过（17警告、0错误）；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有race、三库/Redis/ClickHouse/分发/Desktop合同通过。386定向18父项、含子项55条Test记录、5包全通过且0跳过，后端失败/数据竞争标记0。RUN_BROWSER=0，浏览器安装与全部Chromium步骤跳过。应用源码仍绑定6773332e；收尾仅4份交接，接手读取最新文档提交自身完整CI，不把文档SHA当镜像源码。

- 元数据列表保留原搜索/供应商/状态/同步筛选与全部列，复用首尾/数字/跳页/条数分页；规范出站页码/条数，未知/失败总数不补0、不据此纠正页码，实际0计数保留。按后台模型ID选择，手机完整字段展开复用桌面单元格，长名/说明可读；匹配结果和手机全部标签/端点/绑定连接/分组/计费类型不依赖hover。手机批量入口、原删除确认与准确对象均保留。
- 列渲染稳定，当前账号/角色/SID变化重建ModelsProvider并隔离列表读缓存，清除旧对象/草稿/选择。编辑/描述/新增记录开启动作，关闭优先原节点，刷新导致节点替换时按稳定ID找现动作，保留URL筛选/分页及展开状态。
- 新增/编辑沿用原表单与基本信息/匹配/端点/价格/状态同步分区，补固定分区导航、单滚动区、可读保存对象及44px规则；Root价格门禁不改。描述弹层改为固定标题/关闭区、中间单滚动全文，支持完整换行及返回焦点。
- readPricingConfig与onSubmit函数逐字核对未变；不改价格换算、字段优先级、写入顺序、确认、重试或其他高级工作流。只在已打开任务中保护首次元数据初始化，新的模型/关闭再开仍正常初始化；首次定价读取及现有草稿恢复逻辑保留。部署布局与内部行为不改，供应商/缺失模型/同步/冲突/预填内部流程未改。
- 新增3个t英文键，脚本追加七语言，en/zh译文、其余五种英文回填，旧译文/顺序保留，临时脚本已移除。CI新增整个features/models专项，原公开定价/账号/Key/连接/日志及全量门禁保留。

红绿与修正记录：
- 7f680731 / CI41409883557：29通过1失败，手机没有More details完整字段入口，已读红证据。
- 8c20f8b7 / CI41410492089：30通过1失败，仍是手机入口；早期草稿用例只等待查询缓存更新，未等待UI收到响应，不能作为草稿不丢的证据。
- e5c6c12e / CI41412427760：34通过2失败。加入可见服务端对象名并等待其更新后，草稿用例确证unsaved-model-name被fresh-saved-model-name覆盖；另一个用例误要求默认page=1必须显式写入URL，原hook会省略默认值。
- 6773332e保留原价格/提交函数，仅加同一打开任务的首次元数据初始化保护；分页回归改为严格断言第1页UI与真实请求p=1/page_size=50，不改旧URL默认值语义，并补原删除取消不写入、Admin完整字段/显式false/未知JSON扩展值单次提交且不写价格的契约。上述被新提交替代的运行不计完整全绿；6773332e前端作业success，全部步骤/原始日志已读：7文件38/38后台模型、8文件33/33公开定价、11文件85/85账号、43文件377/377 Key、19文件209/209连接、47文件410/410日志、307文件2213/2213前端全量及同步/构建/typecheck/lint通过（17警告、0错误）。RUN_BROWSER=0，浏览器安装/全部Chromium跳过；后端与全部既有门禁已完整成功，最终以本节源码/CI为准。前述被替代的取证运行不计完整全绿。



## 2026-11-02 UI第七批：模型目录与模型详情（源码/完整CI通过）

源码 `cc78b6a2f03ec7f11e99b371075fd24332a2abc3` / [完整CI41398009702](https://github.com/ForceMind/MyAPI/actions/runs/41398009702) attempt1 success：10作业成功、1浏览器混合作业跳过；已读取全部作业/步骤及前后端原始日志。8文件33/33模型专项、11文件85/85账号窗口、43文件377/377 Key、19文件209/209连接、47文件410/410日志、305文件2204/2204前端全量及同步/构建/typecheck/lint通过（17警告、0错误）；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有竞态、三库/Redis/ClickHouse/分发/Desktop合同通过。386定向18父项、含子项55条Test记录、5包全通过，0跳过；原始后端失败/数据竞争标记为0。RUN_BROWSER=0，浏览器安装和全部Chromium步骤跳过。收尾仅4份交接，应用源码仍绑定cc78b6a2；接手读取最新文档提交自身完整CI，不把文档SHA当镜像源码。

维护者批准沿用pricing/model-price及overview/performance/api各自桌面与手机线稿，不重画SVG，不重搭已完成页面。

- 目录共用搜索、供应商、分组、端点、标签和计费类型筛选；桌面与手机仍复用同一Sidebar。价格模式、Token单位、排序和视图集中，补可访问名称及44px触控目标规则。统计仅覆盖本次返回数据；读取失败显示错误/重试，不把失败或加载中当0，确认空目录与筛选无结果分开。
- 卡片与表格在目录层共用page/pageSize及既有首尾/数字/跳页/20-50-100分页；筛选或改条数回第1页，视图/单位/价格模式切换保留页码，超界合法回落。两路由共用搜索schema，原字段及beforeLoad门禁逐字保留；只新增有界页码/条数和模型名返回状态，不接受凭据或任意返回URL。
- 卡片/表格/详情共用完整身份组件，卡片/表格共用价格展示；price/dynamic-price/billing-expr/model-helpers/filters及后端未改。保留原输入/输出/缓存/按次/阶梯与特殊表达式、全部详情价格字段和精度。金样组件用例保护分组倍率、显式0分组/缓存、缺失缓存、K/M和充值换算，不把缺失定价补零。
- 手机表格复用全部原列，仅挂载当前布局；分组/端点/标签通过展开完整可达，模型名换行，复制名称与详情独立。列定义保持稳定，开关详情不替换当前动作节点；数据刷新后按同一模型寻找当前按钮。原性能/API组件与全部字段保留，API示例只复制、不发送请求。
- 抽屉与独立页继续共用ModelDetailsContent及概览/性能/API三页签。独立页入口/返回携带筛选、排序、价格模式/单位、视图、页码/条数与焦点；只读刷新保持页签和当前任务焦点，实际跳页才记录history返回。由后端DTO可知model.Pricing不含数字id，前端id保留兼容但改为可选；真实同形无ID响应按model_name恢复。DOM标记单独编码，含引号/换行/孤立代理项的名称不直接插入CSS选择器；原模型名、URL值和复制内容不改，旧共享焦点hook不扩改。
- 新增10键经临时add-missing-keys脚本追加七语言，en/zh译文、其余五种英文回填；旧译文/顺序不变，临时脚本已移除。CI新增整个pricing模块专项，原账号/Key/连接/日志/全量门禁均保留。全部构建/同步/typecheck/lint/测试只在Actions；本机无依赖安装、构建测试或构件下载。浏览器视觉、尺寸、滚动和线上未验证；不改生产，不发真实模型请求，不自动打包/发布/部署或合main。

红绿与取消记录（均不冒充最终完整通过）：

- 35f09745 / CI41387241530：11通过1失败，旧目录读取失败没有alert；已取修复前证据。后续产品/夹具修正保持读取失败不补零。
- ed388323 / CI41388129988：22通过6失败，漏改多行Check引用；e3016036补回。e3016036 / CI41388278840：27通过1失败，入场动画透明阶段过早断言；d3a2b0fc保留toBeVisible并等待明确可见，未改时限。
- d3a2b0fc / CI41388481979：28通过1失败，手机关闭详情后焦点落body；da5b5b14稳定列及活节点恢复。da5b5b14 / CI41388772456模型29通过，后因刷新保护提交替代cancelled。
- 7d69011d / CI41388992576：模型30/30、全量2201/2201及同步/构建通过，但类型有币种夹具层级/供应商图标导入错误、lint有非空断言。35d5c22f / CI41390232436补原导入和显式配置，Typecheck/Lint通过，补带图标的供应商夹具；其全量未结束即被真实身份契约用例替代。
- e311aed0 / CI41390891453：无数字ID回归30通过1失败，关闭后焦点落目录标题；c81b1bda改用模型名。c81b1bda / CI41391097822：模型31/31、全量2202/2202及构建/类型通过，lint仅catalog-focus.ts:3的Array.from违反规则；5fd50b95按原规则改展开语法。
- 5fd50b95 / CI41392380523：前端31/31及2202/2202、同步/构建/类型/lint通过，后端未结束即被可达性边界用例替代cancelled。1739e85d / CI41394148902：31通过1失败，含引号/换行的公开focusModel触发Invalid selector使目录不可达；9c3c9058仅编码DOM标记，保留可达性与焦点断言。
- 9c3c9058 / CI41394448883：attempt1后端和其他8作业成功，前端综合分页用例5000ms超时；全轮运行时GitHub拒绝提前重跑，待结束后仅重跑前端attempt2仍相同超时，不能计全绿。cc78b6a2只将“视图保持页码”和“条数改变重置页码”拆成两条独立测试；45条数据、原所有断言和5000ms均保留，并补反向视图保持。应用代码与9c3c9058一致，最终33/33和完整CI通过。

前述被后续提交替代的运行均保留其失败/取消事实。最终源码资格只看本节cc78b6a2与完整CI41398009702；交接文档提交另读自身CI。



## 2026-11-02 UI第六批：账号窗口总览与分额展示（源码/完整CI通过）

源码 `04c17b5830076c58dddd6f2998f4d37a647173cf` / [完整CI41338563368](https://github.com/ForceMind/MyAPI/actions/runs/41338563368) attempt1 success：10作业成功、1浏览器混合作业跳过；已读取全部作业/步骤和前后端原始日志。11文件85/85账号窗口/分额专项、43文件377/377 Key专项、19文件209/209连接专项、47文件410/410日志专项、301文件2182/2182前端全量及同步/构建/typecheck/lint通过（16警告、0错误）；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有race/三库/Redis/分发/Desktop通过，386定向18项/5包全通过。RUN_BROWSER=0，浏览器安装与全部Chromium步骤跳过。应用源码仍绑定04c17b58；收尾仅4份交接，接手读取最新文档提交自身完整CI，不把文档SHA当镜像源码。维护者已批准沿用upstream-accounts/subscription-shares桌面与手机线稿，保留既有连接/Key页面、全部字段/权限/确认与写入状态机；不新增独立分额路由，不重画线稿。

- 总览补账号/连接搜索、记录的窗口周期和快照状态筛选；统计仅指本次返回的codex-percent观察（已返回连接数、近期有效快照数、当前状态未确认的观察数），不冒充全局账号/连接可用性或供应商命中。数据完整性缺失/不完整均提示范围，失败不显示旧数据或零计数。余额/美元不转原生窗口百分比。
- 统一每次渲染的时间，30秒只刷新本地时间判定、60秒只重读既有已存观察；新返回记录不因请求开始时的旧时钟误判未来时间。当前账号重复点击保留Key，首次明确点击锚定账号；同连接不同身份/series分别展示，账号选择器采用返回顺序中的最新身份标签，不合并序列。
- 总览→连接→Key/调用记录的嵌套upstreamReturn只保留ID/筛选/展开状态，不带凭据或任意URL；history优先原开启元素，直接/失败返回携带账号焦点，刷新不抢输入。筛选不发额外窗口读取；窗口变化比较仍按明确展开后读取。账号/Key状态跨返回保留，身份变更重挂载隔离；原fresh-self和Root/连接读取门禁不放宽。
- WindowCard复用既有连接窗口WHAM/relay/historical描述，用全部返回观察作判定上下文，过滤不改变错误历史性质；来源/时间/计划/原因归组，不编造DTO未提供的具体原始错误。Root当前Key分额从实际pool.allocations取得，估算/初始基线/未归属增长/请求证据分区，失败清除旧allocation/chart；PoolUsage没有来源字段，不借用旧share快照来源。
- 分额弹层固定标题/动作区，中间单个滚动区；按Key身份、始终可读的近似误差说明、窗口选择/当前证据、草稿确认、独立关闭区排列。单选精确可访问名称与补充描述分开，候选/池/快照ID和关闭历史完整可达；其他窗口折叠但不删除，输入/确认/展开节点不因只读刷新重置。原百分比解析精度、mutation、基线/池生命周期、并发/幂等/不重放与未知结果恢复逻辑不改。
- 新增23键经临时add-missing-keys脚本追加7语言，en/zh译文、其余5英文回填，保留旧字典顺序和译文。新增账号窗口/分额专项，原Key/连接/日志专项和全量均保留；所有构建/同步/typecheck/lint/测试仅Actions，本机无依赖/构建测试/构件下载。浏览器未验证，不改后端/计费/数据库/生产，不发真实模型请求，不自动打包、发布、部署或合main。

追加身份边界取证：a0c0ae1e / CI41337114734的账号/分额84/84、Key377/377、连接209/209、日志410/410、全量2181/2181、同步/构建/typecheck/lint均通过（16警告、0错误），但完整CI未跑完，被后续权限回归替代取消，不计全绿。3e667c10 / CI41338332630新增同会话Root降级合成用例，84通过、1失败：受控选中Key复用了旧Root查询缓存，降权后出现不应复用的Total allocated。04c17b58只为Key列表与保存分额两个只读queryKey加角色维度，并补明确同SID登录夹具；不改API、权限策略或写入状态机。该负向断言与原全部断言保留，已在04c17b58自身完整CI通过；浏览器仍未验证。

原始证据保留：fc6c2673 / CI41334359561专项80通过、3失败，关联Label使单选可访问名称包含描述；6a36f450 / CI41334840107账号专项83/83通过，Key旧回归375通过、2失败（重复候选名称定位歧义、关键320px长译文误差说明初始不可读）。修正产品名称/描述关联并恢复无条件说明，候选定位改为精确可见radio，安全/金额/可见性断言不降；5fdf661c / CI41335331215随后因我移动说明留下重复JSX闭合片段，4个解析错误、79观察用例通过，未称通过；be56558b / CI41335907911清理结构并保留各身份标签，专项84/84、Key377/377、连接209/209、日志410/410、全量2181/2181和构建/同步通过，但fixture路由父类型不匹配及effect props依赖各1错。a0c0ae1e仅补fixture守卫类型转接与窄化回调依赖，保留真实权限调用和全部断言。上述被替代运行终态cancelled，不能计全绿；本节候选以自身完整CI为准。

## 2026-11-01 UI第五批：API Key列表与详情（源码/完整CI通过）

**2026-11-02 追加交互回归已验证**：0060631e / [红证据CI41322661402](https://github.com/ForceMind/MyAPI/actions/runs/41322661402)的合成Key专项372通过、1失败，明确展开凭据后Full API Key内容即消失；新增同一用例还覆盖30秒列表时间刷新后保持展开且不重复读Key。334087de仅将凭据字段改为稳定React框架渲染组件，未改读取协议/状态机/权限；[完整CI41323019462](https://github.com/ForceMind/MyAPI/actions/runs/41323019462) attempt1 success，Key专项373/373、全量2158/2158通过，全部作业/步骤和前后端日志已读。前一07331605 / [CI41318459312](https://github.com/ForceMind/MyAPI/actions/runs/41318459312)全绿仍为事实，但未覆盖该新增状态保持用例，不再作为最终应用基线；8ea2b7a5交接CI41321275381与红证据运行被追加修正替代后终态cancelled，不计全绿。最终应用绑定334087de，收尾仅4份交接，仍读取其自身完整CI；浏览器与线上未验证。

源码 `334087de491eed0ef725f2c8c549487f0ab5fa6e` / [完整CI41323019462](https://github.com/ForceMind/MyAPI/actions/runs/41323019462) attempt1 success：10作业成功、1浏览器混合作业跳过。已读取全部作业/步骤及前后端原始日志：42文件373/373 Key专项、19文件209/209连接专项、47文件410/410日志专项、298文件2158/2158前端全量、同步/构建/typecheck/lint通过（16警告、0错误）；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有race/三库/Redis/分发/Desktop通过，386定向18项/5包全通过。RUN_BROWSER=0，浏览器安装和全部Chromium步骤跳过。收尾仅4份交接，应用源码仍绑定334087de；接手读取最新文档提交自身完整CI，不把文档SHA当镜像源码。维护者已批准沿用keys/key-requests/key-scope/key-delivery桌面与手机线稿，不重建既有ObjectPage、三个页签或编辑/敏感维护状态机。

- 列表接入已有首尾/数字/跳页、20/50/100条分页，保留旧URL与凭据搜索；出站最多100，未知/非法总数不补0，已知空列表纠正到第1页。状态筛选沿用当前页过滤，明示口径，不增加伪全库模型/分组筛选或后端契约。
- 手机卡复用桌面单元格，保留掩码、模型/IP限制、额度、日期、分组、行选择和全部高级操作，低频字段可展开。按稳定Key ID选择，手机批量入口可达；仅ID选择状态按历史entry保存在最多20项内存中，身份/角色/SID改变清除，不写URL/storage或缓存完整凭据。保持个人模式金额隐藏。
- 返回上下文不带凭据搜索；原history开启元素优先，直接返回时桌面请求按钮/手机Key名称语义保留，一次性恢复焦点，刷新不抢输入焦点。列表与筛选/分页自然滚动，不因新增控件挤掉内容。
- 详情日期集中页头，掩码集中交付；访问范围与预算/分额桌面分列、手机堆叠。个人模式预算策略入口仍保留，仅隐藏原财务展示；分额成功empty才None，失败/无baseline保持Unknown，不把额度、预算和分额混为一个值。
- 统一编辑、启停、复制和低频菜单，详情去掉重复Copy入口；删除/轮换/凭据维护独立确认，原敏感写入状态机/会话核验/不重放规则不变。菜单打开不预取完整Key，显式复制/连接信息/CC操作才读取；同步旧测试触发步骤，保留原401/无refresh/无重放与焦点断言。
- 新增4键经add-missing-keys脚本追加7语言，en/zh译文、其他5英文回填，未覆盖旧译文或排序。新增Key专项包含整个features/keys；全部构建/同步/typecheck/lint/专项/全量仅Actions，本机无依赖/构建测试/构件下载。浏览器未验证，不改后端/计费/数据库/生产，不发真实模型请求，不自动打包/发布/部署或合main。

首轮4e7a9b29 / CI41317439424的Key专项367通过、5失败，集中在旧api-keys-quota-display夹具没有初始化登录身份，列表查询被新门禁禁用；整轮被535cce78补修替代，终态cancelled，不计通过。补修只初始化/恢复测试身份，同步共享单元格的标签形式，保留全部精确金额、安全边界、Block all models与无派生进度断言；生产逻辑没有为测试绕过身份门禁。i18n同步首轮已通过，构建/typecheck/lint/全量在该轮被专项失败跳过，不能标已验证。

补修535cce78 / CI41317717418的Key专项372/372、连接209/209、日志410/410、前端全量2157/2157及同步/构建/typecheck通过，但lint因selectedRows空对象分支引用每次重建报1错（16警告）。07331605仅固定空引用，不改业务断言；原运行被替代后终态cancelled，不能记全绿。最终结果以本节候选自己的完整CI为准。

审查已在源码修正：页内索引选择错绑、手机批量操作缺入口、已知0总数页码不纠偏、返回后选择丢失、选择触控区不足、菜单预取凭据，以及刷新重挂载抢焦点。本节所列CI已实际验证，浏览器/视觉仍未验证；不把静态检查或子智能体汇报单独当成验收通过。

## 2026-10-31 UI第四批：连接卡片、渠道表格与连接详情（源码/完整CI通过）

源码 `d8dbc22e0dfc8d45a08556261d01b92f1ecc01d7` / [完整CI41183238339](https://github.com/ForceMind/MyAPI/actions/runs/41183238339) attempt1 success：10成功、1浏览器混合作业跳过；已读取全部作业/步骤及前后端原始日志。19文件209/209连接专项、47文件408/408日志专项、296文件2126/2126前端全量、同步/构建/typecheck/lint通过（16项既有警告、0错误）；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有竞态/三库/Redis/分发/Desktop通过，386定向18项/5包全通过，无失败或跳过。RUN_BROWSER=0，浏览器安装与全部Chromium步骤跳过。收尾仅4份交接，应用源码仍绑定d8dbc22e；接手读取最新文档提交的完整CI，不能拿文档SHA冒充镜像源码。维护者已明确批准本批，沿用现有 connections/channels/connection-windows/routing/requests/history 桌面与手机线稿；不重做已有编辑器、全局额度/事件/路由预览及此前日志页面。

- 卡片集中身份、提供方、状态、完整模型/分组、优先级/权重、保存窗口及测试结果；常用 View/Test/Edit 可达，编辑复用原 drawer，新建保留显式 Save and test。没有投影的自动禁用历史原因不虚构。
- 公共名称/模型/状态/类型/分组/page/pageSize 在卡片与表格间保留；兼容旧状态数组和合法旧条数，后端仍只支持单值类型/分组，控件单选，旧多值URL保留并说明首值规则，all与原表格一致。表格保留全部高级列、标签聚合、批量动作、排序与敏感内容开关；标签数不拿平铺连接数校验。
- 两种视图复用分页及视图切换；首尾/数字/直接跳页及20/50/100条，未知/失败不伪造零总数。详情链接记录来源与完整筛选，含表格quotaChannelId；返回焦点复用一次性、用户操作可取消的钩子，刷新不反复抢焦点。
- 表格快测与高级测试分开，操作区隔离行点击，读/写/操作权限按原契约保留，新增操作禁用及处理器守卫；换账号不保留旧库存占位数据。连接页既有fresh-self、身份/权限分区不放宽。
- 详情保留四页签和全部字段；顶部只保留一份最后测试（有窗口但未测试也可见），窗口额度与采样证据分列，来源/观察时间/具体未知或历史错误原因集中。历史保留账号/来源及relay事件区别；不修改response_time=0原语义。
- 新增8键en/zh、其他5语言英文回填，经临时add-missing-keys脚本追加且不改旧译文/顺序。全部构建/同步/typecheck/lint/专项/前端全量及既有CI只在Actions；浏览器未验证，没有本机安装依赖/构建测试/构件下载，不改后端/计费/生产，不发真实模型请求、不重放写入。

审查发现的标签计数、反复焦点、禁用动作区误导航、View返回焦点及all语义问题已作源码修正并补对应回归。676d666a首轮专项206/206及翻译同步、日志专项408/408通过，但lint报数组下标key 1错，运行41182857018后被补修替代终态cancelled，不计全绿；a8e9f0e7/41183073086被同批测试定位修正替代，终态cancelled。6dd79670/41183099499连接专项209/209通过后被窗口key修正替代，终态cancelled。d8dbc22e按已存窗口身份生成key，不放宽lint规则或业务断言。最终以本节完整CI为准，不把已取消运行写成通过。

## 2026-10-30 UI第三批：请求详情与已存SSE检视（源码/完整CI通过）

**最终字段补修已通过完整CI**：源码 782c0124b6171d001b3cee682830a04a77c8326e / [CI41077332468](https://github.com/ForceMind/MyAPI/actions/runs/41077332468) attempt1 success，10成功/1浏览器跳过；全部作业/步骤和前端日志已读，47文件408/408专项、295文件2103/2103全量、同步/构建/typecheck/lint及全部既有后端/三库/Redis/分发/Desktop回归通过，RUN_BROWSER=0。此前41f1c9b6主体与40d7357e交接也已通过；最终核对发现仅UI预览受限时totalBytes原提示少显，782c0124补回该字段并新增确定性2MiB用例，保持1MiB预览、完整复制和服务端截断语义，不放宽断言。最终收尾只改4份交接，应用源码仍绑定782c0124，接手读取最新文档提交CI；浏览器与线上展示仍未验证。

源码 41f1c9b67dd5ed72d791611798c530d5238bf8c0 / [完整CI41068500563](https://github.com/ForceMind/MyAPI/actions/runs/41068500563) attempt1 success，10作业成功、1浏览器作业跳过；全部作业/步骤与前端原始日志已读。专项47文件407/407、前端全量295文件2102/2102，i18n同步/构建/typecheck/lint通过（0错误，16警告，含本批测试字符串拼接提示；未改规则）；根模块/独立relaykit vet/build、原生/386、两模块全量与全部既有竞态、三库/Redis/分发/Desktop通过。RUN_BROWSER=0，浏览器依赖安装及全部Chromium步骤跳过。静态审查补修的role缓存、Key面包屑隔离、返回焦点、缺失身份ID和UTF-8预览均有相应合成回归通过；关键断言保留。收尾只改4份既有交接，应用源码仍绑定41f1c9b6，接手读取最新文档提交CI。

维护者批准沿用request-detail/body/sse/raw与Key对应线稿，两个入口继续共用RequestPage，不重搭ObjectPage。基线784a3dcb；本批仅相关前端、回归、37个新增i18n键、必要CI入口及交接；不改后端/计费/生产，不发真实模型请求，不跑浏览器。本机只改源文件和轻量文本/Git检查，无依赖/构建测试/构件下载。

- 身份、四项结果集中顶部；诊断/正文/SSE/原始证据四页签，默认仍正文，旧diagnosis链接映射detail，旧body/sse/raw地址保留；替换页签保留inspectorBack历史状态。顶部保留Key跳转，缺失身份ID明确Not recorded，不伪造#0。
- 诊断仅presentation=request分组为身份协议、选路派发、用量结算及可展开补充证据；Legacy充值/退款/审计保持原分支、字段、条件和顺序。失败原因默认可达，复核active/权限/显式确认及提交条件不改。原详情文案清单静态对照未丢键，但必须由CI验证实际字段和行为。
- 抽共享ContentPanel/纯保存内容renderer，旧弹窗保留入口和Request/Response页签，消除重复解析/模式开关。正文可读/保存原文、头/query/URL可展开；Raw区将账户可读调用记录与已存上游内容分开，不补造缺失正文。旧ContentPanel导入路径兼容。
- SSE仅解析已保存预览；序号/类型/数据/标记来源、本地全预览搜索、50条逐批展开，501以后仍可搜索。UTF-8预览<=1MiB、不拆中文/emoji，复制保留全部已加载原文；预览限制、服务器返回截断、未结束帧、BOM/MIME/编码、缺事件时间分别呈现，不把结束标记当业务成功/结算。
- Key名字/请求与内容按user/session/role隔离：页面重挂载清除旧局部状态，lookup补原始role维度防止100到10复用缓存；全内容hook原管理员与active门禁保留。日志列表只补已有return-focus钩子接线，不重新设计列表；两个入口经过页签切换的原筛选、页码、焦点回归保留并加强。
- 新旧语义回归均只在Actions运行，专项包含usage-logs/full-content-logs及Key页面/最近请求的原工作流，之后前端全量/typecheck/lint/build与完整既有CI；浏览器保持未授权。本批源码及所列CI检查已通过，但浏览器与线上展示未验证。

只读待办（不自动实施）：从代码看，TokenBudgetEvidence的Reserved USD/actual_fee_usd与订阅金额部分沿用原条件，未统一受showCosts控制；需单独确认诊断金额的展示口径，本批未改其财务含义或验证线上影响。新SSE MIME已按essence精确匹配，旧lib/format.ts部分includes判断按原语义保留，后续若统一需单独审查回归。

## 2026-10-30 UI第二批：调用、绘图与后台任务列表（源码/完整CI通过）

最终源码 b9a5d85519a1d8be2164d78cfe31c109fc9f4e56 / [完整CI41041830295](https://github.com/ForceMind/MyAPI/actions/runs/41041830295) attempt1 success：10作业成功、1浏览器作业跳过。已读取全部作业/步骤和前端原始日志：37文件315/315专项、287文件2046/2046全量，同步/构建/typecheck/lint通过（8项既有警告、0错误）；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有竞态/三库/Redis/分发/Desktop通过。RUN_BROWSER=0，浏览器安装及所有Chromium步骤跳过。前两轮被补修替代后终态cancelled，原失败记录保留。收尾仅4份既有交接，应用源码仍绑定b9a5d855，接手读取最新文档提交CI。

初轮候选3f53b0c0 / [CI41040228906](https://github.com/ForceMind/MyAPI/actions/runs/41040228906)的前端专项312通过、3失败，并有1个Pointer Capture环境未处理异常；没有标完成。补修保持精确断言：错误查询等React Query通知后再检查，视频夹具使用实际TASK_ACTIONS.GENERATE值；手机直接Search复用无参动作处理器，抽屉fixture补齐Pointer Capture API并恢复原描述符。完整结果以补修提交CI为准。

补修05dad43a / [CI41040724991](https://github.com/ForceMind/MyAPI/actions/runs/41040724991)专项315/315、全量2046/2046、同步/构建/typecheck通过；前端作业因6个lint错误失败（测试文件单子节点Fragment、非空断言、replaceAll规则），不是单测失败。保持全部业务断言，修正测试源码写法；本机未运行工具链。

维护者已批准三个列表沿用现有requests/drawing/tasks桌面与手机线稿；当前基线7ab47a82（CI41021525656 attempt2 success，首次后端35分钟超时取消）。本批不改后端/生产，不跑浏览器，不在本机安装依赖、构建测试或下载产物。

- 复用单日历，桌面/手机均Apply只入草稿、Search才提交；手机高频日期/模型/类型/Request ID或任务ID直接可达，低频字段归组，44px查询/重置/筛选。保留管理员字段、精确ID与旧URL/Key返回上下文、日期ms/sec单位。
- 共用日志分页接入三类列表及原正文/SSE适配器：首尾/数字/直接跳页和20/50/100条，合法旧链接条数保留但出站最多100；缺失/非法总数不补0，错误或占位查询不显示旧行/总数。缩减总数回最后有效页；类别/user/session/管理范围改变重置局部草稿，缓存仍分区。
- 沿用既有事实解析，调用的业务结果/HTTP/用量证据/结算分列；绘图业务状态与提交返回码分开，任务平台/动作/未知状态明确。新增只读可选quota字段展示遵循原金额可见性和单位，缺值Unknown，不改账务。手机核心信息不依赖桌面隐藏列，其余授权字段可展开，结果/失败/详情/复制入口保留。
- 静态审查补修：收缩页必须显式resetTo:last；日期0用nullish判断保留。双端清空通过前端URL布尔allTime区分首次默认日期，清空不再被工具补回今日范围；API不发送该UI标记，三类原接口不改。共用统计沿用同一日期序列化。单端清除仍只发送另一端。
- 新增9个英文键，en/zh译文、其余5语言英文回填；经临时add-missing-keys脚本写入，临时自有脚本已移除。i18n同步/专项/全量/typecheck/lint/build及所有既有CI只在Actions运行且均已通过；没有本机工具链执行或产物下载。
- 浏览器未验证。未重画652张线稿，未改请求详情布局、连接页、支付/采样/生产/发布配置或真实供应商。此批完成后仍按已有线稿逐批推进，不能称全站UI完成。

## 2026-10-30 UI首批：正文/SSE分页与单日历日期范围（源实现/完整CI通过）

维护者已明确批准此批实现，限制全部构建测试在Actions、本机不安装依赖/下载产物，不改生产、浏览器先不跑。源码3e31517e / [手动CI41013409921](https://github.com/ForceMind/MyAPI/actions/runs/41013409921)首次success（10成功、1浏览器作业跳过），已回读全部作业/步骤。先固定旧index/filter-bar为37450c2a验证2个真实缺陷红证据，恢复候选后11文件78/78专项、283文件1974/1974前端全量及同步/构建/typecheck/lint通过；根模块/独立relaykit vet/build、原生/386构建、两模块全量、全部既有竞态、三库/Redis/分发/Desktop通过。没有运行浏览器，不能称视觉验收通过。自动push运行41013408323被同源码手动运行替代，取消收尾卡住后仅对该旧运行force-cancel，终态cancelled，不算通过。

- 已实现源行为：服务端首尾、前后、数字页、直接跳页和20/50/100条；筛选/换条数回第一页，总数减少回有效页，loading/error/未知不伪装为0。统计卡失败后不显示旧数。查询key保留用户/会话并新增pageSize隔离。
- 返回状态：按浏览器原history entry在有界20份内存快照恢复已应用筛选/page/pageSize；侧栏新导航/新entry默认新状态。用户/会话变化、权限失效时清除；不把正文、查询结果或筛选写到URL/history state/浏览器存储。已补同user/sid降权后再恢复的草稿和页码重置回归。
- 单日历：复用现有Calendar/Popover，起止范围可跨月、同日，预设和手动时分秒先入草稿，Apply更新筛选草稿、Search一次提交；Cancel/Escape不提交。按既有本地时间转Unix秒，整日00:00:00至23:59:59；明确时区，拒绝反向、不完整、非法/DST跳时时间，允许显式清单侧边界/清整段。
- 必要前置（静态发现，候选键盘回归已通过）：现有CalendarDayButton已有焦点效果却未绑定ref，补一行ref让方向键真正转移焦点；不替换基础组件样式。分页/日期新增44px触控与弹层滚动合同，仅组件测试，未作浏览器尺寸/视觉验收。原权限、缓存隔离、不显示旧内容、100条上限等断言保留。
- 21个新增key经临时add-missing-keys.mjs脚本追加到7语言（按OPERATIONS保持原顺序，en/zh译文、其余5英文回填；不覆盖既有译文），临时自有脚本已移除；i18n同步只在Actions执行并通过。新增日期组件本批只接正文/SSE，没有替换usage-logs的旧CompactDateTimeRangePicker或其他页面。
- 新测试采用真实React Query/Router/Calendar/Popover与合成API边界，未请求生产/供应商、不触碰日志删除/模型接口。全部运行在Actions，本机只有源编辑及轻量Git/文本操作；没有本机依赖安装、构建/测试、浏览器、Docker或构件下载。
- 收尾只改本清单、handoff README/OPERATIONS及既有线稿README，应用仍绑定3e31517e，接手读取最新文档CI；不自动合main/发布/部署、不重开诊断、不实施上一轮控制器修复方案。后续UI继续按现有逐页线稿分批，当前仅覆盖这两个痛点，不称全站重设计/视觉收口完成。
- 只读审查留待后续复核：原刷新按钮仍直接调用refetch，读取权限失效时是否应同时禁用客户端刷新入口需在页面权限一致性批次验证。本批没有改该旧行为或验证其服务端响应，不据此声称存在数据泄露；后端原鉴权不变。

## 2026-10-30 部署收尾：只读取证及最小控制器修复方案（未实施）

新版本3e6b8cca已部署且健康，缓存会话开关true、诊断false，本轮只读复核未变。部署当时三阶段9479条消费/9680条日志与各自累计扣费一致、完整性通过，使用最新数据并保留旧资源；不要将这些历史基线当实时余额。单次窗口约2.5秒、挂载日志327字节、自然样本0，未完成真实缓存验收。已证控制器max(日志累计,进程自启动wchar)>=80MiB口径混入非日志写；启动日志有94,726行快照复制，触发点精确计数未存。退出2的启动前默认信号路径有源码依据但无原始信号事件/调用栈，不称完全确证。最小方案限控制器计量、启停时序、留证与异常收尾，全部新回归未来仅Actions；本轮不改产品/生产、不重开、不发模型请求。详细证据、未知项、文件候选及风险见[缓存专项第0节第六轮](handoff/CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。随后回已有UI线稿的正文/SSE分页与单日历日期范围；不重画全站，下一实现批次先给方案。收尾仅4份文档，推送后读自己的完整CI。

## 2026-10-30 插单：缓存修复新镜像在Actions打包验收（生产方案待确认）

源码3e6b8cca / [完整CI40911707379](https://github.com/ForceMind/MyAPI/actions/runs/40911707379)通过；[Full+SQLite 40924927031](https://github.com/ForceMind/MyAPI/actions/runs/40924927031) attempt1 success（2成功、LAN安装跳过），20项HTTP/API/鉴权/精确单扣/静态资源/重启/同镜像SQLite恢复全true，浏览器未验证。镜像包12919793136、报告12919559223及准确manifest/app ID见[缓存专项第0节第五轮](handoff/CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)，有效期至北京时间11月6日01:51/01:53。本机未下载/加载产物、安装依赖或构建测试。缓存身份开关默认关闭；交接服务器任务先给更新/启用的最小方案，维护者明确确认前不执行生产、诊断或主动供应商请求，只拟观察自然流量。收尾只改文档、镜像不重标，接手读取最新收尾CI。

## 2026-10-30 插单：已批准缓存兼容与采样界面最小修复（产品完整CI通过）

候选42a258df / [完整CI40905811752](https://github.com/ForceMind/MyAPI/actions/runs/40905811752)首次success（10成功、1浏览器作业跳过），已读全部作业/步骤：根模块/独立relaykit、原生/386构建、两模块/完整竞态/三库/Redis/分发通过；A新增10族、B新增11族、原转换/存在性6族专项竞态逐族PASS，无FAIL/DATA RACE；前端24/24专项、1917/1917全量及同步/构建/typecheck/lint通过。浏览器未验证。收尾只改交接文档，应用仍绑定42a258df，接手读最新文档CI。[合成对照40905060265](https://github.com/ForceMind/MyAPI/actions/runs/40905060265)绑定后端等价源码9fbf640e成功，兼容两种正缓存均保留7与20/3，未知不补零。A局部usage归一化/四态投影与计费证据隔离；B默认关闭的Codex Chat稳定Key/session、覆盖删除优先；C连接窗口来源/历史分类及只读刷新。不改生产、不发供应商请求、不发镜像/版本、不合main。详细行为、审查补修、中间红/取消与开关边界见[缓存专项第0节第四轮](handoff/CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)及[同一工具说明](../../../tools/cache-compat-compare/README.md)。

## 2026-10-29 缓存身份/usage合成对照与采样显示只读调查（测试通过，产品待确认）

源码8f9e533c的[合成对照40888007112](https://github.com/ForceMind/MyAPI/actions/runs/40888007112)、[完整CI40888007450](https://github.com/ForceMind/MyAPI/actions/runs/40888007450)首次success。6身份/8usage场景用双方实际代码证明：MyAPI缺省不补身份，2种兼容usage形状会丢正缓存数；sub2api也省略明确零，不直接照抄。报告及候选最小产品方案集中在 [工具README](../../../tools/cache-compat-compare/README.md)与[缓存专项第三轮](handoff/CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)，产品未改、product_fixed=false，用户确认前不实施。采样后台近期成功，连接页存在旧error系列误分类/不刷新触发条件，浏览器实例未验证；不隐藏错误或补零。另一会话f0ae4505修复已保留，生产cd35不自动更新；本机无依赖安装/构建测试/外部源码克隆/产物下载。收尾纯文档，继续读最新CI。

## 2026-10-29 空Content-Type日志采集修正（代码CI与镜像验证通过）

源码 `cd35f867d43b0a42d61500221bb9f99eb457e36a` / [完整CI40846159247](https://github.com/ForceMind/MyAPI/actions/runs/40846159247)首次success，10成功、1浏览器混合作业跳过；定向13族及空头流式/非流式Chat四状态8例全部PASS，无失败或竞态报告。产品仅日志观察器16行，按实际出站显式stream解析空头，不改转发/计费、默认关闭、保留缺失/null/0/正数。[Full+SQLite40846469827](https://github.com/ForceMind/MyAPI/actions/runs/40846469827)首次success，通用20项全true，browser=not_run；[新包](https://github.com/ForceMind/MyAPI/actions/runs/40846469827/artifacts/12885307002)/[报告](https://github.com/ForceMind/MyAPI/actions/runs/40846469827/artifacts/12884514813)至11月5日，身份在[缓存专项第0节第二轮](handoff/CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。本轮未部署、未改生产、未开诊断或请求模型；收尾只改4份交接文档，应用/镜像仍绑定cd35f867，接手读最新文档CI。旧d2b1f664包不含此修正，不能把上一轮12条未采集当上游零命中。

## 2026-10-29 插单：默认关闭的Codex上游诊断日志（代码CI与镜像验证通过）

维护者批准补齐实际出站正文与解析前原始usage日志，保留缺失/null/0/正数。代码 `d2b1f66450364a9299f676bfa519aedc692dc367` / [CI40828494736](https://github.com/ForceMind/MyAPI/actions/runs/40828494736) 首次success，10成功、1浏览器混合作业跳过；新增诊断8族与原会话头4族定向竞态全PASS。[Full+SQLite40828937555](https://github.com/ForceMind/MyAPI/actions/runs/40828937555) 首次success，20项检查全true、browser=not_run；[镜像包](https://github.com/ForceMind/MyAPI/actions/runs/40828937555/artifacts/12877962270)与[报告](https://github.com/ForceMind/MyAPI/actions/runs/40828937555/artifacts/12878195602)保留至2026-11-05。默认关闭，不改生产、不发模型请求、不改缓存策略或计费；全程Actions构建测试，本机未下载产物。准确身份、开关和部署交接集中在 [缓存专项第0节](handoff/CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。随后收尾只改4份文档，应用/镜像仍绑定d2b1f664，接手读取最新文档CI。本批只补取证，缓存根因未修；UI线稿及原P1/P2未取消。

维护者需求原话：遵循第一性原理评估整个项目；审查代码结构，精简代码；按项目计划制作完整版、个人版、本地版（具体要求在项目文档里）；保留现有 Logo，整个 UI 重新设计，要易用，PC 与手机版都要适配度高；使用设计 skill 保证 UI。

已确认的执行约束：

- 在 `dev` 上开发，不新建分支，不合入 `main`，不发布，不部署，不调用真实供应商。
- 每一步都提交并推送到 `origin/dev`。
- 允许多角色协作，一个角色一个子代理。
- 数据库表和字段不删除；三库（SQLite / MySQL / PostgreSQL）兼容；`relaykit` 保持独立可构建；账务安全不变量不放宽。
- 下线用户可见功能前逐项等待维护者确认。

三个版本与项目文档的对应（`MYAPI_MASTER_PLAN.md` 第 1、8 节，`RELEASE_MANIFEST.md` 第 2 节）：

| 维护者称呼 | 文档中的版本 | 说明 |
| --- | --- | --- |
| 完整版 | Full | 团队/组织，完整治理 |
| 个人版 | Lite | 个人/小规模，SQLite 优先，保留可靠转发、鉴权、Key、日志、额度安全、必要备份恢复 |
| 本地版 | Desktop | Lite 的桌面安装与管理形态，默认只开放本机访问，不维护第二套业务 |

## 任务状态

### 2026-10-28 UI 优先与逐页线稿（当前批次）

- **维护者最新要求**：UI优先，所有页面、按钮和组件纳入统一设计与合理精简；每个页面先画线稿，确认后再改产品。CLI来源规则后移，既有方向3、Logo、业务能力与安全不变量保留。
- **最新仓库基线**：维护者已单独授权将dev快进合入main；两分支均为`c916051e1ad5d14655829d742f0e7d5efac373b7`。main的[CI40664697561](https://github.com/ForceMind/MyAPI/actions/runs/40664697561)首次success（10成功、1个混合浏览器作业跳过）。此授权不扩展为后续自动合并、发布或生产部署。
- **必须保留的执行偏差**：上述main推送触发既有[网站浏览器40664697462](https://github.com/ForceMind/MyAPI/actions/runs/40664697462)，Chromium已在GitHub runner执行成功，取消请求时已结束。不能将这一轮描述成完全未运行浏览器。网站工作流显式授权开关仍待落实；本线稿批次只改docs，按现有路径规则不匹配该入口，仍须回读真实Actions。
- **本批交付**：[全站低保真线稿目录](ui-design-v2/wireframes/index.html)、[审阅说明](ui-design-v2/wireframes/README.md)、[路由覆盖表](ui-design-v2/wireframes/routes.csv)。326个页面/分区/交互视图、652张独立SVG，桌面1200与手机390各一张；66条路由逐条映射、42个设置分区展开，别名和布局不冒充独立页面。视图数不等于已实现或已验收页面数。
- **优先设计**：正文与SSE增加首尾页、页码、每页20/50/100、直接跳页、返回保留查询位置；日期在同一日历选择起止，支持跨月、快捷范围、精确时间、时区与起止错误。按钮、表单、分页、筛选、状态和确认外壳复用；连接/渠道和两种请求详情入口按同一对象整理，保留旧链接、权限与返回上下文。
- **证据边界**：已从代码核对路由、分区、分页上限、正文权限、缓存身份隔离和不重放边界；线稿是SVG文本设计源文件，无API连接。浏览器未验证，未修改`web/`、CLI、后端、数据库或生产/发布配置。设计图不是功能实现、全量视觉通过或生产升级证据。
- **首轮CI已验证**：首份线稿源提交`88165830dd5658b24f70084def81447821d7a0d7`的[CI40682248336](https://github.com/ForceMind/MyAPI/actions/runs/40682248336)首次success（10成功、1个混合浏览器作业按规则跳过），已读取全部作业和后端步骤。根模块/386/relaykit独立构建、两模块全量测试、数据库回归、竞态检查、前端构建/类型/lint/单测和分发合同通过；实际触发列表只有CI，没有网站浏览器或发布工作流。
- **收尾设计补充**：手机筛选展开态、紧凑桌面分页、独立日期快捷按钮和54个内部页签/模式映射已补齐，合计326视图/652张SVG，含OAuth、支付、价格、编辑器、账户绑定等不同内容状态。自查按单个弹层分离字段与动作，移除重复取消按钮，安装未知结果态只允许状态恢复，公开/只读界面不显示无关保存动作。补充与首轮CI证据同批提交；接手者仍须读取此后提交自己的完整CI，不能用首轮CI替代最终补充的CI。
- **后续顺序**：维护者逐页/逐批审阅线稿 → 先实施正文分页与共用日期范围 → 请求区 → 核心对象 → 管理与设置 → 公共/工具/Desktop；其后恢复P2。每一实施批次仍先列范围、风险和验收方法，不重开UI方向与已确认的部署五项决定。

状态：未开始、进行中、待验证、审查未通过、已完成。

| 编号 | 任务 | 角色 | 状态 |
| --- | --- | --- | --- |
| 1 | 第一性原理架构评估报告（模块价值链归属、重复与死代码、版本能力矩阵差距、精简候选分级），见 [architecture-review.md](architecture-review.md) | 架构评估 | 已完成（主会话抽查了 `middleware/edition.go` 的回落与封锁清单、`main.go` 的订阅重置任务启动，与报告一致；deadcode 与 knip 数字未重跑） |
| 2 | 评估报告审读，整理需要维护者逐项确认的功能下线清单（报告 5.4 节 D1–D14） | 主会话 | 已完成（维护者已按推荐确认，见上节） |
| 2a | 基线（批次 0）与 A 类删除（批次 1 前端、批次 2 后端），不依赖维护者决定 | 核心实现 | 已完成并推送（`f4d0026c` 删 74 个前端文件、5 个依赖共 8,387 行；`ba6a3c41` 删 158 个 Go 函数共 1,947 行）。主会话重跑：`go build ./...` 退出码 0、`go vet ./...` 退出码 0、`cd relaykit && GOWORK=off go build ./...` 退出码 0、`bun run typecheck` 无错误输出。子代理另报：删除后 Go 测试 68 个包通过、前端测试 4 个既有失败（基线 5 个，均为 storage 相关，删除前已存在）、lint 8 个 warning 不变、knip 未引用文件 76→2；这些主会话审读了输出但未重跑。计费相邻的 37 个不可达函数按约定保留，待单独审查 |
| 3 | 精简：删除确认无引用的代码（可逆部分），逐批验证 | 核心实现 | 未开始 |
| 4 | 版本能力矩阵落地：后端版本门禁（`full` / `lite`，兼容 Legacy `lan`）与前端按版本裁剪导航 | 核心实现 | 4a 已完成并推送（`e99d66a9`：`lite` 值、`/api/status` 增 `product_edition` 与 `capabilities`、Electron 传 `lite`；主会话重跑 `go test ./controller/ ./middleware/` 通过）；4b 三项改变现有 LAN 行为的门禁改动待维护者确认；4c 前端按能力裁剪导航并入 UI 外壳步骤；CLI 的 `lite` 接线依赖发行线待决定事项，本轮不做 |
| 5 | Desktop 形态：复用 Lite 的数据模型与业务，补齐安装、后台服务、托盘与本机访问默认值 | 核心实现 | 未开始 |
| 6 | UI 设计系统（色彩、字体、间距、导航、PC/手机布局规则），保留现有 Logo | UI 与视觉 | 方向方案已完成并获确认（[DESIGN_DIRECTION.md](ui-design/DESIGN_DIRECTION.md)）；6a 令牌、主题预设、文档语言已完成并推送（`73577d7d`；主会话重跑 `bun run typecheck` 无错误、新增 15 个测试通过，并查看了概览 1280 亮色与 390 暗色截图；对比度表 188 个组合 2 个不通过：亮色 `--telemetry` 青相对卡片 1.85:1，按设计文档规定遥测填充必须同时显示数值，是否换更深的青待维护者决定）；6b 外壳已完成并推送（`17b27f55`、`30b308ef`、`e890912d`：两套外壳收敛为一套，按版本能力分组导航，手机底部 5 项加"更多"，`Ctrl/⌘ K` 全局搜索；主会话重跑 `typecheck` 无错误、lint 退出码 0 且 warning 为既有项、前端全量测试 4 失败 / 1745 通过，失败的 4 个与基线相同，并查看了 PC、手机、Lite 三张截图。遗留：粘贴 Request ID 只能跳到按 ID 筛选的调用记录，不能直接打开详情，列入页面批次；`tools/` 下 10 个浏览器脚本已按新外壳更新但未运行，因本机没有 Playwright JS 包，由最终 CI 验证）；6c 三个关键页（概览、Key→请求排错、上游）已实现并推送（`74521a97`、`5b608234`、`42848584`、`b90e593b`、`40e7b7e4`、`cdae24df`；主会话重跑 `typecheck` 无错误、`bun run build` 成功、前端全量测试 4 失败 / 1756 通过，失败的 4 个与基线相同，并查看了 Key 检视器、Request ID 直达详情、手机概览三张截图；Request ID 粘贴后回车直接打开详情已达成；`tools/ui` 13 个脚本已更新但未运行，由最终 CI 验证）；待维护者在预览里试用并确认后批量铺开其余页面 |
| 7 | 关键页面样稿，经维护者确认 | UI 与视觉 | 未开始 |
| 8 | 逐页重做 UI（PC 与手机各自设计），七种语言同步 | UI 与视觉 | 未开始 |
| 9 | 测试：受影响 Go 包、`relaykit` 独立构建、前端测试、三库、浏览器多尺寸渲染 | 测试 | 未开始 |
| 10 | 独立质量审查与回归 | 独立审查 | 未开始 |

## 维护者已确认的决定（2026-10-24，原话"都按你说的做"，针对下列推荐）

功能下线（`architecture-review.md` 5.4 节 D 类）：

- D1–D9（支付、商业订阅、兑换码、签到、返利、公开定价/排行、Midjourney、io.net、外部登录）：不下线。Full 保留，Lite / Desktop 按版本隐藏入口并封闭写入，数据和计费路径保持不变。
- D10（冷门适配器）：不删除，只在新建渠道表单中隐藏。
- D11、D13、D14：已查实际依赖（主会话 `git grep`），结论均为保留，不需要再向维护者确认：
  - D11 上游验收运行：`controller/upstream_acceptance_run.go` 被 `.github/workflows/ci.yml`、`controller/relay.go` 等多处引用，在产品内接线，保留。
  - D13 `patches/`：`package.json` 的 `files` 包含它，`patches/manifest.json` 记录对 New API 上游的补丁来源（来源与署名信息），保留；`bin/` 下旧迁移 SQL 与 `time_test.sh` 在 workflow、脚本、makefile、文档中均未发现引用，但体量小，暂不动，维护者要求时再处理。
  - D14 `new-api.service`：`package.json` 的 `files` 包含它，旧单元名安装仍可能使用，保留。
- D12（`docs/evidence` 归档）：保留。

UI 设计（`ui-design/DESIGN_DIRECTION.md` 第 11 节）：

1. 方向 A：工作台 + 检视器。
2. 合并"上游账号 / 连接 / 渠道"为"上游"区，所有网址保留，`/channels` 成为连接的表格视图。
3. 主题预设只保留"MyAPI 默认"与"高对比"，已保存的其他预设回落默认，不报错；亮/暗/跟随系统、字号、圆角、内容宽度偏好保留。
4. 图标库统一为 Hugeicons，随页面批次迁移，单个文件内不混用。
5. Lite / Desktop 精简构建提供七种语言，语言包按需懒加载。
6. 采用术语表（"渠道"改"连接"，"API 密钥/令牌"统一为"API Key"），依赖文案的浏览器验收脚本在同一批次更新。

三项改变现有 LAN 实例行为的门禁改动，维护者于 2026-10-25 以"都按推荐"确认，已在 `07207713` 实现：未知版本值拒绝启动（合法值为空、`full`、`lite`、`lan`）；Lite 下补封 `/api/redemption`、`/api/deployments`、Midjourney（`/api/mj`、`/mj`、`/:mode/mj`，后两者是主会话为避免只封管理接口、留着转发而扩展的，已在汇报中说明）；Lite 下只放开 `GET /api/subscription/self`，其余订阅路径仍封。另一项获批改动：日志新增可选参数 `result=failed`（`eb08c3f3`），只返回请求失败、排除待核对与发出前被拒绝；管理员概览继续读取全部用户的记录，与调用记录页一致。亮色青色对比度保持不变，遥测填充必须同时显示数值。
后续待做：概览"失败 N"链接改用 `result=failed`（前端）；Lite 下已有订阅用户的自助只读订阅视图（前端，放进个人资料页）；日志筛选在 MySQL 与 PostgreSQL 上的测试已接入 CI 的 S1 作业，需读 CI 结果确认。

## UI 方向第二版（2026-10-25）

维护者对第一版（方向 A 工作台 + 检视器）不满意，要求重新设计并参考 CC Switch 与 TokenHub（只吸收思路，不复制，见 `MYAPI_MASTER_PLAN.md` 第 1 节）。第二版方案与三个可点击样稿见 [ui-design-v2/](ui-design-v2/DESIGN_DIRECTION_V2.md)。维护者看过样稿后的选择：**方向 3「流向图」**（原话"3好看"）。第一版的 6a 令牌值、6b 外壳、6c 三个关键页的布局因此被取代；数据钩子、版本能力读取、Request ID 直达、检视器内容区、术语表、Hugeicons 迁移、主题预设精简保留。

截至 2026-10-25，`dev` 的提交 `00763936`（含全部第一版修复）CI 运行 40104526656 结论为 success，作为后续返工的回归基线。

实施步骤（待维护者确认后开工，每步验证后提交并推送）：

1. 令牌与外壳：纯白底加浅灰面加墨色文字，深蓝主色，新增 `--core`、`--core-foreground`、`--line`、`--meter*`、`*-muted` 令牌；顶部 56px 横向导航（概览 / 上游 / API Key / 请求 / 设置，Full 多一项"管理"下拉），内容 1200px 居中，面包屑最多三级，没有侧栏；手机为顶栏加底部 5 项；按版本能力裁剪入口。
2. 首页流向板：新组件 `FlowBoard`（Key → 网关核心卡 → 上游，点节点联动筛选下方请求表）；节点超过 6 个折叠为"前 N 个 + 查看全部"；Lite / Desktop 的核心卡显示本机地址与访问模式，Full 显示公网地址与摘要；手机为纵向管道。
3. Key 与请求：Key 列表、Key 对象页（整页加面包屑）、请求页（四格结果、诊断时间线、正文 / SSE / 原始）；Request ID 直达与返回保留筛选、页码、焦点的行为原样保留。
4. 上游：卡片网格（环形剩余额度、状态、开关、测试）与对象页（窗口、路由与模型、请求、历史）；"添加连接 → 保存并测试"保留。
5. 其余页面（设置、用户、分析、正文日志、任务、系统信息、商业）按新外壳与组件规范分批铺开。
6. 全程验证：七种语言，320 / 390 / 768 / 1024 / 1280 / 1440 宽度，亮暗主题；前端测试、`typecheck`、`lint`、`build`；本机 Playwright 复现 CI 的浏览器验收脚本；每次推送后读取 CI 结果。

维护者确认（2026-10-25，原话"语言暂时中英文就可以，其他的按你说的"）：上述实施方案与"我替你定了的细节"全部按推荐执行（纯白底；请求详情用整页；Full 的用户 / 商业 / 系统放顶栏"管理"下拉；默认首页为概览）；**界面语言暂时只要求中文和英文**：新增文案提供 en 与 zh 译文，其余五种语言（zh-TW、fr、ru、ja、vi）由 `bun run i18n:sync` 以英文回填并在报告里记录未翻译数，视觉与浏览器验收只覆盖中英文。这是对 `AGENTS.md` / `web/AGENTS.md` 七语言要求的临时放宽，需在恢复七语言验收时补齐：五种语言的译文、俄语 / 法语下顶栏导航的实测、七语言截图。

方向 3 进度（2026-10-25）：
- 第 1 步 令牌与顶部导航外壳：已完成并推送（`b28cf752`、`e59adcdd`、`d57a4d7b`、`f5770baa`）；CI 运行 40166671819 结论 success（Linux 上全部浏览器验收通过）。主会话重跑 `typecheck`、`lint`、前端全量测试（3 个既有存储类失败，无新增），并查看了 PC、手机、暗色"更多"菜单截图。
- 第 2 步 首页流向板：已完成并推送（`edf7ae1d`、`cceb4aa5`、`ed45ec3e`）。主会话重跑 `typecheck` 无错误、前端全量测试 1813 个用例 3 个失败（credential-maintenance-dialog、credential-rotation-dialog 为基线既有；handoff-lifecycle 在高负载下偶发，单独重跑通过），并查看了概览 1280、节点选中、手机三张截图。数据口径：24 小时内最近 100 条日志在前端聚合，核心卡总数精确读取，截断时写明；未新增后端接口；缺失数据显示"未知"。与样稿的已知差异：无"今天 / 7 天"切换、Key 节点显示 `#ID` 而非掩码尾号（安全列表不含掩码）、上游节点无"测试失败 401"（连接记录不存测试结果）、Full 无"用户 / 分组"摘要。待 CI 验证。
- 第 3 步 Key 列表、Key 对象页、请求页：已完成并推送（`67be4186`、`b2607555`、`f3b29ffe`、`20ec63f1`，与后端 `07207713`、`eb08c3f3` 同批推送）。主会话重跑 `typecheck` 无错误、`lint` 只有既有 warning、前端全量测试 1843 个用例 4 个失败（credential-maintenance-dialog、credential-rotation-dialog 为基线既有；account-switch、strict-parameter-selection 为高负载偶发，基线与前几步也出现过），并查看了 Key 对象页 1280 与 Key 列表 390 截图。新地址：`/keys/<id>`、`/keys/<id>/requests/<requestId>`、`/usage-logs/request/<requestId>`，旧 `?inspect=` 链接重定向。子代理另报：Key 页删除后列表仍显示已删 Key 的缓存缺陷已修；共享分页组件手机页码按钮 32px 宽（低于 44px 触控合同）留到第 4 步收尾项处理。待 CI 验证。
- 后端门禁与日志筛选（`07207713`、`eb08c3f3`）：已推送，主会话重跑 `go build ./...`、`go vet`、`middleware` / `controller` / `model` 三个包测试通过；MySQL 与 PostgreSQL 上的日志筛选测试接入 CI 的 S1 作业，待 CI 验证。
- 第 4 步 上游卡片网格与连接对象页，加三个收尾项（概览"失败 N"接入 `result=failed`、分页按钮手机 ≥44px、Lite 个人资料页"我的订阅"只读卡片）：代码与单测已本地提交（未推送），待验证。新地址 `/connections/<id>`，旧 `?connection=` 链接重定向。子代理报：最终工作区 `typecheck` 通过；全量前端测试与浏览器验收按维护者要求改由 CI 运行，本机只跑过中途一轮（详见 [step4/README.md](ui-design-v2/step4/README.md) 第 5 节）。
- 第 4 步完成情况（2026-10-26）：代码已推送（`1a025f6b`、`ae4a9f63`、`58be988e`），按维护者"构建等工作都在 GitHub 上完成，不要占用本机空间"的要求，最终提交未在本机构建与测试，完全由 CI 验证。CI 运行 40206993990（提交 `cddc648d`）：后端、数据库、Redis、ClickHouse、桌面合同、发行合同、凭据维护真实 Go 后端等全部作业通过；前端作业的类型检查、全量测试、全部 Chromium 浏览器验收步骤通过，唯一失败是 Lint 步骤的两处 `Expected { after 'if' condition`（`connection-sections.tsx`、`flow-nodes.tsx` 中的无花括号 else-if 链，主会话据此推断并加花括号，待下一次 CI 确认）。
- 部署 B1、B2（`80f9185b`、`cddc648d`）：同一 CI 运行中"Distribution and release contracts"与"Desktop cross-platform contract"作业通过。待维护者决定：docker-build / docker-smoke 工作流是否改用 `lite`（推荐暂不改）；命令行与安装脚本对公网 HTTPS 来源的不一致是否对齐到安装脚本的更严格规则（推荐对齐）。
- 本机清理（2026-10-26）：已删除会话临时目录、`web/node_modules`、`web/dist`、Go 构建缓存与 bun 缓存并停止预览服务；此后构建、测试、浏览器验收全部在 GitHub 上完成。
- 第 5a 步 请求区（调用记录、任务、绘图、正文与 SSE、分析）：已推送，CI 运行 40271665685 通过（首次失败于偶发的 `window-maintenance` 对话框隐藏超时，只重跑失败作业后通过）。
- 第 5b 步 设置、用户、系统信息、个人资料（`df2b96ba`、`acd53ee5`、`403cca15`、`470d0860`、`6e89abd3`）与第 5c 步 商业、认证、安装向导、公共页、Playground / 聊天 / 提示词学习、模型目录与账号窗口（`496dafc5`、`5844e5b8`、`107daf1b`、`58d8ce2c`、`5bbf6cb1`、`3ca80983`、`e44f4056`、`c531c0a8`、`e1aad933`、`ad33f977`）：在不能编译的条件下写成，**未经 CI 验证**，随接手文档一并推送。接手后第一件事是读 CI 并修复。
- 维护者确认（2026-10-26，原话"都按你推荐的"）：Docker 工作流暂不改用 `lite`；命令行对齐安装脚本更严格的来源规则（待实现）；分析图表第 6 名起用中性灰。
- 接手交接（2026-10-26）：见 [handoff/README.md](handoff/README.md)、[PROMPT.md](handoff/PROMPT.md)、[OPERATIONS.md](handoff/OPERATIONS.md)、[DIRECTORY_GUIDE.md](handoff/DIRECTORY_GUIDE.md)。主会话额度用尽，后续由其他 AI 接手。

### 接手核对与缓存专项（2026-10-27）

- **[本机轻量检查]** 主检出已整理为 `dev`，两个嵌套工作区已移除；历史分支、仓库 `.git` 和内置磁盘上的 Codex 工作区保留。整理只使用获维护者逐项确认的临时脚本副本，未修改仓库原脚本。未在本机安装依赖、构建、测试或启动浏览器。
- **[CI]** 接手基线 `d3beeeef` 的 [40284651016](https://github.com/ForceMind/MyAPI/actions/runs/40284651016) 实际为 success，11 个作业全部通过。因此上面的“5b、5c 未经 CI”是交接时历史状态，不是当前构建状态；仍不能据此认定视觉验收完成。
- **[维护者]** 优先检查 OpenAI/Codex 缓存链路；读码报告后仅批准修复 Chat → Responses 丢失 `prompt_cache_retention`，不修改渠道配置、请求头策略、计费或生产，不调用真实供应商。
- **[CI 先红]** 回归提交 `92e81914` 的 [40441341548](https://github.com/ForceMind/MyAPI/actions/runs/40441341548) 已结束：10 个作业成功，后端作业失败于 `TestUpstreamAcceptanceRelayRetainsOperationThroughControllerReturn`。Actions runner 上回环地址的合成 HTTP 上游断言缓存键正确、保留策略预期 `24h` 而实际为空；后端根模块失败后 `make test` 未继续执行 relaykit 单测，不冒称独立转换测试已在这一轮变红。
- **[CI 后绿]** 修复提交 `6392caf9` 仅增加已有 `PromptCacheRetention` 的传递，其余差异是字段对齐空白；[40443871659](https://github.com/ForceMind/MyAPI/actions/runs/40443871659) 为 success，11 个作业全部通过。已读取后端日志：`router`、`relaykit/relayconvert/internal/oai_chat` 及完整两模块测试通过，`Build relaykit independently` 通过（作业 `GOWORK=off`）。新增测试覆盖 `24h`、`in-memory`、只提供保留策略、只提供缓存键及两者缺失；缺失不补值，不生成缓存键，原缓存键特殊字符测试保留。未放宽断言。
- **[验证边界]** 以上只证明转换和合成出站不再丢失参数，不证明真实供应商接受所有缓存保留策略，也不证明生产请求命中缓存；未发布、未部署、未调用供应商。
- **[读码，未修]** 默认 Codex 会话头透传模板依赖亲和规则匹配（原始模型 `^gpt-.*$`、入站 `/v1/responses`、非空 `prompt_cache_key`）；请求体直通绕过参数操作，因此也绕过模板中的 `pass_headers`，显式渠道 Header Override 不同。Chat 转换路径未同等保留原始缓存计数存在性证据；亲和统计的未知用量可能降低其命中比。具体修复均需维护者另行确认，不把这些条件性风险断言为生产不命中根因。
- **[查看 CI 原图，抽查]** 从 `d3beeeef` 的 `myapi-ui-browser-d3beeeef0e2cc26e04a21e3ab6eed095580653f2` 构件（ID `12634283151`）查看 17 张 320/1280 亮色英文代表截图：设置、用户、个人资料、运行状态、钱包、订阅、兑换码、登录、首页、安装向导、Playground、模型目录；对照方向 3 样稿与 step1–step4 说明。报告 `qualification.json` 标明准确提交且 33 个旅程通过，但截图人工检查不是全量：中英文/暗色/其他宽度和全部页面仍待覆盖。下载内容看完已清理，未在本机运行浏览器。
- **[看图待核对，未修]** `models-1280.png` 中固定操作列左侧的 Official Sync 表头/状态只剩首字母；`users-1280.png` 邀请信息在固定操作列边缘只露出部分字符；`setup-interrupted-320.png` 步骤切换后的顶部品牌被裁切。现有页面级几何断言仍通过，不能替代单元格遮挡和公共页品牌完整性验收。尚未在 CI 增加定向复现，不能把这些现象全部认定为本批新增回归；未放宽断言、未擅自改 UI。

### 会话头取证与再次交接（2026-10-27）

- **[CI]** `e7eef352` 的 [40447186866](https://github.com/ForceMind/MyAPI/actions/runs/40447186866) 已确认11/11成功，最近全绿基线不再是旧的a53973d4。
- **[维护者]** 批准“先只新增合成测试，验证Codex亲和未匹配/正文直通是否丢会话头”，未批准产品修复。`e8652185` 只新增 `router/codex_session_headers_test.go`，真实鉴权/分发/转换/出站链路，回环地址白名单网络夹具，未改产品代码。
- **[CI 已复现、未修]** [40462005121](https://github.com/ForceMind/MyAPI/actions/runs/40462005121) 完整结束：10作业成功，后端作业122048552508失败于 `TestCodexSessionHeadersReachSyntheticUpstream` 的5个子场景（无缓存键、模型不匹配默认规则、Chat转Responses、渠道正文直通、全局正文直通）。三种头Session_id/Thread_id/X-Codex-Turn-State预期有值，合成上游实际全空。匹配规则、显式覆盖、请求间隔离、客户端凭据不透传、响应turn-state回传、断连不重放及严格验收拒绝对照未报失败。根测试失败后make未继续relaykit单测，不称整轮通过。
- **[授权边界]** 已给出Codex白名单头解耦修复方案，但维护者未回复批准，随后要求“你把所有工作都更新到仓库，我会让其他ai接手你的工作”。因此只更新交接，不继续实现；不能为绿灯删除/放宽断言或退回旧基线，不部署当前红灯线。
- **[交接]** 最新入口为 [handoff/README.md](handoff/README.md) 零节，[CACHE_REVIEW_20261027.md](handoff/CACHE_REVIEW_20261027.md) 含精确SHA/CI、日志消息节选、调用链、拟修范围与显式删除/严格门禁风险，OPERATIONS和PROMPT同步更新。原P1/P2任务保留。交接文档的新CI需按实际结果读取，预计仍含上述已知取证红灯，不冒称全绿。
- **[UI 进一步读码]** 已见截图裁切现象仍待定向验收；表格具有横向滚动/固定列，setup品牌是普通栏且切步会focus标题，可能随正常滚动出屏。须验证滚动可读和控件可达后才定性，不能因自动验收passed就称细节全合格，也不能凭静态边缘裁切就武断返工。没有产品UI修改。

### Codex 会话头最小修复（2026-10-27，已获批准并通过完整 CI）

- **[维护者]** 本次明确同意按缓存专项第5节修复会话头；保持最小范围、不放宽断言，提交推送 `origin/dev` 后读取完整 CI，构建与测试全部在 GitHub Actions。
- **[已读取 CI]** 接手提交 `50bad4dbc5bbe1ca40e5da195d79f0a81a456026` / [40474459858](https://github.com/ForceMind/MyAPI/actions/runs/40474459858)：10/11作业成功，后端仍仅同一测试族的5个会话头子场景失败。该提交相对 `e8652185` 只更新6份文档，没有产品修复。
- **[实现已通过CI]** 产品写入范围收窄为 `relay/common/override.go` 的有效头映射及参数读取上下文，测试只补 `relay/channel/api_request_test.go`、`router/codex_session_headers_test.go`。仅 Codex 普通转发默认提供 Session_id / Thread_id / X-Codex-Turn-State 的 `client_header` 引用；在参数操作前建立默认项，运行时最终映射不补回缺失项，使显式覆盖、keep_origin、delete_header 继续生效。使用引用而非插入客户端原文，避免客户端内容被当作配置占位符展开；不改冻结渠道配置。正文直通保留跳过参数操作的原有语义。
- **成功标准**：原5条失败路径转绿，缺失不生成，显式覆盖/删除、请求间隔离、凭据隔离、严格门禁、不重放保持。**验证方法**：保留原取证断言；补删除（匹配/不匹配亲和）、keep_origin（客户端/显式配置）、显式set、无关正文操作、客户端占位符字面值、供应商和渠道测试隔离对照，全部由完整 CI 执行；通过后记录提交与运行号。
- **[本机轻量检查]** 仅格式化本次 Go 文件并检查 Git 差异；没有在本机安装依赖、构建、运行测试、浏览器或 Docker。最终验证均来自下列完整CI，原 UI 与 P1/P2 待办继续保留。
- **[自查补充，已随9795c0f5验证]** 默认会话头采用 `client_header` 引用，显式 `set_header` 值映射等参数操作仍须读到真实客户端值。仅在Codex普通请求的三个会话头读取中跳过默认引用、回读既有请求头上下文；其他供应商/渠道测试不启用此内部标记。增加匹配亲和后的显式映射对照，原断言保留；该补充只涉及同一个产品文件与现有router测试。
- **[首轮完整CI已通过]** 修复提交 `069f67d7afa5931c2bb2086d3361c7dfe36acf36` / [40500636411](https://github.com/ForceMind/MyAPI/actions/runs/40500636411) 为success，11/11作业成功。已读取后端日志，`router`、`relay/channel`、`relay/common`、relaykit转换与两模块全套测试通过，relaykit独立构建和后续全部竞态检查成功，未被跳过。原5条会话头失败路径及原保护断言随完整测试转绿；新增显式映射边界补充不在该提交中，以后续9795c0f5的CI为准。
- **[补充提交完整CI已通过]** `9795c0f5d4c3cc52beffcebebee1757364f7a3cf` 已推送并回读 `origin/dev`；[40504125721](https://github.com/ForceMind/MyAPI/actions/runs/40504125721) 为success，11/11作业成功。已读取本轮后端日志：相关包、relaykit独立构建、两模块全套测试与全部后续竞态检查通过；前端类型、lint、测试、浏览器验收及既有数据库/Redis/发行/桌面回归作业成功。保护断言未放宽。验证目标为合成上游，不证明真实缓存命中。

### GitHub Actions Full + SQLite 临时测试部署（2026-10-28，已批准并完成验收）

- **批准范围**：维护者已同意 Full + SQLite、linux/amd64 的临时容器部署。打包和验证全部在 GitHub Actions；不跑浏览器，不在本机安装依赖或下载产物，不连接服务器、生产数据或真实供应商。交付镜像包、脱敏验证报告与CI结果；交付的镜像包和验收报告保留7天。
- **资源边界**：已只读确认仓库为public，标准ubuntu-latest runner、缓存上限10GiB；按GitHub当前公开仓库免费规则执行，不增加缓存配额、不启用付费runner、不修改账单设置。此结论不覆盖账户其他项目或既有账单。
- **基线**：产品提交 `d611da46141503ca81842d1daf1c8885e63be4e0`，原会话头修复及其完整CI证据保留。本批只调整现有CI入口、Docker测试脚本/合同测试和直接相关交接记录，不修改Go业务代码、数据库结构或生产Compose。
- **实现与验收**：现有CI浏览器步骤需显式手动 `run_browser=true` 才能运行，混合Go/浏览器作业同样受控；本次关闭。Docker smoke以 `api_only=true, run_browser=false, personal=false` 手动触发，只构建Full一次、跳过LAN安装与历史升级场景。独立消费者校验同一运行的包哈希、镜像ID、SHA、tree和producer attempt后加载；不回退旧镜像、不重新构建替代验证。
- **本轮验证目标**：真实应用容器的健康、初始化、鉴权拒绝、合成账号/Key/连接/上游、精确用量与日志，HTTP读取嵌入的HTML及JS/CSS，归属校验后的原容器重启和含WAL快照的同版本恢复。HTTP检查不运行前端脚本，报告明确 `browser=not_run` / “浏览器未验证”。不将同版本恢复描述为生产迁移或跨版本回滚验证。
- **补充范围已批准**：维护者明确同意仅同步 `tools/runtime/installer-workflow.test.mjs`。为既有personal用例显式提供 `run_browser=true`，保留“两种Full writer均覆盖”等原断言；补充API-only只运行Full/fresh/legacy、不启动LAN安装或浏览器的对照。
- **首轮CI取证**：实现提交 `dd04079be283d7977f981ae59c7276a0ff81ae80` 已推送；CI40639639962的Distribution作业通过，日志确认浏览器能力开关、HTTP静态资源和容器归属/重启用例通过，混合浏览器作业跳过。随后核对发现原CI命令未选中两份工作流合同测试，补入同一检查入口后推送新提交，首轮CI终态为cancelled。首轮结果不代替这些未执行用例的验证，Docker打包在补齐用例验证后才触发。
- **代码CI已完成**：补齐入口提交 `cf6773fdf7c5c1365291cdee06d7b25897fa7e4f` 的 [CI40640242136](https://github.com/ForceMind/MyAPI/actions/runs/40640242136) 为success：10个作业成功、1个混合浏览器作业按授权规则跳过。Distribution日志确认95/95通过，包含API-only入口、浏览器开关及两种Full writer合同用例；根模块与linux/386构建、relaykit独立构建、两模块测试、全部竞态检查和既有数据库/Redis等回归通过。前端构建、类型、lint、单测通过，浏览器安装及全部Chromium步骤明确skipped，浏览器未验证。
- **临时部署已验证**：[Docker smoke 40640591332](https://github.com/ForceMind/MyAPI/actions/runs/40640591332) 为success，head SHA准确匹配cf6773fd；Full打包与独立消费者通过，LAN安装跳过。消费者报告 `passed=true, test_deployment=true, browser=not_run`，20项检查全true，含7个JS/CSS资源的HTTP读取、原容器重启和SQLite/WAL同版本恢复。应用镜像ID `sha256:78780948305a4b6b9a119e458e4d0b97aba113b2a10863a192a4c96be6c2a699`，平台linux/amd64，版本0.2.0-beta.26。
- **交付物**：[Full测试镜像包](https://github.com/ForceMind/MyAPI/actions/runs/40640591332/artifacts/12794779005)（314159323字节，images.tar与manifest.json，包含app及固定Bun测试夹具镜像）、[脱敏验证报告](https://github.com/ForceMind/MyAPI/actions/runs/40640591332/artifacts/12794639400)（qualification/source/manifest），有效期至2026-11-04。只读取了Actions日志中的JSON报告和构件元数据，没有把构件下载到本机。
- **完整性与收尾**：消费者校验的manifest SHA-256为 `bb362df1a282291cf8541549b5c1342dea2402726bb3403e84f81b2cb8df6851`；镜像包中的manifest包含images.tar哈希与源SHA/tree、平台、镜像ID。后续收尾提交只更新交接文档，交付镜像继续绑定cf6773fd，不重新构建或重标记为文档提交SHA。临时容器的归属校验、清理步骤均成功；不提供常驻在线测试站点，不将本轮结果描述为真实供应商、浏览器或生产迁移/跨版本回滚验收。
- **本机边界**：本机仅编辑源码并检查差异；未安装依赖、编译、运行测试、启动Docker或浏览器，也未下载镜像/构件。运行状态与报告通过GitHub API只读核对，产物保留在Actions。

方向 3 已知风险（来自方案文档）：流向板是全新组件，对象多时拓扑感减弱；顶部导航在俄语、法语下最敏感（1024–1279 宽度需把一项收进"更多"）；任务 1、7 的步数比方向 1 多 1 步；实现代价为三个方向中最高。

## 需求来源与已识别的文档冲突

- 版本定义：`MYAPI_MASTER_PLAN.md` 第 1、8 节；三维模型（功能版 / 安装形态 / 访问模式）与能力矩阵：`RELEASE_MANIFEST.md` 第 2 节；Legacy `lan` 兼容映射：同文档第 2.2 节与 `LAN_LITE.md`。
- UI 合同：`PROJECT_COMPLETION_EXECUTION_PLAN.md` 第 12 节（S6）。要点：先做功能盘点与页面合同；设计系统沿用现有组件与 token 体系；关键页面先出真实可交互预览，经设计方向确认后批量实施；批次顺序为 身份/布局/引导 → Key/调用/用户 → 渠道/额度/日志/Task → 钱包/配置/运维 → 提示词学习 → 安装/更新/Lite/Desktop/官网；七种语言全覆盖，含动态文字与错误文案；代表宽度至少 320 / 390 / 768 / 1440；覆盖加载、空、错误、无权限、过期、处理中、成功、冲突、暂停、未知状态；必须实际查看最终渲染与操作流程。
- 每个工作包的退出合同：同文档第 14 节（目标、依赖、写入范围、不变量、实际验证命令与退出码、审查发现、提交与同步状态、未覆盖项）。本清单按该格式在每步完成后更新。
- 冲突：`MYAPI_MASTER_PLAN.md` 第 11 节 P4 写"后端、Lite/Desktop、发行代码先行，完整 UI 替换暂缓"，版本总表（2026-10-04）则把 UI 重构放在 Desktop 之前。版本总表的 beta.4–7 状态是历史记录，当前代码版本为 0.2.0-beta.26。维护者本次明确要求版本能力与 UI 重做一并推进，因此按"版本门禁先行、设计系统与样稿随后、再逐页重做"的顺序执行，并在任务 4、5 完成后再开始任务 8 的大批量页面改造，使页面按最终版本能力矩阵设计。
- 可选商业模块（支付、订阅、兑换码等）在计划中不阻塞自用版本。评估后若归为 Full 专属，则 Lite / Desktop 隐藏入口而不删除数据；是否整体下线由维护者逐项决定。

## 范围边界

- 本轮不做：Release Manifest 签名与信任根接线、统一安装器（`myapi install` / `switch` / `rollback`）、自动更新器、NPM / GHCR 发布。这些依赖下文的待决定事项，且计划要求决定前只做纯合同与合成测试。
- 本轮做：版本门禁（后端与前端）、Lite 与 Desktop 的业务共用与默认值、代码精简、UI 全面重设计。

## 角色与模型分工

一个角色一个子代理，子代理均显式指定模型；需要更强判断的工作升到更强的模型。

| 角色 | 模型 | 适用 |
| --- | --- | --- |
| 架构评估 | Opus | 全项目评估报告（任务 1） |
| 方案审查 / 咨询 | Fable | 仅限方案生成、咨询、方案或成果审查，如精简方案、版本门禁设计、UI 设计系统的审查 |
| 核心实现 | Sonnet；涉及账务、对外契约、存储结构、跨模块的改动升到 Opus | 任务 3、4、5 |
| UI 与视觉 | Opus | 设计系统、样稿、关键页面；规格明确的页面铺开可用 Sonnet |
| 测试 | Sonnet | 任务 9 |
| 独立质量审查 | Fable | 只审查，不参与实现（任务 10） |
| 机械活（改名、批量替换、收集命令输出） | Haiku | 规格明确、可机械核对 |

升级规则：同一子代理对同一目标连续两次验证不过，先判断是说明不清、环境问题还是能力不足，能力不足才升一级，最高 Opus；Opus 仍不过则停下并告知维护者。子代理汇报不是已验证事实，改代码的结果由主会话审读命令输出，关键路径亲自重跑。

## 每步提交规则

每个任务完成并通过验证后，单独提交并推送到 `origin/dev`。提交只包含该任务的文件，不包含 `._*` 文件和仅权限位变化（本仓库位于 exFAT 卷，使用 `git -c core.fileMode=false`）。提交信息沿用仓库既有习惯，不混入无关改动。

## 待维护者决定（来自 `RELEASE_MANIFEST.md` 第 9 节，阻塞安装器与发行线，不阻塞任务 1–9 中的其余工作）

1. Lite 的 OCI 长期坐标。
2. Release Manifest 的权威托管、信任根与签名门槛。
3. 原生安装的标准路径与服务模型。
4. Desktop 首发平台与更新器模型。
5. 自动更新的默认范围。

- **UI首批收尾修正**：日期预设独立使用 `Current month` / “本月”，避免复用旧键 `This month` 的“本月获得”；七语言补键，本批总计22个。沿用四项预设精确日期断言，完整CI以本次提交为准；此前交接060c4acd的CI41017482486为10成功、1浏览器作业跳过。


### UI14–18集中收口过程与红绿证据（历史记录）

**UI14已完整验证**：源码 `a085460769f2161eaad7facc1fd7e3f5a0db315a` / [CI41523884159](https://github.com/ForceMind/MyAPI/actions/runs/41523884159) attempt1 success，10作业成功、1浏览器/真实API混合作业跳过。已亲读全部作业/步骤及前后端原始日志：认证/个人资料27文件180/180、用户/资金297/297、模型164、公开33、账号85、Key388、原连接209、日志410、全量354文件2658/2658通过；同步/构建/typecheck/lint通过，18警告0错误。根模块/独立relaykit vet/build、原生与386、两模块全量/全部既有race/三库/Redis/ClickHouse/分发/Desktop通过；386定向18父项/55含子项/5包通过，0skip/fail，后端原始失败/race标记0。浏览器依赖及全部Chromium跳过，本机无依赖安装/构建测试/产物下载，未生产/发布/真实模型请求。后续f6b4f042仅测试取证，不替代这个可追溯绿源码。

**2026-11-03 集中收口安排（未验收）**：UI15–18及可达写入补护已全部备齐，合并为同一剩余UI验证迭代，以减少重复Actions消耗，不减少原检查。先提交仅测试/CI门禁的 `f6b4f042`，再验证产品候选；测试提交不含产品源、译文或版本，不作为可部署版本。新增定向门禁覆盖设置/SystemInfo/分析/Playground/PromptLearning/Chat/公共与Setup/错误，以及剩余渠道与模型NoReplay，原全量/三库/race/独立relaykit/浏览器授权门禁全部保留。

当前产品工作区共112个源路径（UI15+16的62个、UI17+18及55调用补护和Root模型三写的50个），保全部字段、价格计算、原接口载荷/确认/权限/返回上下文。共享表单与调用方联合验证；46个补键为32个新增文案和14个既有缺键（含Retirement及源码字面量扫描发现的13项），en/zh及另五语言英文回填，经临时脚本追加保旧译文顺序，i18n同步仍仅在Actions。沿原VERSION/package.json同步一次候选版本0.2.0-beta.27，不创建tag或发布。候选尚未通过，不称完成或可部署。

覆盖只读复核：catalog326视图对应201个source，54 tabs与138 overlays均包含在其中，路径均存在；routes66条中39 page、11 dynamic、13 alias、3 layout，不把wrapper当新页面。UI1–18归属未发现漏项，但归属不等于全部视觉/交互已验证。Desktop原生Tray/help只读映射保留，外部iframe只负责MyAPI容器，不改变第三方界面。已完成页面及成熟共用壳延用，不重搭。

推送前读取工作流发现官网website-browser.yml的Chromium作业原先会随website/**推送自动执行，与本轮“浏览器先不跑”冲突。必要最小修正：新增默认false的run_browser手动输入，两浏览器作业均要求workflow_dispatch及显式开关；全部原浏览器步骤/断言保留。候选同提交官网样式与该授权门禁，另读取官网静态检查、静态打包及浏览器整作业跳过结果；NPM/GHCR/release仍为手动授权，Electron只tag/手动，本轮不触发发布或部署。

**仅测试红证据f6b4f042 / CI41529000899**：前端125564049206的认证专项180/180通过，剩余专项88文件60通过/28失败，657通过/126失败，另有1个suite未收集和7个未处理错误，原始日志已读；随后显式取消其余重复后端工作，不计完整绿。55剩余派发点（48 API+7真实入口）、3模型写、维护12调用中11个新增缺口的401重放已有真实Axios红证据，403/成功原body和3纯读取认证刷新对照保留。站点刷新/保存期间草稿覆盖、business false错误ack、performance未知子字段补零/新输入丢失/旧cleanup多GET亦有直接断言红；布局类部分先停在新增标签，不冒称后续深层断言已独立红。

明确排除的测试环境问题：Rankings suite被第三方VChart Node ESM加载阻断；Chat两个case的Key fixture缺used_quota/时间/model_limits_enabled，不能作为缓存/凭据泄漏的红证明；Dashboard模型统计旧初始化未达到首个GET，亦不据此声称迟到回显已复现。产品候选保真实schema补合法fixture，图表只隔离第三方renderer/ThemeManager，所有核心组件/数据和断言保留；日历测试补jsdom缺失getAnimations原生边界并恢复descriptor，不改浏览器。7个未处理错误中2 HTTP、1 revision conflict属于旧保存错误边界，2 CancelledError仍需候选CI确认，2 getAnimations属于测试环境，不能合并称产品7错。候选继续验证，不减断言、不提时限、不隐藏未处理错误。

**候选fc6a4984 / CI41530134230**：认证180/180、剩余专项88文件784通过/2失败，另1未处理pointer capture错误；新NoReplay/合法Key schema/图表采集/Performance与保存错误边界均已跑过，原始日志已读，仍不计完成。官网静态41530134226与打包41530134298完整成功，浏览器41530134251两作业整项跳过、0步骤，全部作业/步骤已读取。主CI前端红后取消重复后端，终态cancelled，不作完整绿。

两失败的最小修正仅测试：Passkey在Save/Reload前未改字段读取的observer尚未更新，保remote.synthetic.invalid等全部原期望，用默认waitFor等真实可见值；Sonner旧toast/actor用例补并还原jsdom三pointer capture API，保真实点击。跨月用例初始end为00:00却期待保23:59:59，原DateTimeRangePicker刻意保留显式时间；只将该输入fixture明确设为23:59:59并增加初始End time断言，精确出站payload与所有日期/跨月/内外Apply断言不变，不把产品改成覆盖零点。等待完整Actions再确认，无产品源或接口变更。

084a1ba4 / CI41531149395前端125571376511原始日志已读：认证180/180、剩余785/786，Passkey全部4例与真实409 Reload/草稿/身份断言通过，未处理错误0；唯一失败为Root新增初始时间检查把现有End Time写成End time。仅修正该标签的准确大小写，值23:59:59及原最终payload/日期/单日历/双层Apply断言保留，产品不改。已红主CI停止重复后端，不计完整绿；下一CI继续全量与types/lint/build全部门禁。

6dc353c6 / CI41531597732前端125573476564原始日志已读：认证180/180、剩余专项88文件786/786通过，无未处理错误；用户/资金296/297，唯一旧session-api.test断言要求用户策略PUT在401后刷新并重放，与本轮已批准的用户策略写NoReplay直接冲突。不改产品来保留写重放：将该原认证刷新正向用例改为getAssignedPolicy(userTarget)只读GET，保原返回身份/版本、1刷新/2派发与刷新后Authorization精确断言，并新增两次method/URL/无body断言。保存/删除写的401/403与完整CAS/body不重放由新增55调用专项真实Axios用例继续严格验证，已在此CI通过；Token原五类不重放断言不动。其他已存在的unscoped reveal正向仍保留默认行为，不把局部调用保护改成全局禁刷新。此旧合同同步不是放宽写入不重放，后续CI仍须全部通过。

e2ceaa66 / CI41532530830尚排队时，代理定向只读核对55关联既有测试，Root亲读确认另3文件4处旧精确调用期待缺新增flag：Key详情复制、usage-review reconcile/recover及分组详情reconcile。仅补精确undefined body/config skipAuthRefresh:true，原URL/金额/证据/确认/请求ID/一次派发和“不操作不请求”断言全保留，不改成any或objectContaining。此4处是静态合同同步，不冒称已在Actions失败；排队运行提前取消以免重复消耗，下一提交完整验证。未改产品源或全局默认重试。

**9964cd23 / CI41532865450前端125577675901**：全部原始日志与失败annotations已读。认证180、剩余88文件786、用户/资金297、模型173、公开33、账号85、Key388、原连接209、日志410及全量378文件2929/2929通过，同步与构建成功，无未处理错误。typecheck报6条：测试navigate模板/两处role exact选项、OAuth表单默认值推导、RateLimit可选JSON返回类型；lint21警告/11错误，涉及共享设置循环依赖与FastRefresh导出、两处curly、四处非空断言。不能把单测全绿当该提交完成；主CI已红后取消其余重复后端，不计完整绿。

必要修正由原UI15作者处理共享scope/错误模块边界与类型，另一代理仅改PromptLearning/Rankings测试的花括号/显式必要guard。保全部原值和完整assertions，不禁用lint、不放宽TS、范围与时限，不在本机执行工具链；下一完整Actions验证后才更新UI15–18为完成。

该质量候选精确22路径：UI15的2个必要.ts提取与17个既有调用/测试、PromptLearning与Rankings两测试、此清单。Root已读关键diff：原Context/Provider层级、generation/actor与错误WeakSet逻辑同源迁移，不留re-export循环；OAuth表单显式既有类型，无新any/cast；RateLimit当前getter为undefined时拒绝应用/删除，不回落成旧raw或空JSON。路由模板保同原params，role字符串仍精确匹配，非空断言换必要guard，其他全部assertions原样。无新key/版本/后端/依赖/检查规则改动，仍待Actions。

**96285d3d / CI41535822699**：前端125587253594完整成功，全部步骤及原始日志已读；认证180、剩余786、用户/资金297、模型173、公开33、账号85、Key388、原连接209、日志410、全量378文件2929/2929通过；同步/构建/typecheck/lint成功，21警告0错误，所有Chromium/依赖步骤跳过。尚非完整绿：分发125587253576原始日志显示124通过/4失败，皆由Root升级VERSION/package到beta.27时漏同步deploy/.env.example及两Compose默认镜像beta.26引起；CLI原版本/初始化/升级/签名保护断言不改。

必要版本修正只对齐仓库三个分发模板的默认tag到beta.27（继承原MYAPI_IMAGE/NEW_API_IMAGE显式覆盖与其它设置），不改任何运行中的.env/容器/服务器，不发布镜像/tag。此为原版本机制一致性补全，不实现CLI/P2新功能。官网相关源从fc6a4984到96285d3d无差异，复用已读的两个静态绿/两浏览器作业整跳过证据；后续模板修正不改变官网。已红主CI取消重复后端，下一提交仍读完整CI，不把前端绿写成应用完成。

- UI14：个人资料/认证25个UI源与6个新UI测试10例、19新键已冻结；Root另准备auth/profile/passkey/2FA原调用的21组真实Axios401/403/成功合同与inline reset派发flag，原proof/flow_token/轮换/倒计时/自动copy保留。候选未入当前UI13 CI，不宣称已通过；下一步仅测试旧源取证，再推产品与译文。
- UI15：20真实设置分区39个源（含四个内容集合的3个最小共享UI源）、5测试21例、9键冻结；保原保存时机、shared actor/frame/alive与dirty/成功ack，真实CAS 409显式Reload恢复，不改typed协议。唯一数值id0与unknown false/0保留，坏JSON不伪空。RateLimit新增__proto__原mutator边界仅记录为既有待办，本轮不称修复。
- UI16：22真实分区及同页SystemInfo两Panel，20源6测试13例、11新键冻结；expr.md已读，原报价/倍率/变量/len/单位/阈值/覆盖算法与真实路由不改。Root另准备settings/systemInfo/customOAuth12组真实Axios合同和维护inline4调用保护，未入当前UI13 CI。
- UI17：代理13源3测试10例、3键冻结；Root另有ModelsFilter复用单日历及2整合例、Chat active-key的actor cache/reveal scope与route文案/只读retry及2例WIP。Playground引擎/strict参数/附件/历史/编辑/abort保持；不发真实模型或学习任务。
- UI18：公共/法律/榜单/Setup/错误/手机导航17源、2旧测试加强、3新测试6例、7键冻结；website只改styles.css两类布局，原成熟方向3/隔离/Setup unknown恢复/品牌/旧链接保留。Desktop tray/help实际为electron/main.js原生菜单与messageBox，只有只读映射，不新建web页、不改其重启/写配置/平台架构。
- 最终API审查：55个可达派发点静态缺口已分类，Root亲核关键controller与入口后授权逐调用局部NoReplay与真实Axios合成。纯POST状态/preview/金额读取、数据库快照保原认证刷新；原request_id/operation_key/command_id/CAS/params/headers/signal与受理恢复不变，不改全局默认重试。无实际入口exports单列，不自动扩大。此为静态风险，未复现真实副作用。

主Goal持续执行，不逐批等确认。所有后续源码/测试分路径隔离，先完当前完整CI，后续旧源红不当成可部署版本。任何浏览器/真实模型/生产/发布/main仍不执行；本机仅源读写与轻量Git/文本检查。


### 收口交叉审查与必要补护（仅静态发现，未验收）

独立只读交叉审查了另一代理UI14/16/17与Root少量WIP，不把子代理汇报当运行证明。UI14所查securityScope/清secret/验证选择与会话流程保留；报价/倍率/len/serializer及Root/商业门禁未发现差异。实际5项待修：PromptLearning三写NoReplay（已归55 API补护）；Dashboard models local state与flow/users query缺actor tuple隔离；performance cache_stats子字段缺失/null仍??0；performance提交期间新输入可能被旧snapshot reset；旧performance读取/GC等结果在actor/frame销毁后仍可提示并GET。授权同一UI目标最小修正后4项给独立审查代理，复用既有shared preserveSettingsEdits/actor/frame/alive，算法/单位/阈值/normalize/payload不改，真实deferred/逐字段null0/输入中保存合成待Actions；旧已发操作不能宣称撤销。

55静态可达派发点的配置型补护计划仅16生产文件，源/tests分离：Channels常规13+显式探测/在线Codex10+敏感查看/凭据5、上游inline3、Key详情2、PromptLearning3、独立Settings10、其它usage-policy/access-policy-user/usage-review/full-content文件/Profile菜单语言9。Root已亲核Codex controller在线GET会采样/可刷新凭据，以及KeyActions与AccessPolicy user target默认配置，不能仅按HTTP GET/POST分类。纯POST get_key_status/assigned preview/金额估算与DB快照保原认证刷新，未使用exports单列不改。原proof headers、signal、CAS、operation_key/command_id/request_id与body不变，不改全局默认重试。两个新独立测试文件覆盖48 API点×401/403/成功与7实际入口，尚未运行/提交，不称已有真实重复副作用。
