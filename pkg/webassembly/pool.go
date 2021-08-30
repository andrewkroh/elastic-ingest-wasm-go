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
	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
)

type sessionPool struct {
	New func() *session
	C   chan *session
}

func newSessionPool(log *zap.Logger, host Host, store *wasmer.Store, module *wasmer.Module, maxSessions int) (*sessionPool, error) {
	s, err := newSession(log, host, store, module)
	if err != nil {
		return nil, err
	}

	pool := sessionPool{
		New: func() *session {
			s, _ := newSession(log, host, store, module)
			return s
		},
		C: make(chan *session, maxSessions),
	}
	pool.Put(s)

	return &pool, nil
}

func (p *sessionPool) Get() *session {
	select {
	case s := <-p.C:
		return s
	default:
		return p.New()
	}
}

func (p *sessionPool) Put(s *session) {
	if s != nil {
		select {
		case p.C <- s:
		default:
		}
	}
}
