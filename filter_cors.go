// Copyright 2014 Codehack.com All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package relax

import (
	"github.com/codehack/go-strarr"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const defaultMaxAge = 86400 // 24 hours

var (
	// simpleMethods and simpleHeaders per the CORS recommendation - http://www.w3.org/TR/cors/#terminology
	simpleMethods = []string{"GET", "HEAD", "POST"}
	simpleHeaders = []string{"Cache-Control", "Content-Language", "Content-Type", "Expires", "Last-Modified", "Pragma"}

	// allowMethodsDefault are methods generally used in REST, leaving simple methods to be complete.
	allowMethodsDefault = []string{"GET", "POST", "PATCH", "PUT", "DELETE"}

	// allowHeadersDefault are reasonably useful headers in REST.
	allowHeadersDefault = []string{"Authorization", "Content-Type", "If-Match", "If-Modified-Since", "If-None-Match", "If-Unmodified-Since", "X-Requested-With"}

	// exposeHeadersDefault are headers used regularly by both client/server
	exposeHeadersDefault = []string{"Etag", "Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "X-Poll-Interval"}

	// allowOriginRegexp holds our pre-compiled origin regex patterns.
	allowOriginRegexp = []*regexp.Regexp{}
)

// CORSFilter implements the Cross-Origin Resource Sharing (CORS) recommendation, as
// described at http://www.w3.org/TR/cors/ (W3C).
type CORSFilter struct {
	// AllowOrigin is the list of URI patterns that are allowed to use the resource.
	// The patterns consist of text with zero or more wildcards '*' '?' '+'.
	//
	// '*' matches zero or more characters.
	// '?' matches exactly one character.
	// '_' matches zero or one character.
	// '+' matches at least one character.
	//
	// Note that a single pattern of '*' will match all origins, if that's what you need
	// then use AllowAnyOrigin=true instead. If AllowOrigin is empty and AllowAnyOrigin=false,
	// then all CORS requests (simple and preflight) will fail with an HTTP error response.
	//
	// Examples:
	// 	http://*example.com - matches example.com and all its subdomains.
	// 	http_://+.example.com - matches SSL and non-SSL, and subdomains of example.com, but not example.com
	// 	http://foo??.example.com - matches subdomains fooXX.example.com where X can be any character.
	//		chrome-extension://* - good for testing from Chrome.
	//
	// Default: empty
	AllowOrigin []string

	// AllowAnyOrigin if set to true, it will allow all origin requests.
	// This is effectively "Access-Control-Allow-Origin: *" as in the CORS specification.
	//
	// Default: false
	AllowAnyOrigin bool

	// AllowMethods is the list of HTTP methods that can be used in a request. If AllowMethods
	// is empty, all permission requests (preflight) will fail with an HTTP error response.
	//
	// Default: GET, DELETE, HEAD, POST, PUT
	AllowMethods []string

	// AllowHeaders is the list of HTTP headers that can be used in a request. If AllowHeaders
	// is empty, then only simple common HTTP headers are allowed.
	//
	// Default: Accept, Authorization, Content-Type, Origin
	AllowHeaders []string

	// AllowCredentials whether or not to allow user credendials to propagate through a request.
	// If AllowCredentials is false, then all authentication and cookies are disabled.
	//
	// Default: false
	AllowCredentials bool

	// ExposeHeaders is a list of HTTP headers that can be exposed to the API. This list should
	// include any custom headers that are needed to complete the response.
	//
	// Default: empty
	ExposeHeaders []string

	// MaxAge is a number of seconds the permission request (preflight) results should be cached.
	// This number should be large enough to complete all request from a client, but short enough to
	// keep the API secure. Set to -1 to disable caching.
	//
	// Default: 3600
	MaxAge int

	// Strict specifies whether or not to adhere strictly to the W3C CORS recommendation. If
	// Strict=false then the focus is performance instead of correctness. Also, Strict=true
	// will add more security checks to permission requests (preflight) and other security decisions.
	//
	// Default: false
	Strict bool
}

func (self *CORSFilter) corsHeaders(origin string) http.Header {
	headers := make(http.Header, 0)
	if self.AllowCredentials {
		headers.Set("Access-Control-Allow-Origin", origin)
		headers.Set("Access-Control-Allow-Credentials", "true")
		headers.Add("Vary", "Origin")
	} else if self.Strict {
		if self.AllowOrigin == nil {
			headers.Set("Access-Control-Allow-Origin", "null")
		} else {
			headers.Set("Access-Control-Allow-Origin", origin)
			headers.Add("Vary", "Origin")
		}
	} else {
		headers.Set("Access-Control-Allow-Origin", "*")
	}
	return headers
}

func (self *CORSFilter) handlePreflightRequest(origin, rmethod, rheaders string) (http.Header, error) {
	if !strarr.Contains(simpleMethods, rmethod) && !strarr.Contains(self.AllowMethods, rmethod) {
		return nil, &StatusError{http.StatusMethodNotAllowed, "Invalid method in preflight", nil}
	}
	if rheaders != "" {
		arr := strarr.Map(strings.TrimSpace, strings.Split(rheaders, ","))
		if len(strarr.Diff(arr, self.AllowHeaders)) == 0 {
			return nil, &StatusError{http.StatusForbidden, "Invalid header in preflight", nil}
		}
	}

	headers := self.corsHeaders(origin)
	if self.MaxAge > 0 {
		headers.Set("Access-Control-Max-Age", strconv.Itoa(self.MaxAge))
	}
	// BUG(TODO): CORSFilter needs preflight step 9 & 10 checks (too strict?)
	if self.AllowMethods != nil {
		headers.Set("Access-Control-Allow-Methods", strings.Join(self.AllowMethods, ", "))
	}
	if self.AllowHeaders != nil {
		headers.Set("Access-Control-Allow-Headers", strings.Join(self.AllowHeaders, ", "))
	}
	headers.Set("Content-Length", "0")

	return headers, nil
}

func (self *CORSFilter) handleSimpleRequest(origin string) http.Header {
	headers := self.corsHeaders(origin)
	if len(self.ExposeHeaders) > 0 {
		headers.Set("Access-Control-Expose-Headers", strings.Join(self.ExposeHeaders, ", "))
	}
	return headers
}

func (self *CORSFilter) isOriginAllowed(origin string) bool {
	for _, re := range allowOriginRegexp {
		if re.MatchString(origin) {
			return true
		}
	}
	return false
}

func (self *CORSFilter) Run(next HandlerFunc) HandlerFunc {
	if self.AllowMethods == nil {
		self.AllowMethods = allowMethodsDefault
	}
	if self.AllowHeaders == nil {
		self.AllowHeaders = allowHeadersDefault
	}
	if self.ExposeHeaders == nil {
		self.ExposeHeaders = exposeHeadersDefault
	}
	if self.MaxAge == 0 {
		self.MaxAge = defaultMaxAge
	}
	self.AllowMethods = strarr.Map(strings.ToUpper, self.AllowMethods)
	self.AllowHeaders = strarr.Map(http.CanonicalHeaderKey, self.AllowHeaders)
	self.ExposeHeaders = strarr.Map(http.CanonicalHeaderKey,
		strarr.Diff(self.ExposeHeaders, simpleHeaders))

	for _, v := range self.AllowOrigin {
		str := regexp.QuoteMeta(strings.ToLower(v))
		str = strings.Replace(str, `\+`, `.+`, -1)
		str = strings.Replace(str, `\*`, `.*`, -1)
		str = strings.Replace(str, `\?`, `.`, -1)
		str = strings.Replace(str, `_`, `.?`, -1)
		allowOriginRegexp = append(allowOriginRegexp, regexp.MustCompile(str))
	}

	return func(rw ResponseWriter, re *Request) {
		origin := re.Header.Get("Origin")

		re.Info.Set("cors.request", false)

		// This is not a CORS request, carry on.
		if origin == "" {
			next(rw, re)
			return
		}

		if !self.AllowAnyOrigin && !self.isOriginAllowed(origin) {
			if self.Strict {
				rw.Error(http.StatusForbidden, "Invalid CORS origin")
				return
			}
			next(rw, re)
			return
		}

		// Check that Origin: is sane and does not match Host:
		// http://www.w3.org/TR/cors/#resource-security
		if self.Strict {
			u, err := url.ParseRequestURI(origin)
			if err != nil {
				rw.Error(http.StatusBadRequest, err.Error())
				return
			}
			if re.Host == u.Host || u.Path != "" || !strings.HasPrefix(u.Scheme, "http") {
				rw.Error(http.StatusBadRequest, "Invalid CORS origin syntax")
				return
			}
		}

		// Method requested
		method := re.Header.Get("Access-Control-Request-Method")

		// Preflight request
		if re.Method == "OPTIONS" && method != "" {
			headers, err := self.handlePreflightRequest(origin, method, re.Header.Get("Access-Control-Request-Headers"))
			if err != nil {
				rw.Error((err.(*StatusError)).Code, err.Error())
				return
			}
			for k, v := range headers {
				rw.Header()[k] = v
			}
			rw.WriteHeader(http.StatusNoContent)
			return
		}

		// Simple request
		headers := self.handleSimpleRequest(origin)
		for k, v := range headers {
			rw.Header()[k] = v
		}

		// let other downstream filters know that this is a CORS request
		re.Info.Set("cors.request", true)
		re.Info.Set("cors.origin", origin)

		next(rw, re)
	}
}