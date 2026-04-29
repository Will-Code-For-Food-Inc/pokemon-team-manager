package web

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime/debug"
	"time"
)

// version returns build, runtime, and DB diagnostic info as JSON.
// Intended for local-only use — exposes filesystem paths and PID.
func (h *handler) version(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"now":      time.Now().Format(time.RFC3339),
		"pid":      os.Getpid(),
		"uptime_s": int(time.Since(h.startTime).Seconds()),
	}
	if hn, err := os.Hostname(); err == nil {
		info["hostname"] = hn
	}
	if exe, err := os.Executable(); err == nil {
		info["executable"] = exe
		if fi, err := os.Stat(exe); err == nil {
			info["executable_mtime"] = fi.ModTime().Format(time.RFC3339)
		}
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info["go_version"] = bi.GoVersion
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision", "vcs.time", "vcs.modified":
				info[s.Key] = s.Value
			}
		}
	}

	var dbPath string
	if rows, err := h.svc.DB.Query(`PRAGMA database_list`); err == nil {
		var seq int
		var name string
		if rows.Next() {
			_ = rows.Scan(&seq, &name, &dbPath)
		}
		rows.Close()
	}
	info["db_path"] = dbPath

	counts := map[string]int{}
	for label, table := range map[string]string{
		"teams":        "teams",
		"species":      "species",
		"kb_documents": "kb_documents",
		"chat_messages": "chat_messages",
	} {
		var n int
		if err := h.svc.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err == nil {
			counts[label] = n
		}
	}
	info["counts"] = counts

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(info)
}
