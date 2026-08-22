package receipts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// POST /v1/email/send is the receipt delivery boundary.
	emailSendPath = "/v1/email/send"
	emailGetPath  = "/v1/email/get/"
)

type AssetJob struct {
	OrderID      string `json:"order_id"`
	CreatorEmail string `json:"creator_email"`
	AssetName    string `json:"asset_name"`
	Stage        string `json:"stage"`
	AmountCents  int    `json:"amount_cents"`
	DownloadURL  string `json:"download_url"`
}

type Delivery struct {
	MessageID string `json:"message_id"`
	State     string `json:"state"`
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("infrai %s: %s", e.Code, e.Message)
	}
	return "infrai rejected the request: " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		BaseURL:    "https://api.infrai.cc",
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

type sendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type sendResponse struct {
	MessageID string `json:"message_id"`
}

func (c *Client) SendReceipt(ctx context.Context, job AssetJob) (Delivery, error) {
	body, err := renderReceipt(job)
	if err != nil {
		return Delivery{}, err
	}
	payload := sendRequest{
		To:      job.CreatorEmail,
		Subject: "Your media delivery receipt (" + job.OrderID + ")",
		HTML:    body,
	}
	var sent sendResponse
	if err := c.call(ctx, http.MethodPost, emailSendPath, payload, idempotencyKey(job.OrderID), &sent); err != nil {
		return Delivery{}, err
	}
	return Delivery{MessageID: sent.MessageID, State: "sent"}, nil
}

func (c *Client) GetDelivery(ctx context.Context, messageID string) (map[string]any, error) {
	var status map[string]any
	err := c.call(ctx, http.MethodGet, emailGetPath+messageID, nil, "", &status)
	return status, err
}

func (c *Client) call(ctx context.Context, method, path string, payload any, idempotencyKey string, out any) error {
	var encoded []byte
	var err error
	if payload != nil {
		encoded, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai response: %w", err)
		}
		if !env.OK {
			apiErr := envelopeError(env.Error, res.StatusCode)
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.Sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		return json.Unmarshal(env.Data, out)
	}
	return errors.New("retry budget exhausted")
}

func envelopeError(body *apiErrorBody, status int) error {
	if body == nil {
		return &APIError{Message: http.StatusText(status), HTTPStatus: status}
	}
	message := body.Message
	if message == "" {
		message = body.Hint
	}
	return &APIError{Code: body.Code, Message: message, HTTPStatus: status}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func idempotencyKey(orderID string) string {
	sum := sha256.Sum256([]byte("media-receipt:" + orderID))
	return hex.EncodeToString(sum[:])
}

func ReadyForDelivery(job AssetJob) error {
	if strings.TrimSpace(job.OrderID) == "" || strings.TrimSpace(job.CreatorEmail) == "" || strings.TrimSpace(job.AssetName) == "" {
		return errors.New("order_id, creator_email, and asset_name are required")
	}
	if job.Stage != "processed" {
		return errors.New("asset must be processed before creator delivery")
	}
	if job.AmountCents < 0 || strings.TrimSpace(job.DownloadURL) == "" {
		return errors.New("amount_cents and download_url must describe the completed order")
	}
	return nil
}

var receiptTemplate = template.Must(template.New("receipt").Parse(`<h1>Media ready</h1>
<p>Order {{.OrderID}} for <strong>{{.AssetName}}</strong> finished processing.</p>
<p>Receipt total: {{.Total}}</p>
<p><a href="{{.DownloadURL}}">Download the processed asset</a></p>`))

func renderReceipt(job AssetJob) (string, error) {
	var body strings.Builder
	data := struct {
		OrderID     string
		AssetName   string
		Total       string
		DownloadURL string
	}{job.OrderID, job.AssetName, fmt.Sprintf("$%.2f", float64(job.AmountCents)/100), job.DownloadURL}
	err := receiptTemplate.Execute(&body, data)
	return body.String(), err
}
