package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestLoad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/font.ttf":
			w.Write(goregular.TTF)
		case "/invalid.ttf":
			w.Write([]byte("not a font"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if font, err := Load(context.Background(), server.URL+"/font.ttf"); err != nil || font == nil {
		t.Fatalf("load valid font: %v", err)
	}
	for _, path := range []string{"/invalid.ttf", "/missing.ttf"} {
		if _, err := Load(context.Background(), server.URL+path); err == nil {
			t.Errorf("load %s succeeded", path)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, server.URL+"/font.ttf"); err == nil {
		t.Error("cancelled load succeeded")
	}
}
