package main

import (
	"encoding/json"
	_ "embed"
	"log"
	"strings"
	"syscall/js"
)

// Version for Cloudflare Workers deployment
var version = "workers-1.0"

func apiHandler(request, env, ctx js.Value) (js.Value, error) {
	// Get the URL from the request and extract the path after /api/
	reqUrl := request.Get("url").String()
	urlObj := js.Global().Get("URL").New(reqUrl)
	path := urlObj.Get("pathname").String()
	
	// Extract URL after /api/
	if len(path) <= 5 || path[:5] != "/api/" {
		return createErrorResponse(400, "Invalid API path"), nil
	}
	urlQuery := path[5:] // Remove "/api/"

	// Get query parameters (simplified for now)
	queriesMap := make(map[string]string)
	// TODO: In a full implementation, extract query parameters from URL

	body, req, resp, err := fetchSite(urlQuery, queriesMap, env)
	if err != nil {
		log.Println("ERROR:", err)
		return createErrorResponse(500, err.Error()), nil
	}

	response := APIResponse{
		Version: strings.TrimSpace(version),
		Body:    body,
	}

	// Convert request headers
	response.Request.Headers = make([]interface{}, 0, len(req.Header))
	for k, v := range req.Header {
		response.Request.Headers = append(response.Request.Headers, map[string]string{
			"key":   k,
			"value": v[0],
		})
	}

	// Convert response headers
	response.Response.Headers = make([]interface{}, 0, len(resp.Header))
	for k, v := range resp.Header {
		response.Response.Headers = append(response.Response.Headers, map[string]string{
			"key":   k,
			"value": v[0],
		})
	}

	// Convert to JSON
	jsonData, err := json.Marshal(response)
	if err != nil {
		return createErrorResponse(500, "Failed to encode JSON"), nil
	}

	// Create response headers
	headers := js.Global().Get("Object").New()
	headers.Set("Content-Type", "application/json")

	// Create response init object  
	responseInit := js.Global().Get("Object").New()
	responseInit.Set("status", 200)
	responseInit.Set("headers", headers)

	// Return Response with JSON
	return js.Global().Get("Response").New(string(jsonData), responseInit), nil
}

type APIResponse struct {
	Version string `json:"version"`
	Body    string `json:"body"`
	Request struct {
		Headers []interface{} `json:"headers"`
	} `json:"request"`
	Response struct {
		Headers []interface{} `json:"headers"`
	} `json:"response"`
}
