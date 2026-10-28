# -*- coding: utf-8 -*-
from pathlib import Path
import csv
import sys
import html
import io
import json
import re
import textwrap

ROOT = Path(__file__).resolve().parents[5]
OUT = ROOT / 'docs/evidence/redesign-20261024/ui-design-v2/wireframes'
BASE = 'c916051e1ad5d14655829d742f0e7d5efac373b7'
pages = []
route_map = []

def add(id, title, group, url, kind, source, *, sections=(), fields=(), columns=(), filters=(), actions=(), tabs=(), stats=(), notes=(), merge='', access='沿用现有身份、角色和服务端权限', status='设计提案，待维护者确认'):
    def items(value):
        return [item for item in (value.split('|') if isinstance(value,str) else value) if item]
    page = dict(id=id, title=title, group=group, url=url, kind=kind,
                sources=[source] if isinstance(source, str) else source,
                sections=[dict(title=s[0], items=s[1].split('|')) for s in sections],
                fields=items(fields),columns=items(columns),filters=items(filters),
                actions=[dict(label=a[0], target=a[1], effect=a[2]) for a in actions],
                tabs=list(tabs), stats=list(stats), notes=list(notes), merge=merge,
                access=access, status=status)
    pages.append(page)
    return id

def A(label, target, effect='仅跳转到对应线稿，不发出产品请求'):
    return (label, target, effect)

COMMON_NOTES = [
    '所有数值和内容均为合成占位；未知值显示“— / 未知”，不补零。',
    '线稿只定义布局和交互意图，不证明产品已实现或浏览器已验收。',
    'Full / Lite / Desktop 与用户 / 管理员 / Root 沿用既有能力和权限，不因合并入口而扩权。',
    '手机高频触控目标至少44px；桌面保留键盘焦点、可访问名称及清晰的禁用状态。',
]

add('request-bodies', '正文与 SSE', '01 请求工作区', '/full-content-logs', 'table',
    'web/src/features/full-content-logs/index.tsx',
    filters='日期范围〔同一日历〕|模型|API Key|Request ID',
    stats=['匹配请求 —', '日志文件 —', '磁盘占用 —'],
    columns='时间|模型 / 端点|API Key|HTTP 状态|请求 / 响应大小|耗时|查看内容',
    actions=[A('查询', 'request-bodies', '应用筛选并回到第1页；仅GET列表'), A('重置', 'request-bodies', '清空筛选，不删除日志'), A('日期范围', 'date-range'), A('刷新', 'request-bodies', '只重新读取当前筛选结果'), A('日志存储', 'body-files'), A('查看内容', 'body-detail'), A('手机：展开筛选','request-filters','仅手机显示；展开完整字段，应用前不改变已用筛选')],
    notes=['分页含首尾页、页码、20/50/100条选择与输入页码直接跳转；不一次拉取所有正文。', '页码、每页条数、筛选和焦点在返回时保留；缓存继续按用户与会话隔离。', '记录可检索不等于有完整正文；未采集、已过期、截断和加载失败分别说明。', '打开记录只读，绝不重放生成请求。'],
    merge='请求工作区共享筛选和分页；正文权限保持独立，不把正文复制到普通用户的调用记录缓存。', access='管理员；沿用既有正文路由和API权限')

add('request-filters','正文与 SSE · 手机筛选展开态','01 请求工作区','/full-content-logs〔手机筛选〕','overlay',
    'web/src/features/full-content-logs/components/full-content-log-filter-bar.tsx',
    fields='日期范围〔同一日历〕|模型|API Key名称 / ID|Request ID',
    actions=[A('应用并查询','request-bodies','仅GET列表，应用当前草稿并回到第1页'),A('日期范围','date-range','在同一个日历选择起止日期；取消时保留已应用值'),A('重置筛选','request-filters','清空草稿，不删除日志'),A('取消','request-bodies','返回已应用的筛选和原列表，不发送查询或写入')],
    notes=['展开态与正文列表主屏分开画；默认一行筛选摘要，避免手机首屏全被表单占据。','键盘提交与查询按钮等价；日期无效或起止倒置时就近报错并保留草稿。'])

for id, title, section, columns, extra in [
    ('requests', '调用记录', 'common', '时间|Request ID|模型|API Key|业务结果 / HTTP|用量证据|结算|耗时', '用量可信度、结算状态、业务结果和HTTP状态分开，未知用量不当作0。'),
    ('drawing', '绘图记录', 'drawing', '时间|任务ID|平台 / 模型|动作|业务状态|计费证据|结果', '预览只读；失败原因与可用结果分开，不把“查看”做成重新生成。'),
    ('tasks', '后台任务记录', 'task', '时间|任务ID|平台|动作|运行状态|用量 / 费用|结果', '等待、成功、失败、结果未知分别展示；不因刷新重新提交任务。'),
]:
    add(id, title, '01 请求工作区', '/usage-logs/' + section, 'table', 'web/src/features/usage-logs/index.tsx',
        filters='日期范围〔同一日历〕|模型 / 平台|API Key|结果 / 状态|Request ID / 任务ID',
        columns=columns, stats=['匹配记录 —', '成功 / 失败 —', '用量未知 —'],
        actions=[A('查询', id, '只查询当前过滤条件'), A('日期范围', 'date-range'), A('查看详情', 'request-detail' if id == 'requests' else 'task-detail'), A('正文与 SSE', 'request-bodies')],
        notes=[extra, '共用页码、首尾页、每页条数和直接跳页；手机记录卡保留主要状态与详情入口。'],
        merge='调用、绘图、任务放在同一请求区的明确页签内；沿用各自API和权限，不混合总数。')

for base, source, url, title in [
    ('request', 'web/src/features/usage-logs/request-page.tsx', '/usage-logs/request/$requestId', '请求详情'),
    ('key-request', 'web/src/routes/_authenticated/keys/$keyId/requests/$requestId.tsx', '/keys/$keyId/requests/$requestId', 'Key 下的请求详情'),
]:
    # The route and the actual request-page component are both retained below.
    if not (ROOT / source).exists():
        source = 'web/src/routes/_authenticated/usage-logs/request/$requestId.tsx'
    for suffix, tab, content in [
        ('detail', '诊断', [('诊断时间线', '接收请求|鉴权与选路|派发证据|终止 / 结算证据'), ('请求身份', 'Request ID（合成）|API Key（掩码）|模型 / 协议|连接身份')]),
        ('body', '正文', [('已保存请求', '请求头〔脱敏〕|请求JSON / 文本|未采集时说明原因'), ('已保存响应', '响应头〔脱敏〕|响应正文|截断字节与完整大小')]),
        ('sse', 'SSE', [('事件流', '序号 / 相对时间|事件类型|展开事件数据|仅本地文本搜索'), ('证据边界', '结束标记是否存在|业务错误与HTTP分开|缺失不推定成功')]),
        ('raw', '原始证据', [('原始只读记录', '原始字段 / 元数据|来源与采集时间|脱敏与截断标识')]),
    ]:
        pid = base + '-' + suffix
        add(pid, title + ' · ' + tab, '01 请求工作区', url + '?tab=' + suffix, 'detail', source,
            stats=['业务结果 未知', 'HTTP —', '用量证据 未知', '结算 未知'], sections=content,
            tabs=[(base + '-detail', '诊断'), (base + '-body', '正文'), (base + '-sse', 'SSE'), (base + '-raw', '原始')],
            actions=[A('返回列表', 'keys' if base == 'key-request' else 'requests', '恢复原筛选、页码和焦点'), A('复制 Request ID', pid, '仅复制当前已显示的ID'), A('查看已存正文', 'body-detail')],
            notes=['打开和切换页签只读取既有证据，绝不调用供应商或重放请求。', '正文只在既有权限允许时显示；禁止把上个用户/会话的内容作为加载占位。'],
            merge='Key 请求页与调用记录请求页复用同一详情内容；两个入口和返回上下文保留。')

add('task-detail', '绘图 / 任务详情', '01 请求工作区', '/usage-logs/task〔详情状态〕', 'detail', 'web/src/features/usage-logs/index.tsx',
    sections=[('任务身份', '任务ID|平台 / 模型|动作|创建与完成时间'), ('结果与证据', '上游状态|业务失败原文|结果链接〔脱敏〕|费用 / 用量证据')],
    actions=[A('返回结果列表', 'tasks'), A('复制任务ID', 'task-detail', '仅复制ID，不提交任务')],
    notes=['绘图与异步任务各保留自己的字段与平台语义；共用详情外壳，不把成功状态或用量未知互相转换。'])
add('body-detail', '已保存正文 / SSE 内容详情', '01 请求工作区', '/full-content-logs〔查看内容〕', 'detail',
    'web/src/features/full-content-logs/components/full-content-log-details-dialog.tsx',
    sections=[('请求内容', '已脱敏请求头|请求体|编码与内容类型'), ('响应内容', '已保存正文 / SSE事件|视图切换：可读 / 原始|截断与未采集说明')],
    stats=['Request ID 合成示例', '响应大小 —', '截断 未知'],
    actions=[A('返回正文列表', 'request-bodies', '保留页码与筛选'), A('查看关联请求', 'request-detail'), A('切换原始视图', 'request-raw')],
    notes=['设计提案：统一为完整请求详情的内容区，移动端整页；不新增第二套正文权限。', '按明确操作才读取大正文，限制单次响应大小，读失败时不显示上条记录。'],
    merge='合并重复的正文查看外壳，保留普通记录与管理员正文之间的权限分界。')
add('body-files', '正文日志存储管理', '01 请求工作区', '/full-content-logs〔日志存储〕', 'table',
    'web/src/features/full-content-logs/components/full-content-log-files-dialog.tsx',
    columns='文件名|修改时间|大小|下载|删除', stats=['文件数 —', '占用 —'],
    actions=[A('返回正文列表', 'request-bodies'), A('下载选定文件', 'body-files', '产品实现需明确点击；线稿不下载'), A('删除选定文件', 'confirm-delete', '进入单文件删除确认，不直接删除'), A('清理全部文件', 'confirm-delete', '危险操作，逐次明确确认')],
    notes=['删除与下载不藏在模糊图标里；删除范围、不可逆说明及权限同时展示。'])

add('overview', '概览 · 流向板', '02 核心对象', '/dashboard/overview', 'flow', 'web/src/features/dashboard/index.tsx',
    stats=['请求 —', '失败 —', '耗时 —', '用量未知 —'], filters='时间范围|API Key|连接',
    sections=[('API Key 节点', '合成 Key A〔掩码〕|请求 / 失败证据|查看全部 Key'), ('网关核心', '接入地址〔合成〕|Full 用户 / 分组摘要|Lite / Desktop 本机地址与访问模式'), ('连接节点', '合成连接 A|剩余额度：未知|最近观察状态')],
    columns='时间|模型|API Key|连接|业务结果|用量证据',
    actions=[A('创建 API Key', 'key-edit'), A('添加连接', 'connection-edit'), A('查看请求', 'requests'), A('模型分析', 'analytics-models'), A('连接列表', 'connections')],
    notes=['选中节点只筛选下方请求；合成值不能伪装成实时统计。', '保持已确认方向3：顶栏、居中内容、对象整页；手机按Key→网关→连接纵向排列。'])
for pid,title,url,cols,description in [
    ('analytics-models','模型调用分析','/dashboard/models','模型|请求数|失败|用量 / 成本|耗时','模型系列前5项用设计色，其余中性灰；线稿只表达结构。'),
    ('analytics-users','用户调用分析','/dashboard/users','用户|请求数|失败|用量 / 成本|最近调用','仅管理员；不把用户明细与普通用户聚合缓存混用。'),
    ('analytics-flow','流量与性能','/dashboard/flow','时间段|请求|并发 / 性能|失败|采样状态','聚合窗口、数据来源与缺失状态可见；不补造未采集的性能值。'),
]:
    add(pid,title,'02 核心对象',url,'analytics','web/src/features/dashboard/index.tsx', filters='日期范围〔同一日历〕|模型|分组 / 范围', columns=cols, stats=['请求 —','失败 —','用量 —','耗时 —'],
        sections=[('趋势图', '时间横轴|请求与失败序列|图例与数值表'),('分布图','模型 / 用户分布|长名称换行|表格替代视图')],
        actions=[A('应用筛选',pid),A('日期范围','date-range'),A('查看匹配请求','requests')],notes=[description])
add('keys','API Key 列表','02 核心对象','/keys','table','web/src/features/keys/index.tsx',
    filters='搜索名称 / ID|状态|模型范围|分组', columns='名称 / 掩码|启用状态|模型范围|预算 / 分额|最近请求|操作',
    actions=[A('创建 API Key','key-edit'),A('查看 Key','key-requests'),A('编辑','key-edit'),A('更多操作','key-scope')],
    notes=['高频启停、查看、复制交付入口清晰；删除和凭据轮换保持独立确认。','列表返回保留筛选、页码、已选对象和焦点。'])
for tab,title,sections in [
    ('requests','最近请求',[('该 Key 的请求','稳定Key ID筛选|业务结果|用量证据|打开请求详情')]),
    ('scope','范围与分额',[('访问范围','模型范围|分组 / 访问配置|状态'),('预算与分额','有限Key预算|订阅池分额|保存的额度策略')]),
    ('delivery','交付',[('接入信息','Base URL〔合成〕|API Key掩码|环境变量 / curl示例'),('安全说明','凭据仅明确操作后显示|复制与轮换分离|不自动发送测试')]),
]:
    add('key-'+tab,'API Key 详情 · '+title,'02 核心对象','/keys/$keyId?tab='+tab,'detail','web/src/features/keys/key-page.tsx',
        stats=['状态 —','剩余预算 未知','近期请求 —'],sections=sections,tabs=[('key-requests','请求'),('key-scope','范围与分额'),('key-delivery','交付')],
        actions=[A('返回 Key 列表','keys'),A('编辑','key-edit'),A('查看请求','key-request-detail'),A('凭据维护','credential-maintenance')],
        notes=['对象详情整页；模型范围、分额、凭据维护仍受现有权限与状态机控制。'])
add('connections','连接 · 卡片视图','02 核心对象','/connections','cards','web/src/features/connections/connections.tsx',
    filters='名称 / 模型搜索|供应商|状态|优先级',stats=['连接 —','可用 —','未知 —'],
    sections=[('连接 A〔合成〕','类型 / 状态|额度环：未知|启用开关|查看 / 测试 / 编辑'),('连接 B〔合成〕','优先级 / 权重|最近观察|不可用原因|查看 / 测试 / 编辑'),('添加连接','同一对象的卡片和表格视图|配置入口集中|不自动测试')],
    actions=[A('添加连接','connection-edit'),A('切换表格','channels'),A('查看连接','connection-windows'),A('测试连接','provider-test','产品中需主动触发，可能产生供应商用量；线稿不联网')],
    merge='连接与渠道不再表现成两套管理功能：卡片 / 表格是同一对象的两种视图；旧地址保留。')
add('channels','连接 · 表格视图（原渠道）','02 核心对象','/channels','table','web/src/features/channels/index.tsx',
    filters='名称 / 模型搜索|类型|状态|分组',columns='名称|类型|状态|优先级 / 权重|余额证据|最近测试|操作',
    actions=[A('添加连接','connection-edit'),A('切换卡片','connections'),A('查看连接','connection-windows'),A('批量操作','confirm-danger')],
    merge='与连接卡片共用筛选、对象详情、编辑器和动作；保留管理员渠道表格所需的高级列。')
