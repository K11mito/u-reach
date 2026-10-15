package store

// Paid API calls this server makes, one row each, so the analytics page can
// chart what U-Reach really costs to run. Demo resets never touch it.
const usageSchema = `
CREATE TABLE IF NOT EXISTS api_calls (
  id INTEGER PRIMARY KEY,
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read INTEGER NOT NULL DEFAULT 0,
  cache_write INTEGER NOT NULL DEFAULT 0,
  cost_usd REAL NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS api_calls_time ON api_calls (provider, created_at);`

// APICall is one billed request to a model provider.
type APICall struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheRead    int64   `json:"cache_read"`
	CacheWrite   int64   `json:"cache_write"`
	CostUSD      float64 `json:"cost_usd"`
	CreatedAt    int64   `json:"created_at"`
}

// AddAPICall records a billed request.
func (s *Store) AddAPICall(c APICall) error {
	if c.CreatedAt == 0 {
		c.CreatedAt = Now()
	}
	_, err := s.db.Exec(`INSERT INTO api_calls (provider, model, input_tokens, output_tokens, cache_read, cache_write, cost_usd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Provider, c.Model, c.InputTokens, c.OutputTokens, c.CacheRead, c.CacheWrite, c.CostUSD, c.CreatedAt)
	return err
}

// APICalls lists a provider's calls, oldest first. since is a Unix ms time.
func (s *Store) APICalls(provider string, since int64) ([]APICall, error) {
	rows, err := s.db.Query(`SELECT provider, model, input_tokens, output_tokens, cache_read, cache_write, cost_usd, created_at
		FROM api_calls WHERE provider = ? AND created_at >= ? ORDER BY created_at`, provider, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APICall
	for rows.Next() {
		var c APICall
		if err := rows.Scan(&c.Provider, &c.Model, &c.InputTokens, &c.OutputTokens, &c.CacheRead, &c.CacheWrite, &c.CostUSD, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FirstAPICall is when tracking for a provider began (0 if never).
func (s *Store) FirstAPICall(provider string) int64 {
	var t int64
	s.db.QueryRow(`SELECT COALESCE(MIN(created_at), 0) FROM api_calls WHERE provider = ?`, provider).Scan(&t)
	return t
}

// MessagesSince lists every message created at or after since (Unix ms).
func (s *Store) MessagesSince(since int64) ([]Message, error) {
	rows, err := s.db.Query(`SELECT `+messageCols+` FROM messages WHERE created_at >= ? ORDER BY created_at, id`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
