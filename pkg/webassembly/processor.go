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

package webassembly

import (
	"fmt"
	"io"
	"io/ioutil"

	"go.uber.org/zap"

	"github.com/wasmerio/wasmer-go/wasmer"
)

type Event interface {
	GetField(key string) (interface{}, bool)

	PutField(key string, value interface{})
}

type Host interface {
	GetCurrentTimeNanoseconds() int64
}

type Processor struct {
	pool *sessionPool
}

type Config struct {
	MaxCachedSessions int                    `json:"max_cached_sessions" config:"max_cached_sessions"`
	Params            map[string]interface{} `json:"params" config:"params"`
}

func NewProcessor(log *zap.Logger, host Host, wasm io.Reader, config Config) (*Processor, error) {
	wasmModuleBytes, err := ioutil.ReadAll(wasm)
	if err != nil {
		return nil, fmt.Errorf("failed to read module data: %w", err)
	}

	engine := wasmer.NewEngine()
	store := wasmer.NewStore(engine)
	module, err := wasmer.NewModule(store, wasmModuleBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile WASM module: %w", err)
	}

	pool, err := newSessionPool(log, host, store, module, config.MaxCachedSessions)
	if err != nil {
		return nil, fmt.Errorf("failed to create WASM session: %w", err)
	}

	return &Processor{pool: pool}, nil
}

func (p *Processor) Process(event Event) error {
	// Use a pool of sessions to allow concurrent operations.
	session := p.pool.Get()
	defer p.pool.Put(session)

	status, err := session.guestProcess(event)
	if err != nil {
		return err
	}
	if StatusOK != Status(status) {
		return fmt.Errorf("process failed with status %v", status)
	}

	return nil
}
