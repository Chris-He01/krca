package kim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://webhook.example.com/send"

type Client struct {
	webhookURL string
	httpClient *http.Client
}

// NewClient creates a Kim client. baseURL can be empty to use the default (public) endpoint.
// Configure the webhook endpoint through deployment configuration.
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		webhookURL: baseURL + "?key=" + apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type kimBody struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

// SendMarkdown sends a markdown-formatted message to the Kim robot.
func (c *Client) SendMarkdown(content string) error {
	body := kimBody{MsgType: "markdown"}
	body.Markdown.Content = content

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("kim marshal: %w", err)
	}

	req, err := http.NewRequest("POST", c.webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("kim request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("kim send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kim API returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
