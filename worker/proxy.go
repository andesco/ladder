package main

import (
	"fmt"
	"ladder/pkg/ladderlib"
	"log"
	"net/http"
	"net/url"
	"strings"
	"syscall/js"
)

var ladderInstance *ladderlib.Ladder

func initLadder(env js.Value) (*ladderlib.Ladder, error) {
	if ladderInstance != nil {
		return ladderInstance, nil
	}

	rulesetPath := getEnvVar(env, "RULESET", "")
	l, err := ladderlib.NewLadder(rulesetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ladder library: %w", err)
	}
	ladderInstance = l
	return ladderInstance, nil
}

func proxyHandler(request, env, ctx js.Value) (js.Value, error) {
	l, err := initLadder(env)
	if err != nil {
		log.Printf("ERROR: Could not initialize ladder: %v", err)
		return createErrorResponse(500, "Could not initialize ladder library"), nil
	}

	targetURL, err := extractURLFromRequest(request)
	if err != nil {
		log.Printf("ERROR: In URL extraction: %v", err)
		return createErrorResponse(400, err.Error()), nil
	}

	headers := make(http.Header)
	jsHeaders := request.Get("headers")
	// This is a simplified way to iterate headers in JS. A full implementation
	// might need to use `keys()` and `get()` if the object is a Headers object.
	// For now, we assume we can get the referer directly.
	if referer := jsHeaders.Call("get", "referer"); !referer.IsNull() {
		headers.Set("Referer", referer.String())
	}

	body, respHeaders, err := l.ProcessRequest(targetURL, headers)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return createErrorResponse(500, err.Error()), nil
	}

	// Create response headers for the worker
	jsRespHeaders := js.Global().Get("Object").New()
	for key, values := range respHeaders {
		jsRespHeaders.Set(key, strings.Join(values, ", "))
	}

	responseInit := js.Global().Get("Object_").New()
	responseInit.Set("status", 200)
	responseInit.Set("headers", jsRespHeaders)

	return js.Global().Get("Response").New(string(body), responseInit), nil
}

func extractURLFromRequest(request js.Value) (string, error) {
	reqURL := request.Get("url").String()
	urlObj := js.Global().Get("URL").New(reqURL)
	path := urlObj.Get("pathname").String()

	decodedPath, err := url.QueryUnescape(path)
	if err != nil {
		decodedPath = path
	}

	targetURL := strings.TrimPrefix(decodedPath, "/")

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("error parsing target URL '%s': %w", targetURL, err)
	}

	if parsedURL.Scheme == "http" || parsedURL.Scheme == "https" {
		return targetURL, nil
	}

	// Handle relative paths by checking the referer
	jsHeaders := request.Get("headers")
	refererValue := jsHeaders.Call("get", "referer")
	if refererValue.IsNull() {
		return "", fmt.Errorf("relative path with no referer: %s", targetURL)
	}

	refererStr := refererValue.String()
	refererURL, err := url.Parse(refererStr)
	if err != nil {
		return "", fmt.Errorf("error parsing referer URL '%s': %w", refererStr, err)
	}

	realTarget, err := url.Parse(strings.TrimPrefix(refererURL.Path, "/"))
	if err != nil {
		return "", fmt.Errorf("error parsing real target from referer '%s': %w", refererURL.Path, err)
	}

	fullURL := &url.URL{
		Scheme: realTarget.Scheme,
		Host:   realTarget.Host,
		Path:   targetURL,
	}

	return fullURL.String(), nil
}

// Utility function to create error responses for Workers
func createErrorResponse(status int, message string) js.Value {
	responseInit := js.Global().Get("Object").New()
	responseInit.Set("status", status)
	responseInit.Set("statusText", message)

	headers := js.Global().Get("Object").New()
	headers.Set("Content-Type", "text/plain")
	responseInit.Set("headers", headers)

	return js.Global().Get("Response").New(message, responseInit)
}

func getEnvVar(env js.Value, key, fallback string) string {
	if !env.IsUndefined() && !env.Get(key).IsUndefined() {
		return env.Get(key).String()
	}
	return fallback
}