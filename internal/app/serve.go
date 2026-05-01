package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/store"
)

type serveOptions struct {
	storePath       string
	listen          string
	corsOrigin      string
	authUser        string
	authPasswordEnv string
}

type summaryRow struct {
	Venue           string  `json:"venue"`
	Transport       string  `json:"transport"`
	Scenario        string  `json:"scenario"`
	OrderType       string  `json:"order_type"`
	MeasurementMode string  `json:"measurement_mode"`
	Count           int     `json:"count"`
	OK              int     `json:"ok"`
	Failed          int     `json:"failed"`
	MeanMS          float64 `json:"mean_ms"`
	P50MS           float64 `json:"p50_ms"`
	P95MS           float64 `json:"p95_ms"`
	P99MS           float64 `json:"p99_ms"`
	CleanupOK       int     `json:"cleanup_ok"`
	CleanupFail     int     `json:"cleanup_failed"`
}

func newServeCommand() *cobra.Command {
	opts := &serveOptions{}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve read-only benchmark results from SQLite.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serveResults(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.storePath, "store", "data/bench.db", "SQLite result store path.")
	cmd.Flags().StringVar(&opts.listen, "listen", "127.0.0.1:8080", "HTTP listen address.")
	cmd.Flags().StringVar(&opts.corsOrigin, "cors-origin", "*", "CORS Access-Control-Allow-Origin for API responses.")
	cmd.Flags().StringVar(&opts.authUser, "auth-user", "bench", "Basic auth username when auth is enabled.")
	cmd.Flags().StringVar(&opts.authPasswordEnv, "auth-password-env", "PERPS_BENCH_API_PASSWORD", "Environment variable containing the Basic auth password.")
	return cmd
}

func serveResults(ctx context.Context, opts *serveOptions) error {
	password := os.Getenv(opts.authPasswordEnv)
	if password == "" && requiresServeAuth(opts.listen) {
		return fmt.Errorf("serving on %s requires %s to be set", opts.listen, opts.authPasswordEnv)
	}

	db, err := store.OpenSQLite(opts.storePath)
	if err != nil {
		return err
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", withCORS(opts.corsOrigin, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "updated_at": time.Now().UTC()})
	}))
	mux.HandleFunc("/api/latest", withCORS(opts.corsOrigin, func(w http.ResponseWriter, r *http.Request) {
		window := queryDuration(r, "window", 5*time.Minute)
		limit := queryInt(r, "limit", 10000)
		samples, err := db.RecentSamples(r.Context(), time.Now().Add(-window), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{
			"updated_at": time.Now().UTC(),
			"window":     window.String(),
			"summaries":  summarizeGroups(samples),
		})
	}))
	mux.HandleFunc("/api/samples", withCORS(opts.corsOrigin, func(w http.ResponseWriter, r *http.Request) {
		window := queryDuration(r, "window", 5*time.Minute)
		limit := queryInt(r, "limit", 500)
		samples, err := db.RecentSamples(r.Context(), time.Now().Add(-window), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"samples": samples})
	}))

	var handler http.Handler = mux
	if password != "" {
		handler = withBasicAuth(opts.authUser, password, handler)
	}

	server := &http.Server{
		Addr:              opts.listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func requiresServeAuth(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return true
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

func withBasicAuth(user string, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		gotUser, gotPassword, ok := r.BasicAuth()
		if !ok || !basicAuthEqual(gotUser, gotPassword, user, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="perps-bench", charset="UTF-8"`)
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func basicAuthEqual(gotUser string, gotPassword string, wantUser string, wantPassword string) bool {
	gotUserHash := sha256.Sum256([]byte(gotUser))
	wantUserHash := sha256.Sum256([]byte(wantUser))
	gotPasswordHash := sha256.Sum256([]byte(gotPassword))
	wantPasswordHash := sha256.Sum256([]byte(wantPassword))
	userOK := subtle.ConstantTimeCompare(gotUserHash[:], wantUserHash[:])
	passwordOK := subtle.ConstantTimeCompare(gotPasswordHash[:], wantPasswordHash[:])
	return userOK&passwordOK == 1
}

func summarizeGroups(samples []bench.Sample) []summaryRow {
	groups := make(map[string][]bench.Sample)
	for _, sample := range samples {
		key := sample.Venue + "\x00" + sample.Transport + "\x00" + string(sample.Scenario) + "\x00" + sample.OrderType + "\x00" + string(sample.MeasurementMode)
		groups[key] = append(groups[key], sample)
	}
	rows := make([]summaryRow, 0, len(groups))
	for _, grouped := range groups {
		if len(grouped) == 0 {
			continue
		}
		summary := bench.Summarize(grouped)
		first := grouped[0]
		rows = append(rows, summaryRow{
			Venue:           first.Venue,
			Transport:       first.Transport,
			Scenario:        string(first.Scenario),
			OrderType:       first.OrderType,
			MeasurementMode: string(first.MeasurementMode),
			Count:           summary.Count,
			OK:              summary.OK,
			Failed:          summary.Failed,
			MeanMS:          summary.MeanMS,
			P50MS:           summary.P50MS,
			P95MS:           summary.P95MS,
			P99MS:           summary.P99MS,
			CleanupOK:       summary.Cleanup.OK,
			CleanupFail:     summary.Cleanup.Failed,
		})
	}
	slices.SortFunc(rows, func(a, b summaryRow) int {
		if a.Venue != b.Venue {
			if a.Venue < b.Venue {
				return -1
			}
			return 1
		}
		if a.Transport < b.Transport {
			return -1
		}
		if a.Transport > b.Transport {
			return 1
		}
		if a.OrderType < b.OrderType {
			return -1
		}
		if a.OrderType > b.OrderType {
			return 1
		}
		return 0
	})
	return rows
}

func withCORS(origin string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func queryDuration(r *http.Request, key string, fallback time.Duration) time.Duration {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
}
