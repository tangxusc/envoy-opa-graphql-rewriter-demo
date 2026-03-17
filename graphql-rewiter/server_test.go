package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunServer_UsesConfiguredServer(t *testing.T) {
	var output bytes.Buffer
	oldOutput := serverOutput
	serverOutput = &output
	defer func() { serverOutput = oldOutput }()

	var captured *http.Server
	oldListen := listenAndServe
	listenAndServe = func(srv *http.Server) error {
		captured = srv
		return errors.New("stop")
	}
	defer func() { listenAndServe = oldListen }()

	err := runServer(":19091")
	if err == nil || err.Error() != "stop" {
		t.Fatalf("expected stop error, got %v", err)
	}
	if captured == nil {
		t.Fatalf("expected captured server")
	}
	if captured.Addr != ":19091" {
		t.Fatalf("unexpected addr: %q", captured.Addr)
	}
	if captured.Handler == nil {
		t.Fatalf("expected handler to be set")
	}
	if captured.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("unexpected ReadHeaderTimeout: %v", captured.ReadHeaderTimeout)
	}
	if !strings.Contains(output.String(), "listening on http://localhost:19091") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestHandleIndex(t *testing.T) {
	mux := newServerMux()

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "GraphQL Query Rewriter") {
			t.Fatalf("index page content missing expected title")
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/not-found", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", rr.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected status 405, got %d", rr.Code)
		}
	})
}

func TestHandleRewrite(t *testing.T) {
	mux := newServerMux()

	tests := []struct {
		name        string
		method      string
		body        string
		wantStatus  int
		wantErrLike string
		wantNoField string
	}{
		{
			name:       "success object decision",
			method:     http.MethodPost,
			wantStatus: http.StatusOK,
			body: `{
  "graphql": "query Q { employeeByID(id: \"1\") { id salary } }",
  "decision": {"allow": true, "removed_fields": ["employeeByID.salary"]}
}`,
			wantNoField: "salary",
		},
		{
			name:       "success decision_json",
			method:     http.MethodPost,
			wantStatus: http.StatusOK,
			body: `{
  "graphql": "query Q { employeeByID(id: \"1\") { id salary } }",
  "decision_json": "{\"allow\":true,\"removed_fields\":[\"employeeByID.salary\"]}"
}`,
			wantNoField: "salary",
		},
		{
			name:        "method not allowed",
			method:      http.MethodGet,
			wantStatus:  http.StatusMethodNotAllowed,
			wantErrLike: "method not allowed",
		},
		{
			name:        "invalid body",
			method:      http.MethodPost,
			body:        `{invalid-json}`,
			wantStatus:  http.StatusBadRequest,
			wantErrLike: "invalid request body",
		},
		{
			name:        "missing graphql",
			method:      http.MethodPost,
			body:        `{"decision":{"allow":true,"removed_fields":[]}}`,
			wantStatus:  http.StatusBadRequest,
			wantErrLike: "graphql is required",
		},
		{
			name:        "decision required",
			method:      http.MethodPost,
			body:        `{"graphql":"query Q{ping}"}`,
			wantStatus:  http.StatusBadRequest,
			wantErrLike: "decision is required",
		},
		{
			name:        "invalid decision_json",
			method:      http.MethodPost,
			body:        `{"graphql":"query Q{ping}","decision_json":"not-json"}`,
			wantStatus:  http.StatusBadRequest,
			wantErrLike: "decision_json is not valid json",
		},
		{
			name:        "rewrite error from policy deny",
			method:      http.MethodPost,
			body:        `{"graphql":"query Q{ping}","decision":{"allow":false,"removed_fields":[]}}`,
			wantStatus:  http.StatusBadRequest,
			wantErrLike: "request denied by policy",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body == "" {
				req = httptest.NewRequest(tt.method, "/api/rewrite", nil)
			} else {
				req = httptest.NewRequest(tt.method, "/api/rewrite", bytes.NewBufferString(tt.body))
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body=%s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var resp rewriteAPIResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if tt.wantErrLike != "" && !strings.Contains(resp.Error, tt.wantErrLike) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErrLike, resp.Error)
			}
			if tt.wantNoField != "" {
				if strings.Contains(resp.RewrittenQuery, tt.wantNoField) {
					t.Fatalf("rewritten query should not contain %q, got %s", tt.wantNoField, resp.RewrittenQuery)
				}
				if strings.TrimSpace(resp.RewrittenQuery) == "" {
					t.Fatalf("rewritten_query should not be empty")
				}
			}
		})
	}
}

func TestDecisionPayload_Branches(t *testing.T) {
	tests := []struct {
		name        string
		req         rewriteAPIRequest
		wantErrLike string
		wantJSON    string
	}{
		{
			name: "decision json string valid",
			req: rewriteAPIRequest{
				Decision: json.RawMessage(`"{\"allow\":true,\"removed_fields\":[]}"`),
			},
			wantJSON: `{"allow":true,"removed_fields":[]}`,
		},
		{
			name: "decision json string not json",
			req: rewriteAPIRequest{
				Decision: json.RawMessage(`"hello"`),
			},
			wantErrLike: "decision string is not valid json",
		},
		{
			name: "decision raw invalid",
			req: rewriteAPIRequest{
				Decision: json.RawMessage(`{bad`),
			},
			wantErrLike: "invalid decision",
		},
		{
			name: "decision raw quoted malformed",
			req: rewriteAPIRequest{
				Decision: json.RawMessage(`"bad`),
			},
			wantErrLike: "invalid decision",
		},
		{
			name: "decision null",
			req: rewriteAPIRequest{
				Decision: json.RawMessage(`null`),
			},
			wantErrLike: "decision is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := decisionPayload(tt.req)
			if tt.wantErrLike != "" {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErrLike) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErrLike, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Fatalf("unexpected decision payload: %q", string(got))
			}
		})
	}
}
