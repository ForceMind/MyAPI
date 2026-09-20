-- 模型成功请求数硬限额：原子转正（commit）
--
-- 请求成功（HTTP 状态 < 400）后，把在途预留转为已提交成功记录。
-- 只有预留仍然存在时才写入：预留随窗口过期（键 TTL 或被后续 reserve 按窗口
-- 裁剪）后不再补记，防止 committed + reserved 超过上限。
--
-- KEYS[1]: 已提交成功时间戳列表（旧格式：LPUSH 最新在前 + LTRIM 到 max 条）
-- KEYS[2]: 在途预留 ZSET
-- ARGV[1]: 预留令牌 token（非空字符串）
-- ARGV[2]: 已提交时间戳（旧格式 UTC 毫秒字符串 2006-01-02T15:04:05.000Z）
-- ARGV[3]: 上限 max（>0 整数）
-- ARGV[4]: 窗口秒数 window（>0 整数）
--
-- 返回：1 = 已转正并记录；0 = 预留已不存在，未记录。

local max_safe_integer = 9007199254740991

local token = ARGV[1]
local committed_at = ARGV[2]
local max = tonumber(ARGV[3])
local window = tonumber(ARGV[4])

if not token or token == '' or not committed_at or committed_at == '' or
    not max or not window or
    max <= 0 or window <= 0 or
    max ~= math.floor(max) or window ~= math.floor(window) or
    max > max_safe_integer or window > max_safe_integer then
    return redis.error_reply('ERR invalid model success commit arguments')
end

if redis.call('ZREM', KEYS[2], token) == 0 then
    return 0
end

redis.call('LPUSH', KEYS[1], committed_at)
redis.call('LTRIM', KEYS[1], 0, max - 1)
redis.call('EXPIRE', KEYS[1], window)
redis.call('EXPIRE', KEYS[2], window)
return 1
