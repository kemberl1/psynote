// Package handlers — health endpoint (docs/07 §8).
//
// GET /health and GET /api/v1/health report gateway liveness plus downstream
// RAG reachability. The probe is best-effort and short-timeouted inside the
// RAG client; an unreachable RAG yields status "degraded" (200) so liveness
// checks still succeed while signalling the issue.
package handlers

import (
	"net/http"

	"github.com/aimed/gateway/internal/ragclient"
)

type healthRAG struct {
	Reachable bool `json:"reachable"`
}

type healthData struct {
	Status string    `json:"status"`
	RAG    healthRAG `json:"rag"`
}

// newHealthHandler builds the health handler, optionally probing RAG.
func newHealthHandler(rag ragclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ragOK := false
		if rag != nil {
			ragOK = rag.Health(r.Context()) == nil
		}
		status := "ok"
		if rag != nil && !ragOK {
			status = "degraded"
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: healthData{Status: status, RAG: healthRAG{Reachable: ragOK}},
		})
	}
}
