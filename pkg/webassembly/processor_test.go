// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package webassembly_test

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"log"
	"runtime"
	"testing"
	"time"

	"github.com/andrewkroh/elastic-ingest-wasm-go/pkg/webassembly"
	"go.uber.org/zap"

	"github.com/stretchr/testify/require"
)

type defaultHost struct{}

func (_ defaultHost) GetCurrentTimeNanoseconds() int64 {
	return time.Now().UnixNano()
}

type defaultLog struct{}

func (_ defaultLog) Log(level webassembly.LogLevel, msg string) {
	log.Println("wasm", level, msg)
}

type event struct {
	fields map[string]interface{}
}

var _ webassembly.Event = (*event)(nil)

func (e *event) GetField(key string) (interface{}, bool) {
	v, found := e.fields[key]
	return v, found
}

func (e *event) PutField(key string, value interface{}) {
	e.fields[key] = value
}

func TestProcess(t *testing.T) {
	wasmData, err := ioutil.ReadFile("testdata/modify_fields.wasm.gz")
	if err != nil {
		t.Fatal(err)
	}

	r, err := gzip.NewReader(bytes.NewReader(wasmData))
	require.NoError(t, err)
	defer r.Close()

	log, err := zap.NewDevelopment()
	require.NoError(t, err)

	proc, err := webassembly.NewProcessor(
		log.Named("processor"),
		&defaultHost{},
		r,
		webassembly.Config{MaxCachedSessions: 1})
	if err != nil {
		t.Fatal(err)
	}

	evt := &event{fields: map[string]interface{}{}}
	if err = proc.Process(evt); err != nil {
		t.Fatal(err)
	}

	v := evt.fields["string"]
	require.EqualValues(t, "hello", v)

	v = evt.fields["integer"]
	require.EqualValues(t, 1, v)

	v = evt.fields["float"]
	require.EqualValues(t, 1.2, v)

	v = evt.fields["bool"]
	require.EqualValues(t, true, v)

	v = evt.fields["object"]
	require.EqualValues(t, map[string]interface{}{
		"hello": "world!",
	}, v)

	v, found := evt.fields["null"]
	require.True(t, found)
	require.Nil(t, v)

	require.Len(t, evt.fields, 6)
}

func BenchmarkProcess(b *testing.B) {
	wasmData, err := ioutil.ReadFile("testdata/modify_fields.wasm.gz")
	if err != nil {
		b.Fatal(err)
	}

	r, err := gzip.NewReader(bytes.NewReader(wasmData))
	require.NoError(b, err)
	defer r.Close()

	log, err := zap.NewDevelopment()
	require.NoError(b, err)

	proc, err := webassembly.NewProcessor(
		log.Named("processor"),
		&defaultHost{},
		r,
		webassembly.Config{MaxCachedSessions: runtime.GOMAXPROCS(-1)})
	if err != nil {
		b.Fatal(err)
	}

	b.Run("serial", func(b *testing.B) {
		evt := &event{fields: map[string]interface{}{}}
		for i := 0; i < b.N; i++ {
			if err := proc.Process(evt); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			evt := &event{fields: map[string]interface{}{}}
			for pb.Next() {
				if err := proc.Process(evt); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}
