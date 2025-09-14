package main

import (
	"syscall/js"

	"gopkg.in/yaml.v3"
)

func rulesetHandler(request, env, ctx js.Value) (js.Value, error) {
	// Check if ruleset exposure is disabled
	if getEnvVar(env, "EXPOSE_RULESET", "true") == "false" {
		// Return 403 Forbidden response
		responseInit := js.Global().Get("Object").New()
		responseInit.Set("status", 403)
		responseInit.Set("statusText", "Forbidden")
		
		headers := js.Global().Get("Object").New()
		headers.Set("Content-Type", "text/plain")
		responseInit.Set("headers", headers)
		
		return js.Global().Get("Response").New("Ruleset Disabled", responseInit), nil
	}

	// Marshal the current ruleset to YAML
	body, err := yaml.Marshal(rulesSet)
	if err != nil {
		return createErrorResponse(500, err.Error()), nil
	}

	// Create response headers
	headers := js.Global().Get("Object").New()
	headers.Set("Content-Type", "application/x-yaml")

	// Create response init object
	responseInit := js.Global().Get("Object").New()
	responseInit.Set("status", 200)
	responseInit.Set("headers", headers)

	// Return Response with YAML ruleset
	return js.Global().Get("Response").New(string(body), responseInit), nil
}
