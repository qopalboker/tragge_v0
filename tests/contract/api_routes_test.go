package contract

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Route represents an API route found in source code.
type Route struct {
	Method string
	Path   string
	File   string
	Line   int
}

// TestFrontendBackendRouteAlignment verifies that every frontend API call
// has a matching backend route definition. This prevents URL mismatches
// like /me/contest-history vs /me/history from reaching production.
func TestFrontendBackendRouteAlignment(t *testing.T) {
	repoRoot := findRepoRoot(t)

	frontendRoutes := extractFrontendRoutes(t, repoRoot)
	backendRoutes := extractBackendRoutes(t, repoRoot)

	t.Logf("Found %d frontend API calls", len(frontendRoutes))
	t.Logf("Found %d backend route definitions", len(backendRoutes))

	// Build backend route lookup: "METHOD /normalized/path" → true
	backendSet := make(map[string]bool)
	for _, r := range backendRoutes {
		key := r.Method + " " + normalizePath(r.Path)
		backendSet[key] = true
	}

	// Log all backend routes for debugging
	backendKeys := make([]string, 0, len(backendSet))
	for k := range backendSet {
		backendKeys = append(backendKeys, k)
	}
	sort.Strings(backendKeys)
	t.Logf("Backend routes (normalized):")
	for _, k := range backendKeys {
		t.Logf("  %s", k)
	}

	// Check each frontend route
	var mismatches []string
	seen := make(map[string]bool) // deduplicate
	for _, fr := range frontendRoutes {
		normalized := fr.Method + " " + normalizePath(fr.Path)
		if seen[normalized] {
			continue
		}
		seen[normalized] = true

		if !matchesBackend(fr.Method, normalizePath(fr.Path), backendSet) {
			rel, _ := filepath.Rel(repoRoot, fr.File)
			mismatches = append(mismatches,
				fmt.Sprintf("  %s %s  (from %s:%d)", fr.Method, fr.Path, rel, fr.Line))
		}
	}

	if len(mismatches) > 0 {
		sort.Strings(mismatches)
		t.Errorf("Found %d frontend API calls without matching backend routes:\n%s",
			len(mismatches), strings.Join(mismatches, "\n"))
	}
}

// TestBackendRoutesHaveFrontendCallers is informational — it reports backend
// API routes that no frontend code calls. These could be dead routes or
// routes used only by internal services.
func TestBackendRoutesHaveFrontendCallers(t *testing.T) {
	repoRoot := findRepoRoot(t)

	frontendRoutes := extractFrontendRoutes(t, repoRoot)
	backendRoutes := extractBackendRoutes(t, repoRoot)

	// Build frontend route lookup
	frontendSet := make(map[string]bool)
	for _, fr := range frontendRoutes {
		key := fr.Method + " " + normalizePath(fr.Path)
		frontendSet[key] = true
	}

	var uncalled []string
	for _, br := range backendRoutes {
		normalized := br.Method + " " + normalizePath(br.Path)

		// Skip health/metrics/internal endpoints
		if isInternalRoute(br.Path) {
			continue
		}

		if !frontendSet[normalized] {
			rel, _ := filepath.Rel(repoRoot, br.File)
			uncalled = append(uncalled,
				fmt.Sprintf("  %s %s  (defined in %s:%d)", br.Method, br.Path, rel, br.Line))
		}
	}

	if len(uncalled) > 0 {
		sort.Strings(uncalled)
		t.Logf("INFO: %d backend routes have no matching frontend caller (may be internal-only):\n%s",
			len(uncalled), strings.Join(uncalled, "\n"))
	}
}

// findRepoRoot walks up from the test directory to find the repository root.
func findRepoRoot(t *testing.T) string {
	t.Helper()

	// Start from the directory containing this test file
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find repository root (no go.work found)")
		}
		dir = parent
	}
}

// extractFrontendRoutes scans all frontend TypeScript and Vue files for API calls.
func extractFrontendRoutes(t *testing.T, repoRoot string) []Route {
	t.Helper()
	var routes []Route

	frontendDirs := []string{
		filepath.Join(repoRoot, "apps", "frontend", "src"),
	}

	// Patterns to match API calls:
	// api.get<Type>('/api/...'), api.post(`/api/...`), axios.post('/api/...'), etc.
	apiCallRe := regexp.MustCompile(
		"(?:api|axios)\\.(get|post|put|delete|patch)\\s*(?:<[^>]*>)?\\s*\\(\\s*" +
			"['\"`]([^'\"`]*?/api/[^'\"`]*?)['\"`]")

	// fetch('/api/...')
	fetchCallRe := regexp.MustCompile(
		"fetch\\(\\s*['\"`]([^'\"`]*/api/[^'\"`]*)['\"`]")

	for _, dir := range frontendDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Skip node_modules and dist
				if info.Name() == "node_modules" || info.Name() == "dist" {
					return filepath.SkipDir
				}
				return nil
			}

			ext := filepath.Ext(path)
			if ext != ".ts" && ext != ".vue" && ext != ".tsx" {
				return nil
			}

			fileRoutes := scanFrontendFile(t, path, apiCallRe, fetchCallRe)
			routes = append(routes, fileRoutes...)
			return nil
		})
		if err != nil {
			t.Errorf("Error walking %s: %v", dir, err)
		}
	}

	return routes
}

