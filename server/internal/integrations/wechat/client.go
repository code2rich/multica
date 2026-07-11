package wechat

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// defaultWechatBaseURL is the default iLink API host. The QR-login status poll
// returns a per-account base_url which may differ from this (the iLink backend
// shards accounts across hosts); that per-account value is persisted in the
// installation config and used for getupdates/sendmessage. This constant only
// seeds the QR-login flow (the first step, before a base_url is known) and can
// be overridden via the MULTICA_WECHAT_BASE_URL env var for proxy/mock/staging.
const defaultWechatBaseURL = "https://ilinkai.weixin.qq.com"

// requestTimeout bounds a normal (non-long-poll) iLink API call: qrcode,
// qrcode/status, sendmessage. getupdates is a long poll (server holds ~35s) and
// uses its own longer timeout.
const (
	requestTimeout     = 15 * time.Second
	longPollTimeout    = 45 * time.Second // getupdates server hold (~35s) + margin
)

// iLinkClient talks to the WeChat ClawBot (iLink) HTTP API. It is stateless
// apart from the injected base URL and HTTP client; per-call credentials
// (bot_token, base_url) are passed in by the caller. All HTTP requests use the
// caller's context so a cancelled context (lease loss, shutdown) tears down an
// in-flight call in bounded time — required by the supervisor's reconnect
// contract.
//
// PROTOCOL CAVEAT: the iLink API is not fully documented publicly; the request
// shapes below are derived from the official ClawBot doc
// (developers.weixin.qq.com/doc/aispeech/knowledge/openapi/Clawbotrelated.html)
// and community reverse-engineering. Field names / paths may need calibration
// against live traffic (Phase 6); they are concentrated here so a fix touches
// one file.
type iLinkClient struct {
	baseURL  string       // default host for the QR-login flow (before per-account base_url is known)
	httpClient *http.Client // shared short-call client; long polls build their own
	logger   *slog.Logger
}

// newILinkClient builds an iLink HTTP client. A non-empty baseURL overrides the
// default iLink host (for proxy/mock/staging via MULTICA_WECHAT_BASE_URL); an
// empty baseURL falls back to defaultWechatBaseURL.
func newILinkClient(baseURL string, logger *slog.Logger) *iLinkClient {
	if baseURL == "" {
		baseURL = defaultWechatBaseURL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &iLinkClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: requestTimeout},
		logger:     logger,
	}
}

// QRLoginResponse is the result of a successful QR-login status poll: the
// per-account credentials the bot will run on.
type QRLoginResponse struct {
	BotToken    string // bearer token for getupdates / sendmessage
	BaseURL     string // per-account API host for getupdates / sendmessage
	IlinkBotID  string // bot identity, e.g. "xxxxxx@im.bot" (stored as app_id)
	IlinkUserID string // human-readable id of the account that scanned
}

// getQRCode starts a QR-login session. It returns the QR code URL the user
// scans and a session token to poll status with. bot_type=3 selects the
// ClawBot/iLink bot kind.
func (c *iLinkClient) getQRCode(ctx context.Context) (qrCodeURL, sessionToken string, err error) {
	body := map[string]any{"bot_type": 3}
	var resp struct {
		QrcodeURL    string `json:"qrcode_url"`
		SessionToken string `json:"session_token"`
		Code         int    `json:"code"`
		Message      string `json:"message"`
	}
	if err := c.postJSON(ctx, c.baseURL, "/api/v1/wechat/qrcode", body, &resp); err != nil {
		return "", "", err
	}
	if resp.QrcodeURL == "" || resp.SessionToken == "" {
		return "", "", fmt.Errorf("wechat: qrcode response missing fields (code=%d msg=%q)", resp.Code, resp.Message)
	}
	return resp.QrcodeURL, resp.SessionToken, nil
}

// pollQRStatus checks a QR-login session. status is one of "pending",
// "scanned", "confirmed", "expired", "error". On "confirmed" the QRLoginResponse
// is populated; on other statuses it is zero-valued.
func (c *iLinkClient) pollQRStatus(ctx context.Context, sessionToken string) (status string, login QRLoginResponse, err error) {
	body := map[string]any{"session_token": sessionToken}
	var resp struct {
		Status       string `json:"status"`
		BotToken     string `json:"bot_token"`
		BaseURL      string `json:"baseurl"`
		IlinkBotID   string `json:"ilink_bot_id"`
		IlinkUserID  string `json:"ilink_user_id"`
		Code         int    `json:"code"`
		Message      string `json:"message"`
	}
	if err := c.postJSON(ctx, c.baseURL, "/api/v1/wechat/qrcode/status", body, &resp); err != nil {
		return "", QRLoginResponse{}, err
	}
	login = QRLoginResponse{
		BotToken:    resp.BotToken,
		BaseURL:     resp.BaseURL,
		IlinkBotID:  resp.IlinkBotID,
		IlinkUserID: resp.IlinkUserID,
	}
	return resp.Status, login, nil
}

// resetChannel asks the iLink backend to reset the bot's IM channel (used when
// the bot token is suspected stale / the account needs re-linking). It returns
// nil on a successful reset; callers surface the need to re-scan in the UI.
func (c *iLinkClient) resetChannel(ctx context.Context, botToken, baseURL string) error {
	body := map[string]any{}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	return c.postJSONAuthed(ctx, baseURL, "/api/v1/wechat/channel_reset", body, botToken, &resp)
}

