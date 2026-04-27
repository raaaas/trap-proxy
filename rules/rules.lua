-- trap-proxy rule engine
-- function handle_request(method, path, host, user_agent) must return one of:
--   infinite_stream()   -> serve endless random data
--   redirect(url)       -> 302 redirect
--   respond(status, body) -> custom HTTP response
--   forward()           -> pass to Caddy backend

function handle_request(method, path, host, user_agent)
    -- CONNECT trap: any host
    if method == "CONNECT" then
        log("CONNECT to " .. host .. " -> infinite stream")
        return infinite_stream()
    end

    -- Proxy test paths
    if path == "/generate_204" or path == "/success.txt" or path == "/hotspot-detect.html" then
        log("Proxy probe " .. path .. " -> redirect to infinite stream")
        return redirect("/robots.txt?login=true")
    end

    if string.match(path, "^/proxyip=") or string.match(path, "^/eyJ") then
        log("Proxy probe with path " .. path .. " -> redirect")
        return redirect("/robots.txt?login=true")
    end

    -- Absolute URL in request (path contains ://)
    if string.match(path, "://") then
        log("Absolute URL in path -> redirect")
        return redirect("/robots.txt?login=true")
    end

    -- Custom: block python-requests
    if string.match(user_agent, "python%-requests") then
        log("Blocked python-requests user agent")
        return respond(500, "Go away scanner")
    end

    -- Default: forward to Caddy
    return forward()
end
