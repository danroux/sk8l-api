// Package ui serves the embedded sk8l-ui single-page application and injects runtime configuration in memory.
package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
)

const (
	// DefaultClusterWideNamespace is injected into VITE_SK8L_K8_NAMESPACE when K8_NAMESPACE is empty
	// so Vue Router's required :namespace route parameter continues to work in cluster-wide mode.
	DefaultClusterWideNamespace = "all-namespaces"

	// PlaceholderAPIURL is the build-time Vite placeholder for the backend base URL.
	PlaceholderAPIURL = "VITE_SK8L_API_URL"

	// PlaceholderNamespace is the build-time Vite placeholder for the monitored Kubernetes namespace.
	PlaceholderNamespace = "VITE_SK8L_K8_NAMESPACE"

	distDir              = "dist"
	indexFile            = "index.html"
	assetsPrefix         = "assets/"
	headerContentType    = "Content-Type"
	headerCacheControl   = "Cache-Control"
	headerETag           = "ETag"
	headerIfNoneMatch    = "If-None-Match"
	headerContentTypeOpt = "X-Content-Type-Options"
	cacheControlNoCache  = "no-cache"
	cacheControlAsset    = "public, max-age=3600"
	logComponent         = "ui"
	etagByteLen          = 8
)

//go:embed all:dist
var embeddedDist embed.FS

type embeddedAsset struct {
	contentType string
	etag        string
	data        []byte
}

type handler struct {
	assets map[string]embeddedAsset
}

// NewHandler builds an HTTP handler serving the compiled sk8l-ui assets embedded in dist/.
func NewHandler(k8sNamespace, apiURL string) http.Handler {
	subFS, err := fs.Sub(embeddedDist, distDir)
	if err != nil {
		log.Error().Err(err).Str("component", logComponent).Msg("failed to open embedded dist directory")
		return http.NotFoundHandler()
	}

	h, err := NewHandlerFromFS(subFS, k8sNamespace, apiURL)
	if err != nil {
		log.Error().Err(err).Str("component", logComponent).Msg("failed to initialize embedded UI handler")
		return http.NotFoundHandler()
	}

	return h
}

// NewHandlerFromFS builds an HTTP handler from fsys, replacing Vite placeholders once in memory.
func NewHandlerFromFS(fsys fs.FS, k8sNamespace, apiURL string) (http.Handler, error) {
	effectiveNamespace := k8sNamespace
	if effectiveNamespace == "" {
		effectiveNamespace = DefaultClusterWideNamespace
	}

	replacer := strings.NewReplacer(
		PlaceholderAPIURL, apiURL,
		PlaceholderNamespace, effectiveNamespace,
	)

	assets := make(map[string]embeddedAsset)
	walkErr := fs.WalkDir(fsys, ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", relPath, err)
		}
		if d.IsDir() || shouldSkipFile(d.Name()) {
			return nil
		}

		raw, readErr := fs.ReadFile(fsys, relPath)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", relPath, readErr)
		}

		if shouldInjectConfig(relPath) {
			raw = []byte(replacer.Replace(string(raw)))
		}

		sum := sha256.Sum256(raw)
		assets[relPath] = embeddedAsset{
			contentType: contentTypeForAsset(relPath, raw),
			etag:        fmt.Sprintf("%q", hex.EncodeToString(sum[:etagByteLen])),
			data:        raw,
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("ui: walk embedded assets: %w", walkErr)
	}

	return &handler{assets: assets}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	cleanPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if cleanPath == "" {
		cleanPath = indexFile
	}

	if asset, ok := h.assets[cleanPath]; ok {
		serveAsset(w, r, asset, cleanPath == indexFile)
		return
	}

	if isStaticAssetPath(cleanPath) {
		http.NotFound(w, r)
		return
	}

	indexAsset, ok := h.assets[indexFile]
	if !ok {
		http.NotFound(w, r)
		return
	}

	serveAsset(w, r, indexAsset, true)
}

func serveAsset(w http.ResponseWriter, r *http.Request, a embeddedAsset, noCache bool) {
	w.Header().Set(headerContentType, a.contentType)
	w.Header().Set(headerETag, a.etag)
	w.Header().Set(headerContentTypeOpt, "nosniff")
	if noCache {
		w.Header().Set(headerCacheControl, cacheControlNoCache)
	} else {
		w.Header().Set(headerCacheControl, cacheControlAsset)
	}

	if match := r.Header.Get(headerIfNoneMatch); match != "" && match == a.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}

	if _, err := w.Write(a.data); err != nil {
		log.Error().Err(err).Str("component", logComponent).Str("operation", "serveAsset").Send()
	}
}

func shouldSkipFile(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".sh")
}

// shouldInjectConfig limits VITE_SK8L_* placeholder replacement to HTML/JS text bundles so binary assets stay untouched.
func shouldInjectConfig(relPath string) bool {
	ext := strings.ToLower(path.Ext(relPath))
	return ext == ".js" || ext == ".mjs" || ext == ".html"
}

// isStaticAssetPath distinguishes missing static files (404) from client-side Vue Router deep links (which may contain dots).
func isStaticAssetPath(cleanPath string) bool {
	base := path.Base(cleanPath)
	return strings.HasPrefix(cleanPath, assetsPrefix) ||
		cleanPath == "favicon.ico" ||
		cleanPath == "robots.txt" ||
		strings.HasPrefix(base, ".") ||
		strings.HasSuffix(base, ".sh")
}

func contentTypeForAsset(relPath string, data []byte) string {
	if ct := mime.TypeByExtension(path.Ext(relPath)); ct != "" {
		return ct
	}
	return http.DetectContentType(data)
}
