package hpack

import (
	"fmt"
	"sync"
)

type DynamicTable struct {
	entries []nameValueBytes
	mu      sync.RWMutex
}

func NewDynamicTable() *DynamicTable {
	return &DynamicTable{}
}

func (dt *DynamicTable) get(name, value *[]byte, index int) error {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if index <= 0 {
		return fmt.Errorf("invalid index: %d", index)
	}
	if index <= len(staticTable) {
		*name = append((*name)[:0], staticTable[index-1].name...)
		*value = append((*value)[:0], staticTable[index-1].value...)
		return nil
	}
	if index-len(staticTable) > len(dt.entries) {
		return fmt.Errorf("invalid index: %d", index)
	}

	// copy from dynamic table
	*name = append((*name)[:0], dt.entries[index-len(staticTable)-1].name...)
	*value = append((*value)[:0], dt.entries[index-len(staticTable)-1].value...)

	return nil
}

func (dt *DynamicTable) add(name, value []byte) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	kv := acquireNameValueBytes()

	kv.name = append(kv.name[:0], name...)
	kv.value = append(kv.value[:0], value...)
	dt.entries = append([]nameValueBytes{kv}, dt.entries...)
}

func (dt *DynamicTable) find(key, value []byte) int {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	for i, f := range staticTable {
		if f.name == string(key) && f.value == string(value) {
			return i + 1
		}
	}
	for i, f := range dt.entries {
		if string(f.name) == string(key) && string(f.value) == string(value) {
			return len(staticTable) + i + 1
		}
	}
	return 0
}
