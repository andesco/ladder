
package ladderlib

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v3"
)

// #############################################################################
// # Structs (from pkg/ruleset/ruleset.go)
// #############################################################################

type Regex struct {
	Match   string `yaml:"match"`
	Replace string `yaml:"replace"`
}
type KV struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

type RuleSet []Rule

type Rule struct {
	Domain  string   `yaml:"domain,omitempty"`
	Domains []string `yaml:"domains,omitempty"`
	Paths   []string `yaml:"paths,omitempty"`
	Headers struct {
		UserAgent     string `yaml:"user-agent,omitempty"`
		XForwardedFor string `yaml:"x-forwarded-for,omitempty"`
		Referer       string `yaml:"referer,omitempty"`
		Cookie        string `yaml:"cookie,omitempty"`
		CSP           string `yaml:"content-security-policy,omitempty"`
	} `yaml:"headers,omitempty"`
	GoogleCache bool    `yaml:"googleCache,omitempty"`
	RegexRules  []Regex `yaml:"regexRules,omitempty"`

	URLMods struct {
		Domain []Regex `yaml:"domain,omitempty"`
		Path   []Regex `yaml:"path,omitempty"`
		Query  []KV    `yaml:"query,omitempty"`
	} `yaml:"urlMods,omitempty"`

	Injections []struct {
		Position string `yaml:"position,omitempty"`
		Append   string `yaml:"append,omitempty"`
		Prepend  string `yaml:"prepend,omitempty"`
		Replace  string `yaml:"replace,omitempty"`
	} `yaml:"injections,omitempty"`
}

// #############################################################################
// # Core Ladder Library
// #############################################################################

type Ladder struct {
	UserAgent      string
	ForwardedFor   string
	Rules          RuleSet
	AllowedDomains []string
	Timeout        int
}

