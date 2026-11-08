package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danroux/sk8l/internal/ui"
)

const (
	testUINamespace      = "sk8l"
	testJSAssetPath      = "/assets/index-abc123.js"
	testHeaderCT         = "Content-Type"
	testHeaderCC         = "Cache-Control"
	expectedFaviconBytes = 4
)

func newMockUIFS() fstest.MapFS {
	return fstest.MapFS{
		".keep":               {Data: []byte("")},
		"run_app.sh":          {Data: []byte("#!/bin/sh\nnginx -g 'daemon off;'\n")},
		"replace-env-vars.sh": {Data: []byte("#!/bin/sh\nsed -i ...\n")},
		"index.html": {
			Data: []byte("<!DOCTYPE html><html><head><title>sk8l: Cronjob List</title></head><body><div id=\"app\"></div></body></html>"),
		},
		"favicon.ico": {
			Data: []byte{0x00, 0x00, 0x01, 0x00},
		},
		"assets/index-abc123.js": {
			Data: []byte(`const api="VITE_SK8L_API_URL"||"";const ns="VITE_SK8L_K8_NAMESPACE";`),
		},
		"assets/_plugin-vue_export-helper-def456.js": {
			Data: []byte(`export default (sfc, props) => sfc;`),
		},
		"assets/css/modal.vue.css": {
			Data: []byte(`.modal { display: block; }`),
		},
		"assets/fonts/inter.woff2": {
			Data: []byte("wOF2\x00\x01\x00\x00"),
		},
	}
}