// iLinkMessage is one inbound message as delivered by getupdates.
type iLinkMessage struct {
	MsgID        string `json:"msg_id"`
	FromUserID   string `json:"from_user_id"`   // e.g. "xxx@im.wechat"
	ToUserID     string `json:"to_user_id"`     // the bot id, e.g. "xxx@im.bot"
	GroupID      string `json:"group_id,omitempty"`
	MsgType      string `json:"msg_type"`       // "text", "image", ...
	Content      string `json:"content"`        // text body for text messages
	ContextToken string `json:"context_token"`  // MUST be echoed back on sendmessage
	CreateTime   int64  `json:"create_time,omitempty"`
}

// getUpdatesResult is the outcome of one getupdates long poll.
type getUpdatesResult struct {
	Messages       []iLinkMessage
	NextCursor     string // get_updates_buf to use on the next call
}

// getUpdates long-polls the iLink backend for new messages. The server holds the
// connection open for ~35s when there are no messages; the caller's context
// MUST be honoured so lease loss / shutdown can interrupt the poll. cursor is the
// opaque get_updates_buf from the prior call (empty for the first call).
func (c *iLinkClient) getUpdates(ctx context.Context, botToken, baseURL, cursor string) (getUpdatesResult, error) {
	body := map[string]any{}
	if cursor != "" {
		body["get_updates_buf"] = cursor
	}
	// Long poll: use an isolated client with a longer timeout but still bound by
	// the caller's ctx (the per-request deadline is the earlier of ctx and this
	// timeout).
	lpClient := &http.Client{Timeout: longPollTimeout}
	var resp struct {
		Messages     []iLinkMessage `json:"messages"`
		GetUpdatesBuf string        `json:"get_updates_buf"`
		Code         int            `json:"code"`
		Message      string         `json:"message"`
	}
	if err := c.postJSONAuthedWithClient(ctx, lpClient, baseURL, "/ilink/bot/getupdates", body, botToken, &resp); err != nil {
		return getUpdatesResult{}, err
	}
	return getUpdatesResult{Messages: resp.Messages, NextCursor: resp.GetUpdatesBuf}, nil
}

// sendMessage posts a text reply. contextToken MUST be the context_token of the
// inbound message being replied to, or the reply is not associated with the
// conversation (the core iLink quirk). toUserID is the destination WeChat user
// id ("xxx@im.wechat"). Returns the platform message id of the delivered reply.
func (c *iLinkClient) sendMessage(ctx context.Context, botToken, baseURL, contextToken, toUserID, text string) (string, error) {
	if contextToken == "" {
		return "", errors.New("wechat: sendmessage requires a non-empty context_token")
	}
	body := map[string]any{
		"context_token": contextToken,
		"to_user_id":    toUserID,
		"msg_type":      "text",
		"content":       text,
	}
	var resp struct {
		MessageID string `json:"msg_id"`
		Code      int    `json:"code"`
		Message   string `json:"message"`
	}
	if err := c.postJSONAuthed(ctx, baseURL, "/ilink/bot/sendmessage", body, botToken, &resp); err != nil {
		return "", err
	}
	return resp.MessageID, nil
}

// postJSON is an unauthenticated POST (used by the QR-login flow, which runs
// before a bot_token exists).
func (c *iLinkClient) postJSON(ctx context.Context, baseURL, path string, body any, out any) error {
	return c.postJSONAuthedWithClient(ctx, c.httpClient, baseURL, path, body, "", out)
}

// postJSONAuthed is a POST authenticated with a bot_token bearer header plus
// the iLink-specific headers (AuthorizationType, X-WECHAT-UIN).
func (c *iLinkClient) postJSONAuthed(ctx context.Context, baseURL, path string, body any, botToken string, out any) error {
	return c.postJSONAuthedWithClient(ctx, c.httpClient, baseURL, path, body, botToken, out)
}

// postJSONAuthedWithClient is the shared POST worker. It marshals body to JSON,
// attaches the iLink auth headers when botToken is non-empty, and unmarshals the
// JSON response into out. baseURL may differ per call (iLink shards accounts
// across hosts); path selects the endpoint.
func (c *iLinkClient) postJSONAuthedWithClient(ctx context.Context, httpClient *http.Client, baseURL, path string, body any, botToken string, out any) error {
	if baseURL == "" {
		return errors.New("wechat: empty base url")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("wechat: marshal request: %w", err)
	}
	url := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("wechat: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if botToken != "" {
		req.Header.Set("Authorization", "Bearer "+botToken)
		// iLink requires this exact type discriminator alongside the bearer token.
		req.Header.Set("AuthorizationType", "ilink_bot_token")
		// X-WECHAT-UIN is a per-request random uint32 base64-encoded; it is a
		// replay-guard nonce (derived from reverse-engineering).
		req.Header.Set("X-WECHAT-UIN", randomUIN())
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Distinguish ctx cancellation (graceful) from real transport errors so
		// the Connect loop can decide whether to return nil or reconnect.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("wechat: %s: %w", path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MiB cap; messages are text
	if err != nil {
		return fmt.Errorf("wechat: %s: read body: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("wechat: %s: HTTP %d: %s", path, resp.StatusCode, truncate(string(respBody), 300))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("wechat: %s: decode response: %w", path, err)
		}
	}
	return nil
}

// randomUIN returns a random 32-bit unsigned integer base64-encoded, the value
// the iLink backend expects in the X-WECHAT-UIN header. rand failure is treated
// as non-fatal (a zero value), since the header is a nonce, not a credential.
func randomUIN() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	v := binary.BigEndian.Uint32(b[:])
	var enc [8]byte
	binary.BigEndian.PutUint32(enc[:4], v)
	return base64.StdEncoding.EncodeToString(enc[:4])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
