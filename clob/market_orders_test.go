package clob

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const marketOrderTestPrivateKey = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

func marketTestBook() *types.OrderBookSummary {
	return &types.OrderBookSummary{
		TokenID:      "111",
		TickSize:     types.TickSize0_01,
		MinOrderSize: "5",
		Bids: []types.OrderLevel{
			{Price: "0.40", Size: "100"},
			{Price: "0.45", Size: "100"},
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.OrderLevel{
			{Price: "0.60", Size: "100"},
			{Price: "0.55", Size: "100"},
			{Price: "0.50", Size: "100"},
		},
	}
}

func TestCalculateMarketPriceFromBookMatchesOfficialAlgorithm(t *testing.T) {
	book := marketTestBook()
	tests := []struct {
		name      string
		side      types.OrderSide
		amount    float64
		orderType types.OrderType
		want      float64
		wantErr   bool
		available float64
	}{
		{name: "buy FOK reaches second ask", side: types.OrderSideBUY, amount: 75, orderType: types.OrderTypeFOK, want: 0.55},
		{name: "sell FOK reaches second bid", side: types.OrderSideSELL, amount: 150, orderType: types.OrderTypeFOK, want: 0.45},
		{name: "buy FAK partial uses worst ask", side: types.OrderSideBUY, amount: 200, orderType: types.OrderTypeFAK, want: 0.60},
		{name: "sell FAK partial uses worst bid", side: types.OrderSideSELL, amount: 400, orderType: types.OrderTypeFAK, want: 0.40},
		{name: "zero order type defaults FOK", side: types.OrderSideBUY, amount: 50, want: 0.50},
		{name: "buy FOK insufficient", side: types.OrderSideBUY, amount: 200, orderType: types.OrderTypeFOK, wantErr: true, available: 165},
		{name: "sell FOK insufficient", side: types.OrderSideSELL, amount: 301, orderType: types.OrderTypeFOK, wantErr: true, available: 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateMarketPriceFromBook(book, tt.side, tt.amount, tt.orderType)
			if tt.wantErr {
				var liquidityErr *InsufficientLiquidityError
				if !errors.As(err, &liquidityErr) {
					t.Fatalf("error = %v, want InsufficientLiquidityError", err)
				}
				if math.Abs(liquidityErr.Available-tt.available) > 1e-9 {
					t.Fatalf("available = %g, want %g", liquidityErr.Available, tt.available)
				}
				return
			}
			if err != nil {
				t.Fatalf("calculateMarketPriceFromBook: %v", err)
			}
			if got != tt.want {
				t.Fatalf("price = %g, want %g", got, tt.want)
			}
		})
	}
}