func performRequest(t *testing.T, h http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestNewHandlerFromFS_RuntimeConfigInjection(t *testing.T) {
	t.Run("NamespacedDefaultSameOrigin", func(t *testing.T) {
		h, err := ui.NewHandlerFromFS(newMockUIFS(), testUINamespace, "")
		if err != nil {
			t.Fatalf("NewHandlerFromFS returned unexpected error: %v", err)
		}

		rr := performRequest(t, h, http.MethodGet, testJSAssetPath, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		if ct := rr.Header().Get(testHeaderCT); !strings.Contains(ct, "javascript") {
			t.Errorf("expected javascript Content-Type, got %q", ct)
		}
		if cc := rr.Header().Get(testHeaderCC); cc != "public, max-age=3600" {
			t.Errorf("unexpected Cache-Control header: %q", cc)
		}

		body := rr.Body.String()
		want := `const api=""||"";const ns="sk8l";`
		if body != want {
			t.Errorf("unexpected JS bundle content:\ngot:  %s\nwant: %s", body, want)
		}
	})

	t.Run("ClusterWideEmptyNamespaceFallsBackToAllNamespaces", func(t *testing.T) {
		h, err := ui.NewHandlerFromFS(newMockUIFS(), "", "https://api.sk8l.example.com")
		if err != nil {
			t.Fatalf("NewHandlerFromFS returned unexpected error: %v", err)
		}

		rr := performRequest(t, h, http.MethodGet, testJSAssetPath, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}

		body := rr.Body.String()
		want := `const api="https://api.sk8l.example.com"||"";const ns="all-namespaces";`
		if body != want {
			t.Errorf("unexpected JS bundle content:\ngot:  %s\nwant: %s", body, want)
		}
	})
}

func TestNewHandlerFromFS_SPARoutingAndStaticAssets(t *testing.T) {
	h, err := ui.NewHandlerFromFS(newMockUIFS(), testUINamespace, "")
	if err != nil {
		t.Fatalf("NewHandlerFromFS returned unexpected error: %v", err)
	}

	spaRoutes := []string{
		"/",
		"/jobs/sk8l",
		"/jobs/all-namespaces",
		"/cronjob/sk8l/process-videos",
		"/cronjob/sk8l/db.backup.v1",
		"/cronjob/sk8l/db.backup.v1/pods/",
		"/about",
	}

	for _, route := range spaRoutes {
		t.Run("SPARoute_"+route, func(t *testing.T) {
			rr := performRequest(t, h, http.MethodGet, route, nil)
			if rr.Code != http.StatusOK {
				t.Fatalf("GET %s: expected 200 OK, got %d", route, rr.Code)
			}
			if ct := rr.Header().Get(testHeaderCT); ct != "text/html; charset=utf-8" {
				t.Errorf("GET %s: expected text/html; charset=utf-8, got %q", route, ct)
			}
			if cc := rr.Header().Get(testHeaderCC); cc != "no-cache" {
				t.Errorf("GET %s: expected Cache-Control no-cache, got %q", route, cc)
			}
			if !strings.Contains(rr.Body.String(), "<div id=\"app\"></div>") {
				t.Errorf("GET %s: expected index.html body, got %q", route, rr.Body.String())
			}
		})
	}

	t.Run("UnderscorePrefixedViteHelperChunkServed", func(t *testing.T) {
		rr := performRequest(t, h, http.MethodGet, "/assets/_plugin-vue_export-helper-def456.js", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for underscore chunk, got %d", rr.Code)
		}
	})

	t.Run("StylesheetAndBinaryFaviconUnmodified", func(t *testing.T) {
		cssResp := performRequest(t, h, http.MethodGet, "/assets/css/modal.vue.css", nil)
		if cssResp.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for CSS asset, got %d", cssResp.Code)
		}
		if ct := cssResp.Header().Get(testHeaderCT); ct != "text/css; charset=utf-8" {
			t.Errorf("expected text/css; charset=utf-8, got %q", ct)
		}

		icoResp := performRequest(t, h, http.MethodGet, "/favicon.ico", nil)
		if icoResp.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for favicon.ico, got %d", icoResp.Code)
		}
		if ct := icoResp.Header().Get(testHeaderCT); !strings.Contains(ct, "icon") {
			t.Errorf("expected icon Content-Type, got %q", ct)
		}
		if icoResp.Body.Len() != expectedFaviconBytes {
			t.Errorf("expected %d-byte favicon, got %d bytes", expectedFaviconBytes, icoResp.Body.Len())
		}

		fontResp := performRequest(t, h, http.MethodGet, "/assets/fonts/inter.woff2", nil)
		if fontResp.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for woff2 font, got %d", fontResp.Code)
		}
		if ct := fontResp.Header().Get(testHeaderCT); !strings.Contains(ct, "woff2") {
			t.Errorf("expected woff2 Content-Type, got %q", ct)
		}
	})

	t.Run("MissingAssetAndSkippedFilesReturn404", func(t *testing.T) {
		for _, missing := range []string{
			"/assets/nonexistent.js",
			"/robots.txt",
			"/.keep",
			"/run_app.sh",
			"/replace-env-vars.sh",
		} {
			rr := performRequest(t, h, http.MethodGet, missing, nil)
			if rr.Code != http.StatusNotFound {
				t.Errorf("GET %s: expected 404, got %d", missing, rr.Code)
			}
		}
	})

	t.Run("ETagAndHeadSupport", func(t *testing.T) {
		getResp := performRequest(t, h, http.MethodGet, testJSAssetPath, nil)
		etag := getResp.Header().Get("ETag")
		if etag == "" {
			t.Fatal("expected non-empty ETag header")
		}

		notModResp := performRequest(t, h, http.MethodGet, testJSAssetPath, map[string]string{
			"If-None-Match": etag,
		})
		if notModResp.Code != http.StatusNotModified {
			t.Errorf("expected 304 Not Modified, got %d", notModResp.Code)
		}

		headResp := performRequest(t, h, http.MethodHead, "/", nil)
		if headResp.Code != http.StatusOK {
			t.Errorf("expected 200 OK for HEAD /, got %d", headResp.Code)
		}
		if headResp.Body.Len() != 0 {
			t.Errorf("expected empty body on HEAD request, got %d bytes", headResp.Body.Len())
		}
	})

	t.Run("DisallowedMethodReturns405", func(t *testing.T) {
		rr := performRequest(t, h, http.MethodPost, "/", nil)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", rr.Code)
		}
	})
}

func TestNewHandler_EmptyEmbeddedDistGraceful404(t *testing.T) {
	emptyFS := fstest.MapFS{
		".keep": {Data: []byte("")},
	}
	h, err := ui.NewHandlerFromFS(emptyFS, testUINamespace, "")
	if err != nil {
		t.Fatalf("NewHandlerFromFS failed on empty dist: %v", err)
	}
	rr := performRequest(t, h, http.MethodGet, "/", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when index.html is absent, got %d", rr.Code)
	}

	defaultHandler := ui.NewHandler(testUINamespace, "")
	if defaultHandler == nil {
		t.Fatal("expected non-nil handler from ui.NewHandler")
	}
}