for tab,title,sections in [
    ('windows','账号窗口',[('窗口与额度','窗口周期|已用 / 剩余证据|分额与Key关联|刷新观察'),('保存状态','采样时间|来源|未知原因|本次未发起供应商请求')]),
    ('routing','路由与模型',[('路由配置','优先级 / 权重|模型映射|分组范围|故障切换边界'),('模型目录','允许模型|协议能力|配置摘要|编辑入口')]),
    ('requests','近期请求',[('此连接的请求','时间|稳定连接ID|模型与Key|业务结果 / 用量证据')]),
    ('history','历史',[('变更与观察历史','操作时间|变更来源|窗口观察|失败证据')]),
]:
    add('connection-'+tab,'连接详情 · '+title,'02 核心对象','/connections/$connectionId?tab='+tab,'detail','web/src/features/connections/connection-page.tsx',
        stats=['启用状态 —','余额 / 窗口 未知','最近观察 —'],sections=sections,
        tabs=[('connection-windows','窗口'),('connection-routing','路由与模型'),('connection-requests','请求'),('connection-history','历史')],
        actions=[A('返回连接列表','connections'),A('编辑','connection-edit'),A('测试连接','provider-test','明确测试动作才允许联系供应商；线稿不执行'),A('配置分额','subscription-shares')])
add('upstream-accounts','账号窗口总览','02 核心对象','/upstream-accounts','cards','web/src/features/personal-upstream/index.tsx',
    stats=['账号 —','可用窗口 —','额度未知 —'],filters='账号 / 连接|窗口周期|状态',
    sections=[('合成账号 A','关联连接|窗口进度|剩余额度：未知|采样状态'),('合成账号 B','观察时间|错误说明|范围与分额|打开连接')],
    actions=[A('打开连接','connection-windows'),A('查看分额','subscription-shares'),A('刷新已存观察','upstream-accounts')],
    merge='账号窗口与连接详情共用额度、窗口和分额组件；总览保留跨连接比较能力。')
add('models','模型目录','02 核心对象','/models/metadata','table','web/src/features/models/index.tsx',
    filters='模型 / 供应商|协议|能力|状态', stats=['模型 —','供应商 —','能力未知 —'],columns='模型名称|供应商|输入 / 输出能力|价格来源|连接覆盖|操作',
    actions=[A('新建 / 编辑模型','model-edit'),A('预填分组','prefill-group'),A('模型定价详情','model-price'),A('部署','deployments')],
    notes=['数据目录和实际路由可用性分开；不把目录存在当作上游已测试可调用。'])
add('deployments','模型部署','02 核心对象','/models/deployments','table','web/src/features/models/index.tsx',
    filters='名称 / 状态|区域|供应商',columns='部署名称|供应商 / 区域|模型|状态|创建时间|操作',
    actions=[A('创建部署','deployment-edit'),A('查看配置','deployment-edit'),A('删除部署','confirm-delete')],access='Full，按既有部署能力与权限显示',
    notes=['仅设计既有部署入口；线稿不创建云资源、不调用外部供应商或计费服务。'])

add('users','用户管理','03 管理与账户','/users','table','web/src/features/users/index.tsx',
    filters='用户名 / ID|角色|状态|分组',columns='用户|角色 / 分组|状态|余额与用量|最近活动|操作',stats=['用户 —','启用 —','需关注 —'],
    actions=[A('创建用户','user-edit'),A('编辑用户','user-edit'),A('使用策略','usage-policy'),A('分配访问','assigned-access')],
    access='Full 管理员；角色与Root边界保留', notes=['手机优先显示身份、状态、用量证据及详情入口；高级字段仍可完整访问。'])
add('subscriptions','订阅方案管理','03 管理与账户','/subscriptions','table','web/src/features/subscriptions/index.tsx',
    filters='方案名称|状态|分组',columns='方案|价格 / 周期|额度与重置|分组影响|启用状态|操作',
    actions=[A('创建方案','plan-edit'),A('编辑方案','plan-edit'),A('用户订阅','user-subscriptions'),A('停用 / 重置','confirm-danger')],
    access='Full，管理员方案管理权限',notes=['方案编辑、购买、用户分配和历史是不同动作，不能共用一个含糊的“确认”。'])
add('redemptions','兑换码管理','03 管理与账户','/redemption-codes','table','web/src/features/redemption-codes/index.tsx',
    filters='名称 / ID|使用状态|日期范围',columns='名称 / 掩码|额度|状态|到期时间|使用者|操作',
    actions=[A('创建兑换码','redemption-edit'),A('编辑','redemption-edit'),A('删除选定项','confirm-delete')],access='Full 管理员',
    notes=['复制 / 导出明文码为明确动作；手机记录不隐藏到期与使用状态。'])
add('wallet','钱包与资金','03 管理与账户','/wallet','cards','web/src/features/wallet/index.tsx',
    stats=['钱包余额 未知','已用额度 未知','订阅状态 —'],
    sections=[('充值 / 兑换','资金能力已确认后才启用|充值金额 / 方式|兑换码'),('我的订阅','当前方案与到期|额度与周期|只读历史入口'),('奖励与邀请','有效奖励|可转入金额|明确转入操作')],
    actions=[A('查看资金历史','funding-history'),A('确认充值','payment-confirm'),A('购买订阅','subscription-purchase'),A('兑换','confirm-danger')],
    notes=['资金模式未知、停用、退役分别展示；历史可见不代表允许新充值。', '仅页面设计，不改充值 / 结算 / 退款规则，也不接支付网络。'],
    merge='余额、充值、兑换、我的订阅分为清晰区块；资金历史统一入口，管理订阅仍在管理员区域。')
add('funding-history','资金与订单历史','03 管理与账户','/wallet?history=true','table','web/src/features/wallet/components/dialogs/billing-history-dialog.tsx',
    filters='日期范围〔同一日历〕|订单状态|支付方式',columns='订单ID|创建时间|金额|支付方式|状态|处理记录',
    actions=[A('返回钱包','wallet'),A('查看订单','order-detail'),A('日期范围','date-range')],
    notes=['区分待支付、已付、退役与结果未知；读取历史不重放支付请求。'])
for tab,title,sections in [
    ('info','基本资料',[('账户资料','用户名 / 昵称|邮箱|密码修改入口|余额提醒阈值〔能力允许时〕'),('偏好','语言|主题 / 字号|控制台入口偏好')]),
    ('security','安全与登录',[('身份验证','Passkey|两步验证|访问令牌维护'),('登录会话','当前会话标识|设备 / 时间|逐项撤销|撤销其他会话确认')]),
    ('subscription','我的订阅',[('只读订阅','方案名称|当前周期|剩余证据|到期时间'),('账户功能','签到入口〔能力允许时〕|奖励记录|历史资金入口')]),
]:
    add('profile-'+tab,'个人资料 · '+title,'03 管理与账户','/profile〔'+tab+'〕','form','web/src/features/profile/index.tsx',sections=sections,
        tabs=[('profile-info','资料与偏好'),('profile-security','安全与登录'),('profile-subscription','我的订阅')],
        actions=[A('保存当前修改','profile-'+tab,'提交前校验，仅影响当前明确编辑的资料'),A('登录会话','login-sessions'),A('提示词学习','learning-policy')],
        notes=['个人资料拆为明确的子区；Lite 的订阅只读能力不变；隐藏商业写入不隐藏已有历史。'],
        merge='整理长页面为资料、安全、订阅三个清晰分区；所有现有可用能力仍可达。')
for tab,title,columns in [('instances','实例','实例ID|版本|数据库 / 缓存|在线状态|最后心跳'),('jobs','后台任务','任务名称|运行状态|最近执行|错误摘要|来源实例')]:
    add('system-'+tab,'系统信息 · '+title,'03 管理与账户','/system-info〔'+tab+'〕','table','web/src/features/system-info/index.tsx',
        stats=['在线实例 —','失联实例 —','活动任务 —','失败任务 —'],columns=columns,
        tabs=[('system-instances','实例'),('system-jobs','后台任务')],actions=[A('刷新已存状态','system-'+tab),A('系统维护设置','settings-operations-update-checker')],
        notes=['失败与未知状态有独立说明；不把读取状态实现成重启 / 重跑任务。'])

for id,title,kind,source,fields,sections in [
    ('sign-in','登录','auth','web/src/features/auth/sign-in/index.tsx','用户名 / 邮箱|密码',[('其他登录方式','Passkey|已启用的OAuth提供方|人机验证〔如已启用〕')]),
    ('sign-up','注册','auth','web/src/features/auth/sign-up/index.tsx','用户名|邮箱|验证码〔按配置〕|密码|确认密码',[('协议','用户协议 / 隐私政策|注册开关与邮箱限制说明')]),
    ('forgot-password','找回密码','auth','web/src/features/auth/forgot-password/index.tsx','账户邮箱',[('发送结果','显示发送状态|防重复提交|返回登录')]),
    ('reset-password','重设密码','auth','web/src/features/auth/reset-password-confirm/index.tsx','新密码|确认新密码',[('链接状态','邮箱 / Token有效性|无效或过期时说明|重新申请入口')]),
    ('otp','两步验证','auth','web/src/features/auth/otp/index.tsx','六位验证码 / 恢复码',[('登录上下文','明确验证中的账户|切换恢复码|返回登录')]),
    ('oauth-legacy','OAuth 旧回调页','status','web/src/routes/(auth)/oauth.tsx','',[('处理状态','验证授权响应|处理中 / 已失败|不显示提供方密钥')]),
    ('oauth-callback','OAuth 回调页','status','web/src/routes/oauth/$provider.tsx','',[('授权结果','提供方名称|处理中 / 成功 / 失败|失败时返回登录')]),
]:
    url={'sign-in':'/sign-in','sign-up':'/sign-up','forgot-password':'/forgot-password','reset-password':'/reset / /user/reset','otp':'/otp','oauth-legacy':'/oauth','oauth-callback':'/oauth/$provider'}[id]
    add(id,title,'05 公共与工具',url,kind,source,fields=fields,sections=sections,
        actions=[A('继续 / 提交',id,'线稿只展示动作；产品实现须保护重复提交与认证上下文'),A('返回登录','sign-in'),A('用户协议','terms'),A('隐私政策','privacy')],
        notes=['认证状态切换不携带上一账户的缓存或正文；不用“重试”盲目重放OAuth写入。'])

for suffix,title,fields,sections in [
    ('database','1 数据库检查','',[('连接检查','数据库类型|连接状态|已有实例 / 数据检测|失败说明')]),
    ('admin','2 管理员账户','用户名|密码|确认密码',[]),
    ('mode','3 使用模式','',[('模式选择','商业服务 / 自用模式|能力与资金行为说明|保留已确认值')]),
    ('review','4 复核并初始化','',[('复核摘要','数据库状态|管理员身份〔不展示密码〕|使用模式|初始化影响说明')]),
    ('unknown','提交结果未知','',[('只读状态恢复','初始化结果未知|重新读取初始化状态|禁止重复提交|确认成功后进入系统')]),
]:
    add('setup-'+suffix,'安装向导 · '+title,'05 公共与工具','/setup〔'+suffix+'〕','setup','web/src/features/setup/setup-wizard.tsx',fields=fields,sections=sections,
        tabs=[('setup-database','数据库'),('setup-admin','管理员'),('setup-mode','模式'),('setup-review','复核')],
        actions=[A('上一步','setup-database'),A('下一步','setup-review' if suffix=='mode' else 'setup-admin'),A('初始化','setup-unknown','只有复核步骤明确提交；结果未知时仅查询状态，绝不重复初始化')],
        notes=['顶部品牌与当前步骤保持可访问；手机滚动不遮住表单错误或底部动作。','初次安装与已有实例分开，禁止通过线稿重建已有数据库。'])

add('playground','Playground 工作区','05 公共与工具','/playground','chat','web/src/features/playground/index.tsx',
    filters='API Key〔掩码〕|模型|参数设置',sections=[('会话内容','系统提示|用户消息|助手响应 / 用量证据'),('输入区','多行文本|附件〔按现有能力〕|发送 / 停止')],
    actions=[A('参数设置','playground-parameters'),A('选择 API Key','key-delivery'),A('发送','playground','产品中仅用户明确发送才生成；线稿不联网、不消耗额度')],
    notes=['发送、停止、复制、清空明确分离；失败 / 结果未知不得自动再发。'])
add('chat-preset','聊天预设容器','05 公共与工具','/chat/$chatId','embed','web/src/routes/_authenticated/chat/$chatId.tsx',
    sections=[('项目可控的容器','预设名称 / 状态|受限iframe边界|无法加载时说明'),('第三方内容区域','外部站点自有UI，不能由MyAPI重设计|禁止在线稿载入真实外站|本图只画容器与异常状态')],
    actions=[A('返回概览','overview'),A('管理 API Key','keys')],notes=['保留既有iframe安全边界；线稿不嵌入真实页面、不读取或转交凭据。'])
add('chat-link','聊天快捷入口 / 失败状态','05 公共与工具','/chat2link','status','web/src/routes/_authenticated/chat2link.tsx',
    sections=[('打开状态','准备聊天链接|预设不存在|Key不可用 / 获取失败|提供可恢复的明确入口')],
    actions=[A('管理 API Key','keys'),A('返回概览','overview')],merge='统一聊天预设的加载与错误壳，保留旧快捷入口地址；不新增自动发送行为。')
for suffix,title,fields,columns,sections in [
    ('policy','授权与策略','允许学习的范围|脱敏策略|启用状态','',[('授权边界','明确允许的已脱敏请求|当前策略版本|未就绪时禁止提交')]),
    ('versions','指令版本','版本内容 / 备注','版本|创建时间|状态|下载',[('版本编辑','只追加新版本|保留历史|下载当前版本')]),
    ('runs','运行记录','','运行ID|状态|授权策略|输入证据|结果 / 错误',[('运行状态','已排队 / 进行中 / 完成 / 失败|取消请求需明确确认|禁止未知状态自动重发')]),
]:
    add('learning-'+suffix,'提示词学习 · '+title,'05 公共与工具','/prompt-learning〔'+suffix+'〕','form' if suffix!='runs' else 'table','web/src/features/prompt-learning/index.tsx',
        fields=fields,columns=columns,sections=sections,tabs=[('learning-policy','授权策略'),('learning-versions','指令版本'),('learning-runs','运行记录')],
        actions=[A('保存 / 创建新版本','learning-'+suffix,'仅按当前策略明确提交；线稿不执行'),A('查看运行','learning-runs')],
        notes=['只学习明确授权且已脱敏的数据；不把展示线稿当作新训练或真实供应商调用授权。'])

