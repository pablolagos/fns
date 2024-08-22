package hpack

import (
	"bytes"
	"fmt"
)

// Decoder decodes header fields using HPACK
type Decoder struct {
	dynamicTable dynamicTable
}

// NewDecoder creates a new HPACK decoder
func NewDecoder() *Decoder {
	return &Decoder{
		dynamicTable: newDynamicTable(),
	}
}

// Decode decodes header fields using HPACK into http/1.1 header buffer
// Each call to Decode appends a key: value<CRLF> pair to the buffer
func (d *Decoder) Decode(dst *[]byte, data []byte) error {
	buf := bytes.NewBuffer(data)
	dstBuf := bytes.NewBuffer(*dst)

	key := acquireBuffer1K()
	val := acquireBuffer1K()

	defer releaseBuffer1K(key)
	defer releaseBuffer1K(val)

	for buf.Len() > 0 {
		prefix := buf.Next(1)[0]
		switch {
		case prefix&0x80 == 0x80:
			// Indexed Header Field Representation
			index := int(prefix & 0x7f)
			err := d.dynamicTable.get(&key, &val, index)
			if err != nil {
				return err
			}
			d.writeHeader(dstBuf, key, val)
		case prefix&0xc0 == 0x40:
			// Literal Header Field with Incremental Indexing
			index := int(prefix & 0x3f)
			err := d.dynamicTable.get(&key, &val, index)
			if err != nil {
				return err
			}
			err = readDecoded(&val, buf)
			if err != nil {
				return err
			}
			d.writeHeader(dstBuf, key, val)
		case prefix&0xf0 == 0x00:
			// Literal Header Field without Indexing
			err := readDecoded(&key, buf)
			if err != nil {
				return err
			}
			err = readDecoded(&val, buf)
			if err != nil {
				return err
			}
			d.writeHeader(dstBuf, key, val)
		default:
			return fmt.Errorf("unsupported HPACK prefix: 0x%x", prefix)
		}
	}
	return nil
}

// writeHeader writes a header field to the buffer. The keys are sanitized to prevent header smuggling.
func (d *Decoder) writeHeader(dstBuf *bytes.Buffer, key, val []byte) {
	d.fixSmuggling(key)
	d.fixSmuggling(val)
	dstBuf.Write(key)
	dstBuf.WriteString(": ")
	dstBuf.Write(val)
	dstBuf.WriteString(CRLF)
	return
}

// Remove CR and LF characters from the data, moving the remaining data to the left
func (d *Decoder) fixSmuggling(data []byte) []byte {
	var j int
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' || data[i] == '\n' {
			continue
		}
		if i != j {
			data[j] = data[i]
		}
		j++
	}
	return data[:j]

}
