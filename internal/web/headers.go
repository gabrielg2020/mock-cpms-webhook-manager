package web

import "net/http"

// default-src is the fallback for everything the page loads (scripts, styles, images, fonts, requests),
// so all of it must come from this site and inline scripts and styles are blocked.
// The other directives do not fall back to default-src, so they are set one by one.
const contentSecurityPolicy = "default-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Content-Security-Policy", contentSecurityPolicy)
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("Referrer-Policy", "same-origin")
		headers.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=()")
		next.ServeHTTP(w, r)
	})
}