func scanFrontendFile(t *testing.T, filePath string, apiCallRe, fetchCallRe *regexp.Regexp) []Route {
	t.Helper()
	file, err := os.Open(filePath)
	if err != nil {
		t.Errorf("Failed to open %s: %v", filePath, err)
		return nil
	}
	defer file.Close()

	var routes []Route
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Match api.method(...) and axios.method(...) calls
		for _, match := range apiCallRe.FindAllStringSubmatch(line, -1) {
			method := strings.ToUpper(match[1])
			path := cleanFrontendPath(match[2])
			if path != "" && strings.HasPrefix(path, "/api/") {
				routes = append(routes, Route{
					Method: method,
					Path:   path,
					File:   filePath,
					Line:   lineNum,
				})
			}
		}

		// Match fetch(...) calls (assume GET)
		for _, match := range fetchCallRe.FindAllStringSubmatch(line, -1) {
			path := cleanFrontendPath(match[1])
			if path != "" && strings.HasPrefix(path, "/api/") {
				routes = append(routes, Route{
					Method: "GET",
					Path:   path,
					File:   filePath,
					Line:   lineNum,
				})
			}
		}
	}

	return routes
}

// cleanFrontendPath normalizes a frontend API path by:
// - Replacing template literal expressions ${...} with {_}
// - Stripping query strings
// - Removing trailing slashes
func cleanFrontendPath(path string) string {
	// Replace ${...} template expressions with {_}
	templateRe := regexp.MustCompile(`\$\{[^}]+\}`)
	path = templateRe.ReplaceAllString(path, "{_}")

	// Strip query strings
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	// Remove trailing slash
	path = strings.TrimRight(path, "/")

	return path
}

// extractBackendRoutes scans all BFF main.go files for chi router route registrations.
func extractBackendRoutes(t *testing.T, repoRoot string) []Route {
	t.Helper()
	var routes []Route

	// Map of backend services and their main entry points
	bffFiles := []string{
		filepath.Join(repoRoot, "apps", "user-bff", "main.go"),
		filepath.Join(repoRoot, "apps", "trade-bff", "main.go"),
		filepath.Join(repoRoot, "apps", "admin-bff", "main.go"),
		filepath.Join(repoRoot, "apps", "payment-service", "main.go"),
	}

	for _, f := range bffFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Logf("Skipping %s (not found)", f)
			continue
		}
		fileRoutes := parseChiRoutes(t, f)
		routes = append(routes, fileRoutes...)
	}

	return routes
}

// parseChiRoutes extracts route definitions from a Go file that uses the chi router.
// It handles nested r.Route("/prefix", func(r chi.Router) { ... }) blocks
// by tracking the prefix stack using brace depth counting.
func parseChiRoutes(t *testing.T, filePath string) []Route {
	t.Helper()

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	var routes []Route

	// Prefix stack for nested Route() calls
	var prefixStack []string
	var braceDepthStack []int // brace depth when each prefix was pushed
	braceDepth := 0

	// Regex patterns for chi router
	routeDeclRe := regexp.MustCompile(`\.Route\("([^"]+)"`)
	handlerRe := regexp.MustCompile(`\.(Get|Post|Put|Delete|Patch|Handle)\("([^"]+)"`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comment-only lines
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Check for r.Route("/prefix", ...) before counting braces
		if matches := routeDeclRe.FindStringSubmatch(line); matches != nil {
			prefix := matches[1]
			prefixStack = append(prefixStack, prefix)
			braceDepthStack = append(braceDepthStack, braceDepth)
		}

		// Count braces (simple approach — sufficient for well-formatted Go code)
		for _, ch := range line {
			switch ch {
			case '{':
				braceDepth++
			case '}':
				braceDepth--
			}
		}

		// Pop prefixes that have gone out of scope
		for len(braceDepthStack) > 0 && braceDepth <= braceDepthStack[len(braceDepthStack)-1] {
			prefixStack = prefixStack[:len(prefixStack)-1]
			braceDepthStack = braceDepthStack[:len(braceDepthStack)-1]
		}

		// Check for handler registrations: r.Get("/path", ...) or r.With(...).Get("/path", ...)
		if matches := handlerRe.FindStringSubmatch(line); matches != nil {
			method := strings.ToUpper(matches[1])
			path := matches[2]

			// Handle is used for http.Handler (typically metrics), treat as GET
			if method == "HANDLE" {
				method = "GET"
			}

			// Build full path from prefix stack
			fullPath := strings.Join(prefixStack, "") + path

			// Only track /api/ routes (skip healthz, readyz, metrics, etc.)
			if strings.HasPrefix(fullPath, "/api/") {
				routes = append(routes, Route{
					Method: method,
					Path:   fullPath,
					File:   filePath,
					Line:   i + 1,
				})
			}
		}
	}

	return routes
}

// normalizePath converts a route path to a canonical form for comparison.
// All path parameters ({id}, {user_id}, {_}, etc.) become {_}.
// Trailing slashes are stripped since Chi treats /path and /path/ equivalently.
func normalizePath(path string) string {
	paramRe := regexp.MustCompile(`\{[^}]+\}`)
	path = paramRe.ReplaceAllString(path, "{_}")
	// Strip trailing slash for consistent matching (Chi redirects between them)
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	return path
}

// matchesBackend checks if a frontend route matches any backend route.
// It handles the case where the same path might be registered with different
// parameter names (e.g., {id} vs {user_id}).
func matchesBackend(method, normalizedPath string, backendSet map[string]bool) bool {
	key := method + " " + normalizedPath
	return backendSet[key]
}

// isInternalRoute returns true for routes that are infrastructure/internal
// endpoints not expected to be called by frontends.
func isInternalRoute(path string) bool {
	internal := []string{
		"/healthz", "/readyz", "/metrics", "/health/",
		"/ws-stats", "/ws/trade", "/shards", "/rate-limits",
		"/callback/", "/webhooks/",
	}
	for _, prefix := range internal {
		if strings.Contains(path, prefix) {
			return true
		}
	}
	return false
}
