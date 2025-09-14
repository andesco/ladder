
package main

import (
	"fmt"
	"syscall/js"
)

func main() {
	fmt.Println("Go main() function starting...")

	// Export the fetch function to JavaScript
	js.Global().Set("goFetch", js.FuncOf(fetchHandler))

	fmt.Println("Go WASM module loaded and ready")

	// Keep the program running
	select {}
}

func fetchHandler(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return js.Global().Get("Promise").Call("reject", js.ValueOf("Expected 3 arguments: request, env, ctx"))
	}

	request := args[0]
	env := args[1]
	ctx := args[2]

	// Return a Promise that resolves with the response
	return js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, promiseArgs []js.Value) interface{} {
		resolve := promiseArgs[0]
		reject := promiseArgs[1]

		go func() {
			defer func() {
				if r := recover(); r != nil {
					reject.Invoke(js.ValueOf(fmt.Sprintf("Panic: %v", r)))
				}
			}()

			response, err := handleRequest(request, env, ctx)
			if err != nil {
				reject.Invoke(js.ValueOf(err.Error()))
				return
			}
			resolve.Invoke(response)
		}()

		return nil
	}))
}

func handleRequest(request, env, ctx js.Value) (js.Value, error) {
	url := request.Get("url").String()
	urlObj := js.Global().Get("URL").New(url)
	path := urlObj.Get("pathname").String()

	// Route to special handlers
	switch {
	case path == "/ruleset":
		// This handler might need adjustment depending on how rules are exposed
		// return rulesetHandler(request, env, ctx)
		return createErrorResponse(501, "Not Implemented"), nil
	case path == "/test":
		// This handler might need adjustment
		// return testHandler(request, env, ctx)
		return createErrorResponse(501, "Not Implemented"), nil
	case strings.HasPrefix(path, "/raw/"):
		return rawHandler(request, env, ctx)
	case strings.HasPrefix(path, "/api/"):
		return apiHandler(request, env, ctx)
	}

	// For all other paths, attempt to serve a static asset first, then fall back to the proxy.
	assets := env.Get("ASSETS")
	if assets.IsUndefined() {
		// No assets binding, go directly to proxy
		return proxyHandler(request, env, ctx)
	}

	// Return a new Promise that wraps the asset fetching and fallback logic
	return js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, promiseArgs []js.Value) interface{} {
		resolve := promiseArgs[0]
		reject := promiseArgs[1]

		assetResponsePromise := assets.Call("fetch", request)

		assetResponsePromise.Call("then", js.FuncOf(func(this js.Value, thenArgs []js.Value) interface{} {
			response := thenArgs[0]
			if response.Get("status").Int() != 404 {
				// Asset found, return it
				resolve.Invoke(response)
			} else {
				// Asset not found, fall back to the proxy handler
				proxyResponse, err := proxyHandler(request, env, ctx)
				if err != nil {
					reject.Invoke(js.ValueOf(err.Error()))
				} else {
					resolve.Invoke(proxyResponse)
				}
			}
			return nil
		}), js.FuncOf(func(this js.Value, catchArgs []js.Value) interface{} {
			// Error fetching from assets, reject the promise
			reject.Invoke(catchArgs[0])
			return nil
		}))

		return nil
	})), nil
}
