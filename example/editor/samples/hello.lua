-- Original Lua 5.4 sample for SourceView.
local message = [=[
Raw "quotes" and ]] do not close this string.
]=]

--[==[
Long comments use their own matching bracket level.
]==]
local function greet(name)
    local scale = 0x1.fp+2
    return string.format("Hello, %s! %.2f\n", name, scale)
end

print(greet("SourceView"), message)
