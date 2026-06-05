package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ArkhamWebhook Arkham 平台 Webhook 推送的完整结构体
type ArkhamWebhook struct {
	Transfer  Transfer `json:"transfer"`  // 交易核心数据
	AlertName string   `json:"alertName"` // 警报名称（你在 Arkham 设置的名称）
	ID        int      `json:"id"`        // 警报 ID
}

// Transfer 交易详细信息
type Transfer struct {
	ID              string      `json:"id"`              // 内部 transfer ID
	TransactionHash string      `json:"transactionHash"` // 交易哈希
	FromAddress     AddressInfo `json:"fromAddress"`     // 发送方地址信息
	FromIsContract  bool        `json:"fromIsContract"`  // 发送方是否为合约
	ToAddress       AddressInfo `json:"toAddress"`       // 接收方地址信息
	ToIsContract    bool        `json:"toIsContract"`    // 接收方是否为合约
	TokenAddress    string      `json:"tokenAddress"`    // Token 合约地址
	Type            string      `json:"type"`            // 交易类型（token / native 等）
	BlockTimestamp  string      `json:"blockTimestamp"`  // 区块时间戳
	BlockNumber     int         `json:"blockNumber"`     // 区块高度
	BlockHash       string      `json:"blockHash"`       // 区块哈希
	TokenName       string      `json:"tokenName"`       // Token 名称
	TokenSymbol     string      `json:"tokenSymbol"`     // Token 符号
	TokenDecimals   int         `json:"tokenDecimals"`   // Token 小数位
	UnitValue       float64     `json:"unitValue"`       // 转账数量（原始单位）
	TokenID         string      `json:"tokenId"`         // Token 在 Arkham 的 ID（如 usd-coin）
	HistoricalUSD   float64     `json:"historicalUSD"`   // 历史 USD 价值
	Chain           string      `json:"chain"`           // 链名称（ethereum / solana 等）
}

// AddressInfo 地址详细信息（from 和 to 共用）
type AddressInfo struct {
	Address       string       `json:"address"`       // 地址
	Chain         string       `json:"chain"`         // 链名称
	ArkhamEntity  ArkhamEntity `json:"arkhamEntity"`  // Arkham 实体信息（机构、项目等）
	ArkhamLabel   ArkhamLabel  `json:"arkhamLabel"`   // Arkham 标签信息
	IsUserAddress bool         `json:"isUserAddress"` // 是否为用户自己的地址
	Contract      bool         `json:"contract"`      // 是否为合约地址
}

// ArkhamEntity Arkham 实体信息（基金、DEX、项目方等）
type ArkhamEntity struct {
	Name       string `json:"name"`       // 实体名称
	Note       string `json:"note"`       // 实体备注/描述
	ID         string `json:"id"`         // 实体 ID
	Type       string `json:"type"`       // 实体类型（如 fund, dex, market_maker）
	Service    any    `json:"service"`    // 服务信息（可能为空）
	Addresses  any    `json:"addresses"`  // 地址列表（可能为空）
	Website    string `json:"website"`    // 官网
	Twitter    string `json:"twitter"`    // Twitter 链接
	Crunchbase string `json:"crunchbase"` // Crunchbase 链接
	Linkedin   string `json:"linkedin"`   // LinkedIn 链接
}

// ArkhamLabel Arkham 标签信息
type ArkhamLabel struct {
	Name      string `json:"name"`      // 标签名称
	Address   string `json:"address"`   // 地址
	ChainType string `json:"chainType"` // 链类型（evm 等）
}

// 全局去重缓存（简单版，生产建议换 Redis）
var processed = struct {
	sync.RWMutex

	m map[string]time.Time
}{m: make(map[string]time.Time)}

func isProcessed(id string) bool {
	processed.RLock()
	t, exists := processed.m[id]
	processed.RUnlock()

	if exists && time.Since(t) < 30*time.Minute { // 30分钟内视为重复
		return true
	}
	return false
}

func markProcessed(id string) {
	processed.Lock()
	processed.m[id] = time.Now()
	processed.Unlock()
}

// ==================== Webhook 接口 ====================

func arkhamWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST 方法", http.StatusMethodNotAllowed)
		return
	}

	var webhook ArkhamWebhook

	// 限制请求体大小为 1MB
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&webhook); err != nil {
		http.Error(w, "JSON 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	// ==================== 去重核心逻辑 ====================
	key := fmt.Sprintf("%d-%s", webhook.ID, webhook.Transfer.TransactionHash)
	if isProcessed(key) {
		fmt.Printf("⚠️ 重复请求，已忽略: %s\n", key)
		w.WriteHeader(http.StatusOK)
		return
	}
	markProcessed(key)
	// ====================================================

	fmt.Printf("✅ 处理新推送 → Alert: %s | Tx: %s\n", webhook.AlertName, webhook.Transfer.TransactionHash)

	// 控制台打印关键信息（方便调试）
	fmt.Printf("\n=== Arkham Webhook 收到新数据 ===\n")
	fmt.Printf("警报名称:	%s\n", webhook.AlertName)
	fmt.Printf("交易哈希: %s\n", webhook.Transfer.TransactionHash)
	fmt.Printf("From: %s (%s)\n", webhook.Transfer.FromAddress.Address, webhook.Transfer.FromAddress.ArkhamEntity.Name)
	fmt.Printf("To:   %s (%s)\n", webhook.Transfer.ToAddress.Address, webhook.Transfer.ToAddress.ArkhamEntity.Name)
	fmt.Printf("Token: %s (%s) 数量: %.6f  USD价值: %.2f\n",
		webhook.Transfer.TokenSymbol, webhook.Transfer.TokenName,
		webhook.Transfer.UnitValue, webhook.Transfer.HistoricalUSD)
	fmt.Printf("链: %s | 区块: %d\n", webhook.Transfer.Chain, webhook.Transfer.BlockNumber)

	fmt.Printf("交易时间: %s\n", getTime(webhook.Transfer.BlockTimestamp))
	// fmt.Println("✅ 准备推送钉钉测试啊...")
	// 发送到钉钉
	sendToDingTalk(webhook)

	fmt.Println("✅ 钉钉推送完成")

	// 返回完整结构体
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(webhook)
}

// ==================== 发送钉钉消息 ====================

func sendToDingTalk(webhook ArkhamWebhook) {
	dingURL := os.Getenv("DINGTALK_WEBHOOK")
	dingURL = "https://oapi.dingtalk.com/robot/send?access_token=e9bc0cc7028066e180fd4b80886af0db605d33d707e2bc17012f2eb4cdd61912"
	if dingURL == "" {
		fmt.Println("警告：未设置 DINGTALK_WEBHOOK 环境变量，跳过钉钉推送")
		return
	}

	// **🔗 交易详情**
	// - 交易哈希：%s
	// - 区块高度：%d

	// 构造钉钉 Markdown 消息
	text := fmt.Sprintf(`### 🚨🚨🚨MUD跨链信息🚨🚨🚨  

---

**📋 基本信息**
- 警报名称：%s
- 链：%s
- 时间：%s

**💸 资产转移**
- 交易哈希：%s
- From：%s 
- To：%s 

**💰 Token 信息**
- Token：%s (%s)
- 数量：%.6f %s
- USD 价值：$%.2f
`,
		webhook.AlertName,
		webhook.Transfer.Chain,
		getTime(webhook.Transfer.BlockTimestamp),
		webhook.Transfer.TransactionHash,
		// webhook.Transfer.BlockNumber,
		webhook.Transfer.FromAddress.Address,
		// webhook.Transfer.FromAddress.ArkhamEntity.Name,
		webhook.Transfer.ToAddress.Address,
		// webhook.Transfer.ToAddress.ArkhamEntity.Name,
		webhook.Transfer.TokenSymbol,
		webhook.Transfer.TokenName,
		webhook.Transfer.UnitValue,
		webhook.Transfer.TokenSymbol,
		webhook.Transfer.HistoricalUSD,
	)

	message := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": "Arkham 交易警报",
			"text":  text,
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		fmt.Println("JSON 序列化失败:", err)
		return
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Post(dingURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		fmt.Println("钉钉推送失败:", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("钉钉推送返回非 200: %d, 响应: %s\n", resp.StatusCode, string(respBody))
		return
	}

	fmt.Println("✅ 已成功推送到钉钉")
}

// 改为 本地时间区
func getTime(timeStr string) string {
	if timeStr == "" {
		return "时间解析失败"
	}

	fmt.Println("原始时间:", timeStr)

	// 1. 解析 RFC3339 时间（Arkham 返回的是 UTC）
	parsedTime, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		fmt.Println("时间解析失败:", err)
		return timeStr // 解析失败时返回原始字符串
	}

	// 2. 加载中国上海时区（东八区）
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		fmt.Println("加载时区失败:", err)
		return parsedTime.Format("2006-01-02 15:04:05")
	}

	// 3. 转换为中国时间
	chinaTime := parsedTime.In(loc)

	// 4. 格式化输出（你想要的格式）
	formatted := chinaTime.Format("2006-01-02 15:04:05")

	fmt.Println("北京时间:", formatted)
	return formatted
}

// func arkhamWebhookHandler(w http.ResponseWriter, r *http.Request) {
// if r.Method != http.MethodPost {
//     http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
//     return
// }

// var webhook ArkhamWebhook
// if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
//     http.Error(w, "解析失败", http.StatusBadRequest)
//     return
// }

// // ==================== 去重核心逻辑 ====================
// key := fmt.Sprintf("%d-%s", webhook.ID, webhook.Transfer.TransactionHash)
// if isProcessed(key) {
// 	fmt.Printf("⚠️ 重复请求，已忽略: %s\n", key)
// 	w.WriteHeader(http.StatusOK)
// 	return
// }
// markProcessed(key)
// // ====================================================

// fmt.Printf("✅ 处理新推送 → Alert: %s | Tx: %s\n", webhook.AlertName, webhook.Transfer.TransactionHash)

// sendToDingTalk(webhook)

// w.WriteHeader(http.StatusOK)
// json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
// }

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", arkhamWebhookHandler)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 优雅关闭通道
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Println("Arkham Webhook 服务启动成功 → http://localhost:8080/webhook")
		fmt.Println("请使用 ngrok 暴露此端口后填入 Arkham")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	// 等待中断信号
	<-stop
	fmt.Println("\n正在关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("服务器关闭超时: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("服务器已优雅关闭")
}
