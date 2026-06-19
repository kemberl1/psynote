// IngestFile sends a document file to the RAG service POST /ingest for
// extraction → anonymization → chunking → embedding → upsert.
//
// PRIVACY: only the raw file binary is sent; RAG calls back to gateway's
// /anonymize endpoint before writing any data to Qdrant.
package ragclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// IngestResult mirrors the RAG /ingest response.
type IngestResult struct {
	Status                 string         `json:"status"`
	ChunksCount            int            `json:"chunks_count"`
	QdrantIDs              []string       `json:"qdrant_ids"`
	AnonymizerRemovedCount int            `json:"anonymizer_removed_count"`
	RemovedByType          map[string]int `json:"removed_by_type"`
	ErrorMessage           string         `json:"error_message,omitempty"`
}

// ingestEnvelope is the RAG response envelope.
type ingestEnvelope struct {
	Meta struct {
		RequestID string `json:"request_id"`
	} `json:"meta"`
	Data  *IngestResult `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// IngestFile uploads a file to RAG /ingest (multipart/form-data).
func (c *HTTPClient) IngestFile(ctx context.Context, filename string, fileData []byte, contentType string) (*IngestResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("ragclient: create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("ragclient: write file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("ragclient: close writer: %w", err)
	}

	url := c.baseURL + "/ingest"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("ragclient: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}

	var env ingestEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		if resp.StatusCode >= 500 {
			return nil, ErrUnavailable
		}
		return &IngestResult{
			Status:       "failed",
			ErrorMessage: fmt.Sprintf("RAG returned HTTP %d", resp.StatusCode),
		}, nil
	}

	if resp.StatusCode == 422 && env.Error != nil && strings.Contains(env.Error.Code, "PII") {
		return &IngestResult{
			Status:       "pii_blocked",
			ErrorMessage: env.Error.Message,
		}, nil
	}

	if resp.StatusCode != http.StatusOK || env.Error != nil {
		msg := "ошибка RAG-сервиса"
		if env.Error != nil {
			msg = env.Error.Message
		}
		return &IngestResult{
			Status:       "failed",
			ErrorMessage: msg,
		}, nil
	}

	if env.Data != nil {
		return env.Data, nil
	}
	return &IngestResult{Status: "ingested"}, nil
}
