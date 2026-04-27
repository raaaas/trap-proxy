package main

import (
    "bufio"
    "crypto/rand"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
    "strings"
    "sync"

    lua "github.com/yuin/gopher-lua"
)

var (
    caddyHTTP  = &httputil.ReverseProxy{Director: func(r *http.Request) { r.URL, _ = url.Parse("http://caddy:80") }}
    luaState   *lua.LState
    luaMu      sync.RWMutex
    scriptPath = "/etc/trap/rules.lua"
)

func initLua() {
    luaMu.Lock()
    defer luaMu.Unlock()
    if luaState != nil {
        luaState.Close()
    }
    luaState = lua.NewState()
    luaState.SetGlobal("infinite_stream", luaState.NewFunction(infiniteStreamLua))
    luaState.SetGlobal("redirect", luaState.NewFunction(redirectLua))
    luaState.SetGlobal("respond", luaState.NewFunction(respondLua))
    luaState.SetGlobal("forward", luaState.NewFunction(forwardLua))
    luaState.SetGlobal("log", luaState.NewFunction(logLua))

    if err := luaState.DoFile(scriptPath); err != nil {
        log.Printf("Warning: failed to load Lua script %s: %v", scriptPath, err)
    }
}

func infiniteStreamLua(L *lua.LState) int { L.Push(lua.LNumber(1)); return 1 }
func redirectLua(L *lua.LState) int       { url := L.CheckString(1); L.Push(lua.LNumber(2)); L.Push(lua.LString(url)); return 2 }
func respondLua(L *lua.LState) int        { status := L.CheckInt(1); body := L.CheckString(2); L.Push(lua.LNumber(3)); L.Push(lua.LNumber(status)); L.Push(lua.LString(body)); return 3 }
func forwardLua(L *lua.LState) int        { L.Push(lua.LNumber(4)); return 1 }
func logLua(L *lua.LState) int            { msg := L.CheckString(1); log.Printf("[Lua] %s", msg); return 0 }

func main() {
    initLua()
    go listenHTTP(":80")
    go forwardTCP(":443", "caddy:443")
    select {}
}

func listenHTTP(addr string) {
    ln, err := net.Listen("tcp", addr)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("HTTP trap proxy listening on %s (Lua enabled)", addr)
    for {
        conn, err := ln.Accept()
        if err != nil {
            continue
        }
        go handleHTTP(conn)
    }
}

func handleHTTP(conn net.Conn) {
    defer conn.Close()
    r := bufio.NewReader(conn)
    req, err := http.ReadRequest(r)
    if err != nil {
        log.Printf("Invalid HTTP request: %v", err)
        return
    }
    log.Printf("HTTP: %s %s (Host: %s)", req.Method, req.URL.Path, req.Host)

    luaMu.RLock()
    L := luaState
    luaMu.RUnlock()
    if L == nil {
        caddyHTTP.ServeHTTP(&responseWriter{conn: conn, req: req}, req)
        return
    }

    if err := L.CallByParam(lua.P{
        Fn:      L.GetGlobal("handle_request"),
        NRet:    1,
        Protect: true,
    }, lua.LString(req.Method), lua.LString(req.URL.Path), lua.LString(req.Host), lua.LString(req.UserAgent())); err != nil {
        log.Printf("Lua error: %v", err)
        caddyHTTP.ServeHTTP(&responseWriter{conn: conn, req: req}, req)
        return
    }
    ret := L.Get(-1)
    L.Pop(1)
    action, ok := ret.(lua.LNumber)
    if !ok {
        caddyHTTP.ServeHTTP(&responseWriter{conn: conn, req: req}, req)
        return
    }

    switch int(action) {
    case 1: // infinite stream
        log.Println("🔥 Lua: infinite stream")
        conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nCache-Control: no-cache\r\n\r\n"))
        buf := make([]byte, 65536)
        for {
            rand.Read(buf)
            if _, err := conn.Write(buf); err != nil {
                return
            }
        }
    case 2: // redirect
        url := L.Get(-2).(lua.LString)
        log.Printf("🔥 Lua: redirect to %s", url)
        conn.Write([]byte(fmt.Sprintf("HTTP/1.1 302 Found\r\nLocation: %s\r\nContent-Length: 0\r\n\r\n", url)))
        return
    case 3: // custom response
        status := int(L.Get(-3).(lua.LNumber))
        body := L.Get(-2).(lua.LString)
        log.Printf("🔥 Lua: respond %d", status)
        conn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s", status, http.StatusText(status), len(body), body)))
        return
    default:
        log.Printf("➡️ Forwarding to Caddy: %s %s", req.Method, req.URL.Path)
        caddyHTTP.ServeHTTP(&responseWriter{conn: conn, req: req}, req)
    }
}

func forwardTCP(localAddr, remoteAddr string) {
    ln, err := net.Listen("tcp", localAddr)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("TCP forwarder listening on %s -> %s", localAddr, remoteAddr)
    for {
        conn, err := ln.Accept()
        if err != nil {
            continue
        }
        go func() {
            defer conn.Close()
            remote, err := net.Dial("tcp", remoteAddr)
            if err != nil {
                log.Printf("Failed to connect to %s: %v", remoteAddr, err)
                return
            }
            defer remote.Close()
            go io.Copy(remote, conn)
            io.Copy(conn, remote)
        }()
    }
}

type responseWriter struct {
    conn net.Conn
    req  *http.Request
    hdr  http.Header
    code int
}

func (w *responseWriter) Header() http.Header {
    if w.hdr == nil {
        w.hdr = make(http.Header)
    }
    return w.hdr
}
func (w *responseWriter) Write(p []byte) (int, error) { return w.conn.Write(p) }
func (w *responseWriter) WriteHeader(code int) {
    w.code = code
    status := http.StatusText(code)
    w.conn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n", code, status)))
    w.hdr.Write(w.conn)
    w.conn.Write([]byte("\r\n"))
}
