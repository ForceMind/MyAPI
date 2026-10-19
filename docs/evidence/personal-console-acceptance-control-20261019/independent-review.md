# beta.21 独立源码与隔离证据审查

审查者：`acceptance_final_audit`，gpt-6-astra/high，未参与实现；只读源码与已有原始输出，未自行运行 Go 或远程操作。时间：2026-10-19。主代理整理，保留审查适用范围。

结论：最终源码与本地证据审查通过，未发现需要阻断本批交付的 P1/P2 产品缺陷。生成链验证缺口已补齐；当前提交的三库及完整 CI 尚待最终结果，真实账号、供应商兼容性及部署未验证。

已核对：

- `relay/channel/upstream_acceptance_dispatch_test.go`：两 writer 共8例通过。最终准入拒绝零上游发送，正确清理未发送证据并退款；已发送后 EOF 保留不确定性且拒绝退款，上游实际一次。late-budget 用同事务状态注入确定性模拟最后额度被占用，不冒充真实第二次物理发送。
- `router/upstream_acceptance_relay_lifecycle_test.go`：真实 TokenAuth→Distribute→controller.Relay，HTTP结束后operation仍存在，Stop返回stopping，正常结算及handler返回后才stopped；未手工预绑定或调用finisher。正常使用原计费路径，Key身份与渠道凭据保持。
- 实际控制路由的会话、权限、Origin、预算和停止通过；service证据证明范围、存储失败锁存和停止后的旋转凭据安全落库。
- 来源资格检查保持；只有明确未调用transport的错误才清除未发送证据。准入数据库操作有5秒context；Stop先锁存、取消，再持久化与排空。

较早审查提出的四项问题已修正并复核：渠道诊断operation遗留、后台模型更新误占预算、停止等待无界数据库操作、到期后只能由GET推动状态。当前分别采用验收模式拒绝渠道诊断生成、暂停Codex后台模型更新/只接受显式context手动入口、有界数据库读取和取消先行、Start也处理到期收尾。

日志内 `redis projection unavailable` 属于隔离fixture，不能作为Redis集成通过证据。实际三库/Redis/race/完整CI结论由本目录交付索引中的准确运行报告另行说明；本记录不冒称审查者执行或验证真实上游。

文档补审：已修正Root非同SID、既有可信Origin/Referer规则、读取失败与写失败锁存区别、整个验收进程后台采样暂停，以及create未知ID恢复边界；明确预算正常拒绝不锁存drain_error。审查者回读后确认这些勘误与功能提交合同一致，文档审查通过。部署日期及本批收口后新开会话的节点合理；当时CI仍在进行，未被记作通过。

分发修正补审：CI实际发现旧验收.log被docs白名单打包。审查者只读核对docs/evidence/.npmignore和check-pack的整目录禁止规则，确认仓库证据保留、源码/测试未被排除、既有秘密/manifest断言未减弱；未发现阻断问题。实际干净fixture manifest/pack与tracked canary排除由主代理另行执行并通过。

三库fixture补审：第二CI的首个PostgreSQL失败发生在expiry子例清理。既有测试工厂通过postgres.Config.Conn显式注入连接池；重用dialector导致重开与原连接共享池，关闭前者影响后续测试。只改fixture以重新构造dialector/独立池，断言与产品代码不变；独审确认根因和修正合理，无阻断问题。SQLite本地复验通过，实际PostgreSQL仍等待最终准确CI。

首批图像补审：20/20原PNG实际逐张查看（8迁移、6轮换、6高级维护），无新增P1/P2。原M/柔蓝/桌面侧栏与手机底栏一致，未见横向溢出。两张fresh-secret-masked为深色遮罩，遮罩内不验。滚动弹窗首屏不含底部动作不直接判遮挡；对应报告有操作/几何验证。两张窗口替换图functional=false，仅作展示。14张凭据图对应真实Go/Chromium与合成凭据/SQLite，其他读取仍有合成API；不能称本地生产或供应商验收。来源index/reports、0eadc1ac构建与c5e44f6同tree一致。后续页面12图待补审。

最终图像补审：新增12/12实际逐张查看，累计32/32原PNG全部查看，无P1/P2视觉阻断。1440/390页面及1280/320 Key列表未见横向溢出、重叠或明显破损。五页没有金额余额；完整回答缺usage仍成功/Token未知；失败单独呈现；手机上游明确采样已过期，未冒充新鲜快照。Key保留近似及归属不确定性提示。32图未见洋红，秘密图为深色遮罩。个人16、工作台27、分额28流程原报告通过/错误列表空；构建0eadc1ac与beta.21一致。14图是真实Go加合成DB，18图是编译前端加合成API，不证明真实供应商/429/生产；当时完整CI尚未结束。

CI最终收口：主代理回读准确运行39220781498的API结果，11/11作业成功；c5e44f6候选与0eadc1ac checkout同tree，三库新run48叶子、控制race、全部Go/relaykit/386、前端与Chromium通过。此处为主代理最终验证记录，不冒称独审者自行执行CI。

最终交付复核（非实现者）：独审者逐项核对manifest88文件与原始索引45项（32PNG+13报告），bytes/SHA256一致，无缺漏或重复；准确CI39220781498 completed/success、11/11及新增控制race成功，checkout与c5e44f6同tree。交付/部署/续聊/证据说明没有夸大真实供应商、生产部署或发布状态，无阻断项。README未来时按建议勘误后由主代理重建manifest与暂存字节检查；只读审查未改文件、未执行测试或远程操作。
