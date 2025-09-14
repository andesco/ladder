package main

import (
	"fmt"
	"ladderflare/worker/rulesets"
	"syscall/js"
)

// Global ruleset variable
var rulesSet []rulesets.Rule

func main() {
	fmt.Println("Go main() function starting...")
	
	// Export the fetch function to JavaScript with a unique name
	js.Global().Set("goFetch", js.FuncOf(fetchHandler))

	fmt.Println("Go WASM module loaded and ready")
	
	// Keep the program running
	select {}
}

func fetchHandler(this js.Value, args []js.Value) interface{} {
	// The fetch function receives three arguments: request, env, ctx
	if len(args) < 3 {
		return js.Global().Get("Promise").Call("reject", js.ValueOf("Expected 3 arguments: request, env, ctx"))
	}

	request := args[0]
	env := args[1]
	ctx := args[2]

	// Return a Promise
	return js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resolve := args[0]
		reject := args[1]

		go func() {
			defer func() {
				if r := recover(); r != nil {
					reject.Invoke(js.ValueOf(fmt.Sprintf("Error: %v", r)))
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
	// Initialize ruleset on first request if not already done
	if len(rulesSet) == 0 {
		initRuleset(env)
	}

	// Get the URL from the request
	url := request.Get("url").String()

	// Parse the URL to get the path
	urlObj := js.Global().Get("URL").New(url)
	path := urlObj.Get("pathname").String()

	// Route the request based on the path for dynamic handlers
	switch {
	
	case path == "/ruleset":
		return rulesetHandler(request, env, ctx)
	case path == "/test":
		return testHandler(request, env, ctx)
	case len(path) > 5 && path[:5] == "/raw/":
		return rawHandler(request, env, ctx)
	case len(path) > 5 && path[:5] == "/api/":
		return apiHandler(request, env, ctx)
	}

	// For all other paths, try to serve a static asset first.
	// If the asset is not found (404), fall back to the proxy handler.
	// This requires returning a new promise that wraps the async logic.
	handlerPromise := js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resolve := args[0]
		reject := args[1]

		go func() {
			defer func() {
				if r := recover(); r != nil {
					reject.Invoke(js.ValueOf(fmt.Sprintf("Error in asset/proxy fallback: %v", r)))
				}
			}()

			// 1. Try to fetch from ASSETS
			assets := env.Get("ASSETS")
			if assets.IsUndefined() {
				// If ASSETS is not available, go straight to proxy handler
				proxyResponse, proxyErr := proxyHandler(request, env, ctx)
				if proxyErr != nil {
					reject.Invoke(js.ValueOf(proxyErr.Error()))
					return
				}
				resolve.Invoke(proxyResponse)
				return
			}

			assetResponsePromise := assets.Call("fetch", request)
			
			// Use .then() to handle the promise resolution
			assetResponsePromise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				response := args[0] // The resolved value of the promise (the Response object)

				// 2. Check if the asset was found
				if response.Get("status").Int() != 404 {
					// Asset found, resolve the outer promise with the response
					resolve.Invoke(response)
				} else {
					// 3. Asset not found, fall back to proxyHandler
					proxyResponse, proxyErr := proxyHandler(request, env, ctx)
					if proxyErr != nil {
						reject.Invoke(js.ValueOf(proxyErr.Error()))
						return nil // Return nil from this inner callback
					}
					resolve.Invoke(proxyResponse)
				}
				return nil // Always return nil from this inner callback
			}), js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				// Handle promise rejection (e.g., network error)
				err := args[0]
				reject.Invoke(js.ValueOf("Error fetching from assets: " + err.String()))
				return nil // Always return nil from this inner callback
			}))
		}()

		return nil
	}))

	return handlerPromise, nil
}



func initRuleset(env js.Value) {
	// Load rules from generated rules (downloaded from ladder-rules repo)
	rulesSet = rulesets.GeneratedRules
	fmt.Printf("Loaded %d rules from ladder-rules repository\n", len(rulesSet))
}