package web

import (
	"io/fs"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedInterfaceAndSourceBoundary(t *testing.T) {
	handler := Handler()
	for _, asset := range []string{"/", "/app.js", "/styles.css"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", asset, nil))
		if recorder.Code != 200 || recorder.Body.Len() == 0 {
			t.Fatalf("asset %s: %d", asset, recorder.Code)
		}
		if !strings.Contains(recorder.Header().Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatal("scripts must be restricted to local assets")
		}
	}
	for _, private := range []string{"/embed.go", "/../go.mod", "/data/state.json", "/lib/", "/views/", "/package.json", "/tests/forms.test.mjs"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", private, nil))
		if recorder.Code != 404 {
			t.Fatalf("unexpected source access %s: %d", private, recorder.Code)
		}
	}
}

func TestNativeModuleImportsAreEmbeddedAndServed(t *testing.T) {
	// Project modules use explicit relative import/export-from declarations.
	imports := regexp.MustCompile(`(?:from\s*|import\s*)['"]([^'"]+)['"]`)
	handler := Handler()
	err := fs.WalkDir(assets, ".", func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(filename, ".js") {
			return nil
		}
		source, err := assets.ReadFile(filename)
		if err != nil {
			return err
		}
		for _, declaration := range imports.FindAllSubmatch(source, -1) {
			imported := string(declaration[1])
			if !strings.HasPrefix(imported, ".") {
				t.Fatalf("%s has a nonlocal import %s", filename, imported)
			}
			resolved := path.Clean(path.Join(path.Dir(filename), imported))
			if _, err := assets.ReadFile(resolved); err != nil {
				t.Fatalf("%s imports missing asset %s: %v", filename, resolved, err)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/"+resolved, nil))
			if recorder.Code != 200 || !strings.Contains(recorder.Header().Get("Content-Type"), "javascript") {
				t.Fatalf("module %s: status %d, MIME %s", resolved, recorder.Code, recorder.Header().Get("Content-Type"))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
