// Package twitch reads a channel's profile picture from the Twitch Helix
// API. It needs TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET (an app
// registered at dev.twitch.tv); without both there is no client.
package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	// ErrUnknownChannel is returned when Twitch has no channel with that login.
	ErrUnknownChannel = errors.New("twitch: unknown channel")
	// ErrBadImage is returned for a picture that comes from the wrong host,
	// is too large or isn't a JPEG, PNG or WebP image.
	ErrBadImage = errors.New("twitch: the profile picture isn't an acceptable image")
)

// MaxImageSize caps a downloaded profile picture.
const MaxImageSize = 5 << 20

// imageHost is the only host profile pictures are fetched from.
const imageHost = "static-cdn.jtvnw.net"

// tokenMargin is how long before a token expires it is replaced.
const tokenMargin = time.Minute

// Client is safe for concurrent use. Base URLs and the image host are
// fields so tests can point them at an httptest server.
type Client struct {
	HTTP         *http.Client
	ClientID     string
	ClientSecret string

	AuthURL     string // OAuth host, https://id.twitch.tv
	APIURL      string // Helix, https://api.twitch.tv/helix
	ImageHost   string // the one host pictures may come from
	ImageScheme string // "https"

	// Now tells the time; tests replace it to expire tokens.
	Now func() time.Time

	mu      sync.Mutex
	token   string
	expires time.Time
}

// New returns a client from TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET, or
// nil if either is missing.
func New() *Client {
	id, secret := os.Getenv("TWITCH_CLIENT_ID"), os.Getenv("TWITCH_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil
	}
	return NewWith(id, secret)
}

// NewWith returns a client for the given app credentials.
func NewWith(id, secret string) *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 10 * time.Second,
			// pictures must not be bounced to another host
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		ClientID:     id,
		ClientSecret: secret,
		AuthURL:      "https://id.twitch.tv",
		APIURL:       "https://api.twitch.tv/helix",
		ImageHost:    imageHost,
		ImageScheme:  "https",
		Now:          time.Now,
	}
}

var (
	defaultOnce   sync.Once
	defaultClient *Client
)

// Default is the process-wide client, nil when Twitch isn't configured.
func Default() *Client {
	defaultOnce.Do(func() { defaultClient = New() })
	return defaultClient
}

// User is a Twitch channel's owner.
type User struct {
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	ProfileImageURL string `json:"profile_image_url"`
}

// User looks up a channel by its lowercase login. ErrUnknownChannel means
// there is none.
func (c *Client) User(ctx context.Context, login string) (*User, error) {
	var res struct {
		Data []User `json:"data"`
	}
	for attempt := 0; ; attempt++ {
		token, err := c.accessToken(ctx)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL+"/users?login="+url.QueryEscape(login), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Client-Id", c.ClientID)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			// revoked or expired early: get a new token and ask once more
			resp.Body.Close()
			c.dropToken(token)
			continue
		}
		err = decode(resp, &res)
		if err != nil {
			return nil, err
		}
		break
	}
	if len(res.Data) == 0 {
		return nil, ErrUnknownChannel
	}
	return &res.Data[0], nil
}

// decode reads a JSON answer and closes it; any status but 200 is an error.
func decode(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("twitch: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// accessToken returns the cached app token, fetching one when there is none
// or it is about to expire.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.Now().Before(c.expires.Add(-tokenMargin)) {
		return c.token, nil
	}
	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.AuthURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"` // seconds
	}
	if err := decode(resp, &res); err != nil {
		return "", err
	}
	if res.AccessToken == "" {
		return "", errors.New("twitch: no access token in the answer")
	}
	c.token = res.AccessToken
	c.expires = c.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
	return c.token, nil
}

// dropToken forgets token if it is still the cached one.
func (c *Client) dropToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == token {
		c.token = ""
	}
}

// Image downloads a profile picture. imageURL must be the URL Twitch
// returned and on the image host; the answer is capped at MaxImageSize and
// must be a JPEG, PNG or WebP image.
func (c *Client) Image(ctx context.Context, imageURL string) ([]byte, error) {
	u, err := url.Parse(imageURL)
	if err != nil || u.Scheme != c.ImageScheme || u.Host != c.ImageHost {
		return nil, ErrBadImage
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: picture: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxImageSize {
		return nil, ErrBadImage
	}
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/webp":
		return data, nil
	}
	return nil, ErrBadImage
}
