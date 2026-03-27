package connection

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type PartnerClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPartnerClient(baseURL string) *PartnerClient {
	return &PartnerClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type VariantDTO struct {
	Description string `json:"description"`
}

type CreateDisputeRequest struct {
	Description string       `json:"description"`
	MinBet      int64        `json:"min_bet,omitempty"`
	MaxBet      int64        `json:"max_bet,omitempty"`
	StopDate    time.Time    `json:"stop_date"`
	FinishDate  time.Time    `json:"finish_date"`
	Variants    []VariantDTO `json:"variants"`
}

type CreateDisputeResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// sign computes HMAC-SHA256 hex signature for a payload using the partner secret.
func sign(payload, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// setHMACHeaders sets X-Partner-ID, X-Partner-Timestamp, and X-Partner-Signature headers.
func setHMACHeaders(req *http.Request, partnerID, signature string, ts int64) {
	req.Header.Set("X-Partner-ID", partnerID)
	req.Header.Set("X-Partner-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Partner-Signature", signature)
}

// StopBets stops betting on a dispute via the partner backend API.
// Signature payload: "{partner_id}:{dispute_id}:{timestamp}"
func (c *PartnerClient) StopBets(ctx context.Context, partnerID, secret, disputeID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/api/v1/partner/disputes/"+disputeID+"/stopbet", nil)
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	payload := fmt.Sprintf("%s:%s:%d", partnerID, disputeID, ts)
	setHMACHeaders(httpReq, partnerID, sign(payload, secret), ts)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to call partner service stopbet: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("partner service stopbet returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// SetGameInProgress transitions a dispute to game_in_progress status via the partner backend API.
// Signature payload: "{partner_id}:{dispute_id}:{timestamp}"
func (c *PartnerClient) SetGameInProgress(ctx context.Context, partnerID, secret, disputeID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/api/v1/partner/disputes/"+disputeID+"/startgame", nil)
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	payload := fmt.Sprintf("%s:%s:%d", partnerID, disputeID, ts)
	setHMACHeaders(httpReq, partnerID, sign(payload, secret), ts)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to call partner service startgame: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("partner service startgame returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// SetDisputeWinner sets the winner for a dispute via the partner backend API.
// Signature payload: "{partner_id}:{dispute_id}:{winner_team_name}:{timestamp}"
func (c *PartnerClient) SetDisputeWinner(ctx context.Context, partnerID, secret, disputeID, winnerTeamName string) error {
	body, err := json.Marshal(map[string]string{"winner_team_name": winnerTeamName})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/partner/disputes/"+disputeID+"/winner", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	ts := time.Now().Unix()
	payload := fmt.Sprintf("%s:%s:%s:%d", partnerID, disputeID, winnerTeamName, ts)
	setHMACHeaders(httpReq, partnerID, sign(payload, secret), ts)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to call partner service set-winner: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("partner service set-winner returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// CreateDispute creates a new dispute via the partner backend API.
// Signature payload: "{partner_id}:{description}:{timestamp}"
func (c *PartnerClient) CreateDispute(ctx context.Context, partnerID, secret string, req CreateDisputeRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/partner/disputes", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	ts := time.Now().Unix()
	payload := fmt.Sprintf("%s:%s:%d", partnerID, req.Description, ts)
	setHMACHeaders(httpReq, partnerID, sign(payload, secret), ts)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to call partner service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("partner service returned status %d: %s", resp.StatusCode, string(b))
	}
	var result CreateDisputeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode partner response: %w", err)
	}
	return result.Data.ID, nil
}
