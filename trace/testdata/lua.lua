local function private_fn(x) return x * 2 end

function public_fn(x) return x + 1 end

local M = {}

function M.method(self) end

return M
