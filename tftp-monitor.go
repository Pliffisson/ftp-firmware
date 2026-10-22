package main

import (
    "crypto/sha256"
    "encoding/binary"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "log"
    "net"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"
)

const maxPacket = 65535

type transfer struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Client      string `json:"client"`
    State       string `json:"state"`
    Bytes       int64  `json:"bytes"`
    Total       int64  `json:"total"`
    StartedAtMs int64  `json:"started_at_ms"`
    UpdatedAtMs int64  `json:"updated_at_ms"`
}

type packet struct {
    data []byte
    addr *net.UDPAddr
    server bool
}

var active sync.Map

func main() {
    port := 69
    if value := os.Getenv("TFTP_PROXY_PORT"); value != "" {
        parsed, err := strconv.Atoi(value)
        if err != nil || parsed < 1 || parsed > 65535 { log.Fatal("invalid TFTP_PROXY_PORT") }
        port = parsed
    }
    listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
    if err != nil { log.Fatal(err) }
    defer listener.Close()
    log.Printf("TFTP monitor listening on UDP %d", port)
    for {
        buf := make([]byte, maxPacket)
        n, client, err := listener.ReadFromUDP(buf)
        if err != nil { log.Fatal(err) }
        if n < 4 || binary.BigEndian.Uint16(buf[:2]) != 1 { continue }
        name := strings.SplitN(string(buf[2:n]), "\x00", 2)[0]
        key := client.String()
        if _, loaded := active.LoadOrStore(key, true); loaded { continue }
        request := append([]byte(nil), buf[:n]...)
        go func() {
            defer active.Delete(key)
            relay(client, name, request)
        }()
    }
}

func readPackets(conn *net.UDPConn, server bool, out chan<- packet, done <-chan struct{}) {
    for {
        buf := make([]byte, maxPacket)
        n, addr, err := conn.ReadFromUDP(buf)
        if err != nil { return }
        select {
        case out <- packet{buf[:n], addr, server}:
        case <-done: return
        }
    }
}

func relay(client *net.UDPAddr, name string, request []byte) {
    upstream, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
    if err != nil { log.Printf("upstream socket: %v", err); return }
    defer upstream.Close()
    downstream, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
    if err != nil { log.Printf("downstream socket: %v", err); return }
    defer downstream.Close()

    backend := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1069}
    if _, err = upstream.WriteToUDP(request, backend); err != nil { log.Printf("request: %v", err); return }
    idBytes := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", client, time.Now().UnixNano(), name)))
    now := time.Now().UnixMilli()
    status := transfer{ID: hex.EncodeToString(idBytes[:8]), Name: name, Client: client.IP.String(), State: "in_progress", StartedAtMs: now, UpdatedAtMs: now}
    if name != "" && !strings.ContainsAny(name, "/\\") {
        if info, err := os.Stat(filepath.Join("/srv/tftp", name)); err == nil && info.Mode().IsRegular() { status.Total = info.Size() }
    }
    statusPath := statusFile(name)
    saveStatus(statusPath, status)
    log.Printf("RRQ %q from %s", name, client)

    packets := make(chan packet, 128)
    done := make(chan struct{})
    defer close(done)
    go readPackets(upstream, true, packets, done)
    go readPackets(downstream, false, packets, done)
    var serverAddr *net.UDPAddr
    blockSize := 512
    expectedBlock := uint16(1)
    var finalBlock uint16
    hasFinalBlock := false
    lastWrite := time.Now()
    timer := time.NewTimer(30 * time.Second)
    defer timer.Stop()

    for {
        select {
        case item := <-packets:
            if len(item.data) < 2 { continue }
            op := binary.BigEndian.Uint16(item.data[:2])
            if item.server {
                if serverAddr == nil { serverAddr = item.addr }
                if !item.addr.IP.Equal(serverAddr.IP) || item.addr.Port != serverAddr.Port { continue }
                if op == 6 { blockSize = negotiatedBlockSize(item.data, blockSize) }
                if op == 3 && len(item.data) >= 4 {
                    block := binary.BigEndian.Uint16(item.data[2:4])
                    if block == expectedBlock {
                        status.Bytes += int64(len(item.data) - 4)
                        expectedBlock++
                        if len(item.data)-4 < blockSize { finalBlock, hasFinalBlock = block, true }
                        if time.Since(lastWrite) >= 300*time.Millisecond {
                            status.UpdatedAtMs = time.Now().UnixMilli()
                            saveStatus(statusPath, status)
                            lastWrite = time.Now()
                        }
                    }
                }
                downstream.WriteToUDP(item.data, client)
            } else {
                if !item.addr.IP.Equal(client.IP) || item.addr.Port != client.Port { continue }
                if serverAddr == nil { continue }
                upstream.WriteToUDP(item.data, serverAddr)
                if op == 4 && len(item.data) >= 4 && hasFinalBlock && binary.BigEndian.Uint16(item.data[2:4]) == finalBlock {
                    status.State = "complete"
                    status.Bytes = status.Total
                    status.UpdatedAtMs = time.Now().UnixMilli()
                    saveStatus(statusPath, status)
                    log.Printf("completed %q to %s", name, client)
                    return
                }
            }
            if op == 5 {
                status.State = "error"
                status.UpdatedAtMs = time.Now().UnixMilli()
                saveStatus(statusPath, status)
                return
            }
            if !timer.Stop() { select { case <-timer.C: default: } }
            timer.Reset(30 * time.Second)
        case <-timer.C:
            status.State = "timeout"
            status.UpdatedAtMs = time.Now().UnixMilli()
            saveStatus(statusPath, status)
            log.Printf("timed out %q to %s", name, client)
            return
        }
    }
}

func negotiatedBlockSize(data []byte, fallback int) int {
    parts := strings.Split(string(data[2:]), "\x00")
    for i := 0; i+1 < len(parts); i += 2 {
        if strings.EqualFold(parts[i], "blksize") || strings.EqualFold(parts[i], "blksize2") {
            if size, err := strconv.Atoi(parts[i+1]); err == nil && size >= 8 && size <= 65464 { return size }
        }
    }
    return fallback
}

func statusFile(name string) string {
    hash := sha256.Sum256([]byte(name))
    return filepath.Join("/srv/transfers", hex.EncodeToString(hash[:])+".json")
}

func saveStatus(path string, status transfer) {
    // Uma nova transferência do mesmo firmware substitui o estado anterior.
    if old, err := os.ReadFile(path); err == nil {
        var previous transfer
        if json.Unmarshal(old, &previous) == nil && previous.ID != status.ID && previous.StartedAtMs > status.StartedAtMs { return }
    }
    data, err := json.Marshal(status)
    if err != nil { return }
    temp := path + "." + status.ID
    if err = os.WriteFile(temp, data, 0644); err != nil { log.Printf("status write: %v", err); return }
    if err = os.Rename(temp, path); err != nil { log.Printf("status rename: %v", err) }
}
