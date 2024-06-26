package storage

import (
	"fmt"
	"sync"
)

type DataType int

func (st DataType) String() string {
	return [...]string{"none", "string", "stream"}[st]
}

const (
	NONE DataType = iota
	STRINGS
	STREAMS
)

type keyTypeMap struct {
	mu    *sync.RWMutex
	kType map[string]DataType
}

func newKeyType() *keyTypeMap {
	return &keyTypeMap{
		mu:    &sync.RWMutex{},
		kType: make(map[string]DataType),
	}
}

func (kt keyTypeMap) AssertKeyTypeOrNone(key string, t DataType) (found bool, err error) {
	tKey := kt.GetType(key)
	if tKey == NONE {
		return false, nil
	}

	if tKey != t {
		return false, fmt.Errorf("operation againsts wrong type of the Key")
	}

	return true, nil
}

func (kt keyTypeMap) GetType(key string) DataType {
	kt.mu.RLock()
	defer kt.mu.RUnlock()
	tKey, ok := kt.kType[key]
	if !ok {
		return NONE
	}

	return tKey
}

func (kt keyTypeMap) SetType(key string, t DataType) {
	kt.mu.Lock()
	defer kt.mu.Unlock()
	kt.kType[key] = t
}

func (kt keyTypeMap) Delete(key string) {
	kt.mu.Lock()
	defer kt.mu.Unlock()
	delete(kt.kType, key)
}
