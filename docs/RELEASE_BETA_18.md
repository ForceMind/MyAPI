# MyAPI 0.2.0-beta.18 候选说明

状态：功能分支候选已通过隔离验收，未发布、未标签、未部署；不合并 main。上一批 beta.17 资格与原图保持有效历史，本批验收另记。

本批增加 Root 本人 Key 的凭据维护预检、专用安全验证、单 Key 跨 Codex 上游账号原子迁移及脱敏回执；保持 owner、Token ID、secret、旧主体历史和现有目标用量。界面沿用已确认 visual-v0.2 和七语言。

同账号 OAuth 刷新、重新授权和导入补充主体连续性保护，普通渠道编辑不得隐式换账号。API Key 原 ID 单值轮换尚缺在途请求与缓存兼容资格，本候选保持只读，不注册轮换提交。

只使用合成/隔离环境。Token/费用预算与窗口分额互斥；近似误差明确，unknown 不推断为零用量或无提交。历史账务、生产数据和真实凭据继续冻结。

接口和实施记录见 [API 合同](PERSONAL_CONSOLE_CREDENTIAL_API_20261016.md)、[实施记录](PERSONAL_CONSOLE_CREDENTIAL_EXECUTION_20261016.md)。准确源码6274bac、CI38911014066的11项成功作业、1519单元、三库/真实Go维护、21张原图和独立审查已归档，见[交付记录](PERSONAL_CONSOLE_CREDENTIAL_DELIVERY_20261016.md)。