for pid,title,url,kind,source,sections in [
    ('home','产品首页','/','landing','web/src/features/home/index.tsx',[('首屏','MyAPI / 当前实例名称|一句话用途|进入控制台 / 登录'),('接入与信息','已配置公共内容|API地址|公告 / 文档入口')]),
    ('about','关于 MyAPI','/about','document','web/src/features/about/index.tsx',[('项目介绍','产品用途与版本|维护者配置内容|仓库 / 帮助链接'),('法律与来源','保留第三方法律声明|许可证|隐私 / 协议入口')]),
    ('pricing','公开模型定价','/pricing','table','web/src/features/pricing/index.tsx',[('价格证据','价格单位 / 模式|来源更新时间|未知与未配置分开')]),
    ('model-price','公开模型定价详情','/pricing/$modelId','detail','web/src/features/pricing/components/model-details.tsx',[('模型能力','模型名称|协议 / 输入输出能力|上下文信息'),('价格组成','输入 / 输出 / 缓存 / 工具等已有价格|单位与来源|计费表达式的只读说明')]),
    ('rankings','公开排行榜','/rankings','table','web/src/features/rankings/index.tsx',[('统计范围','榜单类型|时间范围|来源与隐私说明')]),
    ('privacy','隐私政策','/privacy-policy','document','web/src/routes/privacy-policy.tsx',[('政策正文','标题与更新时间|可读内容区|章节索引|返回上一页')]),
    ('terms','用户协议','/user-agreement','document','web/src/routes/user-agreement.tsx',[('协议正文','标题与更新时间|可读内容区|章节索引|返回上一页')]),
    ('website','独立静态官网','website/index.html','landing','website/index.html',[('首屏与产品说明','Logo保留|Full / Lite / Desktop说明|文档入口'),('能力介绍','协议与供应商|治理 / 观察 / 本地接入|安全边界'),('安装文档入口','分别指向实际文档|未发布步骤不伪装可安装|不提供真实凭据')]),
]:
    add(pid,title,'05 公共与工具',url,kind,source,sections=sections,
        columns=('模型|供应商|能力|价格与单位|详情' if pid=='pricing' else '排名|对象|指标|统计范围' if pid=='rankings' else ''),
        filters=('模型搜索|供应商|能力|分组' if pid=='pricing' else '榜单|日期范围' if pid=='rankings' else ''),
        actions=[A('进入控制台','overview'),A('登录','sign-in'),A('查看模型','model-price')],
        notes=['公开内容按现有开关与权限显示；模型名 / 提供方名不作项目品牌替换。','仅重组呈现，不修改价格计算、支付或供应商契约。'])

for code,title,message in [('401','需要登录','当前会话不可用，请重新登录。'),('403','无权访问','当前身份没有访问权限；不展示受限数据。'),('404','页面不存在','保留可恢复的返回入口，不猜测或自动提交请求。'),('500','服务出现错误','展示失败与追踪信息；只读重试不重放写入。'),('503','服务维护中','说明暂不可用；不把维护态当成功或空数据。')]:
    for prefix,shell in [('error','公共错误页'),('console-error','控制台内错误页')]:
        add(prefix+'-'+code,shell+' · '+code+' '+title,'06 状态与共用组件','/'+code if prefix=='error' else '/errors/'+{'401':'unauthorized','403':'forbidden','404':'not-found','500':'internal-server-error','503':'maintenance-error'}[code],
            'error','web/src/routes/(errors)/'+code+'.tsx' if prefix=='error' else 'web/src/routes/_authenticated/errors/$error.tsx',
            sections=[(code+' '+title,message+'|返回入口|当前身份 / 状态说明〔不含敏感内容〕')],
            actions=[A('返回上一页','overview'),A('重新登录','sign-in')],notes=['嵌入式错误页保留控制台外壳；公共错误页不重复渲染两套导航。'])

# Settings pages are expanded from the actual registries, rather than counting
# the dynamic /$section route once and leaving its fields undesigned.
settings_specs = {
 'site': {
  'system-info':('站点与品牌','名称 / Logo|站点地址|首页与关于内容|页脚 / 法律入口','品牌与公共内容|系统地址与说明'),
  'notice':('系统通知','通知正文|显示状态','通知预览与可读性'),
  'header-navigation':('公共站点导航','导航名称|目标链接|显示条件|顺序','导航项目列表|新增 / 编辑 / 预览'),
  'sidebar-modules':('控制台模块','模块名称|角色 / 版本条件|显示状态','控制台入口清单|避免与公共导航混淆'),
 },
 'auth': {
  'basic-auth':('账户与注册','密码登录|开放注册|邮箱验证|邮箱域名限制|邮箱别名限制','基础登录|注册安全约束'),
  'oauth':('外部登录提供方','提供方选择|启用状态|Client ID|Client Secret〔掩码〕|回调地址','提供方列表|当前提供方配置'),
  'passkey':('Passkey','启用状态|RP显示名 / ID|允许来源|用户验证要求|设备偏好','站点身份|认证要求'),
  'bot-protection':('人机验证','启用状态|Site Key|Secret Key〔掩码〕','验证配置|失败时行为说明'),
  'custom-oauth':('自定义 OAuth','提供方名称|Client ID / Secret〔掩码〕|授权端点|Token端点|用户信息端点|映射规则','自定义提供方列表|编辑 / 删除确认'),
 },
 'billing': {
  'quota':('额度规则','新用户额度|预扣额度|邀请奖励|充值入口|免费模型预扣开关','额度默认值|既有业务规则与合规状态'),
  'currency':('货币与显示','额度单位换算|显示货币|汇率|自定义符号|Token统计显示','金额单位|界面显示方式'),
  'model-pricing':('模型价格','模型 / 未设置模型|价格模式|输入 / 输出 / 缓存等比例|工具价格|上游同步','模型价格列表|价格来源 / 同步预览'),
  'group-pricing':('分组与访问价格','用户分组|充值比例|组间比例|可用组|自动分组|访问配置','分组列表|当前组详情 / 继承说明'),
  'payment':('支付网关','资金模式|网关列表|当前网关开关|密钥〔掩码〕|商品 / 金额 / 币种|回调地址','资金能力状态|提供方配置分组|提交前影响摘要'),
  'quota-writer':('额度写入模式','当前配置版本|当前写入模式|待处理任务 / 未知|变更说明','只读现状|显式变更确认 / 风险说明'),
  'checkin':('签到奖励','启用状态|奖励范围|奖励规则','资金能力确认|签到业务规则'),
 },
 'models': {
  'openai-pricing-source':('OpenAI 官方价格来源','来源状态|抓取 / 快照时间|已审阅版本|冲突 / 待处理','只读价格来源|明确获取与应用分开'),
  'global':('协议与全局模型','正文直通|Thinking黑名单|Chat转Responses策略|Ping间隔','协议行为配置|高级规则编辑'),
  'routing-reliability':('路由可靠性','重试次数|停用阈值|自动启停|重试状态码|自动测试周期 / 并发|测试模式','请求安全与重试边界|观察 / 测试配置'),
  'gemini':('Gemini 配置','安全设置|版本设置|图像模型|Thinking适配|Function Call规则','Gemini专属参数|高级JSON编辑'),
  'claude':('Claude 配置','模型请求头规则|默认max_tokens|Thinking适配|预算比例','Claude专属参数|高级规则编辑'),
  'grok':('Grok 配置','违规扣费开关|违规扣费金额','供应商专属规则|单位与风险说明'),
  'channel-affinity':('连接亲和','启用状态|成功切换策略|连接停用时策略|条目上限|TTL|亲和规则','亲和策略|规则列表 / 编辑'),
  'model-deployment':('模型部署集成','服务地址|访问凭据〔掩码〕|区域 / 模型配置','外部集成配置|权限 / 费用提示'),
 },
 'operations': {
  'behavior':('系统行为','演示模式|自用模式|既有界面偏好','运行模式|切换影响说明'),
  'alerts':('监控与告警','余额阈值|连接告警|额度同步周期 / 数量|性能采样|数据保留','告警规则|通知投递 / 采样策略'),
  'email':('邮件服务','SMTP地址 / 端口|账号 / 发件人|密码〔掩码〕|TLS / STARTTLS|证书校验|登录认证','连接配置|传输安全'),
  'worker':('Worker 代理','Worker URL|凭据〔掩码〕|HTTP图片请求开关','代理来源|安全边界'),
  'logs':('日志与存储','调用日志开关|正文采集状态|保留 / 清理范围|文件列表','采集策略|日志保留|危险清理操作区'),
  'performance':('性能与缓存','磁盘缓存开关|阈值 / 大小 / 路径|监控开关|CPU / 内存 / 磁盘阈值','缓存配置|观察策略|闲置缓存清理确认'),
  'update-checker':('系统维护','当前版本|运行时间|更新信息|服务状态','只读版本与健康|明确维护动作'),
 },
 'security': {
  'rate-limit':('请求速率限制','启用状态|总请求 / 成功请求上限|时间窗|分组规则','限制规则|范围与错误说明'),
  'sensitive-words':('敏感词策略','启用状态|检查Prompt|词表','检查范围|词表编辑'),
  'ssrf':('SSRF 防护','启用状态|私网访问策略|域名 / IP模式|列表|允许端口|域名IP校验','目标访问边界|高级允许 / 拒绝规则'),
  'token-limits':('API Key 数量限制','单用户Key数量上限','限制与现有账户影响说明'),
 },
 'content': {
  'dashboard':('数据导出与仪表盘','导出开关|导出周期|默认时间粒度','数据采样与导出说明'),
  'announcements':('公告','启用状态|公告标题 / 正文|链接 / 时间','公告列表|新增编辑 / 预览 / 删除确认'),
  'api-info':('API 地址展示','启用状态|名称 / 地址|说明|显示顺序','地址条目列表|新增编辑 / 预览'),
  'faq':('常见问题','启用状态|问题|回答|顺序','问题列表|新增编辑 / 删除确认'),
  'uptime-kuma':('状态页分组','启用状态|分组名称|状态页地址|顺序','分组列表|连接安全说明'),
  'chat':('聊天预设','名称|Web / 客户端类型|链接模板|Key传递提示|显示顺序','预设列表|预览与安全提示'),
  'drawing':('绘图功能','启用状态|Midjourney通知|账号过滤|地址转发|模式清理|成功检查','绘图能力配置|供应商行为说明'),
 },
}

for category, specs in settings_specs.items():
    source = 'web/src/features/system-settings/' + category + '/section-registry.tsx'
    source_text = (ROOT / source).read_text()
    registered = re.findall(r"\n    id: '([^']+)',\n    titleKey: '([^']+)'", source_text)
    for section_id, english_title in registered:
        title, fields, groups = specs[section_id]
        blocks = [(group, '字段按任务归组|高级规则默认折叠但可完整访问|保存前展示字段错误') for group in groups.split('|')]
        actions=[A('保存当前分区','settings-'+category+'-'+section_id,'只提交当前明确修改；失败保留草稿，结果未知不自动重放写入'),A('放弃草稿','confirm-discard','只有存在未保存修改才要求确认')]
        if section_id=='logs': actions.append(A('正文与 SSE','request-bodies'))
        if section_id=='model-pricing': actions.append(A('模型价格详情','model-price'))
        if section_id in ['quota-writer','payment','performance','logs']: actions.append(A('危险操作确认','confirm-danger'))
        add('settings-'+category+'-'+section_id,title,'04 设置 · '+category,'/system-settings/'+category+'/'+section_id,'form',source,
            fields=fields,sections=blocks,actions=actions,
            notes=['分区顶部只保留一个主保存入口；底部未保存栏在手机不遮挡最后一个字段或验证信息。','低保真图表达任务分组和可访问字段范围；最终字段校验与服务端契约保持不变。'],
            merge='同一任务的字段在当前分区内归组；高级参数可展开，旧分区地址和服务端选项不删除。',access='管理员 / Root按既有设置权限，Lite按既有能力裁剪')

# Shared interaction boards are explicit drawings referenced from every page.
add('date-range','共用日期与时间范围','06 状态与共用组件','组件：单个日历范围选择','calendar','web/src/components/ui/calendar.tsx',
    fields='开始日期 + 精确时间|结束日期 + 精确时间|时区标识',
    sections=[('快捷范围','今天|近7天|近30天|本月|清空范围'),('选择规则','同一日历先点开始再点结束|跨月 / 年可导航|整天默认包含结束日|起止倒置显示错误，不提交')],
    actions=[A('应用范围','request-bodies','应用到来源页草稿；来源页确认查询后才读取列表，不隐式修改其他筛选'),A('取消','request-bodies','丢弃当前弹层草稿，保留已应用范围'),A('今天','date-range','只选定当前日历时区下今天的草稿范围'),A('近7天','date-range','只更新包含今天的7个日历日草稿'),A('近30天','date-range','只更新包含今天的30个日历日草稿'),A('本月','date-range','只更新当前月的草稿范围'),A('清空','date-range','清空草稿，应用前保留原筛选')],
    notes=['桌面可在同一个范围选择器内显示相邻月份；手机单月，不拆成两个独立日期选择器。','保留精确时间编辑；同一天、跨月、时间边界、时区与取消行为均需在实现阶段回归。'])
add('pagination','共用分页与直接跳页','06 状态与共用组件','组件：列表分页','table','web/src/components/data-table/core/pagination.tsx',
    columns='记录摘要|当前范围|总数证据',filters='每页20 / 50 / 100|跳至第〔输入〕页',
    actions=[A('第一页','request-bodies'),A('指定页','request-bodies'),A('最后一页','request-bodies')],
    notes=['有效页码范围明确；不接受空值、非整数、负数和超出总页数的跳页。','查询失败显示失败，不把未知总数当0；换筛选 / 每页条数时校正页码。','表格与正文列表复用受控分页，不引入第二套分页样式。'])
add('component-rules','按钮、输入与反馈规范','06 状态与共用组件','组件：设计语言','component','web/src/components/ui/button.tsx',
    sections=[('按钮层级','主操作：每个任务一个|次操作：描边 / 文本|危险操作：明确文案与确认|图标按钮有名称'),('输入控件','输入 / 选择 / 开关 / 多行|标签 / 帮助 / 错误就近|必填与禁用理由|提交中防重复'),('视觉与可达性','方向3令牌 / Hugeicons|浅色 / 暗色同一语义|手机至少44px触控目标|键盘焦点可见')],
    actions=[A('日期范围规范','date-range'),A('分页规范','pagination'),A('各状态规范','state-board')],
    notes=['保留Logo；没有引用真实供应商标志、凭据、金额或在线内容。'])
add('state-board','加载、空、失败、未授权与未知','06 状态与共用组件','组件：页面状态','component','web/src/components/ui/empty.tsx',
    sections=[('加载中','骨架 / 正在读取|不展示上一身份缓存|读操作可取消'),('无匹配结果','保留已应用筛选|清空筛选入口|不误写成系统无数据'),('读取失败','错误与可恢复入口|只读重试|不把失败显示为0条'),('无权限 / 会话变化','受限内容立即移除|返回可用区域|重新登录'),('处理中 / 成功 / 结果未知','提交中禁止重复点击|成功给明确反馈|未知只查状态，不自动重放')],
    actions=[A('返回正文线稿','request-bodies')])
for id,title,fields,note in [
    ('confirm-delete','删除确认','对象名称 / 掩码|具体删除范围|不可逆影响|确认 / 取消','默认焦点放取消；批量删除清楚列出数量与范围，绝不作为普通刷新。'),
    ('confirm-danger','高风险操作确认','当前配置 / 证据|将改变的行为|影响范围|明确确认 / 取消','计费、账户、部署与清理动作保留独立确认，不为精简减少安全步骤。'),
    ('confirm-discard','离开未保存修改','有修改的分区|保留草稿 / 继续编辑|明确放弃','只在存在真实草稿时出现；不能吞掉提交失败或自动覆盖原配置。'),
]:
    add(id,title,'06 状态与共用组件','组件：确认弹层','overlay','web/src/components/confirm-dialog.tsx',fields=fields,
        actions=[A('取消 / 继续编辑','request-bodies'),A('确认范围内动作',id,'线稿仅展示，不执行删除、写入或对外操作')],notes=[note])

add('shell-console','控制台外壳与导航','06 状态与共用组件','布局：/_authenticated','component','web/src/components/layout/components/console-shell.tsx',
    sections=[('桌面顶栏','Logo / 当前实例|概览 / 上游 / API Key / 请求 / 设置|Full管理下拉|搜索 / 账户'),('手机外壳','当前页标题 / 返回|搜索与主操作|底部5项导航 / 更多|内容保留底部安全间距')],
    actions=[A('概览','overview'),A('上游','connections'),A('API Key','keys'),A('请求','requests'),A('设置','settings-site-system-info')],
    notes=['不回退到旧侧栏或双导航；全站顶栏、面包屑、内容宽度与移动返回一致。'])
