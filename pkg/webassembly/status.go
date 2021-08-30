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
	"strconv"

	"github.com/wasmerio/wasmer-go/wasmer"
)

type Status int32

const (
	StatusOK Status = iota
	StatusInternalFailure
	StatusInvalidArgument
	StatusNotFound
)

var statusNames = map[Status]string{
	StatusOK:              "OK",
	StatusInternalFailure: "Internal Failure",
	StatusInvalidArgument: "Invalid Argument",
	StatusNotFound:        "Not Found",
}

var wasmerStatus = map[Status][]wasmer.Value{
	StatusOK:              {wasmer.NewI32(int32(StatusOK))},
	StatusInternalFailure: {wasmer.NewI32(int32(StatusInternalFailure))},
	StatusInvalidArgument: {wasmer.NewI32(int32(StatusInvalidArgument))},
	StatusNotFound:        {wasmer.NewI32(int32(StatusNotFound))},
}

func (s Status) String() string {
	if name, found := statusNames[s]; found {
		return name
	}
	return "Status " + strconv.Itoa(int(s))
}

func (s Status) wasmerValue() []wasmer.Value {
	if v, found := wasmerStatus[s]; found {
		return v
	}
	return []wasmer.Value{wasmer.NewI32(int32(s))}
}
