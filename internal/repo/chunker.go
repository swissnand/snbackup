package repo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"io"
)

// Content-defined chunking (FastCDC-style gear hash with normalized chunking).
// Inserting bytes into a large file only changes the chunks around the edit,
// so forever-incremental backups stay small.
const (
	MinChunk = 256 << 10
	AvgChunk = 1 << 20
	MaxChunk = 4 << 20
)

var (
	maskS uint64 = 0x0003590703530000 // 22 bits: harder to cut before avg
	maskL uint64 = 0x0000d90003530000 // 18 bits: easier to cut after avg
)

type gearTable [256]uint64

func newGear(seed []byte) *gearTable {
	var g gearTable
	m := hmac.New(sha256.New, seed)
	var idx [2]byte
	for i := 0; i < 256; i++ {
		m.Reset()
		binary.BigEndian.PutUint16(idx[:], uint16(i))
		m.Write([]byte("snbackup-gear"))
		m.Write(idx[:])
		g[i] = binary.BigEndian.Uint64(m.Sum(nil))
	}
	return &g
}

// Chunker splits a stream into content-defined chunks.
type Chunker struct {
	r    io.Reader
	gear *gearTable
	buf  []byte
	n    int
	eof  bool
}

func newChunker(r io.Reader, g *gearTable) *Chunker {
	return &Chunker{r: r, gear: g, buf: make([]byte, MaxChunk)}
}

// Next returns the next chunk. The returned slice is only valid until the
// following call. io.EOF is returned at the end.
func (c *Chunker) Next() ([]byte, error) {
	for !c.eof && c.n < MaxChunk {
		m, err := c.r.Read(c.buf[c.n:])
		c.n += m
		if err == io.EOF {
			c.eof = true
		} else if err != nil {
			return nil, err
		}
	}
	if c.n == 0 {
		return nil, io.EOF
	}
	cut := c.cut(c.buf[:c.n])
	out := make([]byte, cut)
	copy(out, c.buf[:cut])
	copy(c.buf, c.buf[cut:c.n])
	c.n -= cut
	return out, nil
}

func (c *Chunker) cut(data []byte) int {
	n := len(data)
	if n <= MinChunk {
		return n
	}
	var h uint64
	i := MinChunk
	normal := AvgChunk
	if n < normal {
		normal = n
	}
	for ; i < normal; i++ {
		h = (h << 1) + c.gear[data[i]]
		if h&maskS == 0 {
			return i + 1
		}
	}
	for ; i < n; i++ {
		h = (h << 1) + c.gear[data[i]]
		if h&maskL == 0 {
			return i + 1
		}
	}
	return n
}
