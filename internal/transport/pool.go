package transport

import "sync"

var bufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 2048)
		return &b
	},
}

// GetBuffer returns a pooled buffer, grown if needed.
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

// PutBuffer returns a buffer to the pool.
func PutBuffer(b []byte) {
	if cap(b) > 64*1024 {
		return
	}
	b = b[:cap(b)]
	bufferPool.Put(&b)
}
