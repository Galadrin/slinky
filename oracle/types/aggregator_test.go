package types_test

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/skip-mev/slinky/oracle/types"
	oracletypes "github.com/skip-mev/slinky/x/oracle/types"
)

var (
	btcusd = oracletypes.NewCurrencyPair("btc", "usd")

	ethusd = oracletypes.NewCurrencyPair("eth", "usd")

	usdtusd = oracletypes.NewCurrencyPair("usdt", "usd")
)

func TestComputeMedian(t *testing.T) {
	testCases := []struct {
		name           string
		providerPrices types.AggregatedProviderPrices
		expectedPrices map[oracletypes.CurrencyPair]*uint256.Int
	}{
		{
			"empty provider prices",
			types.AggregatedProviderPrices{},
			map[oracletypes.CurrencyPair]*uint256.Int{},
		},
		{
			"single provider price",
			types.AggregatedProviderPrices{
				"provider1": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(100),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(200),
					},
				},
			},
			map[oracletypes.CurrencyPair]*uint256.Int{
				btcusd: uint256.NewInt(100),
				ethusd: uint256.NewInt(200),
			},
		},
		{
			"multiple provider prices",
			types.AggregatedProviderPrices{
				"provider1": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(100),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(200),
					},
				},
				"provider2": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(200),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(300),
					},
				},
			},
			map[oracletypes.CurrencyPair]*uint256.Int{
				btcusd: uint256.NewInt(150),
				ethusd: uint256.NewInt(250),
			},
		},
		{
			"multiple provider prices with different assets",
			types.AggregatedProviderPrices{
				"provider1": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(100),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(200),
					},
				},
				"provider2": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(200),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(300),
					},
					usdtusd: types.QuotePrice{
						Price: nil, // should be ignored
					},
				},
			},
			map[oracletypes.CurrencyPair]*uint256.Int{
				btcusd: uint256.NewInt(150),
				ethusd: uint256.NewInt(250),
			},
		},
		{
			"odd number of provider prices",
			types.AggregatedProviderPrices{
				"provider1": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(100),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(200),
					},
				},
				"provider2": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(200),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(300),
					},
				},
				"provider3": {
					btcusd: types.QuotePrice{
						Price: uint256.NewInt(300),
					},
					ethusd: types.QuotePrice{
						Price: uint256.NewInt(400),
					},
				},
			},
			map[oracletypes.CurrencyPair]*uint256.Int{
				btcusd: uint256.NewInt(200),
				ethusd: uint256.NewInt(300),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			medianFn := types.ComputeMedian()
			prices := medianFn(tc.providerPrices)

			if len(prices) != len(tc.expectedPrices) {
				t.Fatalf("expected %d prices, got %d", len(tc.expectedPrices), len(prices))
			}

			for asset, expectedPrice := range tc.expectedPrices {
				price, ok := prices[asset]
				if !ok {
					t.Fatalf("expected price for asset %s", asset)
				}

				if price.Cmp(expectedPrice) != 0 {
					t.Fatalf("expected price %s, got %s", expectedPrice, price)
				}
			}
		})
	}
}
