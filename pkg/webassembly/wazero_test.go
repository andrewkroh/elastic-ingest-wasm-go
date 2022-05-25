package webassembly

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata/modify_fields.wasm.gz
var modifyFieldsWASM []byte

type event struct {
	fields map[string]interface{}
}

var _ Event = (*event)(nil)

func (e *event) GetField(key string) (interface{}, bool) {
	v, found := e.fields[key]
	return v, found
}

func (e *event) PutField(key string, value interface{}) {
	e.fields[key] = value
}

func TestWazeroGuestProcess(t *testing.T) {
	wasmData, err := ioutil.ReadFile("testdata/modify_fields.wasm.gz")
	if err != nil {
		t.Fatal(err)
	}

	r, err := gzip.NewReader(bytes.NewReader(wasmData))
	require.NoError(t, err)
	defer r.Close()

	wasm, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	s, err := newWazeroSession(wasm)
	require.NoError(t, err)

	evt := &event{fields: map[string]interface{}{}}

	_, err = s.guestProcess(evt)
	require.NoError(t, err)

	v := evt.fields["string"]
	require.EqualValues(t, "hello", v)
}
