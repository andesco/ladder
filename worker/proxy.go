package main

import (
	"fmt"
	"io"
	"log"
	"ladderflare/worker/rulesets"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var (
	defaultUserAgent = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	defaultForwardedFor = "66.249.66.1"
	defaultTimeout = 15 // in seconds
)

// Environment variable getter that works with Workers env object
func getEnvVar(env js.Value, key, fallback string) string {
	if !env.IsUndefined() && !env.Get(key).IsUndefined() {
		return env.Get(key).String()
	}
	return fallback
}

func proxyHandler(request, env, ctx js.Value) (js.Value, error) {
	// Get the URL from the request
	url, err := extractUrlFromRequest(request)
	if err != nil {
		log.Println("ERROR In URL extraction:", err)
		return createErrorResponse(500, err.Error()), nil
	}

	// Get query parameters (simplified for now)
	queriesMap := make(map[string]string)
	// TODO: In a full implementation, extract query parameters from URL
	
	body, _, resp, err := fetchSite(url, queriesMap, env)
	if err != nil {
		log.Println("ERROR:", err)
		return createErrorResponse(500, err.Error()), nil
	}

	// Create response headers
	headers := js.Global().Get("Object").New()
	headers.Set("Content-Type", resp.Header.Get("Content-Type"))
	if csp := resp.Header.Get("Content-Security-Policy"); csp != "" {
		headers.Set("Content-Security-Policy", csp)
	}

	// Create and return Response
	responseInit := js.Global().Get("Object").New()
	responseInit.Set("status", 200)
	responseInit.Set("headers", headers)
	
	return js.Global().Get("Response").New(body, responseInit), nil
}

// extracts a URL from the Workers request. If the URL in the request
// is a relative path, it reconstructs the full URL using the referer header.
func extractUrlFromRequest(request js.Value) (string, error) {
	reqUrl := request.Get("url").String()
	urlObj := js.Global().Get("URL").New(reqUrl)
	path := urlObj.Get("pathname").String()

	// try to extract url-encoded
	decodedPath, err := url.QueryUnescape(path)
	if err != nil {
		// fallback
		decodedPath = path
	}

	// Extract the actual path from request
	urlQuery, err := url.Parse(decodedPath)
	if err != nil {
		return "", fmt.Errorf("error parsing request URL '%s': %v", decodedPath, err)
	}
	

	// Check if this is a relative path or an absolute URL embedded in the path
	// If the path contains http:// or https://, it's an absolute URL with leading slash
	isRelativePath := urlQuery.Scheme == "" && !strings.Contains(decodedPath, "http")

	// eg: https://worker.domain/images/foobar.jpg -> https://realsite.com/images/foobar.jpg
	if isRelativePath {
		// Get referer from request headers
		headers := request.Get("headers")
		refererHeader := ""
		
		// Check if headers.get is available
		if !headers.IsUndefined() {
			refererValue := headers.Call("get", "referer")
			if !refererValue.IsUndefined() && !refererValue.IsNull() {
				refererHeader = refererValue.String()
			}
		}

		if refererHeader == "" {
			return "", fmt.Errorf("no referer header found for relative URL: %s", decodedPath)
		}

		// Parse the referer URL from the request header.
		refererUrl, err := url.Parse(refererHeader)
		if err != nil {
			return "", fmt.Errorf("error parsing referer URL from req: '%s': %v", decodedPath, err)
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

		log.Printf("modified relative URL: '%s' -> '%s'", decodedPath, fullUrl.String())
		return fullUrl.String(), nil
	}

	// default behavior:
	// eg: https://worker.domain/https://realsite.com/images/foobar.jpg -> https://realsite.com/images/foobar.jpg
	// Remove the leading slash from the path to get the actual target URL
	targetURL := strings.TrimPrefix(decodedPath, "/")
	return targetURL, nil
}

func modifyURL(uri string, rule rulesets.Rule) (string, error) {
	newUrl, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	for _, urlMod := range rule.URLMods.Domain {
		re := regexp.MustCompile(urlMod.Match)
		newUrl.Host = re.ReplaceAllString(newUrl.Host, urlMod.Replace)
	}

	for _, urlMod := range rule.URLMods.Path {
		re := regexp.MustCompile(urlMod.Match)
		newUrl.Path = re.ReplaceAllString(newUrl.Path, urlMod.Replace)
	}

	v := newUrl.Query()
	for _, query := range rule.URLMods.Query {
		if query.Value == "" {
			v.Del(query.Key)
			continue
		}
		v.Set(query.Key, query.Value)
	}
	newUrl.RawQuery = v.Encode()

	if rule.GoogleCache {
		newUrl, err = url.Parse("https://webcache.googleusercontent.com/search?q=cache:" + newUrl.String())
		if err != nil {
			return "", err
		}
	}

	return newUrl.String(), nil
}

func fetchSite(urlpath string, queries map[string]string, env js.Value) (string, *http.Request, *http.Response, error) {
	defer func() {
		// Clear queries map to help GC
		for k := range queries {
			delete(queries, k)
		}
	}()
	
	urlQuery := "?"
	if len(queries) > 0 {
		for k, v := range queries {
			urlQuery += k + "=" + v + "&"
		}
	}
	urlQuery = strings.TrimSuffix(urlQuery, "&")
	urlQuery = strings.TrimSuffix(urlQuery, "?")

	u, err := url.Parse(urlpath)
	if err != nil {
		return "", nil, nil, err
	}

	// Check allowed domains
	allowedDomainsStr := getEnvVar(env, "ALLOWED_DOMAINS", "")
	if allowedDomainsStr != "" {
		allowedDomains := strings.Split(allowedDomainsStr, ",")
		if !StringInSlice(u.Host, allowedDomains) {
			return "", nil, nil, fmt.Errorf("domain not allowed. %s not in %s", u.Host, allowedDomains)
		}
	}

	if getEnvVar(env, "LOG_URLS", "") == "true" {
		log.Println(u.String() + urlQuery)
	}

	// Modify the URI according to ruleset
	rule := fetchRule(u.Host, u.Path)
	url, err := modifyURL(u.String()+urlQuery, rule)
	if err != nil {
		return "", nil, nil, err
	}

	// Get timeout from environment
	timeoutStr := getEnvVar(env, "HTTP_TIMEOUT", "15")
	timeout, err := strconv.Atoi(timeoutStr)
	if err != nil {
		timeout = defaultTimeout
	}

	// Fetch the site
	client := &http.Client{
		Timeout: time.Second * time.Duration(timeout),
	}
	// Ensure URL is absolute for WASM/fetch compatibility
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + strings.TrimPrefix(url, "/")
	}
	req, _ := http.NewRequest("GET", url, nil)

	// Set User-Agent
	userAgent := getEnvVar(env, "USER_AGENT", defaultUserAgent)
	if rule.Headers.UserAgent != "" {
		req.Header.Set("User-Agent", rule.Headers.UserAgent)
	} else {
		req.Header.Set("User-Agent", userAgent)
	}

	// Set X-Forwarded-For
	forwardedFor := getEnvVar(env, "X_FORWARDED_FOR", defaultForwardedFor)
	if rule.Headers.XForwardedFor != "" {
		if rule.Headers.XForwardedFor != "none" {
			req.Header.Set("X-Forwarded-For", rule.Headers.XForwardedFor)
		}
	} else {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}

	if rule.Headers.Referer != "" {
		if rule.Headers.Referer != "none" {
			req.Header.Set("Referer", rule.Headers.Referer)
		}
	} else {
		req.Header.Set("Referer", u.String())
	}

	if rule.Headers.Cookie != "" {
		req.Header.Set("Cookie", rule.Headers.Cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, nil, err
	}
	defer resp.Body.Close()

	bodyB, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, nil, err
	}
	
	// Explicitly close body and force garbage collection of large responses
	resp.Body.Close()
	if len(bodyB) > 1024*1024 { // Large responses > 1MB
		defer func() {
			bodyB = nil // Help GC reclaim memory
		}()
	}

	if rule.Headers.CSP != "" {
		// log.Println(rule.Headers.CSP)
		resp.Header.Set("Content-Security-Policy", rule.Headers.CSP)
	}

	// log.Print("rule", rule) TODO: Add a debug mode to print the rule
	body := rewriteHtml(bodyB, u, rule, env)
	return body, req, resp, nil
}

func rewriteHtml(bodyB []byte, u *url.URL, rule rulesets.Rule, env js.Value) string {
	// Rewrite the HTML
	body := string(bodyB)

	// images
	imagePattern := `<img\s+([^>]*\s+)?src="(/)([^\"]*)"`
	re := regexp.MustCompile(imagePattern)
	body = re.ReplaceAllString(body, fmt.Sprintf(`<img $1 src="%s$3"`, "/https://"+u.Host+"/"))

	// scripts
	scriptPattern := `<script\s+([^>]*\s+)?src="(/)([^\"]*)"`
	reScript := regexp.MustCompile(scriptPattern)
	body = reScript.ReplaceAllString(body, fmt.Sprintf(`<script $1 script="%s$3"`, "/https://"+u.Host+"/"))

	// body = strings.ReplaceAll(body, "srcset=\"/", "srcset=\"/https://"+u.Host+"/") // TODO: Needs a regex to rewrite the URL's
	body = strings.ReplaceAll(body, "href=\"/", "href=\"/https://"+u.Host+"/")
	body = strings.ReplaceAll(body, "url('/", "url('/https://"+u.Host+"/")
	body = strings.ReplaceAll(body, "url(/", "url(/https://"+u.Host+"/")
	body = strings.ReplaceAll(body, "href=\"https://"+u.Host, "href=\"/https://"+u.Host+"/")

	if len(rulesSet) > 0 {
		body = applyRules(body, rule)
	}
	return body
}

func fetchRule(domain string, path string) rulesets.Rule {
	if len(rulesSet) == 0 {
		return rulesets.Rule{}
	}
	rule := rulesets.Rule{}
	for _, rule := range rulesSet {
		domains := rule.Domains
		if rule.Domain != "" {
			domains = append(domains, rule.Domain)
		}
		for _, ruleDomain := range domains {
			if ruleDomain == domain || strings.HasSuffix(domain, ruleDomain) {
				if len(rule.Paths) > 0 && !StringInSlice(path, rule.Paths) {
					continue
				}
				// return first match
				return rule
			}
		}
	}
	return rule
}

func applyRules(body string, rule rulesets.Rule) string {
	if len(rulesSet) == 0 {
		return body
	}

	for _, regexRule := range rule.RegexRules {
		re := regexp.MustCompile(regexRule.Match)
		body = re.ReplaceAllString(body, regexRule.Replace)
	}
	for _, injection := range rule.Injections {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		if err != nil {
			log.Fatal(err)
		}
		if injection.Replace != "" {
			doc.Find(injection.Position).ReplaceWithHtml(injection.Replace)
		}
		if injection.Append != "" {
			doc.Find(injection.Position).AppendHtml(injection.Append)
		}
		if injection.Prepend != "" {
			doc.Find(injection.Position).PrependHtml(injection.Prepend)
		}
		body, err = doc.Html()
		if err != nil {
			log.Fatal(err)
		}
	}

	return body
}

func StringInSlice(s string, list []string) bool {
	for _, x := range list {
		if strings.HasPrefix(s, x) {
			return true
		}
	}
	return false
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

// rulesSet is declared in main.go

