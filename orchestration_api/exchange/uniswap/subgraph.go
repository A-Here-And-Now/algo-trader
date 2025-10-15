//go:build uniswap

package uniswap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SubgraphConfig contains endpoint for historical backfill.
type SubgraphConfig struct {
	URL string
}

// PoolHourData represents a minimal subset of Uniswap v3 poolHourData.
type PoolHourData struct {
	PeriodStartUnix int64  `json:"periodStartUnix"`
	Open            string `json:"open"`
	High            string `json:"high"`
	Low             string `json:"low"`
	Close           string `json:"close"`
	VolumeToken0    string `json:"volumeToken0"`
	VolumeToken1    string `json:"volumeToken1"`
}

type poolHourDataResp struct {
	Data struct {
		Pool struct {
			PoolHourData []PoolHourData `json:"poolHourData"`
		} `json:"pool"`
	} `json:"data"`
	Errors any `json:"errors"`
}

// FetchPoolHourData fetches recent hour snapshots for a pool.
func FetchPoolHourData(cfg SubgraphConfig, poolAddr string, first int) ([]PoolHourData, error) {
	q := fmt.Sprintf(`{"query":"{ pool(id:\"%s\"){ poolHourData(first:%d, orderBy: periodStartUnix, orderDirection: desc){ periodStartUnix open high low close volumeToken0 volumeToken1 } } }"}`,
		toLower0x(poolAddr), first)
	req, err := http.NewRequest("POST", cfg.URL, bytes.NewBufferString(q))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out poolHourDataResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Errors != nil {
		return nil, fmt.Errorf("subgraph error: %v", out.Errors)
	}
	return out.Data.Pool.PoolHourData, nil
}

func toLower0x(s string) string {
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		return "0x" + s[2:]
	}
	return s
}
