package main

import (
	"syscall/js"
	"ladderflare/worker/rulesets"
)

func testHandler(request, env, ctx js.Value) (js.Value, error) {
	// Get a random test URL from the generated list (from everywall/ladder-rules repo)
	testURL := rulesets.GetRandomTestURL()

	if testURL == "" {
		// No test URLs available, return 404
		headers := js.Global().Get("Object").New()
		headers.Set("Content-Type", "text/plain")

		responseInit := js.Global().Get("Object").New()
		responseInit.Set("status", 404)
		responseInit.Set("headers", headers)

		return js.Global().Get("Response").New("No test URLs available", responseInit), nil
	}

	// Redirect to the proxied version of the test URL
	// Get the current host from the request
	requestURL := request.Get("url").String()
	urlObj := js.Global().Get("URL").New(requestURL)
	host := urlObj.Get("host").String()

	// If testURL is already a complete URL, use it directly as the path
	// Otherwise treat it as a domain and construct a basic URL
	var proxyURL string
	if len(testURL) > 8 && (testURL[:7] == "http://" || testURL[:8] == "https://") {
		// testURL is a complete URL, use it as the proxy path
		proxyURL = "https://" + host + "/" + testURL
	} else {
		// testURL is just a domain, construct a basic URL
		proxyURL = "https://" + host + "/" + testURL
	}

	headers := js.Global().Get("Object").New()
	headers.Set("Location", proxyURL)

	responseInit := js.Global().Get("Object").New()
	responseInit.Set("status", 302)
	responseInit.Set("headers", headers)

	return js.Global().Get("Response").New("", responseInit), nil
}