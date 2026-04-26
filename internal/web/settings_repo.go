package web

import (
	"database/sql"
	"strconv"
)

type agentConfig struct {
	OllamaURL   string
	Model       string
	NumCtx      int
	Temperature float64
	TopP        float64
	TopK        int
	Repeat      float64
	KeepAlive   string
	Lookback    int
	Prompt      string
}

func loadConfig(db *sql.DB) agentConfig {
	get := func(key, def string) string {
		var v string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
			return def
		}
		return v
	}
	parseInt := func(key string, def int) int {
		v, err := strconv.Atoi(get(key, ""))
		if err != nil {
			return def
		}
		return v
	}
	parseFloat := func(key string, def float64) float64 {
		v, err := strconv.ParseFloat(get(key, ""), 64)
		if err != nil {
			return def
		}
		return v
	}
	return agentConfig{
		OllamaURL:   get("ollama_url", "http://localhost:11434"),
		Model:       get("ollama_model", "qwen3.5:9b"),
		NumCtx:      parseInt("ollama_num_ctx", 16000),
		Temperature: parseFloat("ollama_temp", 0.3),
		TopP:        parseFloat("ollama_top_p", 0.7),
		TopK:        parseInt("ollama_top_k", 20),
		Repeat:      parseFloat("ollama_repeat", 1.1),
		KeepAlive:   get("ollama_keep_alive", "15m"),
		Lookback:    parseInt("agent_lookback", 10),
		Prompt:      get("agent_prompt", ptmSystemPrompt),
	}
}

func saveConfig(db *sql.DB, cfg agentConfig) error {
	pairs := [][2]string{
		{"ollama_url", cfg.OllamaURL},
		{"ollama_model", cfg.Model},
		{"ollama_num_ctx", strconv.Itoa(cfg.NumCtx)},
		{"ollama_temp", strconv.FormatFloat(cfg.Temperature, 'f', -1, 64)},
		{"ollama_top_p", strconv.FormatFloat(cfg.TopP, 'f', -1, 64)},
		{"ollama_top_k", strconv.Itoa(cfg.TopK)},
		{"ollama_repeat", strconv.FormatFloat(cfg.Repeat, 'f', -1, 64)},
		{"ollama_keep_alive", cfg.KeepAlive},
		{"agent_lookback", strconv.Itoa(cfg.Lookback)},
		{"agent_prompt", cfg.Prompt},
	}
	for _, p := range pairs {
		if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}