add('shell-auth','认证页公共外壳','06 状态与共用组件','布局：/(auth)','auth','web/src/routes/(auth)/route.tsx',
    fields='认证任务区域',sections=[('公共区域','Logo / 语言 / 主题|简洁任务标题|协议 / 隐私入口')],actions=[A('登录','sign-in'),A('注册','sign-up')])
add('shell-settings','设置分区导航','06 状态与共用组件','布局：/system-settings','component','web/src/routes/_authenticated/system-settings/route.tsx',
    sections=[('设置分区','站点|认证|额度与商业|模型|安全|运行维护|内容'),('当前分区','标题与作用说明|任务字段组|高级项|有草稿时显示保存条')],
    actions=[A('站点','settings-site-system-info'),A('认证','settings-auth-basic-auth'),A('安全','settings-security-rate-limit')],
    notes=['导航搜索 / 更多分区不重复渲染顶栏；手机可展开并完整访问分区。'])
add('desktop-tray','Desktop 托盘与本机接入','07 Desktop','原生托盘 / LAN帮助','overlay','electron/main.js',
    fields='服务状态|本机Base URL〔合成〕|访问模式：仅本机 / 局域网',
    sections=[('托盘动作','显示MyAPI|复制接入地址|LAN帮助|明确重启并变更访问模式|退出应用')],
    actions=[A('打开控制台','overview'),A('接入说明','key-delivery'),A('改变访问范围','confirm-danger','产品实现需明确确认后才重启和变更访问模式')],
    notes=['复用Lite页面，不维护第二套业务；窗口关闭、退出应用、停止服务的语义必须分别说明。','本图是原生菜单 / 系统对话框的结构线稿，不伪装为已实现的HTML设置页。'])

overlay_specs = [
 ('key-edit','创建 / 编辑 API Key','web/src/features/keys/components/api-keys-mutate-drawer.tsx','名称|启用状态|分组 / 模型范围|有限预算|有效期|允许IP / 高级参数','keys','新建和编辑复用字段组；密钥明文的展示、复制和轮换是独立动作。'),
 ('connection-edit','添加 / 编辑连接','web/src/features/channels/components/drawers/channel-mutate-drawer.tsx','名称 / 类型|来源地址|凭据〔掩码〕|模型|分组|优先级 / 权重|高级映射与覆盖规则','connections','把基本接入、路由和高级参数分组；保存与主动测试分开。'),
 ('provider-test','连接测试与结果','web/src/features/channels/components/dialogs/channel-test-dialog.tsx','连接身份|待测模型|明确测试按钮|结果 / HTTP / 错误|采样时间','connections','测试可能联系真实供应商并产生用量；设计稿绝不执行测试。'),
 ('subscription-shares','订阅窗口分额','web/src/features/keys/components/subscription-share-dialog.tsx','订阅池 / 窗口|关联Key|当前有限额度|使用证据|保存草稿|关闭池确认入口','connection-windows','额度未知不补零；关闭池保持独立确认和既有并发 / 幂等边界。'),
 ('model-edit','新增 / 编辑模型','web/src/features/models/components/drawers/model-mutate-drawer.tsx','模型名称|供应商|输入 / 输出能力|说明|标签|启用状态','models','模型目录配置与实际上游测试分开；不删除第三方名称。'),
 ('prefill-group','预填模型分组','web/src/features/models/components/drawers/prefill-group-form-drawer.tsx','名称|模型选择|预填参数|应用范围','models','只改变明确选定的分组；保留编辑前配置和错误信息。'),
 ('deployment-edit','创建模型部署','web/src/features/models/components/dialogs/create-deployment-drawer.tsx','供应商 / 区域|模型 / 名称|资源参数|费用与权限提示','deployments','部署不是普通保存；产品实现必须有明确外部操作确认，线稿不创建资源。'),
 ('user-edit','创建 / 编辑用户','web/src/features/users/components/users-mutate-drawer.tsx','用户名 / 昵称|邮箱|密码〔仅明确更改〕|角色|分组|状态','users','角色变更与额度调整保留既有权限；不自动覆盖空密码。'),
 ('usage-policy','用户使用策略','web/src/features/users/components/user-usage-policy-dialog.tsx','目标用户|允许范围|预算 / 额度策略|当前状态|变更影响','users','读取与保存分别受权限控制，未知状态不可当作无限或零。'),
 ('assigned-access','分配访问权限','web/src/features/access-policy/components/access-policy-dialog.tsx','对象身份|可分配项|当前访问|新增 / 移除差异','users','仅在已有授权范围内分配；清晰展示变更摘要。'),
 ('plan-edit','创建 / 编辑订阅方案','web/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx','方案名称|价格 / 周期|额度|重置规则|分组影响|状态','subscriptions','显示金额和周期单位；不改变计费表达式和额度转换。'),
 ('user-subscriptions','用户订阅管理','web/src/features/subscriptions/components/dialogs/user-subscriptions-dialog.tsx','用户身份|当前订阅列表|分配方案|到期 / 周期|停用 / 重置确认','subscriptions','历史、分配、停用、重置是明确分开的动作。'),
 ('redemption-edit','创建 / 编辑兑换码','web/src/features/redemption-codes/components/redemptions-mutate-drawer.tsx','名称|额度|数量|到期时间|启用状态','redemptions','批量数量与额度仍受既有服务端校验；不在图中放真实兑换码。'),
 ('payment-confirm','充值支付确认','web/src/features/wallet/components/dialogs/payment-confirm-dialog.tsx','订单上下文|支付方式|金额 / 币种|资金能力状态|确认影响','wallet','显示金额与订单身份；提交中禁止重复点击，结果未知只查询，不再支付。'),
 ('subscription-purchase','订阅购买确认','web/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx','方案 / 周期|金额 / 币种|额度与重置说明|支付方式|当前能力状态','wallet','购买是明确写入操作；产品关闭或退役时只保留历史入口。'),
 ('order-detail','订单详情与完成状态','web/src/features/wallet/components/dialogs/billing-history-dialog.tsx','订单ID〔合成〕|金额|提供方 / 支付方式|状态证据|已有处理记录','funding-history','不把未知支付结果当未支付并重放；补单保留原有明确确认。'),
 ('login-sessions','登录会话管理','web/src/features/profile/components/login-session-dialogs.tsx','当前会话|设备与时间|待撤销会话|范围与影响','profile-security','撤销当前会话、其他会话和单个会话明确区分；不刷新登录来规避维护门禁。'),
 ('credential-maintenance','凭据维护','web/src/features/keys/components/credential-maintenance-dialog.tsx','目标Key / 掩码|当前维护状态|可执行动作|认证上下文|操作结果','key-scope','已验证的会话保护、精确单次写入、未知结果恢复和错误内容隔离不放宽。'),
 ('credential-rotation','凭据轮换','web/src/features/keys/components/credential-rotation-dialog.tsx','目标Key|轮换影响|明确确认|处理中 / 结果未知|新凭据单次展示','key-scope','轮换不自动重放；新凭据不得进入列表摘要、截图证据或历史日志。'),
 ('playground-parameters','Playground 参数','web/src/features/playground/components/input/playground-parameter-panel.tsx','模型能力|温度 / 采样等已支持参数|输出预算|流式设置|高级参数','playground','显式0 / false仍是有效值；未知能力不伪装支持，不修改发送契约。'),
]
for pid,title,source,fields,parent,note in overlay_specs:
    add(pid,title,'08 编辑与弹层', '弹层：'+title,'overlay',source,fields=fields,
        actions=[A('取消 / 返回',parent,'保留来源页筛选、焦点和草稿'),A('提交 / 执行当前动作',pid,'图中仅说明；实际动作沿用既有确认、权限、幂等和防重放规则')],
        notes=[note,'桌面采用带标题与固定动作区的聚焦弹层；手机全屏或单列，不堆叠窄抽屉。'])

# Resolve actual project-owned entries and their reachable imports. This is a
# static source inventory, not a claim that dynamic runtime permissions passed.
src = ROOT / 'web/src'
def resolve_import(from_path, spec):
    if spec.startswith('@/'):
        p = src / spec[2:]
    elif spec.startswith('.'):
        p = from_path.parent / spec
    else:
        return None
    for candidate in [p, p.with_suffix('.tsx'), p.with_suffix('.ts'), p.with_suffix('.jsx'), p.with_suffix('.js'), p/'index.tsx', p/'index.ts']:
        if candidate.is_file():
            return candidate.resolve()
    return None

route_files = sorted(p for p in (src/'routes').rglob('*.tsx') if not p.name.startswith('._'))
reachable = set()
pending = list(route_files)
for seed in [src/'main.tsx',src/'routeTree.gen.ts']:
    if seed.is_file(): pending.append(seed)
while pending:
    p = pending.pop().resolve()
    if p in reachable: continue
    reachable.add(p)
    s=p.read_text()
    specs=re.findall(r"(?:from\s*|import\s*\(\s*)['\"]([^'\"]+)['\"]",s)
    for spec in specs:
        target=resolve_import(p,spec)
        if target and target not in reachable: pending.append(target)

zh_data=json.loads((src/'i18n/locales/zh.json').read_text())
zh=zh_data.get('translation',zh_data)
already_sources={s for page in pages for s in page['sources']}
overlay_inventory=[]
unconfirmed=[]
def translate(key):
    value=zh.get(key,key)
    return value if isinstance(value,str) else key

def jsx_tag_end(s,start):
    depth=0;quote=None;escaped=False
    for i in range(start,len(s)):
        c=s[i]
        if quote:
            if escaped: escaped=False
            elif c=='\\': escaped=True
            elif c==quote: quote=None
            continue
        if c in ['\"',"'",'`']: quote=c
        elif c=='{': depth+=1
        elif c=='}': depth=max(0,depth-1)
        elif c=='>' and depth==0: return i+1
    return len(s)

def jsx_nodes(s,names):
    roots=[]
    pattern=re.compile(r'<('+names+r')\b')
    for m in pattern.finditer(s):
        opening_end=jsx_tag_end(s,m.end());tag=m.group(1)
        if s[:opening_end].rstrip().endswith('/>'):
            roots.append((m.start(),opening_end,opening_end));continue
        token=re.compile(r'</?'+re.escape(tag)+r'\b')
        depth=1;end=opening_end
        for nested in token.finditer(s,opening_end):
            nested_end=jsx_tag_end(s,nested.end())
            if s[nested.start():nested.start()+2]=='</':depth-=1
            elif not s[:nested_end].rstrip().endswith('/>'):depth+=1
            if depth==0:end=nested_end;break
        if end>opening_end:roots.append((m.start(),end,opening_end))
    if not roots:return [s]
    result=[]
    for start,end,opening_end in roots:
        children=sorted((a,b) for a,b,_ in roots if start<a and b<=end)
        cursor=start;chunks=[]
        for a,b in children:
            if a<cursor:continue
            chunks.append(s[cursor:a]);cursor=b
        chunks.append(s[cursor:end]);result.append(''.join(chunks))
    return result

def overlay_nodes(s):
    return jsx_nodes(s,'Dialog|ConfirmDialog|RiskAcknowledgementDialog|Sheet|Drawer|AlertDialog')

def title_prop_keys(s):
    match=re.search(r'(?<![-\w])title\s*=\s*\{',s)
    if not match:return []
    depth=1;i=match.end();start=i;quote=None;escaped=False
    while i<len(s) and depth:
        c=s[i]
        if quote:
            if escaped:escaped=False
            elif c=='\\':escaped=True
            elif c==quote:quote=None
        elif c in ['\"',"'",'`']:quote=c
        elif c=='{':depth+=1
        elif c=='}':depth-=1
        i+=1
    return list(dict.fromkeys(re.findall(r"\bt\(\s*['\"]([^'\"]+)['\"]",s[start:i-1])))

friendly_titles={
 'dialog':'共用任务弹层外壳','risk-acknowledgement-dialog':'风险说明与明确确认','command':'全局搜索与命令入口',
 'secure-verification-dialog':'敏感操作的身份验证','edit-tag-dialog':'编辑连接标签及关联规则','status-code-risk-dialog':'状态码策略风险确认',
 'models-filter-dialog':'模型分析筛选','description-dialog':'模型描述','update-config-dialog':'模型配置更新预览','vendor-mutate-dialog':'创建 / 编辑供应商目录',
 'view-logs-dialog':'模型部署日志','delete-account-dialog':'删除当前账户确认','two-fa-backup-dialog':'两步验证恢复码','two-fa-disable-dialog':'停用两步验证确认',
 'toggle-status-dialog':'订阅方案启停确认','provider-form-dialog':'自定义OAuth提供方编辑','form-navigation-guard':'离开未保存设置确认',
 'chat-dialog':'编辑聊天预设','rule-editor-dialog':'编辑连接亲和规则','amount-discount-dialog':'充值金额折扣规则','creem-product-dialog':'支付商品配置',
 'payment-method-dialog':'支付方式配置','waffo-settings-section':'Waffo支付配置确认','model-pricing-sheet':'模型价格明细编辑',
 'rate-limit-dialog':'分组限流规则','audio-preview-dialog':'已保存音频预览',
}
special_fields={
 'config-drawer':['主题 / 浅暗偏好','颜色预设','字体与字号','圆角 / 密度','内容宽度','方向与恢复默认'],
 'command':['搜索页面 / Request ID','按权限筛选的结果','快捷键 / 返回入口'],
 'model-group-selector':['搜索','可选模型 / 分组','当前选择','清空 / 应用'],
 'secure-verification-dialog':['当前敏感任务','可用Passkey / 两步验证','验证码〔不记录〕','验证中 / 失败 / 不可用状态'],
 'delete-account-dialog':['当前账户名称','不可逆影响与数据范围','输入账户名确认','提交中 / 结果未知时状态恢复'],
 'two-fa-backup-dialog':['恢复码仅本次展示','明确复制 / 下载','已保存确认','不进入日志或截图的提醒'],
 'model-pricing-sheet':['模型身份','价格 / 比例模式','输入 / 输出 / 缓存等价格项','单位与来源','变更摘要'],
 'rate-limit-dialog':['分组名称','请求上限','成功请求上限','既有时间窗说明'],
 'audio-preview-dialog':['媒体类型 / 已存来源','播放 / 暂停','进度 / 音量','不存在 / 加载失败说明'],
}

def source_parent(relative):
    feature=relative.split('/')[3] if relative.startswith('web/src/features/') else ''
    mapping={'channels':'connections','connections':'connections','keys':'key-scope','models':'models','profile':'profile-security','wallet':'wallet','subscriptions':'subscriptions','users':'users','full-content-logs':'request-bodies','usage-logs':'requests','dashboard':'overview','auth':'sign-in','playground':'playground','access-policy':'users','system-info':'system-instances','pricing':'pricing','redemption-codes':'redemptions'}
    if feature=='system-settings':return 'shell-settings'
    return mapping.get(feature,'shell-console')

