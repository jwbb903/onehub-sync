package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	dbPath         string
	defaultTimeout = 15 * time.Second
	clearAll       bool
	debugMode      bool
	helpMode       bool
)

type ModelResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type Channel struct {
	ID        int
	Name      string
	Type      int
	Key       string
	BaseURL   string
	OldModels string
}

type ChangeRecord struct {
	ChannelID int
	ChannelName string
	AddCount  int
	DelCount  int
	Timestamp time.Time
}

func getFinalBaseURL(c Channel) string {
	if c.BaseURL != "" {
		return strings.TrimSuffix(c.BaseURL, "/v1")
	}

	switch c.Type {
	case 28:
		return "https://api.deepseek.com"
	case 45:
		return "https://api.siliconflow.cn"
	case 25:
		return "https://generativelanguage.googleapis.com"
	case 31:
		return "https://api.groq.com/openai"
	case 14:
		return "https://api.anthropic.com"
	}
	return ""
}

func parseGeminiNative(body []byte) string {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}

	var models []string
	if mList, ok := raw["models"].([]interface{}); ok {
		for _, m := range mList {
			if mObj, ok := m.(map[string]interface{}); ok {
				if name, ok := mObj["name"].(string); ok {
					models = append(models, strings.TrimPrefix(name, "models/"))
				}
			}
		}
	}
	return strings.Join(models, ",")
}

func fetchFromAPI(targetURL string, key string) (string, error) {
	client := &http.Client{Timeout: defaultTimeout}
	targetURL = strings.TrimSuffix(targetURL, "/")

	var lastErr error

	if strings.Contains(targetURL, "v1beta") {
		apiURL := targetURL + "/models"
		models, err := tryFetch(client, apiURL, key, true)
		if err == nil {
			return models, nil
		}
		lastErr = err
	} else {
		paths := []struct {
			path   string
			gemini bool
		}{
			{"/v1/models", false},
			{"/v1beta/models", true},
		}

		for _, p := range paths {
			apiURL := targetURL + p.path
			models, err := tryFetch(client, apiURL, key, p.gemini)
			if err == nil {
				return models, nil
			}
			lastErr = err
		}
	}

	return "", lastErr
}

func tryFetch(client *http.Client, apiURL string, key string, isGeminiFormat bool) (string, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var models []string

	if isGeminiFormat || strings.Contains(string(body), "\"models\"") && !strings.Contains(string(body), "\"id\"") {
		mStr := parseGeminiNative(body)
		if mStr != "" {
			sort.Strings(models)
			return mStr, nil
		}
	}

	var mResp ModelResponse
	if err := json.Unmarshal(body, &mResp); err == nil && len(mResp.Data) > 0 {
		for _, m := range mResp.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
	}

	if len(models) > 0 {
		sort.Strings(models)
		return strings.Join(models, ","), nil
	}

	return "", fmt.Errorf("no models found")
}

func diffModels(oldStr, newStr string) (added, removed []string) {
	oldMap := make(map[string]bool)
	newMap := make(map[string]bool)

	for _, m := range strings.Split(oldStr, ",") {
		if m = strings.TrimSpace(m); m != "" {
			oldMap[m] = true
		}
	}
	for _, m := range strings.Split(newStr, ",") {
		if m = strings.TrimSpace(m); m != "" {
			newMap[m] = true
		}
	}

	for m := range newMap {
		if !oldMap[m] {
			added = append(added, m)
		}
	}
	for m := range oldMap {
		if !newMap[m] {
			removed = append(removed, m)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return
}

func truncateModels(models []string, limit int) string {
	if len(models) <= limit {
		return strings.Join(models, ", ")
	}
	return strings.Join(models[:limit], ", ") + fmt.Sprintf(" ... (%d 个)", len(models)-limit)
}

func truncateURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) >= 3 {
		host := parts[2]
		if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
			host = host[:colonIdx]
		}
		if len(host) > 6 {
			host = host[:4] + "..."
		}
		if colonIdx := strings.Index(parts[2], ":"); colonIdx != -1 {
			host = host + parts[2][colonIdx:]
		}
		return parts[0] + "//" + host + "/..."
	}
	if len(url) > 40 {
		return url[:40] + "..."
	}
	return url
}

// ANSI escape codes
const (
	ClearScreen   = "\033[2J"
	CursorHome    = "\033[H"
	ClearLine     = "\033[K"
	MoveCursorUp  = "\033[1A"
)

type StatusManager struct {
	mu           sync.Mutex
	statusLines  []string
	maxLines     int
	currentIndex int
}

func NewStatusManager(maxLines int) *StatusManager {
	return &StatusManager{
		statusLines: make([]string, maxLines),
		maxLines:    maxLines,
	}
}

func (sm *StatusManager) UpdateStatus(index int, status string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if index < sm.maxLines {
		sm.statusLines[index] = status
	}
}

