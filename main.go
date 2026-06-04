package main

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// ==================== Webhook 接口 ====================

func arkhamWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST 方法", http.StatusMethodNotAllowed)
		return
	}

	var webhook ArkhamWebhook

	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		http.Error(w, "JSON 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 控制台打印关键信息（方便调试）
	fmt.Printf("\n=== Arkham Webhook 收到新数据 ===\n")
	fmt.Printf("警报名称: %s\n", webhook.AlertName)
	fmt.Printf("交易哈希: %s\n", webhook.Transfer.TransactionHash)
	fmt.Printf("From: %s (%s)\n", webhook.Transfer.FromAddress.Address, webhook.Transfer.FromAddress.ArkhamEntity.Name)
	fmt.Printf("To:   %s (%s)\n", webhook.Transfer.ToAddress.Address, webhook.Transfer.ToAddress.ArkhamEntity.Name)
	fmt.Printf("Token: %s (%s) 数量: %.6f  USD价值: %.2f\n",
		webhook.Transfer.TokenSymbol, webhook.Transfer.TokenName,
		webhook.Transfer.UnitValue, webhook.Transfer.HistoricalUSD)
	fmt.Printf("链: %s | 区块: %d\n", webhook.Transfer.Chain, webhook.Transfer.BlockNumber)

	// 返回完整结构体
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(webhook)
}

func main() {
	http.HandleFunc("/webhook", arkhamWebhookHandler)

	fmt.Println("Arkham Webhook 服务启动成功 → http://localhost:8080/webhook")
	fmt.Println("请使用 ngrok 暴露此端口后填入 Arkham")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
