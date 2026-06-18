package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveSkinImageSource(t *testing.T) {
	source, err := resolveSkinImageSource(true, "anime sky", "https://example.com/api?name={skin}")
	if err != nil {
		t.Fatalf("resolveSkinImageSource returned error: %v", err)
	}
	if source != "https://example.com/api?name=anime+sky" {
		t.Fatalf("unexpected source url: %q", source)
	}
}

func TestResolveSkinImageSourceRejectsUnsupportedScheme(t *testing.T) {
	if _, err := resolveSkinImageSource(true, "default", "javascript:alert(1)"); err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
}

func TestFetchSkinImageFollowsRedirect(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte("RIFFxxxxWEBPVP8 "))
	}))
	defer imageServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, imageServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	resp, err := fetchSkinImage(redirectServer.URL)
	if err != nil {
		t.Fatalf("fetchSkinImage returned error: %v", err)
	}
	defer resp.Body.Close()

	if got := normalizedImageContentType(resp.Header.Get("Content-Type")); got != "image/webp" {
		t.Fatalf("unexpected content type: %q", got)
	}
}

func TestNormalizedImageContentType(t *testing.T) {
	if got := normalizedImageContentType("image/webp; charset=utf-8"); got != "image/webp" {
		t.Fatalf("unexpected normalized content type: %q", got)
	}
	if got := normalizedImageContentType("text/html"); got != "" {
		t.Fatalf("expected non-image content type to be rejected, got %q", got)
	}
}

func TestHandleSkinImageRejectsHTMLResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not image</html>"))
	}))
	defer upstream.Close()

	server := &Server{
		cfg: &Config{
			Panel: PanelConfig{
				SkinEnabled:     true,
				SkinName:        "default",
				SkinAPITemplate: upstream.URL,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/skin-image", nil)
	rec := httptest.NewRecorder()
	server.handleSkinImage(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "皮肤图片暂时不可用") {
		t.Fatalf("unexpected response body: %q", rec.Body.String())
	}
}
