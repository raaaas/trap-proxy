## 🚪 trap-proxy

[![Go Version](https://img.shields.io/github/go-mod/go-version/yourusername/trap-proxy)](https://github.com/yourusername/trap-proxy)
[![Docker Pulls](https://img.shields.io/docker/pulls/yourusername/trap-proxy)](https://hub.docker.com/r/yourusername/trap-proxy)
[![License](https://img.shields.io/github/license/yourusername/trap-proxy)](https://github.com/yourusername/trap-proxy)

**trap-proxy** is a lightweight, Lua-scriptable reverse proxy that detects and punishes malicious scanners (CONNECT, proxy test paths, absolute URLs, etc.) by feeding them infinite random data – wasting their bandwidth and resources. Legitimate traffic is forwarded transparently to your backend (Caddy, Nginx, WordPress, etc.) while HTTPS is passed through raw, preserving TLS and certificates.

Perfect for anyone running public web services who wants to fight back against automated proxy scanners, vulnerability bots, and script-kiddies – all without changing your existing setup.

## 🖩 Features

- 🔥 Bandwidth burning – Send endless random data to scanners (CONNECT, proxy probes, absolute URL requests).
- 🧩 Lua scripting – Write custom rules (inspect method, path, host, user-agent) without recompiling.
- ⚡🕊 Zero-SSL-overhead – HTTPS traffic is forwarded raw (TCP level) to your backend – no TLS termination in the proxy.
- 🌈 Transparent backend forwarding – Normal requests go to your existing Caddy/Nginx/WordPress.
- 🐳 Docker/Portainer ready – Single `docker-compose.yml` or `docker run` command.
- 🔄 Live rule updates – Mount a volume for `rules.lua` and restart the container (or add HTTP reload endpoint).
- 📦 Pre-built image – Available on Docker Hub (soon) or build yourself.

## 🚏 How it works

```
Internet ── trap-proxy (ports 80 & 443)
                  │
                  ├─ HTTP: inspect request → if malicious → serve infinite random data
                  │                           else       → forward to backend (caddy:80)
                  │
                  └─ HTTPS: raw TCP forward to backend (caddy:443) – untouched
```

- Port `80` receives plain HTTP – the proxy parses the request, calls your Lua script, and decides: serve infinite stream, redirect, respond with custom HTTP status, or forward to backend.
- Port `443` is handled as a raw TCP tunnel to your backend's port `443`. No decryption, no inspection. Your backend (Caddy) continues to handle TLS, certificates, and HTTPS completely.

## 🌊 Quick Start

### Prerequisites

- Docker and Docker Compose
- A backend service (e.g., Caddy) running on the **same Docker network** with container name `caddy` and internal ports `80` and `443`.
  - Your backend must **not** publish ports `80`/`443` to the host – host ports will be taken by trap-proxy.
  - Example Caddy service in `docker-compose.yml` (internal only):
    ```yaml
    services:
      caddy:
        image: caddy:latest
        container_name: caddy
        ports:
          - "127.0.0.1:8080:80"
          - "127.0.0.1:8443:443"
        networks:
          - caddy_network
    ```

### Run with Docker Compose

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/trap-proxy.git
   cd trap-proxy
   ```

2. (Optional) Edit `rules/rules.lua` to add custom logic.

3. Start the proxy:
   ```bash
   docker compose up -d
   ```

### Run with plain Docker

```bash
docker build -t trap-proxy .
docker run -d --name trap-proxy --restart unless-stopped \
  --network caddy_network \
  -v $(pwd)/rules:/etc/trap \
  -p 80:80 -p 443:443 \
  trap-proxy
```

## 🍎 Configuration

All rules are written in **Lua** – no need to recompile Go. The file `rules/rules.lua` is mounted inside the container at /etc/trap/rules.lua.

### Required Lua function

```lua
function handle_request(method, path, host, user_agent)
    -- return one of the following actions
end
```

### Available actions

| Action           | Lua code                                 | Effect                                                         |
|-----------------|------------------------------------------|----------------------------------------------------------------|
| Infinite stream | `return infinite_stream()`               | Client receives endless random data. Connection stays open.    |
| Redirect        | `return redirect("http://example.com")`  | Sends `302 Found` with the given Location header.              |
| Custom response | `return respond(404, "Not found")`       | Sends any HTTP status code and a body.                         |
| Forward to backend | `return forward()`                     | Passes the request untouched to your backend (e.g., Caddy).    |

### Logging from Lua

```lua
log("Something happened")
```

Logs appear in the trap-proxy container logs (`docker logs trap-proxy`).

### Example `rules.lua` (default)

```lua
function handle_request(method, path, host, user_agent)
    if method == "CONNECT" then
        log("CONNECT to " .. host .. " -> infinite stream")
        return infinite_stream()
    end

    if path == "/generate_204" or path == "/success.txt" or path == "/hotspot-detect.html" then
        return redirect("/robots.txt?login=true")
    end

    if string.match(path, "^/proxyip=") or string.match(path, "^/eyJ") then
        return redirect("/robots.txt?login=true")
    end

    if string.match(path, "://") then
        return redirect("/robots.txt?login=true")
    end

    if string.match(user_agent, "python%-requests") then
        return respond(500, "Go away scanner")
    end

    return forward()
end
```

## 🔄 Updating rules without rebuilding

1. Edit `rules/rules.lua` on your host.
2. Restart the container:
   ```bash
   docker restart trap-proxy
   ```

## 🍌 Deploying in Portainer

- Create a new stack.
- Paste the `docker-compose.yml` from this repo.
