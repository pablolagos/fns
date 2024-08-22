package hpack

import "sync"

const CRLF = "\r\n"

var buffer1KPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 0, 1024)
	},
}

func acquireBuffer1K() []byte {
	return buffer1KPool.Get().([]byte)
}

func releaseBuffer1K(buf []byte) {
	buf = buf[:0]
	if cap(buf) > 1024 {
		return
	}
	buffer1KPool.Put(buf)
}

var buffer256Pool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 0, 256)
	},
}

func acquireBuffer256() []byte {
	return buffer1KPool.Get().([]byte)[:0]
}

func releaseBuffer256(buf []byte) {
	buf = buf[:0]
	if cap(buf) > 256 {
		return
	}
	buffer256Pool.Put(buf)
}