func (sm *StatusManager) Refresh() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// 清屏并回到顶部
	fmt.Print(ClearScreen)
	fmt.Print(CursorHome)
	
	// 打印状态行（上面5行：工作状态）
	for i := 0; i < sm.maxLines; i++ {
		fmt.Printf("%s%s\n", sm.statusLines[i], ClearLine)
	}
}

type ChangeRecordManager struct {
	mu      sync.Mutex
	records []ChangeRecord
}

func (crm *ChangeRecordManager) AddRecord(record ChangeRecord) {
	crm.mu.Lock()
	defer crm.mu.Unlock()
	
	crm.records = append(crm.records, record)
	
	// 限制显示数量，最多显示20个记录
	if len(crm.records) > 20 {
		crm.records = crm.records[len(crm.records)-20:]
	}
}

func (crm *ChangeRecordManager) Display() {
	crm.mu.Lock()
	defer crm.mu.Unlock()
	
	fmt.Printf("=== 最近变更记录 ===\n")
	for i := len(crm.records) - 1; i >= 0 && i >= len(crm.records)-10; i-- {
		record := crm.records[i]
		fmt.Printf("渠道 [%d] %s: 新增 %d 个, 删除 %d 个\n", 
			record.ChannelID, record.ChannelName, record.AddCount, record.DelCount)
	}
}

func drawProgress(current, total int) {
	width := 40
	percent := float64(current) / float64(total)
	filled := int(percent * float64(width))
	bar := strings.Repeat("=", filled) + strings.Repeat("-", width-filled)
	fmt.Printf("[%s] %d/%d (%.0f%%)\n", bar, current, total, percent*100)
}

type Progress struct {
	mu      sync.Mutex
	current int
	total   int
	done    bool
}

func (p *Progress) Increment() {
	p.mu.Lock()
	p.current++
	p.mu.Unlock()
}

func (p *Progress) GetProgress() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current, p.total
}

func debugPrint(format string, args ...interface{}) {
	if debugMode {
		log.Printf(format, args...)
	}
}

func printHelp() {
	fmt.Println("模型更新工具 - 帮助菜单")
	fmt.Println("=========================")
	fmt.Println("-db <path>: 指定数据库文件路径 (默认: /root/onehub/one-api.db)")
	fmt.Println("-clear-all: 清除所有渠道的模型")
	fmt.Println("-debug: 启用调试模式，显示详细日志")
	fmt.Println("-help: 显示此帮助信息")
	fmt.Println("")
	fmt.Println("使用示例:")
	fmt.Println("./model-updater                # 正常运行")
	fmt.Println("./model-updater -debug         # 调试模式")
	fmt.Println("./model-updater -clear-all     # 清除所有模型")
	fmt.Println("./model-updater -db my.db      # 指定数据库")
	os.Exit(0)
}

func processChannel(db *sql.DB, c Channel, progress *Progress, statusManager *StatusManager, 
					workerID int, changeRecordManager *ChangeRecordManager) {
	finalURL := getFinalBaseURL(c)
	if finalURL == "" {
		if debugMode {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 已跳过: BaseURL 为空", c.ID, c.Name))
		} else {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 处理中...", c.ID, c.Name))
		}
		progress.Increment()
		return
	}

	statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 正在获取: %s", c.ID, c.Name, truncateURL(finalURL)))

	newModels, err := fetchFromAPI(finalURL, c.Key)
	if err != nil {
		if debugMode {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 失败: %v", c.ID, c.Name, err))
		} else {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 失败", c.ID, c.Name))
		}
		progress.Increment()
		return
	}

	if newModels == "" {
		if debugMode {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 未找到模型", c.ID, c.Name))
		} else {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 无模型", c.ID, c.Name))
		}
		progress.Increment()
		return
	}

	if newModels == c.OldModels {
		if debugMode {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 无变化", c.ID, c.Name))
		} else {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 无更新", c.ID, c.Name))
		}
		progress.Increment()
		return
	}

	added, removed := diffModels(c.OldModels, newModels)

	_, uErr := db.Exec("UPDATE channels SET models = ? WHERE id = ?", newModels, c.ID)
	if uErr != nil {
		if debugMode {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 数据库错误: %v", c.ID, c.Name, uErr))
		} else {
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 更新失败", c.ID, c.Name))
		}
	} else {
		if debugMode {
			msg := fmt.Sprintf("[%d] %s | 成功", c.ID, c.Name)
			if len(added) > 0 {
				msg += fmt.Sprintf(" [新增: %s]", truncateModels(added, 2))
			}
			if len(removed) > 0 {
				msg += fmt.Sprintf(" [删除: %s]", truncateModels(removed, 2))
			}
			statusManager.UpdateStatus(workerID, msg)
		} else {
			// 记录渠道变更
			record := ChangeRecord{
				ChannelID: c.ID,
				ChannelName: c.Name,
				AddCount: len(added),
				DelCount: len(removed),
				Timestamp: time.Now(),
			}
			changeRecordManager.AddRecord(record)
			statusManager.UpdateStatus(workerID, fmt.Sprintf("[%d] %s | 已更新", c.ID, c.Name))
		}
	}
	progress.Increment()
}