for p in sorted(src.rglob('*.tsx')):
    if p.name.startswith('._') or '__tests__' in p.parts or p.name.endswith('.test.tsx'): continue
    s=p.read_text();relative=p.relative_to(ROOT).as_posix()
    explicit=bool(re.search(r'<(?:DialogContent|SheetContent|DrawerContent|AlertDialogContent)\b',s))
    wrapper=bool(re.search(r"from ['\"]@/components/(?:dialog|confirm-dialog|risk-acknowledgement-dialog)['\"]",s)) and bool(re.search(r'<(?:Dialog|ConfirmDialog|RiskAcknowledgementDialog)\b',s))
    if not(explicit or wrapper): continue
    if p.resolve() not in reachable and not relative.startswith('web/src/components/'):
        unconfirmed.append(relative)
        continue
    mapped=[page['id'] for page in pages if relative in page['sources']]
    if mapped:
        overlay_inventory.append(dict(source=relative,wireframes=mapped,status='明确设计或复用模板'))
        continue
    suffix=re.sub('[^a-z0-9-]+','-',str(p.relative_to(src)).lower().replace('/','-').removesuffix('.tsx')).strip('-')
    mapped=[];seen_layouts=set()
    for idx,node in enumerate(overlay_nodes(s)):
        title_chunks=re.findall(r'<(?:DialogTitle|SheetTitle|DrawerTitle|AlertDialogTitle)[^>]*>([\s\S]*?)</(?:DialogTitle|SheetTitle|DrawerTitle|AlertDialogTitle)>',node)
        title_keys=[]
        for chunk in title_chunks:
            match=re.search(r"\bt\(\s*['\"]([^'\"]+)['\"]",chunk)
            if match and match.group(1) not in title_keys:title_keys.append(match.group(1))
        if not title_keys:title_keys=title_prop_keys(node)
        title=' / '.join(translate(k) for k in title_keys) or friendly_titles.get(p.stem,'连接亲和规则管理' if p.stem=='index' and 'channel-affinity' in relative else p.stem.replace('-',' ')+' · 独立任务')
        label_keys=list(dict.fromkeys(re.findall(r'(?:(?<![-\w])label=\{t\(|<(?:FormLabel|Label)[^>]*>\s*\{t\()[\s\n]*[\'\"]([^\'\"]+)[\'\"]',node)))
        fields=special_fields.get(p.stem) or [translate(k) for k in label_keys] or ['作用对象 / 范围','当前值或已存证据','明确动作与影响说明']
        button_keys=[]
        for button in jsx_nodes(node,'Button'):
            match=re.search(r'<Button\b',button)
            if not match:continue
            chunk=button[jsx_tag_end(button,match.end()):]
            keys=list(dict.fromkeys(re.findall(r"\bt\(['\"]([^'\"]+)['\"]",chunk)))
            keys=[key for key in keys if not re.search(r'\b(Loading|Saving|Deleting|Submitting|Creating|Updating|Processing|Connecting|Rotating)\b|加载中|保存中|删除中|提交中|创建中|更新中|处理中|连接中|轮换中|请稍候',key)]
            if keys:button_keys.append(' / '.join(keys))
        button_keys.extend(re.findall(r"\b(?:confirmText|confirmLabel|cancelText|submitText)=\{t\(['\"]([^'\"]+)['\"]",node))
        button_keys=list(dict.fromkeys(button_keys))
        signature=(title,tuple(fields),tuple(button_keys))
        if signature in seen_layouts:continue
        seen_layouts.add(signature)
        pid='surface-'+suffix+('-'+str(idx+1) if idx else '')
        actions=[A('取消 / 关闭',source_parent(relative),'实际返回调用页并恢复焦点；此链接示意该业务的来源页')]
        for key in button_keys:
            label=' / '.join(translate(part) for part in key.split(' / '))
            if label in ['取消','关闭','Cancel','Close','取消操作']:continue
            if label not in [a[0] for a in actions]:
                actions.append(A(label,pid,'仅设计该既有动作的位置；权限、确认、状态与是否对外调用仍按原实现'))
        if len(actions)==1: actions.append(A('确认当前动作',pid,'设计稿不提交任何业务请求'))
        add(pid,title,'08 编辑与弹层','弹层：'+title,'overlay',relative,fields=fields,actions=actions,
            notes=['按单个弹层而非整个文件提取标题、字段标签和动作，避免把相邻编辑器或确认框的按钮混入。','条件性字段与动作仍按原权限、模式和状态显隐；不得因统一模板删去业务参数。','若为确认操作，正文必须展示准确对象、范围、影响及取消入口；若为编辑，错误就近显示并保留草稿。'],
            merge='同类弹层复用标题、字段、错误与动作区组件；确认语义与提交契约仍按各自业务保留。')
        mapped.append(pid)
    overlay_inventory.append(dict(source=relative,wireframes=mapped,status='提取既有标题与字段的逐弹层布局草案'))

context_titles={
 'web/src/features/auth/sign-in/components/user-auth-form.tsx':'登录 · 微信验证',
 'web/src/features/auth/sign-up/components/sign-up-form.tsx':'注册 · 微信验证',
 'web/src/features/keys/components/api-keys-delete-dialog.tsx':'删除 API Key 确认',
 'web/src/features/redemption-codes/components/redemptions-delete-dialog.tsx':'删除兑换码确认',
 'web/src/features/users/components/users-delete-dialog.tsx':'删除用户确认',
 'web/src/features/system-settings/content/announcements-section.tsx':'删除公告条目确认',
 'web/src/features/system-settings/content/api-info-section.tsx':'删除 API 地址条目确认',
 'web/src/features/system-settings/content/faq-section.tsx':'删除常见问题确认',
 'web/src/features/system-settings/content/uptime-kuma-section.tsx':'删除状态页分组确认',
 'web/src/features/models/components/deployments-table.tsx':'删除模型部署确认',
 'web/src/features/channels/components/dialogs/ollama-models-dialog.tsx':'删除 Ollama 模型确认',
 'web/src/features/pricing/components/pricing-toolbar.tsx':'公开定价 · 筛选',
 'web/src/features/usage-logs/components/logs-filter-toolbar.tsx':'调用记录 · 更多筛选',
 'web/src/features/profile/components/tabs/account-bindings-tab.tsx':'个人账户 · 解除绑定确认',
 'web/src/features/users/components/dialogs/user-binding-dialog.tsx':'用户管理 · 解除账户绑定确认',
}
for page in pages:
    if page['id'].startswith('surface-') and page['title'] in ['微信登录','您确定吗？','确认删除','筛选','确认解绑']:
        title=context_titles.get(page['sources'][0])
        if title:page['title']=title;page['url']='弹层：'+title

tab_inventory=[]
def design_subviews(source, rows, parent=None, group=None):
    if parent is None:parent=next(p['id'] for p in pages if source in p['sources'])
    parent_page=next(p for p in pages if p['id']==parent)
    tabs=[(parent+'-view-'+value,title) for value,title,kind,fields,columns,note in rows]
    parent_page['tabs']=tabs
    for value,title,kind,fields,columns,note in rows:
        pid=parent+'-view-'+value
        readonly=source in ['web/src/features/channels/components/dialogs/codex-usage-dialog.tsx','web/src/features/pricing/components/model-details.tsx']
        if readonly:
            actions=[A('返回来源页',parent,'只读查看，不发起生成、购买或配置变更')]
        elif kind=='table':
            actions=[A('查看 / 编辑选定条目',pid,'打开或选择当前条目的编辑草稿；确认与提交沿用既有契约'),A('返回来源页',parent,'恢复调用页上下文，不执行隐藏写入')]
        else:
            actions=[A('应用当前草稿',pid,'只表示当前任务的明确保存/应用；校验、权限、确认与写入契约保留，线稿不提交'),A('返回来源页',parent,'恢复调用页上下文，不执行隐藏写入')]
        add(pid,parent_page['title']+' · '+title,group or parent_page['group'],parent_page['url']+'〔'+value+'〕',kind,source,
            fields=fields,columns=columns,tabs=tabs,
            sections=[(title+'的任务范围',note)],actions=actions,
            notes=['不同内容页签单独成图，不用一个未展开的“高级设置”占位替代。','模式切换保留草稿；从结构化视图切到JSON时不得丢失未知字段或合法的显式0 / false。'],access=parent_page['access'])
        tab_inventory.append(dict(source=source,value=value,wireframes=[pid],note='独立内容状态，桌面与手机各一张。'))

design_subviews('web/src/features/system-settings/auth/oauth-section.tsx',[
 ('github','GitHub','form','启用状态|Client ID|Client Secret〔掩码〕|回调地址','', '仅配置GitHub登录|回调地址可复制|密钥不进入摘要与截图'),
 ('discord','Discord','form','启用状态|Client ID|Client Secret〔掩码〕|回调地址','', 'Discord专属登录配置|不改变提供方协议'),
 ('oidc','OIDC','form','启用状态 / 显示名|Client ID / Secret〔掩码〕|Well-known URL|授权 / Token / 用户信息端点','', '发现配置与显式端点分组|高级端点可以展开完整访问'),
 ('telegram','Telegram','form','启用状态|Bot名称|Bot Token〔掩码〕','', '账户展示名与凭据分开|不在设计阶段联系Telegram'),
 ('linuxdo','LinuxDO','form','启用状态|Client ID / Secret〔掩码〕|最低信任等级','', '保留信任等级与认证边界'),
 ('wechat','WeChat','form','启用状态|服务地址|服务Token〔掩码〕|二维码图片地址','', '既有登录与绑定语义不改变|不载入真实二维码或外部图片'),
],parent='settings-auth-oauth')
design_subviews('web/src/features/system-settings/integrations/payment-settings-section.tsx',[
 ('general','资金模式与通用设置','form','资金模式|能力与配置版本|金额预设 / 折扣|支付方式|最低充值|合规确认','', '当前能力与配置影响摘要|不将未知配置当作允许充值'),
 ('epay','Epay','form','服务地址|商户ID|密钥〔掩码〕|单价 / 最小金额|回调地址|支付方式','', 'Epay参数按任务归组|配置保存不发起支付'),
 ('stripe','Stripe','form','API Secret〔掩码〕|Webhook Secret〔掩码〕|Price ID|单价 / 最小金额|优惠码开关','', '支付与Webhook凭据分开|单位清楚，不修改计费或回调契约'),
 ('creem','Creem','form','API Key〔掩码〕|Webhook Secret〔掩码〕|测试模式|商品列表','', '商品配置与实际购买分开|不把测试模式当作调用授权'),
 ('waffo-pancake','Waffo Pancake','form','商户ID|私钥〔掩码〕|返回地址|App / SubApp ID|支付类型','', '保留当前提供方必需字段|不联系支付服务'),
 ('waffo','Waffo','form','生产 / 沙盒模式|API Key / 私钥〔掩码〕|证书配置|商户 / 币种|单价 / 最低充值|支付方式','', '生产和沙盒分组可见|切换时不静默覆盖另一组凭据'),
],parent='settings-billing-payment')
design_subviews('web/src/features/system-settings/models/ratio-settings-card.tsx',[
 ('models','已配置模型价格','table','','模型|计费模式|价格来源|单位|编辑','价格与比例明细在编辑器内展示|默认未知不补零'),
 ('unset-models','未设置价格模型','table','','模型|使用范围|缺失项|配置入口','未设置明确提示|不得把缺失价格默认为免费'),
 ('tool-prices','工具价格','table','','工具|计费单位|价格|来源|编辑','保留工具单位与精度|不修改计算规则'),
 ('upstream-sync','上游价格同步预览','table','','来源连接|模型|差异|冲突|待应用项','拉取预览与应用变更分开|冲突逐项确认|线稿不读取供应商'),
],parent='settings-billing-model-pricing')
tab_inventory.append(dict(source='web/src/features/system-settings/models/ratio-settings-card.tsx',value='groups',wireframes=['settings-billing-group-pricing'],note='分组价格已有独立分区线稿，不复制为第二套页面。'))
design_subviews('web/src/features/system-settings/models/model-pricing-sheet.tsx',[
 ('per-token','按Token计费','overlay','模型|输入 / 输出价格|缓存读取 / 写入|图像 / 音频等既有价格项|单位与来源','', '仅重组既有价格字段|不改变估算、预扣、结算与饱和审计'),
 ('per-request','按请求计费','overlay','模型|每请求价格|货币 / 单位|生效范围','', '固定价格与Token价格清楚区分|不把模式切换当作价格推断'),
 ('tiered_expr','分层 / 表达式计费','overlay','模型|现有表达式版本|表达式编辑|参数与单位说明|校验错误','', '只设计已有表达式编辑布局|不改语言、计算规则或金额安全边界'),
])
design_subviews('web/src/features/channels/components/dialogs/advanced-custom-editor-dialog.tsx',[
 ('forwarding','转发路由','overlay','路径匹配|上游路径|认证方式 / 凭据掩码|协议转换|规则顺序|兜底路由','', '路由按优先级排列|移动规则与替换全部路由分开确认'),
 ('models','模型规则','overlay','客户端模型匹配|上游模型名|允许模型|转换器参数','', '显式模型映射|未知字段与不支持能力保持原语义'),
 ('balance','余额查询规则','overlay','余额查询路径|认证方式〔掩码〕|结果字段 / 单位|错误处理','', '配置与实际查询分开|未知不补零|不调用供应商'),
 ('json','原始JSON','overlay','完整JSON草稿|结构校验错误|与结构化视图差异|替换确认','', '保留未知或直通字段|切换视图不静默丢失配置'),
])
design_subviews('web/src/features/channels/components/dialogs/codex-usage-dialog.tsx',[
 ('current','当前用量与窗口','detail','','','账号身份〔掩码〕|窗口周期 / 剩余额度证据|观察时间|未知与错误分开'),
 ('history','历史观察','table','','时间|窗口|用量证据|来源|错误','只展示已存历史|读取失败不冒充空历史'),
])
design_subviews('web/src/features/channels/components/dialogs/fetch-models-dialog.tsx',[
 ('new','新增模型候选','table','','模型|已选状态|来源|计划动作','显式选择要添加的模型|不直接改变已有模型范围'),
 ('existing','已存在模型','table','','模型|当前配置|来源|差异','明确当前值与新观察的差异|查看不写入'),
 ('removed','上游已移除候选','table','','模型|当前使用范围|影响|待移除选择','不得因上游缺失自动删除已有路由|移除需明确确认'),
])
design_subviews('web/src/features/channels/components/dialogs/upstream-update-dialog.tsx',[
 ('add','待新增模型','table','','模型|来源|待添加范围|选择','预览后只应用明确选中的新增项'),
 ('remove','待移除模型','table','','模型|现有配置|影响范围|选择','移除动作与新增分开|保留原配置与取消入口'),
])
add('connection-mapping','连接模型映射编辑器','08 编辑与弹层','组件：连接模型映射','form','web/src/features/channels/components/model-mapping-editor.tsx',
    fields='客户端模型名|上游模型名|映射规则草稿',actions=[A('返回连接编辑','connection-edit')],
    notes=['嵌入连接编辑器的模型分区，结构化 / JSON只是此组件的两种视图，不增加连接编辑器的全局导航层级。'])
