//go:build js && wasm

package main

import "syscall/js"

// bindGlobalFetch gives Go's fetch-based HTTP transport the right receiver.
//
// workers-go hands Go a Proxy of globalThis so it can inject the request
// context. Go's net/http transport calls fetch through that proxy, so `this`
// is the Proxy and workerd aborts the call with:
//
//	Illegal invocation: function called with incorrect `this` reference
//
// Rebinding globalThis.fetch to the real globalThis keeps outbound HTTP
// (Privy passwordless sign-in, JWKS) working.
func bindGlobalFetch() {
	global := js.Global()
	fetchFn := global.Get("fetch")
	if fetchFn.Type() != js.TypeFunction {
		return
	}
	global.Set("fetch", fetchFn.Call("bind", global.Get("globalThis")))
}
