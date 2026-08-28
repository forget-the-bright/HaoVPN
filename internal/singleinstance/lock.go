package singleinstance

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// Lock 持有单实例协调监听；进程退出或 Release 后释放。
type Lock struct {
	listener net.Listener
	stop     chan struct{}
	wg       sync.WaitGroup
}

// AcquireClient 尝试成为唯一客户端实例（127.0.0.1 协调口 Listen）。
func AcquireClient() (*Lock, error) {
	if ClientAlreadyRunning() {
		return nil, ErrAlreadyRunning
	}
	ln, err := net.Listen("tcp", coordAddr())
	if err != nil {
		if ClientAlreadyRunning() {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("单实例监听 %s: %w", coordAddr(), err)
	}
	lock := &Lock{listener: ln, stop: make(chan struct{})}
	lock.wg.Add(1)
	go lock.acceptLoop()
	return lock, nil
}

// acceptLoop 接受探测连接并立即关闭，避免 Listen  backlog 堆积。
func (l *Lock) acceptLoop() {
	defer l.wg.Done()
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.stop:
				return
			default:
			}
			return
		}
		_ = conn.Close()
	}
}

// Release 关闭协调监听。
func (l *Lock) Release() {
	if l == nil {
		return
	}
	if l.stop != nil {
		close(l.stop)
	}
	if l.listener != nil {
		_ = l.listener.Close()
	}
	l.wg.Wait()
	l.listener = nil
}

// ErrAlreadyRunning 表示已有客户端实例在运行。
var ErrAlreadyRunning = fmt.Errorf("HaoVPN 客户端已在运行")

// ClientAlreadyRunning 探测协调口是否已有实例在监听（短超时 Dial）。
//
// 跨平台：Windows 上非管理员可探测管理员 Listen 的 127.0.0.1 端口，避免重复 UAC。
func ClientAlreadyRunning() bool {
	conn, err := net.DialTimeout("tcp", coordAddr(), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// AlreadyRunningMessage 返回面向用户的提示文案。
func AlreadyRunningMessage() string {
	return "HaoVPN 客户端已在运行。请先退出已有实例再启动。"
}
