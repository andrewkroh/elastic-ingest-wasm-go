# elastic-ingest-wasm-go

elastic-ingest-wasm-go is a proof-of-concept library to execute a WebAssembly
module to process an event. This project uses Wasmer as the WebAssembly runtime.
The WebAssembly module should make use of
[elastic-ingest-wasm-rust-sdk](https://github.com/andrewkroh/elastic-ingest-wasm-rust-sdk)
for making calls into the "host" for getting and putting fields in an event or
reading the current system time.

## ABI (Application Binary Interface)

The interface between the WebAssembly module ("guest") and host is defined as
an ABI. The SDK implementations provide a higher level API around this interface.

WebAssembly can only pass integers and floats across the guest/host boundary. In
order to pass complex data types we serialize data into the guest memory and 
pass pointers to those memory locations. Serialization is accomplished with
protocol buffers.

The guest must export `process` and `malloc` functions. This host will invoke
`process` for each event to process. It will invoke malloc to allocate space
within the guest for values that are passed into the guest. The guest is
responsible for freeing that memory (the SDKs handle this automatically).

Guest modules must also export a function the specifies the ABI version it uses.
For example an export like `elastic_ingest_wasm_abi_1_3` indicates the module
is using ABI version 1.3. The SDKs provide this automatically. The host can use
this to determine compatability (either adapt to the specified version or
reject the module).

Guest module may implement a `register` function that is invoked once at
initialization and is passed configuration data.

The host provides a module named `elastic` that contains these functions:
- `elastic_emit_event` - Output an event. This passes the address and length
  of a serialized event to the host. If no event is emitted then the event is
  dropped.
- `elastic_get_current_time_nanoseconds` - Returns the current time as
  nanoseconds since Unix epoch.
- `elastic_log` - Logs a message from the guest.

You can see details by looking at the
[Rust SDK](https://github.com/andrewkroh/elastic-ingest-wasm-rust-sdk/blob/main/src/hostcalls.rs).
