-- 令牌桶限流器
-- KEYS[1]: 限流器唯一标识
-- ARGV[1]: 请求令牌数 (通常为1)
-- ARGV[2]: 令牌生成速率 (每秒)
-- ARGV[3]: 桶容量

local key = KEYS[1]
local requested = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])
-- Lua stores numbers as IEEE-754 doubles. Keep EXPIRE arguments within the
-- largest integer that Lua can represent exactly.
local max_safe_ttl_seconds = 9007199254740991

if not requested or not rate or not capacity or
    requested <= 0 or rate <= 0 or capacity <= 0 or
    requested ~= math.floor(requested) or rate ~= math.floor(rate) or capacity ~= math.floor(capacity) or
    requested > capacity or
    requested > max_safe_ttl_seconds or rate > max_safe_ttl_seconds or capacity > max_safe_ttl_seconds then
    return redis.error_reply('ERR invalid rate limit config')
end

-- 获取当前时间（Redis服务器时间）
local now = redis.call('TIME')
local nowInSeconds = tonumber(now[1])

-- 获取桶状态
local bucket = redis.call('HMGET', key, 'tokens', 'last_time')
local tokens = tonumber(bucket[1])
local last_time = tonumber(bucket[2])

-- 初始化桶（首次请求或过期）
if not bucket[1] and not bucket[2] then
    tokens = capacity
    last_time = nowInSeconds
else
    if not tokens or not last_time or
        tokens ~= math.floor(tokens) or last_time ~= math.floor(last_time) or
        tokens < 0 or tokens > capacity or tokens > max_safe_ttl_seconds or
        last_time < 0 or last_time > nowInSeconds or last_time > max_safe_ttl_seconds then
        return redis.error_reply('ERR invalid rate limit bucket state')
    end

    -- 计算新增令牌
    local elapsed = nowInSeconds - last_time
    if elapsed > (capacity - tokens) / rate then
        tokens = capacity
    else
        tokens = tokens + elapsed * rate
    end
    last_time = nowInSeconds
end

-- 判断是否允许请求
local allowed = false
if tokens >= requested then
    tokens = tokens - requested
    allowed = true
end

-- 更新桶状态并在每次访问后续期。桶复满后丢弃状态与满桶重新初始化等价。
redis.call('HMSET', key, 'tokens', tokens, 'last_time', last_time)

local missing_tokens = capacity - tokens
local ttl = math.max(1, math.ceil(missing_tokens / rate))
redis.call('EXPIRE', key, ttl)

return allowed and 1 or 0
