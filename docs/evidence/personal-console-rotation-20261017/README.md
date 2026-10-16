# beta.19 原 ID 单值轮换证据

状态：最终完整CI的11项作业全部成功，真实Go/浏览器、三库、Redis、race及相关实图审查通过。精确结果见verification-summary.json和交付记录。

- 分支资格源码：`5536cf8601afe512bbefb92e86b99b038398a133`。
- CI merge checkout：`0c72690c19f1a738f61d6ec54c86c470f679f27f`，与分支文件树均为`ad2289bf3a13b3f3ea354d9e5705a8120456545f`。
- [CI38935299710](https://github.com/ForceMind/MyAPI/actions/runs/38935299710)。浏览器/真实Go job117089700365；三库job117089700451；Redis job117089700271。
- 原图与报告包：myapi-credential-maintenance-0c72690c19f1a738f61d6ec54c86c470f679f27f，artifact SHA256 `bc9de85c2530e47e4da4d727cc1e22796f19ef4533e9d59012ce87b756e6fe40`。
- 准确前端包：myapi-console-build-0c72690c19f1a738f61d6ec54c86c470f679f27f，artifact SHA256 `57b8275ab3a999a60bf2813fa78ee63a8942875fa1b2ce66bf51e60f1e86a2a4`。

## 原始文件

| 目录 | PNG / JSON | 证明范围 |
| --- | --- | --- |
| myapi-rotation-runtime-artifacts | 6 / 2 | 1440英文、390中文生产前端 → 真实Go Root会话/2FA/轮换/回执/重放；旧Key401、新Key200同ID；Go回读历史/预算/策略不变 |
| myapi-credential-runtime-artifacts | 8 / 2 | 两设备真实Go迁移保留；有效rotation proof业务409/not_accepted、零轮换回执、legacy writer不变 |
| myapi-credential-rotation-artifacts | 9 / 1 | 生产前端+明确synthetic APIs，含fresh、390预检重试、320越南语深色unknown同op恢复、blocked |
| myapi-credential-browser-artifacts | 13 / 1 | 既有维护合成异常及键盘/重试/unknown回归 |

总计36张原PNG、6份原JSON。按下载字节原样复制，没有裁切、重画、改图或事后遮盖。敏感输入在Playwright拍摄前由测试遮罩；只遮值，不遮标签/说明/操作。`original-manifest.json`记录每文件长度、SHA256和原图分辨率；不包含数据库、私密browser配置、JWT、proof或原secret。

## 读者路径

- `database-summary.json`：SQLite、MySQL5.7、PostgreSQL9.6的轮换/迁移/采样合同，及独立Redis7实例的缓存栅栏/零写/提交后unknown恢复。
- `review-summary.json`：独立于所审UI与Root集成实现的credential_guard实际查看23张相关原图和5份报告的范围、已关闭问题和未验证边界。旧13张合成图本轮不重复计入人工查看数。
- `preview-summary.json`：本地4176准确前端的只读HTTP核对、构建和隔离范围；仅假Key内存交互，不能当作真实Go证据。
- `verification-summary.json`：最终完整CI、前端单元、构建、race、原始作业/来源摘要。
- [交付记录](../../PERSONAL_CONSOLE_ROTATION_DELIVERY_20261017.md)、[接口合同](../../PERSONAL_CONSOLE_ROTATION_API_20261017.md)、[实施与修正](../../PERSONAL_CONSOLE_ROTATION_EXECUTION_20261016.md)。

截图证明静态显示；交互及真实API由原报告与Go不变量测试共同证明。真实Go同样使用临时SQLite和合成凭据，其他控制台读取明确fixture；不证明真实provider、生产、混合旧worker或实体Passkey。历史账务与生产凭据/数据冻结，没有部署、发布或main合并。
