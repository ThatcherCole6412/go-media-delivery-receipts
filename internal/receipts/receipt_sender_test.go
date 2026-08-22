package receipts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadyForDelivery(t *testing.T) {
	base := AssetJob{OrderID: "ord-42", CreatorEmail: "creator@example.com", AssetName: "trailer.mp4", Stage: "processed", AmountCents: 2400, DownloadURL: "https://media.example/download/42"}
	tests := []struct {
		name    string
		mutate  func(*AssetJob)
		wantErr string
	}{
		{name: "processed asset is deliverable", mutate: func(*AssetJob) {}},
		{name: "queued asset stays internal", mutate: func(j *AssetJob) { j.Stage = "queued" }, wantErr: "must be processed"},
		{name: "missing delivery link is rejected", mutate: func(j *AssetJob) { j.DownloadURL = "" }, wantErr: "completed order"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := base
			tt.mutate(&job)
			err := ReadyForDelivery(job)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ReadyForDelivery() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ReadyForDelivery() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestSendReceiptRequest(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != emailSendPath {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") == "" {
			t.Fatal("missing auth or idempotency header")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"message_id":"msg-42"},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.BaseURL = server.URL
	job := AssetJob{OrderID: "ord-42", CreatorEmail: "creator@example.com", AssetName: "trailer.mp4", Stage: "processed", AmountCents: 2400, DownloadURL: "https://media.example/download/42"}
	delivery, err := client.SendReceipt(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.MessageID != "msg-42" || gotBody["to"] != job.CreatorEmail {
		t.Fatalf("delivery = %#v, body = %#v", delivery, gotBody)
	}
	if _, exists := gotBody["from"]; exists {
		t.Fatal("request should use the account default sender")
	}
}
