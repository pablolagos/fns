package hpack

import (
	"encoding/hex"
	"testing"

	"golang.org/x/net/http2/hpack"
)

func TestHuffmanDecode(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected string
		err      bool
	}{
		{
			name:     "simple a",
			input:    []byte{0x1F}, // Huffman code for 'a'
			expected: "a",
			err:      false,
		},
		{
			name:     "simple space",
			input:    []byte{0x53}, // Huffman code for ' ' (space)
			expected: " ",
			err:      false,
		},
		{
			name:     "no-cache",
			input:    []byte{0xa8, 0xeb, 0x10, 0x64, 0x9c, 0xbf},
			expected: "no-cache",
			err:      false,
		},
		{
			name:     "www.example.com",
			input:    []byte{0xf1, 0xe3, 0xc2, 0xe5, 0xf2, 0x3a, 0x6b, 0xa0, 0xab, 0x90, 0xf4, 0xff},
			expected: "www.example.com",
			err:      false,
		},
		{
			name:     "custom-key",
			input:    []byte{0x25, 0xa8, 0x49, 0xe9, 0x5b, 0xa9, 0x7d, 0x7f},
			expected: "custom-key",
			err:      false,
		},
		{
			name:     "custom-value",
			input:    []byte{0x25, 0xa8, 0x49, 0xe9, 0x5b, 0xb8, 0xe8, 0xb4, 0xbf},
			expected: "custom-value",
			err:      false,
		},
		{
			name:     "private",
			input:    []byte{0xae, 0xc3, 0x77, 0x1a, 0x4b},
			expected: "private",
			err:      false,
		},
		{
			name:     "empty string",
			input:    []byte{},
			expected: "",
			err:      false,
		},
		{
			name:     "single character",
			input:    []byte{0x07},
			expected: "0",
			err:      false,
		},
		{
			name:     "all ASCII printable characters",
			input:    []byte{0x99, 0xcb, 0xaa, 0xfc, 0x8d, 0x5c, 0x28, 0xd9, 0x61, 0x3d, 0x76, 0x3d, 0x10, 0x1c, 0x9d, 0x24, 0xcb, 0x61, 0x8c, 0x89, 0x6d, 0x3d, 0x76, 0x3d, 0xf7, 0x01, 0xab, 0x90, 0xc2, 0x62, 0x29, 0x0a, 0x77, 0x37, 0x30, 0xba, 0x7c, 0x1f},
			expected: " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~",
			err:      false,
		},
		{
			name:     "invalid code",
			input:    []byte{0xFF, 0xFF, 0xFF, 0xFF},
			expected: "",
			err:      true,
		},
		{
			name:     "incomplete code",
			input:    []byte{0xae}, // "pr" without the last bits
			expected: "",
			err:      true,
		},
		{
			name:     "invalid padding",
			input:    []byte{0x40, 0x7f}, // "a" with invalid padding
			expected: "",
			err:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			var correctEncoded []byte
			correctEncoded = make([]byte, 0, 1024)

			correctEncoded = hpack.AppendHuffmanString(correctEncoded, string(tc.expected))
			if string(correctEncoded) != string(tc.input) {
				t.Errorf("Expected encoded string %q, but got %q", hex.EncodeToString(correctEncoded), hex.EncodeToString(tc.input))
				t.Logf("Decoded Length: %d, Enmcoded Length: %d", len(tc.expected), len(correctEncoded))
			}

			var result []byte
			err := huffmanDecode(&result, tc.input)
			if tc.err {
				if err == nil {
					t.Errorf("Expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
				if string(tc.expected) != string(result) {
					t.Errorf("Expected result %q, but got %q", tc.expected, result)
				}
			}
		})
	}
}

func TestHuffmanDecode0Allocs(t *testing.T) {
	testCases := []struct {
		name   string
		buffer []byte
	}{
		{
			name:   "nil buffer",
			buffer: nil,
		},
		{
			name:   "empty buffer",
			buffer: []byte{},
		},
		{
			name:   "single byte buffer",
			buffer: []byte{0x07},
		},
	}
	// This test is used to measure the number of allocations made by the huffmanDecode function

	input := []byte{0xf1, 0xe3, 0xc2, 0xe5, 0xf2, 0x3a, 0x6b, 0xa0, 0xab, 0x90, 0xf4, 0xff}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n := testing.AllocsPerRun(10, func() {
				if len(tc.buffer) != 0 {
					tc.buffer = tc.buffer[:0]
				}
				err := huffmanDecode(&tc.buffer, input)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if string(tc.buffer) != "www.example.com" {
					t.Errorf("Expected result www.example.com, but got %q", tc.buffer)
				}
			})

			if n != 0 {
				t.Fatalf("expected 0 allocations, got %f", n)
			}
		})
	}
}
