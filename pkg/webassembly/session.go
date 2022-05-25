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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"runtime"

	"go.uber.org/zap"

	"github.com/wasmerio/wasmer-go/wasmer"
)

const (
	moduleName = "elastic"
)

type session struct {
	host     Host
	log      *zap.Logger
	event    Event
	instance *wasmer.Instance

	// Exports from the "guest" module being run.
	guestExports struct {
		malloc  *wasmer.Function
		process *wasmer.Function
		memory  *wasmer.Memory
	}
	guestLog *zap.Logger
}

func newSession(log *zap.Logger, host Host, store *wasmer.Store, module *wasmer.Module) (*session, error) {
	// This needs an instance and an event to be usable.
	hc := &session{host: host, log: log, guestLog: log.Named("webassembly")}
	imports := wasmer.NewImportObject()
	imports.Register(moduleName, hc.makeElasticExports(store))

	var err error
	hc.instance, err = wasmer.NewInstance(module, imports)
	if err != nil {
		return nil, fmt.Errorf("failed to create WASM module instance: %w", err)
	}

	hc.guestExports.malloc, err = getExportedFunc(hc.instance, "malloc", 1, 1)
	if err != nil {
		return nil, err
	}
	hc.guestExports.process, err = getExportedFunc(hc.instance, "process", 0, 1)
	if err != nil {
		return nil, err
	}
	hc.guestExports.memory, err = hc.instance.Exports.GetMemory("memory")
	if err != nil {
		return nil, fmt.Errorf("failed to get memory export from guest: %w", err)
	}

	return hc, nil
}

func (hc *session) makeElasticExports(store *wasmer.Store) map[string]wasmer.IntoExtern {
	return map[string]wasmer.IntoExtern{
		"elastic_get_field": wasmer.NewFunction(
			store,
			wasmer.NewFunctionType(
				wasmer.NewValueTypes(wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
				wasmer.NewValueTypes(wasmer.I32)),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				keyPtr := args[0].I32()
				keyLen := args[1].I32()
				rtnPtr := args[2].I32()
				rtnLen := args[3].I32()

				keyBytes, err := hc.getGuestMemory(keyPtr, keyLen)
				if err != nil {
					hc.log.Warn("Failed to access guest memory.", zap.Error(err))
					return StatusInvalidArgument.wasmerValue(), nil
				}

				key := string(keyBytes)
				data, err := hc.elasticGetField(key)
				if err != nil {
					hc.log.Warn("elastic_get_field failed.", zap.String("field", key), zap.Error(err))
					return StatusInternalFailure.wasmerValue(), nil
				}
				if data == nil {
					return StatusNotFound.wasmerValue(), nil
				}
				dataSize := len(data)

				addr, slice, err := hc.guestMalloc(int32(len(data)))
				if err != nil {
					hc.log.Warn("Guest malloc failed.", zap.Int("length", len(data)), zap.Error(err))
					return StatusInternalFailure.wasmerValue(), nil
				}
				copy(slice, data)

				slice, err = hc.getGuestMemory(rtnPtr, 4)
				if err != nil {
					hc.log.Warn("Failed to access guest memory.", zap.Error(err))
					return StatusInvalidArgument.wasmerValue(), nil
				}
				binary.LittleEndian.PutUint32(slice, uint32(addr))

				slice, err = hc.getGuestMemory(rtnLen, 4)
				if err != nil {
					hc.log.Warn("Failed to access guest memory.", zap.Error(err))
					return StatusInvalidArgument.wasmerValue(), nil
				}
				binary.LittleEndian.PutUint32(slice, uint32(dataSize))

				return StatusOK.wasmerValue(), nil
			},
		),
		"elastic_put_field": wasmer.NewFunction(
			store,
			wasmer.NewFunctionType(
				wasmer.NewValueTypes(wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
				wasmer.NewValueTypes(wasmer.I32)),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				keyPtr := args[0].I32()
				keyLen := args[1].I32()
				valuePtr := args[2].I32()
				valueLen := args[3].I32()

				key, err := hc.getGuestMemory(keyPtr, keyLen)
				if err != nil {
					return nil, err
				}
				value, err := hc.getGuestMemory(valuePtr, valueLen)
				if err != nil {
					return nil, err
				}
				if err = hc.elasticPutField(string(key), value); err != nil {
					return nil, err
				}
				return []wasmer.Value{wasmer.NewI32(int32(StatusOK))}, nil
			},
		),
		"elastic_log": wasmer.NewFunction(
			store,
			wasmer.NewFunctionType(
				wasmer.NewValueTypes(wasmer.I32, wasmer.I32, wasmer.I32),
				wasmer.NewValueTypes(wasmer.I32),
			),
			func(values []wasmer.Value) ([]wasmer.Value, error) {
				level := values[0].I32()
				msgAddr := values[1].I32()
				msgLen := values[2].I32()

				msg, err := hc.getGuestMemory(msgAddr, msgLen)
				if err != nil {
					return nil, err
				}

				hc.elasticLog(LogLevel(level), string(msg))
				return []wasmer.Value{wasmer.NewI32(int32(StatusOK))}, nil
			},
		),
		"elastic_get_current_time_nanoseconds": wasmer.NewFunction(
			store,
			wasmer.NewFunctionType(
				wasmer.NewValueTypes(wasmer.I32),
				wasmer.NewValueTypes(wasmer.I32),
			),
			func(values []wasmer.Value) ([]wasmer.Value, error) {
				uint64Ptr := values[0].I32()

				data, err := hc.getGuestMemory(uint64Ptr, 8)
				if err != nil {
					return nil, err
				}

				now := hc.elasticGetCurrentTimeNanoseconds()
				binary.LittleEndian.PutUint64(data, uint64(now))
				return []wasmer.Value{wasmer.NewI32(int32(StatusOK))}, nil
			},
		),
	}
}