next(p for p in pages if p['id']=='connection-edit')['actions'].append(dict(label='编辑模型映射',target='connection-mapping',effect='进入模型分区的映射草稿，不保存连接或触发测试'))
design_subviews('web/src/features/channels/components/model-mapping-editor.tsx',[
 ('visual','结构化模型映射','form','客户端模型名|上游模型名|规则列表|新增 / 删除草稿','', '明确映射方向|重复或冲突规则就近提示'),
 ('json','JSON模型映射','form','完整JSON草稿|校验错误|原始字段保留说明','', 'JSON与结构化视图往返不丢值|保存仍由来源连接编辑器明确提交'),
],parent='connection-mapping')
design_subviews('web/src/features/system-settings/content/chat-settings-section.tsx',[
 ('visual','聊天预设列表','table','','名称|Web / 客户端|链接模板|顺序|编辑','安全提示与模板变量可见|预览不打开外部客户端'),
 ('json','聊天预设JSON','form','完整预设JSON|结构校验|凭据变量风险提示','', '保留合法模板变量和所有现有字段|不载入真实聊天站点'),
],parent='settings-content-chat')
design_subviews('web/src/features/auth/secure-verification/components/secure-verification-dialog.tsx',[
 ('2fa','两步验证码验证','overlay','当前敏感任务|验证码 / 恢复码|错误 / 处理中状态','', '验证绑定当前安全任务和会话|失败不绕过维护门禁'),
 ('passkey','Passkey验证','overlay','当前敏感任务|设备验证提示|取消 / 失败状态','', 'Passkey不可用时说明原因|不自动降级为无验证的写入'),
])
design_subviews('web/src/features/pricing/components/model-details.tsx',[
 ('overview','能力与价格概览','detail','','','输入 / 输出能力|上下文信息|价格组成、单位与来源|未配置不冒充0'),
 ('performance','性能信息','detail','','','只展示实际存在的性能证据|采样窗口与来源|未提供明确标记未知'),
 ('api','API接入示例','detail','','','端点 / 协议|合成请求示例|掩码Key占位|复制示例不发送请求'),
],parent='model-price')
profile=next(p for p in pages if p['id']=='profile-info')
add('profile-bindings','个人资料 · 账户绑定','03 管理与账户','/profile〔bindings〕','form','web/src/features/profile/components/profile-settings-card.tsx',
    sections=[('已绑定身份','邮箱|GitHub / Discord / OIDC等既有绑定|Telegram / WeChat等已启用方式|明确绑定 / 解绑入口')],
    actions=[A('返回个人资料','profile-info'),A('安全验证','profile-security')],
    notes=['与基本资料设置分开成图；解绑前确认身份与目标，不删除账户或刷新登录来绕过验证。'])
profile['actions'].append(dict(label='账户绑定',target='profile-bindings',effect='进入当前身份的绑定管理；不自动绑定、解绑或切换账户'))
tab_inventory.extend([dict(source='web/src/features/profile/components/profile-settings-card.tsx',value='bindings',wireframes=['profile-bindings'],note='独立账户绑定布局。'),dict(source='web/src/features/profile/components/profile-settings-card.tsx',value='settings',wireframes=['profile-info'],note='复用基本资料与偏好布局。')])
for value in ['request','response']:
    tab_inventory.append(dict(source='web/src/features/full-content-logs/components/full-content-log-details-dialog.tsx',value=value,wireframes=['body-detail'],note='拟将原请求/响应两页签合并为桌面并列、手机上下两个内容面板；两部分都在图中，不删除内容。'))
for value in ['body','sse','raw']:
    tab_inventory.append(dict(source='web/src/features/usage-logs/request-page.tsx',value=value,wireframes=['request-'+value,'key-request-'+value],note='两种请求入口共用内容，但保留返回上下文。'))
for value in ['requests','scope','delivery']:
    tab_inventory.append(dict(source='web/src/features/keys/key-page.tsx',value=value,wireframes=['key-'+value],note='独立对象页签。'))
for value in ['windows','routing','requests','history']:
    tab_inventory.append(dict(source='web/src/features/connections/connection-page.tsx',value=value,wireframes=['connection-'+value],note='独立连接页签。'))

# Aliases, layouts, object identifiers and dynamic sections are all traceable.

# Aliases, layouts, object identifiers and dynamic sections are all traceable.
static_route_targets={
 '/(auth)/forgot-password':['forgot-password'], '/(auth)/oauth':['oauth-legacy'], '/(auth)/otp':['otp'],
 '/(auth)/register':['sign-up'], '/(auth)/reset':['reset-password'], '/(auth)':['shell-auth'],
 '/(auth)/sign-in':['sign-in'], '/(auth)/sign-up':['sign-up'], '/(auth)/user/reset':['reset-password'],
 '/_authenticated/channels/':['channels'], '/_authenticated/chat/$chatId':['chat-preset'], '/_authenticated/chat2link':['chat-link'],
 '/_authenticated/connections/$connectionId/':['connection-windows','connection-routing','connection-requests','connection-history'],
 '/_authenticated/connections/':['connections'], '/_authenticated/dashboard/':['overview'],
 '/_authenticated/full-content-logs/':['request-bodies','request-filters','body-detail','body-files'],
 '/_authenticated/keys/$keyId/':['key-requests','key-scope','key-delivery'],
 '/_authenticated/keys/$keyId/requests/$requestId':['key-request-detail','key-request-body','key-request-sse','key-request-raw'],
 '/_authenticated/keys/':['keys'], '/_authenticated/models/':['models'], '/_authenticated/playground/':['playground'],
 '/_authenticated/profile/':['profile-info','profile-security','profile-subscription'], '/_authenticated/prompt-learning/':['learning-policy','learning-versions','learning-runs'],
 '/_authenticated/redemption-codes/':['redemptions'], '/_authenticated':['shell-console'], '/_authenticated/subscriptions/':['subscriptions'],
 '/_authenticated/system-info/':['system-instances','system-jobs'], '/_authenticated/system-settings/':['settings-site-system-info'],
 '/_authenticated/system-settings':['shell-settings'], '/_authenticated/upstream-accounts/':['upstream-accounts'],
 '/_authenticated/usage-logs/':['requests'], '/_authenticated/usage-logs/request/$requestId':['request-detail','request-body','request-sse','request-raw'],
 '/_authenticated/users/':['users'], '/_authenticated/wallet/':['wallet','funding-history'], '/about/':['about'], '/':['home'],
 '/oauth/$provider':['oauth-callback'], '/pricing/$modelId/':['model-price'], '/pricing/':['pricing'],
 '/privacy-policy':['privacy'], '/rankings/':['rankings'], '/setup/':['setup-database','setup-admin','setup-mode','setup-review','setup-unknown'], '/user-agreement':['terms'],
}
for code in ['401','403','404','500','503']:
    static_route_targets['/(errors)/'+code]=['error-'+code]
static_route_targets['/_authenticated/errors/$error']=['console-error-'+c for c in ['401','403','404','500','503']]
static_route_targets['/_authenticated/dashboard/$section']=['overview','analytics-models','analytics-flow','analytics-users']
static_route_targets['/_authenticated/models/$section']=['models','deployments']
static_route_targets['/_authenticated/usage-logs/$section']=['requests','drawing','tasks']
for category,specs in settings_specs.items():
    static_route_targets['/_authenticated/system-settings/'+category+'/$section']=['settings-'+category+'-'+s for s in specs]
    first=next(iter(specs))
    static_route_targets['/_authenticated/system-settings/'+category+'/']=['settings-'+category+'-'+first]

for p in route_files:
    s=p.read_text();match=re.search(r"createFileRoute\(\s*['\"]([^'\"]+)",s)
    if not match: continue
    route=match.group(1)
    targets=static_route_targets[route]
    kind='layout' if route in ['/(auth)','/_authenticated','/_authenticated/system-settings'] else 'dynamic' if '$section' in route or '$error' in route else 'alias' if not re.search(r'\bcomponent:',s) or route in ['/(auth)/user/reset'] else 'page'
    route_map.append(dict(route=route,source=p.relative_to(ROOT).as_posix(),kind=kind,wireframes=targets,
                          note='同一实际界面的别名共用线稿，不把别名虚增为独立产品页。' if kind=='alias' else '对象ID共用该对象类型布局，具体权限和数据不在设计图中模拟成功。' if '$' in route else '源码登记与设计提案对应，不是运行时验收证据。'))

def normalize_source(value):
    p=ROOT/value
    if p.exists(): return value
    if p.suffix=='.tsx' and p.with_suffix('.ts').exists(): return p.with_suffix('.ts').relative_to(ROOT).as_posix()
    if p.name=='index.tsx':
        candidates=[q for q in p.parent.glob('*.tsx') if not q.name.startswith('._')]
        if len(candidates)==1: return candidates[0].relative_to(ROOT).as_posix()
    raise FileNotFoundError('Wireframe source needs explicit correction: '+value)

setup_actions = {
 'setup-database': [A('重新读取数据库状态','setup-database','只读检查，不创建或清空数据库'), A('下一步：管理员','setup-admin','只有状态确认可初始化时继续')],
 'setup-admin': [A('上一步','setup-database'),A('下一步：使用模式','setup-mode','字段验证通过后保存向导草稿，不提交初始化')],
 'setup-mode': [A('上一步','setup-admin'),A('下一步：复核','setup-review','仅进入复核，不提交初始化')],
 'setup-review': [A('返回修改','setup-mode'),A('确认初始化','setup-unknown','仅用户在此明确提交一次；处理中禁用，结果未知时不再提交')],
 'setup-unknown': [A('查询初始化状态','setup-unknown','只读GET状态恢复，绝不重放初始化写入'),A('已确认成功后进入系统','home','只有服务端明确确认完成才允许进入；未知时此项禁用')],
}
auth_actions = {
 'sign-in':[A('登录','otp','用户明确提交认证；按服务端结果进入验证或控制台，不预设成功'),A('Passkey / 外部登录','oauth-callback','只通过已配置且用户明确选择的方式继续'),A('找回密码','forgot-password'),A('注册','sign-up')],
 'sign-up':[A('创建账户','sign-in','用户明确提交注册一次，失败保留安全字段并显示验证结果'),A('已有账户：登录','sign-in'),A('用户协议','terms'),A('隐私政策','privacy')],
 'forgot-password':[A('发送恢复邮件','forgot-password','只有用户明确操作才请求一次；线稿不发送任何邮件'),A('返回登录','sign-in')],
 'reset-password':[A('确认新密码','sign-in','验证有效链接及密码后明确提交，结果未知不自动重放'),A('链接过期：重新申请','forgot-password')],
 'otp':[A('验证','overview','明确认证请求，成功与失败由服务端确认'),A('改用恢复码','otp','切换输入方式，不提交验证'),A('取消登录','sign-in')],
 'oauth-legacy':[A('失败后返回登录','sign-in','不自动再次交换授权码或重放登录写入')],
 'oauth-callback':[A('失败后返回登录','sign-in','不自动再次交换授权码；成功仅按已有结果进入系统')],
}
for page in pages:
    if page['id'] in setup_actions: page['actions']=[dict(label=a[0],target=a[1],effect=a[2]) for a in setup_actions[page['id']]]
    if page['id'] in auth_actions: page['actions']=[dict(label=a[0],target=a[1],effect=a[2]) for a in auth_actions[page['id']]]
    page['sources']=[normalize_source(s) for s in page['sources']]
    page['common_notes']=COMMON_NOTES
    page['variants']='桌面1200px、手机390px；320/768/1024/1280/1440与亮暗/英文的实际渲染仍待获批CI浏览器验收。'

public_actions={
 'home':[A('进入控制台','overview'),A('查看模型目录','pricing'),A('了解MyAPI','about')],
 'about':[A('返回首页','home'),A('隐私政策','privacy'),A('用户协议','terms')],
 'pricing':[A('查看模型详情','model-price'),A('按现有要求登录','sign-in','仅在服务端要求认证时显示登录提示'),A('返回首页','home')],
 'model-price':[A('返回定价目录','pricing'),A('查看接入示例','model-price-view-api','只展示合成示例，不发出请求')],
 'rankings':[A('应用榜单筛选','rankings','只读取允许公开的聚合统计'),A('返回首页','home')],
 'privacy':[A('返回来源页','home','恢复实际调用页；目录以首页作为示意'),A('用户协议','terms')],
 'terms':[A('返回来源页','home','恢复实际调用页；目录以首页作为示意'),A('隐私政策','privacy')],
 'website':[A('产品版本说明','website','导航到官网的版本说明区'),A('部署文档入口','website','保留实际已存在的文档地址，不执行安装、构建或部署')],
}
for page in pages:
    actions=public_actions.get(page['id'])
    if page['kind']=='error':
        code=page['id'].rsplit('-',1)[-1]
        target='overview' if page['id'].startswith('console-') else 'home'
        actions=[A('重新登录','sign-in'),A('返回首页',target)] if code=='401' else [A('返回可用页面',target)]
        if code in ['500','503']:actions.append(A('重新读取页面状态',page['id'],'仅重试读取，绝不重新提交失败或结果未知的业务写入'))
    if actions:page['actions']=[dict(label=a[0],target=a[1],effect=a[2]) for a in actions]

known={p['id'] for p in pages}
for page in pages:
    for action in page['actions']:
        if action['target'] not in known: raise ValueError('Unmapped design action: '+str(action))
    for target,label in page['tabs']:
        if target not in known: raise ValueError('Unmapped design tab: '+target)
for entry in route_map:
    for target in entry['wireframes']:
        if target not in known: raise ValueError('Unmapped route: '+target)

catalog=dict(title='MyAPI 全站线稿目录',source_sha=BASE,date='2026-10-28',direction='已确认方向3：流向图 / 顶部导航 / 对象整页',
             evidence='设计源文件；未运行浏览器、产品构建或本机测试。',pages=pages,routes=route_map,tabs=tab_inventory,overlay_inventory=overlay_inventory,
             unconfirmed_overlay_sources=unconfirmed,
             scope_note='覆盖当前66条文件路由并展开动态分区、对象页签、安装步骤和静态官网。弹层清单来自静态导入图；未确认可达候选单列保留，不认定死代码或删除。')

import unicodedata

def units(value):
    return sum(2 if unicodedata.east_asian_width(c) in 'WF' else 1 for c in str(value))

def wrap(value, width, font=14):
    limit=max(8,int(width/(font*0.56)))
    result=[];line='';used=0
    for char in str(value):
        weight=2 if unicodedata.east_asian_width(char) in 'WF' else 1
        if char=='\n' or (used+weight>limit and line):
            result.append(line);line='';used=0
        if char!='\n': line+=char;used+=weight
    if line:result.append(line)
    return result or ['']

