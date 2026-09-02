package main

import (
	"fmt"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	pk := "0x9dd18f63a7af74e3265ca22fd86ebe8bbf00a8109a1c28916ded2d65a8c9a5d1"
	for _, t := range []types.SignatureType{1, 2, 3} {
		c, _ := web3.NewClient(pk, t, types.Polygon)
		eoa := c.GetBaseAddress()
		px, _ := c.GetPolyProxyAddress()
		fmt.Printf("type=%d EOA=%s proxy=%s\n", t, eoa, px)
		c.Close()
	}
}
