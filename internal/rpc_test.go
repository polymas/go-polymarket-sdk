package internal

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

const rpcTestTimeout = 10 * time.Second

// TestPolygonRPCMainnetList_Availability 验证 constants 中主网 RPC 列表各节点是否可用（需网络，-short 时跳过）。
func TestPolygonRPCMainnetList_Availability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping RPC availability test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), rpcTestTimeout)
	defer cancel()

	for i, url := range PolygonRPCMainnetList {
		name := url
		t.Run(name, func(t *testing.T) {
			client, err := ethclient.DialContext(ctx, url)
			if err != nil {
				t.Errorf("Dial: %v", err)
				return
			}
			defer client.Close()

			chainID, err := client.ChainID(ctx)
			if err != nil {
				t.Errorf("ChainID: %v", err)
				return
			}
			if chainID.Uint64() != uint64(Polygon) {
				t.Errorf("ChainID = %s, want Polygon (137)", chainID.String())
				return
			}

			blockNum, err := client.BlockNumber(ctx)
			if err != nil {
				t.Errorf("BlockNumber: %v", err)
				return
			}
			t.Logf("[%d] %s OK (chainId=137, block=%d)", i+1, url, blockNum)
		})
	}
}

// TestPolygonWSSMainnetList_Availability 验证 constants 中主网 WSS 节点列表是否可用（需网络，-short 时跳过）。
func TestPolygonWSSMainnetList_Availability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping WSS availability test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), rpcTestTimeout)
	defer cancel()

	for i, url := range PolygonWSSMainnetList {
		name := url
		t.Run(name, func(t *testing.T) {
			client, err := ethclient.DialContext(ctx, url)
			if err != nil {
				t.Errorf("Dial: %v", err)
				return
			}
			defer client.Close()

			chainID, err := client.ChainID(ctx)
			if err != nil {
				t.Errorf("ChainID: %v", err)
				return
			}
			if chainID.Uint64() != uint64(Polygon) {
				t.Errorf("ChainID = %s, want Polygon (137)", chainID.String())
				return
			}

			blockNum, err := client.BlockNumber(ctx)
			if err != nil {
				t.Errorf("BlockNumber: %v", err)
				return
			}
			t.Logf("[%d] %s OK (chainId=137, block=%d)", i+1, url, blockNum)
		})
	}
}
