// Package management talks to a server's local management API.
//
// The API only listens when the server was started with a token, and the
// generated Compose file publishes it to 127.0.0.1 only, so this reaches a
// server running on this machine and nothing else can reach it at all.
package management

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// Settings are the values that live in the server's own database rather than
// its environment, so changing one needs the server to be running.
type Settings struct {
	DisplayName          string `json:"displayName"`
	Description          string `json:"description"`
	JoinPolicy           string `json:"joinPolicy"`
	Discoverable         bool   `json:"discoverable"`
	LANOpen              bool   `json:"lanOpen"`
	ProfanityMode        string `json:"profanityMode"`
	ProfanityCensorStyle string `json:"profanityCensorStyle"`
	UploadMaxBytes       int64  `json:"uploadMaxBytes"`
	AvatarMaxBytes       int64  `json:"avatarMaxBytes"`
	EmojiMaxBytes        int64  `json:"emojiMaxBytes"`
}

// ErrUnsupported means the server answered and has no management API. Older
// images predate it, so this is an ordinary thing to meet rather than a fault.
var ErrUnsupported = errors.New("this server was built before the management API")

// ErrUnreachable means nothing answered, which for a stopped server is simply
// what it looks like.
var ErrUnreachable = errors.New("the server is not answering on its management port")

type Client struct {
	Port  int
	Token string
	HTTP  *http.Client
}

func (c Client) url(path string) string {
	return "http://127.0.0.1:" + strconv.Itoa(c.Port) + "/management" + path
}

func (c Client) do(ctx context.Context, method, path string, body any) (*Settings, error) {
	if c.Port == 0 || c.Token == "" {
		return nil, ErrUnsupported
	}

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = encoded
	}

	req, err := http.NewRequestWithContext(ctx, method, c.url(path), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, ErrUnreachable
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusNotFound:
		// Up, answering, and has no such route: an image from before the
		// management API existed.
		return nil, ErrUnsupported
	case res.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("the server rejected the management token; restart it so it picks up the current one")
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return nil, fmt.Errorf("the server answered %s", res.Status)
	}

	var settings Settings
	if err := json.NewDecoder(res.Body).Decode(&settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (c Client) Get(ctx context.Context) (*Settings, error) {
	return c.do(ctx, http.MethodGet, "/settings", nil)
}

// Patch sends only the keys given, so changing one setting cannot reset
// another to whatever this side happened to be holding.
func (c Client) Patch(ctx context.Context, patch map[string]any) (*Settings, error) {
	return c.do(ctx, http.MethodPatch, "/settings", patch)
}
