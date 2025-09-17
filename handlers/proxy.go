// The original proxy.go contained both server-specific logic (Fiber web framework)
// and core fetching/HTML rewriting logic.
//
// To enable WebAssembly (WASM) compatibility and compiling,
// the original proxy.go was split into two files:
//
// 1. proxy_server.go:
//    - contains server-specific, Fiber-dependent code
//    - excluded from WASM builds using build tags
// 2. proxy_fetch.go
//    - contains core fetching and HTML rewriting logic
//    - compatible with WASM
package handlers