func (hc *session) elasticLog(level LogLevel, msg string) {
	if ce := hc.log.Check(level.toZapLevel(), msg); ce != nil {
		ce.Write()
	}
}

func (hc *session) elasticGetField(key string) ([]byte, error) {
	v, exists := hc.event.GetField(key)
	if !exists {
		return nil, nil
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (hc *session) elasticPutField(key string, value []byte) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	runtime.Goexit()
	var v interface{}
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}

	hc.event.PutField(key, v)
	return nil
}

func (hc *session) elasticGetCurrentTimeNanoseconds() int64 {
	return hc.host.GetCurrentTimeNanoseconds()
}

func (hc *session) guestMalloc(size int32) (address int32, slice []byte, err error) {
	rtn, err := hc.guestExports.malloc.Call(size)
	if err != nil {
		return 0, nil, err
	}
	address = rtn.(int32)

	slice, err = hc.getGuestMemory(address, size)
	if err != nil {
		return 0, nil, err
	}

	return address, slice, nil
}

func (hc *session) guestProcess(evt Event) (int32, error) {
	hc.event = evt

	rtn, err := hc.guestExports.process.Call()
	if err != nil {
		return 0, err
	}

	status := rtn.(int32)
	return status, nil
}

func (hc *session) getGuestMemory(address, size int32) ([]byte, error) {
	if uint(address+size) > hc.guestExports.memory.DataSize()-1 {
		return nil, fmt.Errorf("impossible address given size of available memory")
	}
	return hc.guestExports.memory.Data()[address : address+size], nil
}

func getExportedFunc(instance *wasmer.Instance, name string, parameterArity, resultArity uint) (*wasmer.Function, error) {
	f, err := instance.Exports.GetRawFunction(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get %v export: %w", name, err)
	}
	if f.ParameterArity() != parameterArity {
		return nil, fmt.Errorf("%v export must accept 1 parameter but has %d",
			name, f.ParameterArity())
	}
	if f.ResultArity() != resultArity {
		return nil, fmt.Errorf("%v export must return 1 value but has %d",
			name, f.ResultArity())
	}

	return f, nil
}
