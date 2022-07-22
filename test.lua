local headers_tab = ngx.req.get_headers()
--value = ""
cookie = headers_tab["Cookie"]
ngx.say(cookie)
ngx.say(ngx.var.cookie_bi-token-31606-9527)
--for k, v in pairs(headers_tab) do
--ngx.say(k..":"..v)
--end
local redis = require "resty.redis"
local red = redis:new()
red:set_timeout(1000)
local ok, err = red:connect("127.0.0.1", 6379)
if not ok then
   return ngx.say("Connect to Redis failed")
end
--密码和选择的库
red:auth('Test000000')
red:select(0)
bitoken = string.match(cookie, "bi%-token%-%d+%-%d+")
if bi_token then
   res, err = red:get(bitoken)
   red:set_keepalive(3000, 2)
   value = ngx.var['cookie_'..bitoken]
end
if res == value then
   ngx.say("cookie_bitoken"..value..":res:"..res)
else
   ngx.say("Not Authorized")
end


