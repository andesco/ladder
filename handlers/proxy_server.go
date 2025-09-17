//go:build !js

package handlers

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/andesco/ladder/pkg/ruleset"
	"github.com/gofiber/fiber/v2"
)

// extracts a URL from the request ctx. If the URL in the request
// is a relative path, it reconstructs the full URL using the referer header.
func extractUrl(c *fiber.Ctx) (string, error) {
	// try to extract url-encoded
	reqUrl, err := url.QueryUnescape(c.Params("*"))
	if err != nil {
		// fallback
		reqUrl = c.Params("*")
	}

	// Extract the actual path from req ctx
	urlQuery, err := url.Parse(reqUrl)
	if err != nil {
		return "", fmt.Errorf("error parsing request URL '%s': %v", reqUrl, err)
	}

	isRelativePath := urlQuery.Scheme == ""

	// eg: https://localhost:8080/images/foobar.jpg -> https://realsite.com/images/foobar.jpg
	if isRelativePath {
		// Parse the referer URL from the request header.
		refererUrl, err := url.Parse(c.Get("referer"))
		if err != nil {
			return "", fmt.Errorf("error parsing referer URL from req: '%s': %v", reqUrl, err)
		}

		// Extract the real url from referer path
		realUrl, err := url.Parse(strings.TrimPrefix(refererUrl.Path, "/"))
		if err != nil {
			return "", fmt.Errorf("error parsing real URL from referer '%s': %v", refererUrl.Path, err)
		}

		// reconstruct the full URL using the referer's scheme, host, and the relative path / queries
		fullUrl := &url.URL{
			Scheme:   realUrl.Scheme,
			Host:     realUrl.Host,
			Path:     urlQuery.Path,
			RawQuery: urlQuery.RawQuery,
		}

		if os.Getenv("LOG_URLS") == "true" {
			log.Printf("modified relative URL: '%s' -> '%s'", reqUrl, fullUrl.String())
		}
		return fullUrl.String(), nil

	}

	// default behavior:
	// eg: https://localhost:8080/https://realsite.com/images/foobar.jpg -> https://realsite.com/images/foobar.jpg
	return urlQuery.String(), nil
}

func ProxySite(rulesetPath string) fiber.Handler {
	if rulesetPath != "" {
		rs, err := ruleset.NewRuleset(rulesetPath)
		if err != nil {
			panic(err)
		}
		rulesSet = rs
	}

	return func(c *fiber.Ctx) error {
		// Get the url from the URL
		url, err := extractUrl(c)
		if err != nil {
			log.Println("ERROR In URL extraction:", err)
		}

		queries := c.Queries()
		body, _, resp, err := FetchSite(url, queries)
		if err != nil {
			log.Println("ERROR:", err)
			c.SendStatus(fiber.StatusInternalServerError)
			return c.SendString(err.Error())
		}

		c.Cookie(&fiber.Cookie{})
		c.Set("Content-Type", resp.Header.Get("Content-Type"))
		c.Set("Content-Security-Policy", resp.Header.Get("Content-Security-Policy"))

		return c.SendString(body)
	}
}