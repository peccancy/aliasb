package handler

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/peccancy/aliasb/internal/stats"
)

var statusTmpl = template.Must(template.New("status").Funcs(template.FuncMap{
	"fmtDuration": func(secs float64) string {
		if secs <= 0 {
			return "—"
		}
		m := int(secs) / 60
		s := int(secs) % 60
		return fmt.Sprintf("%dm %ds", m, s)
	},
}).Parse(`<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Alias — Statistics</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: #0f1117;
    color: #e2e8f0;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    font-size: 15px;
    padding: 40px 24px;
  }
  h1 {
    font-size: 22px;
    font-weight: 600;
    color: #f8fafc;
    margin-bottom: 8px;
    letter-spacing: -0.3px;
  }
  .subtitle {
    color: #64748b;
    font-size: 13px;
    margin-bottom: 32px;
  }
  .cards {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
    margin-bottom: 36px;
  }
  .card {
    background: #1e2330;
    border: 1px solid #2d3548;
    border-radius: 10px;
    padding: 20px 24px;
    min-width: 160px;
    flex: 1;
  }
  .card-label {
    font-size: 12px;
    color: #64748b;
    text-transform: uppercase;
    letter-spacing: 0.8px;
    margin-bottom: 8px;
  }
  .card-value {
    font-size: 28px;
    font-weight: 700;
    color: #7c3aed;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    background: #1e2330;
    border-radius: 10px;
    overflow: hidden;
    border: 1px solid #2d3548;
  }
  thead th {
    background: #161b27;
    padding: 12px 16px;
    text-align: left;
    font-size: 12px;
    color: #64748b;
    text-transform: uppercase;
    letter-spacing: 0.8px;
    font-weight: 500;
    border-bottom: 1px solid #2d3548;
  }
  tbody tr {
    border-bottom: 1px solid #252d3d;
    transition: background 0.15s;
  }
  tbody tr:last-child { border-bottom: none; }
  tbody tr:hover { background: #252d3d; }
  tbody td {
    padding: 12px 16px;
    font-size: 14px;
    color: #cbd5e1;
  }
  .date-cell { color: #94a3b8; font-variant-numeric: tabular-nums; }
  .num { color: #e2e8f0; font-variant-numeric: tabular-nums; font-weight: 500; }
  .dispute { color: #f59e0b; }
  .duration { color: #34d399; font-variant-numeric: tabular-nums; }
  .empty { text-align: center; padding: 48px; color: #475569; }
  .updated { font-size: 12px; color: #374151; margin-top: 24px; }
</style>
</head>
<body>
<h1>Alias — Statistics</h1>
<p class="subtitle">Last 30 days · updated {{.Now}}</p>

{{if .Stats}}
<div class="cards">
  <div class="card">
    <div class="card-label">Total launched</div>
    <div class="card-value">{{.TotalLaunched}}</div>
  </div>
  <div class="card">
    <div class="card-label">Completed</div>
    <div class="card-value">{{.TotalCompleted}}</div>
  </div>
  <div class="card">
    <div class="card-label">With disputes</div>
    <div class="card-value">{{.TotalDisputes}}</div>
  </div>
</div>

<table>
  <thead>
    <tr>
      <th>Date</th>
      <th>Launched</th>
      <th>Completed</th>
      <th>With disputes</th>
      <th>Avg duration</th>
    </tr>
  </thead>
  <tbody>
    {{range .Stats}}
    <tr>
      <td class="date-cell">{{.Date}}</td>
      <td class="num">{{.Launched}}</td>
      <td class="num">{{.Completed}}</td>
      <td class="num dispute">{{.WithDisputes}}</td>
      <td class="duration">{{fmtDuration .AvgDurationSecs}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{else}}
<div class="empty">No data yet</div>
{{end}}

<p class="updated">Generated at {{.Now}} UTC</p>
</body>
</html>`))

type StatusHandler struct {
	stats *stats.Store
	log   zerolog.Logger
}

func NewStatusHandler(s *stats.Store, log zerolog.Logger) *StatusHandler {
	return &StatusHandler{stats: s, log: log}
}

type statusPageData struct {
	Stats          []stats.DayStat
	Now            string
	TotalLaunched  int64
	TotalCompleted int64
	TotalDisputes  int64
}

func (h *StatusHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if h.stats == nil {
		http.Error(w, "stats unavailable (mongodb not connected)", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	daily, err := h.stats.GetDailyStats(ctx)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to get daily stats")
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}

	data := statusPageData{
		Stats: daily,
		Now:   time.Now().UTC().Format("2006-01-02 15:04:05"),
	}
	for _, d := range daily {
		data.TotalLaunched += d.Launched
		data.TotalCompleted += d.Completed
		data.TotalDisputes += d.WithDisputes
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	var buf strings.Builder
	if err := statusTmpl.Execute(&buf, data); err != nil {
		h.log.Error().Err(err).Msg("failed to render status template")
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Write([]byte(buf.String()))
}
