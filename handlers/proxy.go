package handlers

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"ladder/pkg/ladderlib"

	"github.com/gofiber/fiber/v2"
)

// ProxySite is a Fiber handler that proxies requests through the ladderlib.
func ProxySite(rulesetPath string) fiber.Handler {
	// Initialize the shared ladder library
	l, err := ladderlib.NewLadder(rulesetPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize ladder library: %v", err))
	}

	return func(c *fiber.Ctx) error {
		// Extract the target URL
		targetURL, err := extractURL(c)
		if err != nil {
			log.Printf("ERROR: Could not extract URL: %v", err)
			return c.Status(fiber.StatusBadRequest).SendString("Could not extract URL")
		}

		// Convert Fiber headers to http.Header
		headers := make(http.Header)
		c.Request().Header.VisitAll(func(key, value []byte) {
			headers.Add(string(key), string(value))
		})

		// Process the request using the shared library
		body, respHeaders, err := l.ProcessRequest(targetURL, headers)
		if err != nil {
			log.Printf("ERROR: Failed to process request for %s: %v", targetURL, err)
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Set response headers from the proxied response
		for key, values := range respHeaders {
			for _, value := range values {
				c.Set(key, value)
			}
		}

		return c.Send(body)
	}
}

// extractURL extracts a URL from the request context. If the URL in the request
// is a relative path, it reconstructs the full URL using the referer header.
func extractURL(c *fiber.Ctx) (string, error) {
	path := c.Params("*")
	// Gofiber automatically decodes path parameters.

	urlQuery, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("error parsing request path '%s': %w", path, err)
	}

	// If the path is a full URL, return it directly.
	if urlQuery.Scheme == "http" || urlQuery.Scheme == "https" {
		return path, nil
	}

	// It's a relative path. Reconstruct it based on the referer.
	refererHeader := c.Get("Referer")
	if refererHeader == "" {
		return "", fmt.Errorf("cannot resolve relative path without a referer: %s", path)
	}

	refererURL, err := url.Parse(refererHeader)
	if err != nil {
		return "", fmt.Errorf("error parsing referer URL '%s': %w", refererHeader, err)
	}

	// The actual target site is in the referer's path.
	realTarget, err := url.Parse(strings.TrimPrefix(refererURL.Path, "/"))
	if err != nil {
		return "", fmt.Errorf("error parsing real target from referer path '%s': %w", refererURL.Path, err)
	}

	fullURL := &url.URL{
		Scheme:   realTarget.Scheme,
		Host:     realTarget.Host,
		Path:     path,           // The path from the current request
		RawQuery: c.Request().URI().QueryString(), // Preserve query params
	}

	if os.Getenv("LOG_URLS") == "true" {
		log.Printf("Resolved relative URL: '%s' -> '%s'", path, fullURL.String())
	}

	return fullURL.String(), nil
}