func TestCalculateMarketPriceFromBookRejectsInvalidInputs(t *testing.T) {
	book := marketTestBook()
	tests := []struct {
		name      string
		book      *types.OrderBookSummary
		side      types.OrderSide
		amount    float64
		orderType types.OrderType
		want      string
	}{
		{name: "nil book", side: types.OrderSideBUY, amount: 5, want: "nil"},
		{name: "invalid side", book: book, side: "HOLD", amount: 5, want: "BUY or SELL"},
		{name: "invalid amount", book: book, side: types.OrderSideBUY, amount: math.NaN(), want: "finite"},
		{name: "invalid type", book: book, side: types.OrderSideBUY, amount: 5, orderType: types.OrderTypeGTC, want: "FOK or FAK"},
		{name: "empty asks", book: &types.OrderBookSummary{}, side: types.OrderSideBUY, amount: 5, want: "insufficient"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := calculateMarketPriceFromBook(tt.book, tt.side, tt.amount, tt.orderType)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestCalculateMarketOrderAmounts(t *testing.T) {
	tests := []struct {
		name      string
		side      types.OrderSide
		amount    float64
		price     float64
		tick      types.TickSize
		protected bool
		wantMaker string
		wantTaker string
	}{
		{name: "buy official 4 decimal floor", side: types.OrderSideBUY, amount: 5, price: 0.37, tick: types.TickSize0_01, wantMaker: "5000000", wantTaker: "13513500"},
		{name: "buy protected rounds in safe direction", side: types.OrderSideBUY, amount: 5, price: 0.37, tick: types.TickSize0_01, protected: true, wantMaker: "5000000", wantTaker: "13513600"},
		{name: "buy tick 0.001 keeps official 5 decimals", side: types.OrderSideBUY, amount: 5, price: 0.998, tick: types.TickSize0_001, wantMaker: "5000000", wantTaker: "5010020"},
		{name: "buy protected tick 0.001 rounds up at 5", side: types.OrderSideBUY, amount: 5, price: 0.998, tick: types.TickSize0_001, protected: true, wantMaker: "5000000", wantTaker: "5010030"},
		{name: "tick 0.1 keeps official 3 decimals", side: types.OrderSideBUY, amount: 5, price: 0.3, tick: types.TickSize0_1, wantMaker: "5000000", wantTaker: "16666000"},
		{name: "sell tick 0.0001 keeps official 6 decimals", side: types.OrderSideSELL, amount: 5.01, price: 0.3333, tick: types.TickSize0_0001, protected: true, wantMaker: "5010000", wantTaker: "1669833"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maker, taker, err := calculateMarketOrderAmounts(tt.side, tt.amount, tt.price, tt.tick, tt.protected)
			if err != nil {
				t.Fatalf("calculateMarketOrderAmounts: %v", err)
			}
			if maker.String() != tt.wantMaker || taker.String() != tt.wantTaker {
				t.Fatalf("amounts = %s/%s, want %s/%s", maker, taker, tt.wantMaker, tt.wantTaker)
			}
		})
	}
}

func TestPrepareProtectedMarketOrderSkipsOrderBook(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "must not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	negRisk := false
	client := &orderClientImpl{baseClient: &baseClient{baseURL: server.URL}}
	prepared, err := client.prepareMarketOrder(types.MarketOrderArgs{
		TokenID:  "111",
		Side:     types.OrderSideBUY,
		Amount:   5,
		MaxPrice: 0.50,
		TickSize: types.TickSize0_01,
		NegRisk:  &negRisk,
	})
	if err != nil {
		t.Fatalf("prepareMarketOrder: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("protected market order made %d HTTP requests", requests.Load())
	}
	if prepared.orderType != types.OrderTypeFOK || prepared.orderArgs.Price != 0.50 {
		t.Fatalf("prepared = %+v", prepared)
	}
}

func TestPrepareUnprotectedMarketOrderUsesBookAndRejectsStaleTick(t *testing.T) {
	book := marketTestBook()
	book.NegRisk = true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/book" || r.URL.Query().Get("token_id") != "111" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(book)
	}))
	defer server.Close()

	client := &orderClientImpl{baseClient: &baseClient{baseURL: server.URL}}
	prepared, err := client.prepareMarketOrder(types.MarketOrderArgs{
		TokenID:   "111",
		Side:      types.OrderSideSELL,
		Shares:    150,
		TickSize:  types.TickSize0_01,
		OrderType: types.OrderTypeFOK,
	})
	if err != nil {
		t.Fatalf("prepareMarketOrder: %v", err)
	}
	if prepared.orderArgs.Price != 0.45 || prepared.orderArgs.NegRisk == nil || !*prepared.orderArgs.NegRisk {
		t.Fatalf("prepared order = %+v", prepared.orderArgs)
	}

	_, err = client.prepareMarketOrder(types.MarketOrderArgs{
		TokenID:   "111",
		Side:      types.OrderSideSELL,
		Shares:    150,
		TickSize:  types.TickSize0_001,
		OrderType: types.OrderTypeFOK,
	})
	if err == nil || !strings.Contains(err.Error(), "stale tick_size") {
		t.Fatalf("stale tick error = %v", err)
	}
}

