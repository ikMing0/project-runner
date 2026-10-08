package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type RunRecord struct {
	ID        string    `json:"id"`
	ServiceID string    `json:"serviceId"`
	Name      string    `json:"name"`
	StartedAt int64     `json:"startedAt"`
	EndedAt   int64     `json:"endedAt"`
	Status    Status    `json:"status"`
	Logs      []LogLine `json:"logs,omitempty"`
}

func newRecordID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func privateJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func readBoundedJSON(path string, target any, limit int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("数据文件过大")
	}
	return json.Unmarshal(data, target)
}

var privateValueKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|api.?key|credential|authorization|cookie)`)
var credentialToken = regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[A-Z0-9]{16}|AIza[A-Za-z0-9_-]{30,})`)

func redactRunText(text string, p Project) string {
	text = credentialToken.ReplaceAllString(redactCodexLog(plainLogText(text)), "[已隐藏凭据]")
	for key, value := range p.Environment {
		if privateValueKey.MatchString(key) && len(value) >= 4 {
			text = strings.ReplaceAll(text, value, "[已隐藏]")
		}
	}
	return text
}

func archiveLogs(logs []LogLine, p Project) []LogLine {
	out := make([]LogLine, 0, min(len(logs), 1000))
	privateBlock := false
	for _, line := range logs {
		line.Text = plainLogText(line.Text)
		if strings.Contains(line.Text, "-----BEGIN ") && strings.Contains(line.Text, "PRIVATE KEY-----") {
			privateBlock = true
		}
		if privateBlock {
			end := strings.Contains(line.Text, "-----END ") && strings.Contains(line.Text, "PRIVATE KEY-----")
			line.Text = "[已隐藏私钥内容]"
			if end {
				privateBlock = false
			}
		} else {
			line.Text = redactRunText(line.Text, p)
		}
		if len(line.Text) > 8192 {
			line.Text = line.Text[len(line.Text)-8192:]
			for !utf8.ValidString(line.Text) {
				line.Text = line.Text[1:]
			}
			line.Text = "[长行仅保留末尾] " + line.Text
		}
		out = append(out, line)
	}
	if len(out) > 1000 {
		out = out[len(out)-1000:]
	}
	size := 0
	start := len(out)
	for start > 0 {
		encoded, _ := json.MarshalIndent(out[start-1], "", "  ")
		next := len(encoded) + 128
		if size+next > 256*1024 {
			break
		}
		size += next
		start--
	}
	return append([]LogLine{}, out[start:]...)
}

// Caller holds a.mu; completed runs are persisted before r.done closes.
func (a *App) archiveRunLocked(id string, p Project, status Status) {
	if a.configPath == "" {
		return
	}
	a.archiveMu.Lock()
	defer a.archiveMu.Unlock()
	path := filepath.Join(filepath.Dir(a.configPath), "run-history.json")
	var records []RunRecord
	if err := readBoundedJSON(path, &records, 20*1024*1024); err != nil && !os.IsNotExist(err) {
		return
	}
	recordID, err := newRecordID()
	if err != nil {
		return
	}
	status.Error = redactRunText(status.Error, p)
	if len(status.Error) > 8192 {
		status.Error = string([]rune(status.Error)[:min(2000, len([]rune(status.Error)))])
	}
	record := RunRecord{ID: recordID, ServiceID: id, Name: p.Name, StartedAt: status.StartedAt, EndedAt: time.Now().UnixMilli(), Status: status, Logs: archiveLogs(a.history[id], p)}
	records = append([]RunRecord{record}, records...)
	counts := map[string]int{}
	kept := make([]RunRecord, 0, min(len(records), 50))
	for _, record := range records {
		if counts[record.ServiceID] >= 10 {
			continue
		}
		counts[record.ServiceID]++
		kept = append(kept, record)
		if len(kept) == 50 {
			break
		}
	}
	_ = privateJSON(path, kept)
}

func (a *App) archiveRecords() ([]RunRecord, error) {
	a.mu.Lock()
	config := a.configPath
	a.mu.Unlock()
	if config == "" {
		return []RunRecord{}, nil
	}
	a.archiveMu.Lock()
	defer a.archiveMu.Unlock()
	var records []RunRecord
	err := readBoundedJSON(filepath.Join(filepath.Dir(config), "run-history.json"), &records, 20*1024*1024)
	if os.IsNotExist(err) {
		return []RunRecord{}, nil
	}
	return records, err
}

func (a *App) ListRunHistory(serviceID string) ([]RunRecord, error) {
	records, err := a.archiveRecords()
	if err != nil {
		return nil, err
	}
	result := []RunRecord{}
	for _, record := range records {
		if record.ServiceID == serviceID {
			record.Logs = nil
			result = append(result, record)
		}
	}
	return result, nil
}

func (a *App) ReadRunHistory(id string) (RunRecord, error) {
	records, err := a.archiveRecords()
	if err != nil {
		return RunRecord{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return RunRecord{}, fmt.Errorf("启动记录不存在或已超过保留上限")
}

func (a *App) RunHistoryText(id string) (string, error) {
	record, err := a.ReadRunHistory(id)
	if err != nil {
		return "", err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "项目：%s\n启动时间：%s\n状态：%s\n%s\n", record.Name, time.UnixMilli(record.StartedAt).Format(time.RFC3339), record.Status.State, record.Status.Error)
	for _, line := range record.Logs {
		fmt.Fprintf(&text, "%s [%s] %s\n", line.Time, line.Level, line.Text)
	}
	return text.String(), nil
}
