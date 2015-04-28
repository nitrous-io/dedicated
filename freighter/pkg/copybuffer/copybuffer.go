package copybuffer

import (
	"bytes"
	"io"
	"sync"
)

type CopyBuffer struct {
	Src io.Reader
	buf *bytes.Buffer

	l sync.Mutex
}

// bufferdcopy.WriteTo buffers bytes read to buf if dst is unavailable.
// if buf is not empty, it is written to dst if dst is available.
func (c *CopyBuffer) WriteTo(dst io.Writer) (written int64, err error) {
	c.l.Lock()
	defer c.l.Unlock()
	var src = c.Src
	if c.buf == nil {
		c.buf = &bytes.Buffer{}
	}

	if bn := c.buf.Len(); bn > 0 {
		n, err := c.buf.WriteTo(dst)
		if err != nil || int(n) != bn {
			return n, err
		}
		written += n
	}

	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				c.buf.Write(buf[nw:nr])
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				c.buf.Write(buf[nw:nr])
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}
