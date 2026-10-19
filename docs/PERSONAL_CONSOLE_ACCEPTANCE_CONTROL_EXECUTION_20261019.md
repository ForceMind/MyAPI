# beta.21 隔离验收控制执行记录

## 已确认范围

维护者在前置核查交付后确认继续推荐方案。本批实施beta.21：单个既有Codex测试channel/account的持久验收run、总/分类HTTP预算、单生成并发、截止、停止与安全收尾。新SIWC入口仍为后续候选，不在本批。只用合成凭据、临时数据库、fake transport/loopback及既有Actions；真实账号/凭据、生产数据/Redis/备份、历史账务冻结，不main合并、标签、发布或部署。

起点`b6390e442d06f80b81a4e8e175744729ea90a433`、`codex/subscription-key-share-20261014`、beta.20。保留用户未提交的`docs/MYAPI_MASTER_PLAN.md`、`docs/NEXT_SESSION_HANDOFF.md`和设计草稿。沿用本轮Goal、已确认视觉与Key/账务安全边界。正常完成批次提交并推送既有分支/Draft PR #17；重型构建、三库、Redis、浏览器优先现有Actions。

## 实施与所有权

1. `acceptance_model`（gpt-6-astra/high）只写`model/upstream_acceptance_run.go`及同名测试；新增脱敏run表、条件UPDATE计数/预算/生成槽、owner/期限/停止栅栏。主代理负责`model/main.go`两个迁移入口注册。无新历史账务访问。
2. `acceptance_boundary_review`（gpt-6-astra/high）先只读核查后，只写`service/upstream_acceptance_http.go`及测试：专用局部HTTP1 transport，停用keepalive/透明HTTP重试/重定向，原共享client及来源资格不变，body结束幂等释放。此前Sol capacity已有证据，本批用既定可用Astra高风险组合；不递归。
3. 主代理写service运行生命周期、controller/路由、出站调用接入、调度保护、生成发送前/未发送收尾、版本/脚本/文档集成。只有主代理串行运行Go。最后安排非实现者独审与必要Actions。

每步先核对源码和接口依赖。默认模式关闭，普通实例及其他provider请求处理保持，验收进程整体后台采样暂停；验收模式不是全进程网络防火墙。只允许单个既有Codex channel/account，不支持新建channel=0的OAuth或reset-credit consume；其正常模式功能保持。进程owner栅栏防重启/其他节点自动续跑；不会把此范围冒称跨集群停止。

## 核心契约与验收

- 运行控制只接受Root真实维护者会话；PAT/普通Key不可启动/停止；写请求沿用既有控制台可信来源校验。run ID不是凭据。
- 所有受控OAuth换码/refresh、WHAM/模型（含client-version查询）和Codex生成都经过派发栅栏。预算统计每次派发尝试，失败/重试仍计数；未知/缺失scope拒绝。浏览器IdP导航不计入MyAPI进程HTTP数。
- 先登记逻辑operation，再逐HTTP原子申请预算。生成响应body未结束不可释放并发槽；refresh operation直到WithoutCancel有界持久化结束。停止先拒绝新准入并取消context，任何operation/body/必要写入未结束时返回stopping，全部排空才stopped。
- 最后HTTP拒绝且未发送时，同时清除现有text usage与subscription share未发送证据；已派发/未知结果不能伪称未发送。旧writer、轮换资格、近似原生池及缺usage成功规则保持。
- 定向合成测试先验证预算/范围/并发/停止/重启/失败/正常模式保持，三数据库兼容由既有CI补验；版本统一递增beta.21。若本批无UI改动，实际运行证明为真实Go控制接口/脚本，不虚构新页面截图；原个人页面视觉证据沿用并作回归。

## 部署与新会话节点

本批交付不自动进入部署。候选验收通过后，再独立授权真实测试账号；实际旧路线可用时不必等待可选SIWC入口。已有实例升级仍需目标只读核查、历史兼容门禁和原生备份恢复，最后对准确制品/目标/维护窗口单独批准。不为赶部署解冻历史或切writer。

推荐在beta.21收口、提交和证据齐备后开新会话，再进入真实测试资料/授权阶段。届时提供准确HEAD、版本、Goal/完成项、剩余门禁、用户未提交文件和禁止事项的续接提示词；目前不能给可靠部署日期。

## 状态

实现及本地定向验证完成，等待本批提交的Actions。控制接口与操作边界见[接口说明](PERSONAL_CONSOLE_ACCEPTANCE_CONTROL_API_20261019.md)。真实路由测试已证明Root会话/Origin、计数与停止；两writer发送边界8例通过；真实TokenAuth→Distribute→Relay验证HTTP结束后仍等结算与handler返回才完成停止。刷新成功后停止仍安全写入合成旋转凭据；CLI三例通过。

`acceptance_final_audit`（gpt-6-astra/high，非实现者）只读复核源码及原始测试输出，未发现阻断交付的P1/P2。剩余条件为当前提交的三库、race与完整CI；本地MySQL/PostgreSQL明确跳过，不能据此认定三库通过。测试中的Redis projection unavailable来自临时fixture，不作为Redis集成通过证据。真实账号/供应商兼容性与部署尚未执行。

首次CI39218472756分发合同发现此前归档的`docs/evidence/**/*.log`被`docs/`打包白名单纳入。新增证据目录的嵌套`.npmignore`排除验收档案，并在pack校验中禁止整个证据目录；未放宽原有安全检查，运行源码/测试保持包含。干净archive副本加合成canary后实际manifest/pack验证通过；本机npm缓存改用`/tmp`以适应写入边界。独审确认修正范围合理，准确修正提交将重新跑既有CI。
