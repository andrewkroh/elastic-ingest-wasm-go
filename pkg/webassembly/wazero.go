package webassembly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
)

type contextKey string

type wazeroSession struct {
	processFunc  api.Function
	mallocFunc   api.Function
	registerFunc api.Function
}

func newWazeroSession(wasm []byte) (*wazeroSession, error) {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Instantiate a Go-defined module named "env" that exports a function to
	// log to the console.
	elasticMod, err := r.NewModuleBuilder("elastic").
		ExportFunction("elastic_get_field", elasticGetField).
		ExportFunction("elastic_put_field", elasticPutField).
		ExportFunction("elastic_get_current_time_nanoseconds", elasticGetField).
		ExportFunction("elastic_log", elasticGetField).
		Compile(ctx, wazero.NewCompileConfig())
	if err != nil {
		return nil, err
	}

	_, err = r.InstantiateModule(nil, elasticMod, wazero.NewModuleConfig())
	if err != nil {
		return nil, err
	}

	guestMod, err := r.CompileModule(ctx, wasm, wazero.NewCompileConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}

	mod, err := r.InstantiateModule(ctx, guestMod, wazero.NewModuleConfig().WithName("instance_1"))
	if err != nil {
		return nil, err
	}

	// TODO: List export functions to determine the ABI version.

	// Get references to WebAssembly functions we'll use in this example.
	process := mod.ExportedFunction("process")
	malloc := mod.ExportedFunction("malloc")
	register := mod.ExportedFunction("register")

	return &wazeroSession{
		processFunc:  process,
		mallocFunc:   malloc,
		registerFunc: register,
	}, nil
}

func elasticGetField(ctx context.Context, m api.Module, keyAddr, keySize, rtnPtrPtr, rtnSizePtr uint32) int32 {
	log.Println("elastic_get_field")
	return int32(StatusOK)
}

func elasticPutField(ctx context.Context, m api.Module, keyAddr, keySize, valueAddr, valueSize uint32) int32 {
	var key string
	var value interface{}
	if data, ok := m.Memory().Read(ctx, keyAddr, keySize); ok {
		key = string(data)
	}
	if data, ok := m.Memory().Read(ctx, valueAddr, valueSize); ok {
		if err := json.Unmarshal(data, &value); err != nil {
			return int32(StatusInvalidArgument)
		}
	}

	evt := ctx.Value(contextKey("event")).(Event)
	evt.PutField(key, value)

	return int32(StatusOK)
}

//     fn elastic_get_current_time_nanoseconds(return_time: *mut u64) -> Status;
func elasticGetCurrentTimeNanoseconds(ctx context.Context, m api.Module, returnTime int64) int32 {
	log.Println("elastic_get_current_time_nanoseconds")
	return int32(StatusOK)
}

func elasticLog(ctx context.Context, m api.Module, messageDataAddr, messageSize uint32) int32 {
	log.Println("elastic_log")
	return int32(StatusOK)
}

func (s *wazeroSession) malloc(size int32) (int32, error) {
	rtn, err := s.mallocFunc.Call(context.Background(), uint64(size))
	if err != nil {
		return 0, err
	}

	// Return address of allocated memory.
	return int32(rtn[0]), nil
}

func (s *wazeroSession) guestProcess(evt Event) (int32, error) {
	ctx := context.WithValue(context.Background(), contextKey("event"), evt)

	rtn, err := s.processFunc.Call(ctx)
	if err != nil {
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 0 {
				return int32(rtn[0]), nil
			}
		}

		return 0, err
	}

	return int32(rtn[0]), nil
}