// 检查数据库是否包含 deleted_at 字段
func hasDeletedAtColumn(db *sql.DB) bool {
	rows, err := db.Query("PRAGMA table_info(channels)")
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dflt_value interface{}
		var pk int
		
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt_value, &pk); err == nil {
			if name == "deleted_at" {
				return true
			}
		}
	}
	return false
}

func clearAllModels(db *sql.DB) error {
	// 检查数据库结构并选择合适的查询语句
	if hasDeletedAtColumn(db) {
		// 如果有 deleted_at 字段，使用带条件的更新
		_, err := db.Exec("UPDATE channels SET models = '' WHERE deleted_at IS NULL")
		if err != nil {
			return err
		}
	} else {
		// 如果没有 deleted_at 字段，使用全局更新
		_, err := db.Exec("UPDATE channels SET models = ''")
		if err != nil {
			return err
		}
	}
	return nil
}

func getQueryForChannels(db *sql.DB) string {
	// 根据数据库结构选择合适的查询语句
	if hasDeletedAtColumn(db) {
		return "SELECT id, name, type, key, base_url, models FROM channels WHERE deleted_at IS NULL"
	}
	return "SELECT id, name, type, key, base_url, models FROM channels"
}

func main() {
	flag.StringVar(&dbPath, "db", "/root/onehub/one-api.db", "数据库文件路径")
	flag.BoolVar(&clearAll, "clear-all", false, "清除所有渠道的模型")
	flag.BoolVar(&debugMode, "debug", false, "启用调试模式")
	flag.BoolVar(&helpMode, "help", false, "显示帮助信息")
	flag.Parse()

	if helpMode {
		printHelp()
	}

	if clearAll {
		fmt.Println("正在清除所有渠道的模型...")
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			log.Fatalf("无法打开数据库 [%s]: %v", dbPath, err)
		}
		defer db.Close()

		err = clearAllModels(db)
		if err != nil {
			log.Fatalf("清除模型失败: %v", err)
		}
		fmt.Println("所有渠道的模型已成功清除！")
		os.Exit(0)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("无法打开数据库 [%s]: %v", dbPath, err)
	}
	defer db.Close()

	// 根据数据库结构选择查询语句
	query := getQueryForChannels(db)
	
	rows, err := db.Query(query)
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Key, &c.BaseURL, &c.OldModels); err == nil {
			channels = append(channels, c)
		}
	}

	if debugMode {
		fmt.Printf("检测到 %d 个渠道，开始批量更新...\n\n", len(channels))
	} else {
		fmt.Printf("正在处理 %d 个渠道...\n\n", len(channels))
	}

	progress := &Progress{total: len(channels)}

	// 创建状态管理器，显示最多5个工作线程的状态
	statusManager := NewStatusManager(5)

	// 创建变更记录管理器
	changeRecordManager := &ChangeRecordManager{}

	// 限制并发数为5
	semaphore := make(chan struct{}, 5)
	var wg sync.WaitGroup
	
	// 启动一个goroutine来定期刷新显示
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				current, total := progress.GetProgress()
				
				// 清屏并重新绘制
				statusManager.Refresh()
				drawProgress(current, total)
				
				// 显示最近的变更记录（中间部分）
				changeRecordManager.Display()
				
				// 显示固定统计信息（最下面）
				fmt.Printf("\n=== 统计摘要 ===\n")
				fmt.Printf("已处理渠道: %d/%d\n", current, total)
				fmt.Printf("变更记录总数: %d\n", len(changeRecordManager.records))
			}
		}
	}()

	for i, c := range channels {
		wg.Add(1)
		go func(chanInfo Channel, workerID int) {
			defer wg.Done()
			
			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			processChannel(db, chanInfo, progress, statusManager, workerID%5, changeRecordManager)
		}(c, i)
	}

	wg.Wait()
	
	// 最终显示
	current, total := progress.GetProgress()
	statusManager.Refresh()
	drawProgress(current, total)
	
	// 显示最终的变更记录
	changeRecordManager.Display()
	
	// 显示最终统计信息
	fmt.Printf("\n=== 最终统计摘要 ===\n")
	fmt.Printf("已处理渠道: %d/%d\n", current, total)
	fmt.Printf("变更记录总数: %d\n", len(changeRecordManager.records))
	
	fmt.Println("\n\n所有任务已完成。")
	os.Exit(0)
}