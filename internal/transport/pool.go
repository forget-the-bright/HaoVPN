package transport

import "sync"

var bufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 2048)
		return &b
	},
}

// GetBuffer 从 sync.Pool 取缓冲区；容量不足时重新分配 size 长度切片。
//
// 参数：size — 期望可用长度；readLoop 等调用方须 PutBuffer 归还。
func GetBuffer(size int) []byte {
	bp := bufferPool.Get().(*[]byte)
	b := *bp
	if cap(b) < size {
		b = make([]byte, size)
	} else {
		b = b[:size]
	}
	return b
}

// PutBuffer 将缓冲区归还池；cap 超过 64KiB 的丢弃不回收。
func PutBuffer(b []byte) {
	if cap(b) > 64*1024 {
		return
	}
	b = b[:cap(b)]
	bufferPool.Put(&b)
}
