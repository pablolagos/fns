package hpack

import (
	"bytes"
	"fmt"
)

// Encoder encodes header fields using HPACK
type Encoder struct {
	dynamicTable *DynamicTable
}

// NewEncoder creates a new HPACK encoder
func NewEncoder(dt *DynamicTable) Encoder {
	return Encoder{
		dynamicTable: dt,
	}
}

// Encode encodes raw header fields using HPACK
func (e *Encoder) Encode(buf *bytes.Buffer, key, value []byte) error {
	// Check if the field is in the static table
	index := e.dynamicTable.find(key, value)
	if index > 0 {
		// Indexed Header Field
		buf.WriteByte(0x80 | byte(index))
	} else {
		// Literal Header Field with Incremental Indexing
		buf.WriteByte(0x40)
		buf.Write(key)
		e.writeData(buf, value)
		field := acquireNameValueBytes()
		field.name = append(field.name[:0], key...)
		field.value = append(field.value[:0], value...)
		e.dynamicTable.entries = append(e.dynamicTable.entries, field)
	}

	return nil
}

func (e *Encoder) writeData(buf *bytes.Buffer, data []byte) {
	length := len(data)
	buf.WriteByte(byte(length))
	buf.Write(data)
}

// readDecoded reads a string from the buffer into dst, decoding it if necessary
func readDecoded(dst *[]byte, buf *bytes.Buffer) error {
	if buf.Len() < 1 {
		return fmt.Errorf("buffer too short to read length")
	}

	lengthByte := buf.Next(1)[0]
	length := int(lengthByte & 0x7F)       // The length is the lower 7 bits of the length byte
	huffmanEncoded := lengthByte&0x80 != 0 // The most significant bit indicates Huffman encoding

	if buf.Len() < length {
		return fmt.Errorf("buffer too short to read string of length %d", length)
	}

	strData := buf.Next(length)

	if huffmanEncoded {
		// Huffman decoding
		err := huffmanDecode(dst, strData)
		if err != nil {
			return err
		}
		return nil
	}

	// Return the string as-is if not Huffman encoded
	return nil
}
