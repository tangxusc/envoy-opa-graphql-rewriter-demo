package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type rewriteAPIRequest struct {
	GraphQL      string          `json:"graphql"`
	Decision     json.RawMessage `json:"decision"`
	DecisionJSON string          `json:"decision_json"`
}

type rewriteAPIResponse struct {
	RewrittenQuery string `json:"rewritten_query,omitempty"`
	Error          string `json:"error,omitempty"`
}

var serverOutput io.Writer = os.Stdout

var listenAndServe = func(srv *http.Server) error {
	return srv.ListenAndServe()
}

func runServer(addr string) error {
	mux := newServerMux()
	fmt.Fprintf(serverOutput, "listening on http://localhost%s\n", addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return listenAndServe(srv)
}

func newServerMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/rewrite", handleRewrite)
	return mux
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func handleRewrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, rewriteAPIResponse{Error: "method not allowed"})
		return
	}

	var req rewriteAPIRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, rewriteAPIResponse{Error: "invalid request body"})
		return
	}

	if strings.TrimSpace(req.GraphQL) == "" {
		writeJSON(w, http.StatusBadRequest, rewriteAPIResponse{Error: "graphql is required"})
		return
	}

	decisionBytes, err := decisionPayload(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, rewriteAPIResponse{Error: err.Error()})
		return
	}

	rewritten, err := RewriteQuery(req.GraphQL, decisionBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, rewriteAPIResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, rewriteAPIResponse{RewrittenQuery: rewritten})
}

func decisionPayload(req rewriteAPIRequest) ([]byte, error) {
	if s := strings.TrimSpace(req.DecisionJSON); s != "" {
		b := []byte(s)
		if !json.Valid(b) {
			return nil, fmt.Errorf("decision_json is not valid json")
		}
		return b, nil
	}

	raw := bytes.TrimSpace(req.Decision)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, fmt.Errorf("decision is required")
	}

	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("invalid decision")
		}
		b := []byte(strings.TrimSpace(s))
		if !json.Valid(b) {
			return nil, fmt.Errorf("decision string is not valid json")
		}
		return b, nil
	}

	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid decision")
	}
	return raw, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>GraphQL Rewriter Tester</title>
  <style>
    :root {
      --bg-1: #f8fafc;
      --bg-2: #e2e8f0;
      --panel: #ffffff;
      --ink: #0f172a;
      --muted: #475569;
      --accent: #0f766e;
      --accent-2: #134e4a;
      --error: #b91c1c;
      --border: #cbd5e1;
      --radius: 14px;
      --shadow: 0 18px 40px rgba(15, 23, 42, 0.10);
    }

    * { box-sizing: border-box; }

    body {
      margin: 0;
      color: var(--ink);
      font-family: "IBM Plex Sans", "Helvetica Neue", sans-serif;
      background:
        radial-gradient(circle at top right, rgba(15, 118, 110, 0.20), transparent 45%),
        linear-gradient(160deg, var(--bg-1), var(--bg-2));
      min-height: 100vh;
      padding: 32px 18px;
    }

    .wrap {
      width: min(1100px, 100%);
      margin: 0 auto;
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      box-shadow: var(--shadow);
      padding: 20px;
      animation: rise 360ms ease-out;
    }

    h1 {
      margin: 0 0 6px;
      font-size: 24px;
      letter-spacing: 0.3px;
    }

    p {
      margin: 0 0 18px;
      color: var(--muted);
      font-size: 14px;
    }

    .grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 14px;
    }

    .block {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }

    label {
      font-size: 13px;
      color: var(--muted);
    }

    textarea {
      width: 100%;
      min-height: 270px;
      resize: vertical;
      padding: 12px;
      border-radius: 10px;
      border: 1px solid var(--border);
      background: #f8fafc;
      color: var(--ink);
      font: 13px/1.45 "JetBrains Mono", "SFMono-Regular", monospace;
      outline: none;
      transition: border-color 150ms ease, box-shadow 150ms ease;
    }

    textarea:focus {
      border-color: var(--accent);
      box-shadow: 0 0 0 3px rgba(15, 118, 110, 0.14);
    }

    .actions {
      margin-top: 12px;
      display: flex;
      align-items: center;
      gap: 10px;
      flex-wrap: wrap;
    }

    button {
      border: 0;
      border-radius: 10px;
      padding: 10px 16px;
      font-weight: 600;
      color: #fff;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      cursor: pointer;
      transition: transform 110ms ease, filter 110ms ease;
    }

    button:hover { filter: brightness(1.03); }
    button:active { transform: translateY(1px); }

    .status {
      font-size: 13px;
      color: var(--muted);
      min-height: 18px;
    }

    .status.error { color: var(--error); }

    @keyframes rise {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }

    @media (max-width: 900px) {
      .grid { grid-template-columns: 1fr; }
      textarea { min-height: 220px; }
    }
  </style>
</head>
<body>
  <main class="wrap">
    <h1>GraphQL Query Rewriter</h1>
    <p>输入 GraphQL 与权限决策 JSON，点击 Rewrite 获取过滤后的查询。</p>

    <section class="grid">
      <div class="block">
        <label for="graphql">GraphQL</label>
        <textarea id="graphql">query EmployeeSalaryWithFragment($id: String!) {
  employeeByID(id: $id) {
    id
    ...SalaryPart
  }
}

fragment SalaryPart on Employee {
  salary
}
</textarea>
      </div>
      <div class="block">
        <label for="decision">Decision JSON</label>
        <textarea id="decision">{
  "allow": true,
  "removed_fields": [
    "employeeByID.salary"
  ]
}</textarea>
      </div>
    </section>

    <div class="actions">
      <button id="rewriteBtn" type="button">Rewrite</button>
      <span id="status" class="status"></span>
    </div>

    <section class="block" style="margin-top:14px;">
      <label for="output">Rewritten Query</label>
      <textarea id="output" readonly></textarea>
    </section>
  </main>

  <script>
    const btn = document.getElementById('rewriteBtn');
    const statusEl = document.getElementById('status');
    const gqlEl = document.getElementById('graphql');
    const decisionEl = document.getElementById('decision');
    const outEl = document.getElementById('output');

    async function rewrite() {
      statusEl.classList.remove('error');
      statusEl.textContent = 'Processing...';
      outEl.value = '';

      let decisionObj;
      try {
        decisionObj = JSON.parse(decisionEl.value);
      } catch (err) {
        statusEl.classList.add('error');
        statusEl.textContent = 'Decision JSON 格式错误';
        return;
      }

      try {
        const resp = await fetch('/api/rewrite', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            graphql: gqlEl.value,
            decision: decisionObj
          })
        });

        const data = await resp.json();
        if (!resp.ok) {
          throw new Error(data.error || 'rewrite failed');
        }

        outEl.value = data.rewritten_query || '';
        statusEl.textContent = 'Done';
      } catch (err) {
        statusEl.classList.add('error');
        statusEl.textContent = err.message || 'request failed';
      }
    }

    btn.addEventListener('click', rewrite);
  </script>
</body>
</html>
`
