-- 模型成功请求数硬限额：原子释放（release）
--
-- 请求失败（HTTP 状态 >= 400）后释放其在途预留，释放的额度立即可被新请求
-- 使用。重复释放或预留已过期时为无害空操作。
--
-- KEYS[1]: 在途预留 ZSET
-- ARGV[1]: 预留令牌 token（非空字符串）
-- ARGV[2]: 窗口秒数 window（>0 整数）
--
-- 返回：1 = 已释放；0 = 预留不存在（空操作）。

local max_safe_integer = 9007199254740991

local token = ARGV[1]
local window = tonumber(ARGV[2])

if not token or token == '' or
    not window or window <= 0 or
    window ~= math.floor(window) or window > max_safe_integer then
    return redis.error_reply('ERR invalid model success release arguments')
end

if redis.call('ZREM', KEYS[1], token) == 0 then
    return 0
end

redis.call('EXPIRE', KEYS[1], window)
return 1