func TestPrepareMarketOrderValidatesSideSpecificFieldsAndMinimum(t *testing.T) {
	client := &orderClientImpl{baseClient: &baseClient{}}
	tests := []struct {
		name string
		args types.MarketOrderArgs
		want string
	}{
		{name: "BUY shares forbidden", args: types.MarketOrderArgs{TokenID: "111", Side: types.OrderSideBUY, Amount: 5, Shares: 5, MaxPrice: .5, TickSize: types.TickSize0_01}, want: "zero Shares"},
		{name: "SELL amount forbidden", args: types.MarketOrderArgs{TokenID: "111", Side: types.OrderSideSELL, Shares: 5, Amount: 5, MinPrice: .5, TickSize: types.TickSize0_01}, want: "zero Amount"},
		{name: "wrong guard", args: types.MarketOrderArgs{TokenID: "111", Side: types.OrderSideBUY, Amount: 5, MinPrice: .5, TickSize: types.TickSize0_01}, want: "MinPrice"},
		{name: "off grid guard", args: types.MarketOrderArgs{TokenID: "111", Side: types.OrderSideBUY, Amount: 5, MaxPrice: .505, TickSize: types.TickSize0_01}, want: "not aligned"},
		{name: "GTC forbidden", args: types.MarketOrderArgs{TokenID: "111", Side: types.OrderSideBUY, Amount: 5, MaxPrice: .5, TickSize: types.TickSize0_01, OrderType: types.OrderTypeGTC}, want: "FOK or FAK"},
		{name: "below five shares", args: types.MarketOrderArgs{TokenID: "111", Side: types.OrderSideBUY, Amount: 4.94, MaxPrice: .99, TickSize: types.TickSize0_01}, want: "below default minimum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.prepareMarketOrder(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestCreateAndPostMarketOrderInstantUsesMarketAmountWireValues(t *testing.T) {
	var posted []orderRequestV2
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders" || r.Method != http.MethodPost {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"success":true,"orderID":"0x1111111111111111111111111111111111111111111111111111111111111111","status":"matched","makingAmount":"5000000","takingAmount":"13513600","errorMsg":""}]`))
	}))
	defer server.Close()

	w3, err := web3.NewClient(marketOrderTestPrivateKey, types.EOASignatureType, types.Polygon, server.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer w3.Close()
	base := &baseClient{
		address:       w3.GetBaseAddress(),
		proxyAddress:  w3.GetBaseAddress(),
		baseURL:       server.URL,
		signatureType: types.EOASignatureType,
		deriveCreds: &types.ApiCreds{
			Key:        "00000000-0000-0000-0000-000000000001",
			Secret:     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			Passphrase: "test-passphrase",
		},
		negRisk:    make(map[string]bool),
		web3Client: w3,
	}
	client := &orderClientImpl{baseClient: base}
	negRisk := false
	response, err := client.CreateAndPostMarketOrderInstant(types.MarketOrderArgs{
		TokenID:   "111",
		Side:      types.OrderSideBUY,
		Amount:    5,
		MaxPrice:  0.37,
		TickSize:  types.TickSize0_01,
		OrderType: types.OrderTypeFAK,
		NegRisk:   &negRisk,
	})
	if err != nil {
		t.Fatalf("CreateAndPostMarketOrderInstant: %v", err)
	}
	if response == nil || !response.Accepted() {
		t.Fatalf("response = %+v", response)
	}
	if len(posted) != 1 {
		t.Fatalf("posted %d orders, want 1", len(posted))
	}
	request := posted[0]
	if request.Order.MakerAmount != "5000000" || request.Order.TakerAmount != "13513600" {
		t.Fatalf("wire amounts = %s/%s, want 5000000/13513600", request.Order.MakerAmount, request.Order.TakerAmount)
	}
	if request.OrderType != string(types.OrderTypeFAK) || request.PostOnly || request.DeferExec {
		t.Fatalf("wire execution fields = %+v", request)
	}
}
