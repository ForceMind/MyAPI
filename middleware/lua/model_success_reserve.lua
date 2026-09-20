-- 模型成功请求数硬限额：原子预留（reserve）
--
-- 语义：在滑动窗口内原子占位，保证 LLEN(committed) + ZCARD(reserved) 永不超过
-- 上限 max。窗口时长与维度键与旧版 recordRedisRequest / checkRedisRateLimit
-- 完全一致，滚动升级期间新旧实例共享同一份已提交记录。
--
-- KEYS[1]: 已提交成功时间戳列表（旧格式兼容：LPUSH 最新在前，元素为 UTC
--          毫秒时间戳字符串，形如 2006-01-02T15:04:05.000Z）
-- KEYS[2]: 在途预留 ZSET（member = 预留令牌，score = 预留时的毫秒时间戳）
-- ARGV[1]: 上限 max（>0 整数）
-- ARGV[2]: 窗口秒数 window（>0 整数）
-- ARGV[3]: 当前毫秒时间 now_ms（>0 整数，由调用方 Go 时钟提供，与旧版实现一致）
-- ARGV[4]: 预留令牌 token（非空字符串）
--
-- 返回：1 = 已预留；0 = 窗口内已满，拒绝。
--
-- 注意：本脚本同时访问两个键，部署为 Redis Cluster 时需保证两键落在同一
-- slot（与现有部署一致的单节点/哨兵模式无此限制）。

local max_safe_integer = 9007199254740991

local max = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local token = ARGV[4]

-- Lua 数字为 IEEE-754 双精度：window * 1000 必须保持整数精确。
if not max or not window or not now_ms or
    max <= 0 or window <= 0 or now_ms <= 0 or
    max ~= math.floor(max) or window ~= math.floor(window) or now_ms ~= math.floor(now_ms) or
    max > max_safe_integer or window > math.floor(max_safe_integer / 1000) or now_ms > max_safe_integer or
    not token or token == '' then
    return redis.error_reply('ERR invalid model success reserve arguments')
end

local window_ms = window * 1000
local cutoff = now_ms - window_ms

-- 解析旧版写入的 UTC 毫秒时间戳（2006-01-02T15:04:05.000Z）。
-- 纯 Lua days-from-civil 算法，确定性、不依赖 os.time，毫秒精度下与旧版
-- time.Parse 后的窗口比较（now-old >= window 视为过期）等价。
local function parse_committed_ms(value)
    local y, mo, d, h, mi, s, ms = string.match(value, '^(%d+)%-(%d+)%-(%d+)T(%d+):(%d+):(%d+)%.(%d%d%d)Z$')
    if not y then
        return nil
    end
    y = tonumber(y); mo = tonumber(mo); d = tonumber(d)
    h = tonumber(h); mi = tonumber(mi); s = tonumber(s); ms = tonumber(ms)
    if mo < 1 or mo > 12 or d < 1 or d > 31 or h > 23 or mi > 59 or s > 61 then
        return nil
    end
    local yy = y
    if mo <= 2 then
        yy = yy - 1
    end
    local era = math.floor(yy / 400)
    local yoe = yy - era * 400
    local mp = (mo + 9) % 12
    local doy = math.floor((153 * mp + 2) / 5) + d - 1
    local doe = yoe * 365 + math.floor(yoe / 4) - math.floor(yoe / 100) + doy
    local days = era * 146097 + doe - 719468
    return (((days * 24 + h) * 60 + mi) * 60 + s) * 1000 + ms
end

-- 裁剪窗口外的在途预留（结果未知的预留由此随窗口过期自然回收）。
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', cutoff)

-- 裁剪窗口外的已提交记录。列表按新到旧排列，过期项必在尾部。
-- 列表长度始终 <= max（commit 时 LTRIM），循环有界。
while true do
    local oldest = redis.call('LINDEX', KEYS[1], -1)
    if not oldest then
        break
    end
    local committed_ms = parse_committed_ms(oldest)
    if not committed_ms then
        -- 与旧版 checkRedisRateLimit 的 time.Parse 失败一致：报错，由调用方
        -- fail-close（HTTP 500），而不是放行。
        return redis.error_reply('ERR invalid model success committed entry')
    end
    if committed_ms > cutoff then
        break
    end
    redis.call('RPOP', KEYS[1])
end

local committed = redis.call('LLEN', KEYS[1])
local reserved = redis.call('ZCARD', KEYS[2])
if committed + reserved >= max then
    -- 与旧版一致：窗口内满额拒绝时刷新键过期时间。
    redis.call('EXPIRE', KEYS[1], window)
    redis.call('EXPIRE', KEYS[2], window)
    return 0
end

redis.call('ZADD', KEYS[2], now_ms, token)
redis.call('EXPIRE', KEYS[2], window)
redis.call('EXPIRE', KEYS[1], window)
return 1