// NewLadder creates and initializes a new Ladder instance.
func NewLadder(rulesetPath string) (*Ladder, error) {
	rules, err := loadRuleset(rulesetPath)
	if err != nil {
		return nil, err
	}

	allowedDomains := strings.Split(os.Getenv("ALLOWED_DOMAINS"), ",")
	if os.Getenv("ALLOWED_DOMAINS_RULESET") == "true" {
		allowedDomains = append(allowedDomains, rules.Domains()...)
	}

	timeout := 15
	if timeoutStr := os.Getenv("HTTP_TIMEOUT"); timeoutStr != "" {
		timeout, _ = strconv.Atoi(timeoutStr)
	}

	return &Ladder{
		UserAgent:      getenv("USER_AGENT", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"),
		ForwardedFor:   getenv("X_FORWARDED_FOR", "66.249.66.1"),
		Rules:          rules,
		AllowedDomains: allowedDomains,
		Timeout:        timeout,
	}, nil
}

// ProcessRequest is the core, environment-agnostic function for proxying a request.
func (l *Ladder) ProcessRequest(targetURL string, requestHeader http.Header) ([]byte, http.Header, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing target URL: %w", err)
	}

	if len(l.AllowedDomains) > 0 && !stringInSlice(u.Host, l.AllowedDomains) {
		return nil, nil, fmt.Errorf("domain not allowed: %s", u.Host)
	}

	if os.Getenv("LOG_URLS") == "true" {
		log.Println(u.String())
	}

	rule := l.fetchRule(u.Host, u.Path)
	finalURL, err := modifyURL(u.String(), rule)
	if err != nil {
		return nil, nil, fmt.Errorf("error modifying URL: %w", err)
	}

	client := &http.Client{
		Timeout: time.Second * time.Duration(l.Timeout),
	}
	req, _ := http.NewRequest("GET", finalURL, nil)

	// Set headers from rules or defaults
	if rule.Headers.UserAgent != "" {
		req.Header.Set("User-Agent", rule.Headers.UserAgent)
	} else {
		req.Header.Set("User-Agent", l.UserAgent)
	}

	if rule.Headers.XForwardedFor != "" {
		if rule.Headers.XForwardedFor != "none" {
			req.Header.Set("X-Forwarded-For", rule.Headers.XForwardedFor)
		}
	} else {
		req.Header.Set("X-Forwarded-For", l.ForwardedFor)
	}

	if rule.Headers.Referer != "" {
		if rule.Headers.Referer != "none" {
			req.Header.Set("Referer", rule.Headers.Referer)
		}
	} else {
		// Use the original referer from the incoming request if available
		if incomingReferer := requestHeader.Get("Referer"); incomingReferer != "" {
			req.Header.Set("Referer", incomingReferer)
		} else {
			req.Header.Set("Referer", u.String())
		}
	}

	if rule.Headers.Cookie != "" {
		req.Header.Set("Cookie", rule.Headers.Cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching site: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading response body: %w", err)
	}

	if rule.Headers.CSP != "" {
		resp.Header.Set("Content-Security-Policy", rule.Headers.CSP)
	}

	rewrittenBody := rewriteHTML(bodyBytes, u, rule)

	return []byte(rewrittenBody), resp.Header, nil
}

// #############################################################################
// # Helper Functions (adapted from handlers/proxy.go and pkg/ruleset/ruleset.go)
// #############################################################################

func (l *Ladder) fetchRule(domain string, path string) Rule {
	if len(l.Rules) == 0 {
		return Rule{}
	}
	for _, rule := range l.Rules {
		ruleDomains := rule.Domains
		if rule.Domain != "" {
			ruleDomains = append(ruleDomains, rule.Domain)
		}
		for _, ruleDomain := range ruleDomains {
			if ruleDomain == domain || strings.HasSuffix(domain, "."+ruleDomain) {
				if len(rule.Paths) > 0 && !stringInSlice(path, rule.Paths) {
					continue
				}
				return rule // Return first match
			}
		}
	}
	return Rule{}
}

func modifyURL(uri string, rule Rule) (string, error) {
	newURL, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	for _, urlMod := range rule.URLMods.Domain {
		re := regexp.MustCompile(urlMod.Match)
		newURL.Host = re.ReplaceAllString(newURL.Host, urlMod.Replace)
	}

	for _, urlMod := range rule.URLMods.Path {
		re := regexp.MustCompile(urlMod.Match)
		newURL.Path = re.ReplaceAllString(newURL.Path, urlMod.Replace)
	}

	v := newURL.Query()
	for _, query := range rule.URLMods.Query {
		if query.Value == "" {
			v.Del(query.Key)
			continue
		}
		v.Set(query.Key, query.Value)
	}
	newURL.RawQuery = v.Encode()

	if rule.GoogleCache {
		newURL, err = url.Parse("https://webcache.googleusercontent.com/search?q=cache:" + newURL.String())
		if err != nil {
			return "", err
		}
	}

	return newURL.String(), nil
}

func rewriteHTML(bodyBytes []byte, u *url.URL, rule Rule) string {
	body := string(bodyBytes)

	// Basic URL rewriting
	body = strings.ReplaceAll(body, "href="/", "href="/https://"+u.Host+"/")
	body = strings.ReplaceAll(body, "src="/", "src="/https://"+u.Host+"/")
	body = strings.ReplaceAll(body, "url('/", "url('/https://"+u.Host+"/")
	body = strings.ReplaceAll(body, "url(/", "url(/https://"+u.Host+"/")
	body = strings.ReplaceAll(body, "href="https://"+u.Host, "href="/https://"+u.Host)

	// Apply advanced rules
	if (len(rule.RegexRules) > 0 || len(rule.Injections) > 0) {
		body = applyRules(body, rule)
	}

	return body
}

func applyRules(body string, rule Rule) string {
	for _, regexRule := range rule.RegexRules {
		re := regexp.MustCompile(regexRule.Match)
		body = re.ReplaceAllString(body, regexRule.Replace)
	}
	for _, injection := range rule.Injections {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		if err != nil {
			log.Printf("WARN: Could not parse HTML for injection: %v", err)
			continue
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
		
		html, err := doc.Html()
		if err != nil {
			log.Printf("WARN: Could not render HTML after injection: %v", err)
			continue
		}
		body = html
	}
	return body
}

func loadRuleset(rulePaths string) (RuleSet, error) {
	if rulePaths == "" {
		log.Printf("WARN: No ruleset specified. Set the `RULESET` environment variable to load one.")
		return RuleSet{}, nil
	}

	var ruleSet RuleSet
	var errs []error

	for _, rulePath := range strings.Split(rulePaths, ";") {
		trimmedPath := strings.TrimSpace(rulePath)
		if trimmedPath == "" {
			continue
		}

		var rules RuleSet
		err := filepath.Walk(trimmedPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
				yamlFile, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read rules file '%s': %w", path, err)
				}
				var r RuleSet
				if err := yaml.Unmarshal(yamlFile, &r); err != nil {
					return fmt.Errorf("syntax error in rules file '%s': %w", path, err)
				}
				rules = append(rules, r...)
			}
			return nil
		})

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to load rules from '%s': %w", trimmedPath, err))
		} else {
			ruleSet = append(ruleSet, rules...)
		}
	}

	if len(errs) > 0 {
		// In a library, we return the error rather than panic or log fatals.
		return nil, fmt.Errorf("errors while loading rulesets: %v", errs)
	}

	log.Printf("INFO: Loaded %d rules for %d domains
", ruleSet.Count(), ruleSet.DomainCount())
	return ruleSet, nil
}

func (rs *RuleSet) Domains() []string {
	var domains []string
	for _, rule := range *rs {
		if rule.Domain != "" {
			domains = append(domains, rule.Domain)
		}
		domains = append(domains, rule.Domains...)
	}
	return domains
}

func (rs *RuleSet) DomainCount() int {
	return len(rs.Domains())
}

func (rs *RuleSet) Count() int {
	return len(*rs)
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func stringInSlice(s string, list []string) bool {
	for _, x := range list {
		if strings.HasPrefix(s, x) {
			return true
		}
	}
	return false
}
