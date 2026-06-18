package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const skinImageFetchTimeout = 15 * time.Second

func resolveSkinImageSource(enabled bool, skinName, apiTemplate string) (string, error) {
	if !enabled {
		return "", nil
	}
	template := strings.TrimSpace(apiTemplate)
	if template == "" {
		return "", nil
	}
	name := strings.TrimSpace(skinName)
	if name == "" {
		name = "default"
	}
	resolved := strings.ReplaceAll(template, "{skin}", url.QueryEscape(name))
	parsed, err := url.Parse(resolved)
	if err != nil {
		return "", fmt.Errorf("parse skin image url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported skin image scheme: %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("missing skin image host")
	}
	return parsed.String(), nil
}

func (s *Server) handleSkinImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	s.cfgMu.RLock()
	sourceURL, err := resolveSkinImageSource(s.cfg.Panel.SkinEnabled, s.cfg.Panel.SkinName, s.cfg.Panel.SkinAPITemplate)
	s.cfgMu.RUnlock()
	if err != nil || sourceURL == "" {
		http.NotFound(w, r)
		return
	}

	resp, err := fetchSkinImage(sourceURL)
	if err != nil {
		http.Error(w, "皮肤图片暂时不可用。", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	sniff, _ := reader.Peek(512)
	contentType := normalizedImageContentType(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = normalizedImageContentType(http.DetectContentType(sniff))
	}
	if contentType == "" {
		http.Error(w, "皮肤图片暂时不可用。", http.StatusBadGateway)
		return
	}

	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", contentType)
	_, _ = io.Copy(w, reader)
}

func fetchSkinImage(sourceURL string) (*http.Response, error) {
	client := &http.Client{
		Timeout: skinImageFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AriaMX/skin-fetch")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected skin image status: %d", resp.StatusCode)
	}
	return resp, nil
}

func normalizedImageContentType(value string) string {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	if contentType == "" {
		return ""
	}
	if strings.HasPrefix(contentType, "image/") {
		return contentType
	}
	return ""
}