class Drawing:
    def __init__(self,page,mobile):
        self.page=page;self.mobile=mobile;self.width=390 if mobile else 1200
        self.margin=16 if mobile else 40;self.cw=self.width-self.margin*2
        self.parts=[];self.y=0
    def rect(self,x,y,w,h,fill='#ffffff',stroke='#777777',dash=False,rx=5):
        self.parts.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{rx}" fill="{fill}" stroke="{stroke}"'+(' stroke-dasharray="5 4"' if dash else '')+'/>')
    def text(self,x,y,value,size=14,weight='normal',color='#262626',width=None):
        lines=wrap(value,width,size) if width else [str(value)]
        for idx,line in enumerate(lines):
            self.parts.append(f'<text x="{x}" y="{y+idx*(size+7)}" font-size="{size}" font-weight="{weight}" fill="{color}">{html.escape(line)}</text>')
        return len(lines)*(size+7)
    def line(self,x1,y1,x2,y2,color='#aaaaaa',dash=False):
        self.parts.append(f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" stroke="{color}"'+(' stroke-dasharray="4 4"' if dash else '')+'/>')
    def banner(self,text):
        h=max(48,24+len(wrap(text,self.cw-24,12))*19)
        self.rect(self.margin,self.y,self.cw,h,'#f5f5f5','#aaaaaa',dash=True)
        self.text(self.margin+12,self.y+22,text,12,width=self.cw-24)
        self.y+=h+16
    def block(self,title,items,x=None,w=None,y=None):
        x=self.margin if x is None else x;w=self.cw if w is None else w;y=self.y if y is None else y
        inner=w-28
        title_h=len(wrap(title,inner,16))*23
        lines=[wrap(item,inner-12,13) for item in items]
        h=24+title_h+sum(len(a)*20+10 for a in lines)+10
        self.rect(x,y,w,h,'#ffffff','#888888')
        cursor=y+26
        self.text(x+14,cursor,title,16,'600',width=inner);cursor+=title_h+12
        self.line(x+14,cursor-6,x+w-14,cursor-6)
        for item_lines in lines:
            self.text(x+14,cursor+14,'· '+item_lines[0],13,width=inner)
            for extra in item_lines[1:]:
                cursor+=20;self.text(x+24,cursor+14,extra,13,width=inner-10)
            cursor+=30
        return h
    def buttons(self,actions=None):
        actions=actions if actions is not None else self.page['actions']
        x=self.margin;row_y=self.y;row_h=48;gap=8
        for a in actions:
            if not self.mobile and a['label'].startswith('手机：'):continue
            idx=self.page['actions'].index(a)
            label=f'A{idx+1} '+a['label']
            w=min(self.cw,max(120,min(300,units(label)*7+28)))
            if self.mobile:w=min(self.cw,max(160,min(self.cw,units(label)*7+28)))
            if x+w>self.width-self.margin and x>self.margin:
                row_y+=row_h+gap;x=self.margin
            h=max(44,16+len(wrap(label,w-20,13))*20)
            self.rect(x,row_y,w,h,'#eeeeee' if idx==0 else '#ffffff','#555555')
            self.text(x+10,row_y+23,label,13,'600' if idx==0 else 'normal',width=w-20)
            x+=w+gap;row_h=max(row_h,h)
        self.y=row_y+row_h+16
    def fields(self,values,label='查询与筛选'):
        if not values:return
        self.text(self.margin,self.y+18,label,15,'600');self.y+=32
        cols=1 if self.mobile else min(4 if label=='查询与筛选' else 3,len(values));gap=12;cw=(self.cw-gap*(cols-1))/cols
        row_heights=[]
        for start in range(0,len(values),cols):
            row=values[start:start+cols]
            h=max(82,42+max(len(wrap(v,cw-22,12)) for v in row)*19)
            for j,v in enumerate(row):
                x=self.margin+j*(cw+gap)
                self.text(x,self.y+16,v,12,'600',width=cw-8)
                text_h=len(wrap(v,cw-8,12))*19
                self.rect(x,self.y+text_h+6,cw,44,'#fafafa','#999999')
                self.text(x+10,self.y+text_h+33,'选择 / 输入 …',13,color='#666666',width=cw-20)
            self.y+=h+12
        self.y+=4
    def stats(self):
        values=self.page['stats']
        if not values:return
        cols=min(2 if self.mobile else 4,len(values));gap=10;cw=(self.cw-gap*(cols-1))/cols
        for start in range(0,len(values),cols):
            row=values[start:start+cols]
            h=max(80,28+max(len(wrap(v,cw-24,14)) for v in row)*22)
            for j,value in enumerate(row):
                x=self.margin+j*(cw+gap)
                self.rect(x,self.y,cw,h,'#f7f7f7','#aaaaaa')
                self.text(x+12,self.y+28,value,14,'600',width=cw-24)
            self.y+=h+10
        self.y+=6
    def tabs(self):
        tabs=self.page['tabs']
        if not tabs:return
        x=self.margin;row_y=self.y
        for target,label in tabs:
            w=max(88,units(label)*7+24)
            if x+w>self.width-self.margin:
                row_y+=52;x=self.margin
            selected=target==self.page['id']
            self.rect(x,row_y,w,44,'#dedede' if selected else '#ffffff','#555555' if selected else '#aaaaaa')
            self.text(x+10,row_y+27,label,13,'600' if selected else 'normal')
            x+=w+8
        self.y=row_y+60
    def sections(self,columns=True):
        values=self.page['sections']
        cols=2 if columns and not self.mobile and len(values)>1 else 1
        gap=16;cw=(self.cw-gap*(cols-1))/cols
        for start in range(0,len(values),cols):
            hs=[]
            for j,section in enumerate(values[start:start+cols]):
                hs.append(self.block(section['title'],section['items'],x=self.margin+j*(cw+gap),w=cw,y=self.y))
            self.y+=max(hs)+16
    def table(self):
        columns=self.page['columns'] or ['对象 / 内容','状态证据','操作']
        if self.mobile:
            for row in range(2):
                vals=[col+'：'+sample_value(col,row) for col in columns if not (self.page['id']=='request-bodies' and col=='查看内容')]
                h=self.block('记录 '+str(row+1)+'〔合成占位〕',vals)
                self.y+=h+8
                self.rect(self.margin,self.y,self.cw,44,'#fafafa','#888888')
                self.text(self.margin+12,self.y+27,'A6 查看内容 · 返回保留位置' if self.page['id']=='request-bodies' else '查看该记录 · 返回保留位置',13)
                self.y+=60
        else:
            cw=self.cw/len(columns)
            header_h=max(52,18+max(len(wrap(v,cw-20,12)) for v in columns)*19)
            cell_lines=max(len(wrap(sample_value(c,0),cw-20,12)) for c in columns)
            row_h=max(46,16+cell_lines*19)
            height=header_h+row_h*4
            self.rect(self.margin,self.y,self.cw,height,'#ffffff','#777777')
            self.rect(self.margin,self.y,self.cw,header_h,'#eeeeee','#777777')
            for i,col in enumerate(columns):
                x=self.margin+cw*i
                self.text(x+10,self.y+23,col,12,'600',width=cw-20)
                if i:self.line(x,self.y,x,self.y+height)
                for row in range(4):
                    y=self.y+header_h+row*row_h
                    value='A6 查看内容' if self.page['id']=='request-bodies' and col=='查看内容' else sample_value(col,row)
                    self.text(x+10,y+24,value,12,width=cw-20)
            for row in range(1,4):self.line(self.margin,self.y+header_h+row*row_h,self.width-self.margin,self.y+header_h+row*row_h)
            self.y+=height+20
        self.pager()
    def pager(self):
        title='共 — 条 · 第 p / N 页〔从真实响应读取；未知时禁用跳页〕'
        self.text(self.margin,self.y+18,title,12,width=self.cw);self.y+=40 if self.mobile else 32
        rows=[('每页条数','20 / 50 / 100'),('页码','首页  ‹  1  2  …  N  ›  末页'),('直接跳页','第 [ 输入页码 ] 页  [ 跳转 ]')]
        if not self.mobile:
            widths=[250,440,self.cw-706];x=self.margin
            for (label,content),w in zip(rows,widths):
                self.rect(x,self.y,w,48,'#fafafa','#999999')
                self.text(x+10,self.y+29,label+'：'+content,12,width=w-20)
                x+=w+8
            self.y+=68
            return
        for label,content in rows:
            self.rect(self.margin,self.y,self.cw,48,'#fafafa','#999999')
            self.text(self.margin+12,self.y+29,label+'：'+content,12,width=self.cw-24)
            self.y+=58
        self.y+=8
    def calendar(self):
        self.buttons(self.page['actions'][2:])
        count=1 if self.mobile else 2;gap=16;w=(self.cw-gap*(count-1))/count
        box_y=self.y
        for idx in range(count):
            x=self.margin+idx*(w+gap);cell=min(56,(w-28)/7);left=x+(w-cell*7)/2
            h=116+6*cell
            self.rect(x,box_y,w,h,'#ffffff','#777777')
            self.text(x+14,box_y+30,('‹  2026 年 10 月  ›' if idx==0 else '‹  2026 年 11 月  ›'),15,'600',width=w-28)
            self.text(x+14,box_y+55,'起点 / 终点在同一选区内选择〔示例〕',11,width=w-28)
            for j,day in enumerate(['一','二','三','四','五','六','日']):self.text(left+j*cell+cell*.35,box_y+90,day,12)
            offset=3 if idx==0 else 6;days=31 if idx==0 else 30
            for d in range(1,days+1):
                pos=d-1+offset;row=pos//7;col=pos%7;cy=box_y+104+row*cell;cx=left+col*cell
                selected=idx==0 and 24<=d<=28
                self.rect(cx,cy,cell,cell,'#d5d5d5' if selected else '#ffffff','#bbbbbb',rx=0)
                self.text(cx+cell*.30,cy+cell*.64,str(d),13,'600' if selected else 'normal')
                if selected and d in [24,28]:self.rect(cx+2,cy+2,cell-4,cell-4,'none','#333333',rx=4)
        self.y=box_y+116+6*cell+18
        self.fields(self.page['fields'],'精确时间与时区')
        self.sections()
    def draw(self):
        p=self.page
        public=p['kind'] in ['auth','setup'] or p['id'] in ['oauth-legacy','oauth-callback','home','about','pricing','model-price','rankings','privacy','terms','website','shell-auth'] or p['id'].startswith('error-')
        self.rect(0,0,self.width,56,'#f0f0f0','#777777',rx=0)
        self.text(self.margin,34,'MyAPI  [Logo保留]',14,'600')
        if not self.mobile:self.text(250,34,'文档 / 登录 / 控制台' if public else '概览    上游    API Key    请求    设置    管理 ▾       搜索 / 账户',14)
        else:self.text(266,34,'搜索  /  菜单',12)
        self.y=78
        self.y+=self.text(self.margin,self.y+18,p['title'],22 if self.mobile else 26,'600',width=self.cw)+10
        self.y+=self.text(self.margin,self.y+14,p['url'],11,color='#666666',width=self.cw)+12
        self.banner('低保真设计提案 · 合成占位 · 待确认 · 不连接API / 不执行业务动作')
        self.tabs();self.stats()
        if p['id']=='request-bodies':
            self.buttons([p['actions'][3],p['actions'][4]])
            if self.mobile:
                self.rect(self.margin,self.y,self.cw,64,'#fafafa','#888888')
                self.text(self.margin+12,self.y+24,'A7 筛选：日期 / 模型 / Key / Request ID',12,width=self.cw-24)
                self.text(self.margin+12,self.y+48,'已应用条件摘要                       展开 ›',12,width=self.cw-24)
                self.y+=80
            else:
                self.fields(['A3 日期范围〔同一日历〕','模型','API Key','Request ID'])
                self.buttons(p['actions'][:2])
        elif p['filters']:self.fields(p['filters'])
        kind=p['kind']
        footer_actions=kind in ['form','overlay','auth','setup','calendar','chat']
        if not footer_actions and p['id']!='request-bodies':self.buttons()
        if kind=='flow':
            values=p['sections'];cols=1 if self.mobile else 3;gap=24;cw=(self.cw-gap*(cols-1))/cols
            for start in range(0,len(values),cols):
                hs=[]
                for j,s in enumerate(values[start:start+cols]):
                    x=self.margin+j*(cw+gap)
                    hs.append(self.block(s['title'],s['items'],x=x,w=cw,y=self.y))
                    if j and not self.mobile:self.text(x-22,self.y+66,'→',19,'600')
                self.y+=max(hs)+20
                if self.mobile and start<len(values)-1:self.text(self.width/2-7,self.y,'↓',19);self.y+=25
            self.table()
        elif kind in ['table','analytics']:
            if kind=='analytics':
                h=self.block('趋势与分布〔结构线稿，不模拟真实统计〕',['横轴：时间 / 采样窗口','纵轴与单位明确；缺失数据断开','图例和数值表均可访问'])
                self.y+=h+20
                self.rect(self.margin,self.y,self.cw,150,'#fafafa','#aaaaaa',dash=True)
                for j in range(1,4):self.line(self.margin+20,self.y+j*30,self.width-self.margin-20,self.y+j*30,dash=True)
                self.text(self.margin+20,self.y+132,'图表占位 · 数据未知 · 保留表格替代视图',13);self.y+=170
            self.sections();self.table()
        elif kind=='calendar':self.calendar()
        elif kind=='overlay':
            self.banner('手机：单列全屏任务；桌面：聚焦弹层。关闭后返回原列表并恢复焦点。')
            self.fields(p['fields'],'任务字段 / 当前证据');self.sections(columns=False)
        elif kind in ['auth','setup','form']:
            if kind=='setup':self.banner('数据库检查 → 管理员 → 使用模式 → 复核；提交结果未知时只读恢复')
            self.fields(p['fields'],'当前任务字段');self.sections()
        elif kind=='detail':
            self.sections()
            if not p['sections']:
                h=self.block('对象详情',['身份与来源','已存状态 / 证据','明确动作与影响']);self.y+=h+16
        elif kind=='chat':
            self.sections(columns=not self.mobile)
            h=self.block('输入区 / 发送前检查',['多行输入，不遮挡错误与附件','模型、Key与费用边界明确','仅明确发送才产生请求；结果未知不重发']);self.y+=h+16
        elif kind=='embed':
            self.sections(columns=False)
            self.rect(self.margin,self.y,self.cw,260,'#fafafa','#777777',dash=True)
            self.text(self.margin+20,self.y+80,'外部内容边界〔不载入实际iframe〕',16,'600',width=self.cw-40)
            self.text(self.margin+20,self.y+125,'第三方界面不属于MyAPI可重设计范围；这里只设计项目容器与异常入口。',13,width=self.cw-40)
            self.y+=280
        else:self.sections(columns=kind not in ['document','status','error'])
        if p['fields'] and kind not in ['auth','setup','form','calendar','overlay']:self.fields(p['fields'],'补充任务字段')
        if p['notes']:
            h=self.block('设计约束 / 恢复行为',p['notes']);self.y+=h+18
        if p['merge']:
            h=self.block('拟精简 / 合并方式',[p['merge'],'本轮只提交设计，不删除页面、参数、权限或业务功能。']);self.y+=h+18
        if footer_actions:self.buttons(p['actions'][:2] if kind=='calendar' else None)
        if self.mobile and not public and kind!='overlay':
            self.rect(0,self.y,self.width,64,'#eeeeee','#777777',rx=0)
            self.text(20,self.y+38,'概览      上游      Key      请求      更多',13)
            self.y+=80
        self.y+=self.text(self.margin,self.y+14,p['id']+' · '+('手机390' if self.mobile else '桌面1200')+' · 来源 '+BASE[:8]+' · 浏览器未验证',10,color='#666666',width=self.cw)+22
        height=max(650,self.y)
        title=html.escape(p['title']+' · '+('手机' if self.mobile else '桌面'))
        desc=html.escape('低保真设计源文件，合成数据，待确认。'+p['access'])
        return f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.width}" height="{height}" viewBox="0 0 {self.width} {height}" role="img" aria-labelledby="title desc"><title id="title">{title}</title><desc id="desc">{desc}</desc><rect width="100%" height="100%" fill="#ffffff"/><g font-family="system-ui, -apple-system, BlinkMacSystemFont, sans-serif">'+''.join(self.parts)+'</g></svg>\n'

def sample_value(column,row):
    if any(k in column for k in ['时间','心跳','创建']):return '10-28 10:20〔合成〕'
    if any(k in column for k in ['ID','id']):return 'SYNTHETIC-'+str(row+1)
    if 'Key' in column or '掩码' in column:return '演示 Key · ••••'
    if any(k in column for k in ['状态','结果','证据','结算','余额','金额','价格','用量','大小','额度','耗时','费用','成本']):return '未知 / —'
    if '操作' in column or '查看' in column or '详情' in column:return '查看 / 更多'
    return '合成示例 '+chr(65+row)

