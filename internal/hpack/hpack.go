package hpack

import (
	"bytes"
	"fmt"
)

const (
	huffmanEOI = 256 // Huffman End-Of-Input symbol
)

// HeaderField represents a single header field
type HeaderField struct {
	Name  string
	Value string
}

// Encoder encodes header fields using HPACK
type Encoder struct {
	dynamicTable dynamicTable
}

// Decoder decodes header fields using HPACK
type Decoder struct {
	dynamicTable dynamicTable
}

// NewEncoder creates a new HPACK encoder
func NewEncoder() *Encoder {
	return &Encoder{
		dynamicTable: newDynamicTable(),
	}
}

// NewDecoder creates a new HPACK decoder
func NewDecoder() *Decoder {
	return &Decoder{
		dynamicTable: newDynamicTable(),
	}
}

// Encode encodes header fields using HPACK
func (e *Encoder) Encode(headerFields []HeaderField) ([]byte, error) {
	var buf bytes.Buffer
	for _, field := range headerFields {
		// Check if the field is in the static table
		index := e.dynamicTable.find(field)
		if index > 0 {
			// Indexed Header Field
			buf.WriteByte(0x80 | byte(index))
		} else {
			// Literal Header Field with Incremental Indexing
			buf.WriteByte(0x40)
			writeString(&buf, field.Name)
			writeString(&buf, field.Value)
			e.dynamicTable.add(field)
		}
	}
	return buf.Bytes(), nil
}

// Decode decodes header fields using HPACK
func (d *Decoder) Decode(data []byte) ([]HeaderField, error) {
	var headerFields []HeaderField
	buf := bytes.NewBuffer(data)
	for buf.Len() > 0 {
		prefix := buf.Next(1)[0]
		switch {
		case prefix&0x80 == 0x80:
			// Indexed Header Field Representation
			index := int(prefix & 0x7f)
			field, err := d.dynamicTable.get(index)
			if err != nil {
				return nil, err
			}
			headerFields = append(headerFields, field)
		case prefix&0xc0 == 0x40:
			// Literal Header Field with Incremental Indexing
			name, err := readString(buf)
			if err != nil {
				return nil, err
			}
			value, err := readString(buf)
			if err != nil {
				return nil, err
			}
			field := HeaderField{Name: name, Value: value}
			headerFields = append(headerFields, field)
			d.dynamicTable.add(field)
		case prefix&0xf0 == 0x00:
			// Literal Header Field without Indexing
			name, err := readString(buf)
			if err != nil {
				return nil, err
			}
			value, err := readString(buf)
			if err != nil {
				return nil, err
			}
			headerFields = append(headerFields, HeaderField{Name: name, Value: value})
		default:
			return nil, fmt.Errorf("unsupported HPACK prefix: 0x%x", prefix)
		}
	}
	return headerFields, nil
}

func writeString(buf *bytes.Buffer, s string) {
	length := len(s)
	buf.WriteByte(byte(length))
	buf.WriteString(s)
}

func readString(buf *bytes.Buffer) (string, error) {
	length := int(buf.Next(1)[0])
	str := string(buf.Next(length))
	return str, nil
}

// Static table based on the HPACK specification
var staticTable = []HeaderField{
	{":authority", ""},
	{":method", "GET"},
	{":method", "POST"},
	{":path", "/"},
	{":path", "/index.html"},
	{":scheme", "http"},
	{":scheme", "https"},
	{":status", "200"},
	{":status", "204"},
	{":status", "206"},
	{":status", "304"},
	{":status", "400"},
	{":status", "404"},
	{":status", "500"},
	{"accept-charset", ""},
	{"accept-encoding", "gzip, deflate"},
	{"accept-language", ""},
	{"accept-ranges", ""},
	{"accept", ""},
	{"access-control-allow-origin", ""},
	{"age", ""},
	{"allow", ""},
	{"authorization", ""},
	{"cache-control", ""},
	{"content-disposition", ""},
	{"content-encoding", ""},
	{"content-language", ""},
	{"content-length", ""},
	{"content-location", ""},
	{"content-range", ""},
	{"content-type", ""},
	{"cookie", ""},
	{"date", ""},
	{"etag", ""},
	{"expect", ""},
	{"expires", ""},
	{"from", ""},
	{"host", ""},
	{"if-match", ""},
	{"if-modified-since", ""},
	{"if-none-match", ""},
	{"if-range", ""},
	{"if-unmodified-since", ""},
	{"last-modified", ""},
	{"link", ""},
	{"location", ""},
	{"max-forwards", ""},
	{"proxy-authenticate", ""},
	{"proxy-authorization", ""},
	{"range", ""},
	{"referer", ""},
	{"refresh", ""},
	{"retry-after", ""},
	{"server", ""},
	{"set-cookie", ""},
	{"strict-transport-security", ""},
	{"transfer-encoding", ""},
	{"user-agent", ""},
	{"vary", ""},
	{"via", ""},
	{"www-authenticate", ""},
}

type dynamicTable struct {
	entries []HeaderField
}

func newDynamicTable() dynamicTable {
	return dynamicTable{}
}

func (dt *dynamicTable) get(index int) (HeaderField, error) {
	if index <= 0 {
		return HeaderField{}, fmt.Errorf("invalid index: %d", index)
	}
	if index <= len(staticTable) {
		return staticTable[index-1], nil
	}
	if index-len(staticTable) > len(dt.entries) {
		return HeaderField{}, fmt.Errorf("invalid index: %d", index)
	}
	return dt.entries[index-len(staticTable)-1], nil
}

func (dt *dynamicTable) add(field HeaderField) {
	dt.entries = append([]HeaderField{field}, dt.entries...)
}

func (dt *dynamicTable) find(field HeaderField) int {
	for i, f := range staticTable {
		if f.Name == field.Name && f.Value == field.Value {
			return i + 1
		}
	}
	for i, f := range dt.entries {
		if f.Name == field.Name && f.Value == field.Value {
			return len(staticTable) + i + 1
		}
	}
	return 0
}
