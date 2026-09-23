// Package remote loads fonts over HTTP without embedding font data.
package remote

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/ironpark/ggui"
)

// Load downloads and parses a TTF or OTF font. Call it from a worker, or use
// it as a ggui.Resource loader. The caller applies the font on the UI thread.
// Browser URLs may be relative to the page; cross-origin URLs require CORS.
func Load(ctx context.Context, url string) (*ggui.Font, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("remote font: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote font: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote font: HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("remote font: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ggui.LoadFont(data)
}
