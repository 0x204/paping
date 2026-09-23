// Package ipinfo looks up who owns an IP address using the ipinfo.io API.
package ipinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
)

const defaultBaseURL = "https://ipinfo.io"

// Client is an ipinfo.io API client. The zero value is ready to use.
type Client struct {
	// HTTPClient makes the requests. If nil, http.DefaultClient is used.
	HTTPClient *http.Client

	// BaseURL is the API endpoint. If empty, https://ipinfo.io is used.
	BaseURL string
}

// Org returns the organization that owns addr, such as "AS15169 Google LLC",
// or "" if ipinfo.io has none on record. Addresses that are not publicly
// routable, such as loopback or private ones, are never sent to ipinfo.io.
func (c *Client) Org(ctx context.Context, addr netip.Addr) (string, error) {
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return "", nil
	}

	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	url := baseURL + "/" + addr.WithZone("").String() + "/json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ipinfo.io returned %s", resp.Status)
	}
	var info struct {
		Org string `json:"org"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("decoding ipinfo.io response: %w", err)
	}
	return info.Org, nil
}
