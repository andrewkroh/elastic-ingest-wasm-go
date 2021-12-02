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
order to pass other data types we pass pointers to WebAssembly guest memory locations.
To keep the PoC simple, all get and put field operations pass serialized JSON
strings.

The guest must export a `process` and `malloc` function. This host will invoke
`process` for each event to process. It will invoke malloc to allocate space
within the guest for values that are passed into the guest. The guest is
responsible for freeing that memory (the SDKs handle this automatically).

In the future guest modules should be required to export a function the specifies
the ABI version it used (the SDK would implement this). Then the host could try
to provide compatability or fail hard if incompatible
(like `elastic_ingest_wasm_abi_1_3`).

Also in the future these should be an optional function like `configure` or
`register` that is invoked once at initialization and passed in configuration.

The host provides a module named `elastic` that contains these functions:
- `elastic_get_field` - Get a field.
- `elastic_put_field` - Put a fields into the event.
- `elastic_get_current_time_nanoseconds` - Returns the current time as
  nanoseconds since unix epoch.
- `elastic_log` - Logs a message from the guest.
- TODO: provide an `elastic_remove_field`.

You can see details by looking at the
[Rust SDK](https://github.com/andrewkroh/elastic-ingest-wasm-rust-sdk/blob/main/src/hostcalls.rs).
