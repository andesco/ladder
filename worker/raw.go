package main

import (
	"log"
	"syscall/js"
)

func rawHandler(request, env, ctx js.Value) (js.Value, error) {
	// Get the URL from the request and extract the path after /raw/
	reqUrl := request.Get("url").String()
	urlObj := js.Global().Get("URL").New(reqUrl)
	path := urlObj.Get("pathname").String()
	
	// Extract URL after /raw/
	if len(path) <= 5 || path[:5] != "/raw/" {
		return createErrorResponse(400, "Invalid raw path"), nil
	}
	urlQuery := path[5:] // Remove "/raw/"

	// Get query parameters (simplified for now)
	queriesMap := make(map[string]string)
	// TODO: In a full implementation, extract query parameters from URL

	body, _, _, err := fetchSite(urlQuery, queriesMap, env)
	if err != nil {
		log.Println("ERROR:", err)
		return createErrorResponse(500, err.Error()), nil
	}

	// Create response init object
	responseInit := js.Global().Get("Object").New()
	responseInit.Set("status", 200)

	// Return raw HTML response (no specific Content-Type set, let browser determine)
	return js.Global().Get("Response").New(body, responseInit), nil
}