def review_html(catalog):
    groups=list(dict.fromkeys(p['group'] for p in pages))
    groups.sort(key=lambda group:group[:2])
    nav=''
    for group in groups:
        group_pages=[p for p in pages if p['group']==group]
        nav+='<details open><summary>'+html.escape(group)+f' ({len(group_pages)})</summary>'
        for p in group_pages:
            nav+=f'<a data-page-link data-search="{html.escape(p["title"]+" "+p["url"],quote=True)}" href="#{p["id"]}">{html.escape(p["title"])}</a>'
        nav+='</details>'
    articles=''
    for p in pages:
        actions=''.join(f'<tr><td>A{i+1}</td><td>{html.escape(a["label"])}</td><td><a href="#{a["target"]}">对应线稿</a></td><td>{html.escape(a["effect"])}</td></tr>' for i,a in enumerate(p['actions']))
        tabs=''.join(f'<a href="#{target}">{html.escape(label)}</a>' for target,label in p['tabs'])
        notes=''.join('<li>'+html.escape(note)+'</li>' for note in p['notes']+COMMON_NOTES)
        sources=''.join(f'<li><a href="https://github.com/ForceMind/MyAPI/blob/{BASE}/{html.escape(s,quote=True)}">{html.escape(s)}</a></li>' for s in p['sources'])
        articles+=f'''<article id="{p['id']}" data-wireframe><header><p class="eyebrow">{html.escape(p['group'])} · 设计提案 / 待确认</p><h1>{html.escape(p['title'])}</h1><p class="route">{html.escape(p['url'])}</p><p>{html.escape(p['access'])}</p><nav class="tabs">{tabs}</nav></header><div class="figures"><figure class="desktop"><figcaption>桌面 1200px · <a href="svg/{p['id']}-desktop.svg">单独打开SVG</a></figcaption><img loading="lazy" src="svg/{p['id']}-desktop.svg" alt="{html.escape(p['title'],quote=True)} 桌面线稿"></figure><figure class="mobile"><figcaption>手机 390px · <a href="svg/{p['id']}-mobile.svg">单独打开SVG</a></figcaption><img loading="lazy" src="svg/{p['id']}-mobile.svg" alt="{html.escape(p['title'],quote=True)} 手机线稿"></figure></div><section class="spec"><h2>按钮与交互去向</h2><div class="table-wrap"><table><thead><tr><th>编号</th><th>操作</th><th>设计去向</th><th>行为 / 边界</th></tr></thead><tbody>{actions}</tbody></table></div><h2>拟精简 / 合并</h2><p>{html.escape(p['merge'] or '复用方向3外壳与共用控件，保留该页独立业务与权限。')}</p><h2>状态、恢复与权限</h2><ul>{notes}</ul><p><a href="#state-board">共用状态线稿</a> · <a href="#component-rules">按钮与控件规范</a> · <a href="#date-range">日期范围</a> · <a href="#pagination">分页</a></p><h2>源码依据</h2><ul>{sources}</ul><p class="muted">{html.escape(p['variants'])}</p></section></article>'''
    return '''<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self' file: data:; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; connect-src 'none'"><title>MyAPI 全站低保真线稿</title><style>
*{box-sizing:border-box}[hidden]{display:none!important}html{color-scheme:light}body{margin:0;background:#eee;color:#222;font:14px/1.65 system-ui,-apple-system,sans-serif}a{color:inherit;text-underline-offset:3px}button,input{font:inherit}button{min-height:44px;background:#fff;border:1px solid #777;border-radius:6px;padding:8px 14px;cursor:pointer}button:focus-visible,a:focus-visible,input:focus-visible{outline:3px solid #333;outline-offset:3px}.review-header{position:sticky;top:0;z-index:2;background:#fafafa;border-bottom:1px solid #999;padding:12px 24px;display:flex;gap:16px;align-items:center;flex-wrap:wrap}.review-header strong{font-size:18px}.review-header p{margin:0}.badge{font-size:12px;border:1px dashed #777;padding:3px 7px}.layout{display:grid;grid-template-columns:272px minmax(0,1fr)}aside{position:sticky;top:90px;max-height:calc(100vh - 90px);overflow:auto;background:#fafafa;padding:16px;border-right:1px solid #aaa}input{width:100%;min-height:44px;padding:9px;border:1px solid #999;border-radius:5px}details{margin:16px 0}summary{cursor:pointer;font-weight:650;min-height:36px}aside a{display:block;padding:8px;border-radius:5px;text-decoration:none;overflow-wrap:anywhere}aside a[aria-current=page]{background:#dedede;font-weight:650}aside a:hover{background:#e8e8e8}main{min-width:0;padding:24px}article{display:none}article.active{display:block}article header,.spec{background:#fff;border:1px solid #aaa;padding:20px;margin-bottom:20px;border-radius:8px}.eyebrow,.muted{color:#555;font-size:12px}.route{overflow-wrap:anywhere;font-family:ui-monospace,monospace}h1{font-size:26px;margin:8px 0}h2{font-size:18px;margin-top:24px}.tabs{display:flex;flex-wrap:wrap;gap:8px}.tabs a{border:1px solid #aaa;border-radius:5px;padding:7px 12px}.figures{display:grid;grid-template-columns:minmax(0,1fr) minmax(280px,390px);gap:20px;align-items:start}figure{margin:0 0 20px;min-width:0}figcaption{padding:10px;background:#fafafa;border:1px solid #aaa;font-weight:600}img{width:100%;display:block;background:#fff;border:1px solid #aaa;border-top:0}figure.mobile{max-width:390px;width:100%;justify-self:center}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse}td,th{padding:10px;border:1px solid #bbb;text-align:left;vertical-align:top;min-width:80px}td:last-child{min-width:220px}[data-viewport=desktop] .figures{grid-template-columns:1fr}[data-viewport=desktop] .mobile{display:none}[data-viewport=mobile] .figures{grid-template-columns:1fr}[data-viewport=mobile] .desktop{display:none}.not-found{display:none;padding:24px;background:#fff}.footer-note{font-size:12px;color:#555;margin:16px 0;overflow-wrap:anywhere}@media(max-width:1100px){.figures{grid-template-columns:1fr}.layout{grid-template-columns:220px minmax(0,1fr)}}@media(max-width:720px){.review-header{position:static;padding:12px}.layout{display:block}aside{position:static;max-height:320px;border-bottom:1px solid #999}main{padding:12px}article header,.spec{padding:16px}h1{font-size:22px}.figures{gap:10px}}@media print{.review-header,aside{display:none}.layout{display:block}main{padding:0}article{display:block!important;break-before:page}.figures{display:block}figure{break-inside:avoid}img{max-width:100%}}
</style></head><body data-viewport="both"><header class="review-header"><strong>MyAPI 全站线稿</strong><span class="badge">方向3 · 灰度结构图 · 待确认</span><p>源代码 '''+BASE[:8]+f''' · {len(pages)} 个页面/分区/交互视图 · {len(pages)*2} 张SVG</p><button type="button" data-view="both">桌面与手机</button><button type="button" data-view="desktop">只看桌面</button><button type="button" data-view="mobile">只看手机</button></header><div class="layout"><aside><label for="search">搜索页面 / 路径</label><input id="search" type="search" placeholder="正文、设置、Key…"><p class="footer-note">本目录只展示设计，不连接API，不调用供应商，不执行保存、删除、部署或支付。<br><a href="README.md">审阅说明</a> · <a href="routes.csv">66条路由覆盖表</a> · <a href="catalog.json">设计数据</a></p>'''+nav+'''</aside><main><p class="not-found" role="status">未找到此线稿，请从目录选择。所有产品操作均只是设计说明。</p>'''+articles+'''<p class="footer-note">浏览器未验证；静态线稿不等于页面实现、视觉验收或生产升级证明。</p></main></div><script>
(() => {
 const articles = [...document.querySelectorAll('[data-wireframe]')];
 const links = [...document.querySelectorAll('[data-page-link]')];
 function show() {
   let id = 'request-bodies';
   try { id = decodeURIComponent(location.hash.slice(1)) || id; } catch { id = ''; }
   const found = articles.some(article => article.id === id);
   for (const article of articles) article.classList.toggle('active', article.id === id);
   for (const link of links) {
     if (link.getAttribute('href') === '#' + id) link.setAttribute('aria-current', 'page');
     else link.removeAttribute('aria-current');
   }
   document.querySelector('.not-found').style.display = found ? 'none' : 'block';
   const active = articles.find(article => article.id === id);
   if (active) document.title = active.querySelector('h1').textContent + ' · MyAPI 线稿';
 }
 window.addEventListener('hashchange', show);
 document.querySelector('#search').addEventListener('input', event => {
   const query = event.target.value.trim().toLocaleLowerCase();
   for (const link of links) link.hidden = !link.dataset.search.toLocaleLowerCase().includes(query);
 });
 for (const button of document.querySelectorAll('[data-view]')) {
   button.addEventListener('click', () => { document.body.dataset.viewport = button.dataset.view; });
 }
 show();
})();
</script></body></html>\n'''

if OUT.exists() and '--replace-generated' not in sys.argv:
    raise FileExistsError('Design directory already exists; review changes before explicitly choosing --replace-generated')
if OUT.exists():
    own_catalog=json.loads((OUT/'catalog.json').read_text())
    if own_catalog['source_sha']!=BASE or own_catalog['title']!='MyAPI 全站线稿目录':
        raise FileExistsError('Refuse to overwrite unrelated files: '+str(OUT))
    old_ids={p['id'] for p in own_catalog['pages']}
    for removed_id in old_ids-{p['id'] for p in pages}:
        for variant in ['desktop','mobile']:
            owned=OUT/'svg'/(removed_id+'-'+variant+'.svg')
            if owned.exists():owned.unlink()
else:
    OUT.mkdir()
    (OUT/'svg').mkdir()
for page in pages:
    for mobile in [False,True]:
        filename=page['id']+('-mobile.svg' if mobile else '-desktop.svg')
        with (OUT/'svg'/filename).open('w') as stream:stream.write(Drawing(page,mobile).draw())
with (OUT/'catalog.json').open('w') as stream:stream.write(json.dumps(catalog,ensure_ascii=False,indent=2)+'\n')
with (OUT/'index.html').open('w') as stream:stream.write(review_html(catalog))
csv_buffer=io.StringIO();writer=csv.writer(csv_buffer)
writer.writerow(['route','kind','source','wireframe_ids','note'])
for entry in route_map:writer.writerow([entry['route'],entry['kind'],entry['source'],';'.join(entry['wireframes']),entry['note']])
with (OUT/'routes.csv').open('w') as stream:stream.write(csv_buffer.getvalue())
sections_count=sum(len(specs) for specs in settings_specs.values())
readme=f'''# MyAPI 全站低保真线稿 · 2026-10-28

本批是设计提案，不是产品实现。维护者要求“每个页面都要画线稿”，方向3「流向图」继续沿用。

## 查看入口

- 打开 [离线线稿目录](index.html)，默认显示“正文与 SSE”。目录可按名称/路径搜索，可切换桌面与手机。
- 每个视图均有独立SVG，可直接打开；目录不需要Node、Bun、Go、Docker、服务器或第三方依赖。
- [正文与SSE桌面](svg/request-bodies-desktop.svg)、[手机](svg/request-bodies-mobile.svg)、[单个日历范围选择桌面](svg/date-range-desktop.svg)、[手机](svg/date-range-mobile.svg)。
- 灰度只表达信息层级、布局、动作和状态。Logo保持占位标记“保留”，没有重画或替换现有Logo。

## 当前覆盖与证据

- 源码基线：`{BASE}`。66条`createFileRoute`登记逐条映射在 [routes.csv](routes.csv)。布局、重定向、兼容地址与实际内容页分别标记，不能把66当作66个独立页面。
- 设置的{sections_count}个独立分区逐项展开；另含概览/模型/请求动态分区、Key与连接页签、两种请求详情入口、安装向导4步及提交结果未知态、公共/嵌入式错误页、静态官网、Desktop托盘/帮助。
- 页面内部的{len(tab_inventory)}个内容页签/模式对应记录在`catalog.json.tabs`，包括OAuth、支付、价格、编辑器、请求详情与账户绑定；不同内容各自成图，拟合并到同一图的请求/响应明确说明布局与保留内容。
- 总计 **{len(pages)} 个页面、分区与交互视图，{len(pages)*2} 张SVG**，每个视图各有桌面1200px、手机390px一张。不能把视图数当作实际路由数或已实现页面数。
- 弹层候选按现有源码与静态导入图整理，记录于`catalog.json.overlay_inventory`（{len(overlay_inventory)}个源文件）；通用壳与各业务弹层分别表达，图中提取现有标题、字段标签和动作。静态导入图不是运行时权限或所有条件分支的验证。
- `catalog.json.unconfirmed_overlay_sources` 的{len(unconfirmed)}个候选未从静态图确认可达，单列保留，不称死代码，不删除；它们不作为已存在用户页面计数。
- 字段与行为依据来自源码；设计图未在浏览器渲染，**浏览器未验证**。本机未安装依赖、运行产品构建、单测、Playwright或Docker，也未下载CI构件。

## 优先改进

1. 正文与SSE：保留服务端分页，增加页码、首尾页、20/50/100条、直接跳页；回到列表保留筛选、页码和焦点。后端`common/page_info.go`现有限制最多100条，不扩大限额。
2. 单个日期范围组件：同一日历点起止日期，跨月选择、快捷范围、精确时间、时区、取消/应用和起止错误。不是把两个独立`datetime-local`框简单放进弹层。
3. 共用分页、筛选、按钮、表单、反馈、确认外壳；连接/渠道是同一对象的卡片/表格视图；两种请求入口共用详情内容但保留返回上下文与权限。
4. 每页有动作编号与目标线稿，手机列出全部字段而非缩小桌面表格。高频动作直接显示，低频动作归组；危险操作保留清楚的对象、范围和取消，不通过删除确认步骤“精简”。

## 逐页审阅方法

1. 在目录查看桌面和手机，核对首屏目标、信息层级、字段、按钮与状态。
2. 检查动作表的目标和影响，尤其只读查看、真实测试、删除、支付、凭据和初始化。
3. 确认拟合并项是否保留现有能力、旧链接、角色/版本权限及返回状态。
4. 按页面批次确认后才修改产品；本批未修改`web/`、CLI、后端、数据库、生产或发布配置。

## 不应从本批推导的结论

- 线稿中的按钮不调用API；SVG及目录的点击仅用于设计导航。不存在测试支付、删除日志、初始化、生成或部署。
- 实际320/768/1024/1280/1440、英文、亮暗主题、可访问性和交互渲染尚待获批的GitHub Actions浏览器验收。390/1200为线稿画布，不是已通过的浏览器尺寸。
- 复杂表单保留现有完整字段集与校验，图中按任务归组；条件性字段/提供方配置需在对应页面实现批次进一步核对，不得按示意删去参数。
- `/chat/$chatId` 的外部iframe内容由第三方控制；本批只设计MyAPI容器与加载/失败界面，不声称重设计外站。
- CLI来源规则、生产升级、发布、真实供应商与自动合入main均不属于本批授权。

## 未确认可达的弹层候选（只读记录）

'''+''.join('- `'+p+'`\n' for p in unconfirmed)
if not unconfirmed:readme=readme.replace('\n## 未确认可达的弹层候选（只读记录）\n\n','\n')
readme+='\n## 设计源文件\n\n`catalog.json`保存每个视图的字段、动作、来源和拟合并项；`author.py`是仅使用Python标准库的SVG文本作者文件，不是产品构建脚本。查看线稿无需运行它。默认拒绝覆盖已有目录；需要重新生成时必须先审查设计差异，再明确使用`--replace-generated`。不据此修改产品或运行浏览器。\n'
with (OUT/'README.md').open('w') as stream:stream.write(readme)
print(json.dumps(dict(output=str(OUT),source_sha=BASE,registered_routes=len(route_map),settings_sections=sections_count,views=len(pages),svg_files=len(pages)*2,overlay_source_files=len(overlay_inventory),unconfirmed_overlay_sources=len(unconfirmed),bytes=sum(p.stat().st_size for p in OUT.rglob('*') if p.is_file())),ensure_ascii=False))
