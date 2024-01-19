package config

import (
	"fmt"

	oracletypes "github.com/skip-mev/slinky/x/oracle/types"
)

// MarketConfig represents the provider specific configurations for different
// currency pairs and the corresponding markets they are traded on.
type MarketConfig struct {
	// Name identifies which provider this config is for.
	Name string `mapstructure:"name" toml:"name"`

	// CurrencyPairToMarketConfigs is the config the provider uses to create mappings
	// between on-chain and off-chain currency pairs. In particular, this config
	// maps the on-chain currency pair representation (i.e. BITCOIN/USD) to the
	// off-chain currency pair representation (i.e. BTC/USD).
	CurrencyPairToMarketConfigs map[string]CurrencyPairMarketConfig `mapstructure:"currency_pair_to_market_configs" toml:"currency_pair_to_market_configs"`
}

// CurrencyPairMarketConfig is the config the provider uses to create mappings
// between on-chain and off-chain currency pairs.
type CurrencyPairMarketConfig struct {
	// Ticker is the ticker symbol for the currency pair.
	Ticker string `mapstructure:"ticker" toml:"ticker"`

	// CurrencyPair is the on-chain representation of the currency pair.
	CurrencyPair oracletypes.CurrencyPair `mapstructure:"currency_pair" toml:"currency_pair"`
}

// InvertedCurrencyPairMarketConfig is the config the provider uses to create mappings
// between on-chain and off-chain currency pairs.
type InvertedCurrencyPairMarketConfig struct {
	// Name identifies which provider this config is for.
	Name string

	// MarketToCurrencyPairConfigs is the config the provider uses to create mappings
	// between on-chain and off-chain currency pairs. In particular, this config
	// maps the off-chain currency pair representation (i.e. BTC/USD) to the
	// on-chain currency pair representation (i.e. BITCOIN/USD).
	MarketToCurrencyPairConfigs map[string]CurrencyPairMarketConfig
}

// NewMarketConfig returns a new MarketConfig instance.
func NewMarketConfig() MarketConfig {
	return MarketConfig{
		CurrencyPairToMarketConfigs: make(map[string]CurrencyPairMarketConfig),
	}
}

// Invert returns the inverted currency pair market config. This is used to
// create the inverse currency pair market config for the provider.
func (c *MarketConfig) Invert() InvertedCurrencyPairMarketConfig {
	invertedConfig := InvertedCurrencyPairMarketConfig{
		Name:                        c.Name,
		MarketToCurrencyPairConfigs: make(map[string]CurrencyPairMarketConfig),
	}

	for _, marketConfig := range c.CurrencyPairToMarketConfigs {
		invertedConfig.MarketToCurrencyPairConfigs[marketConfig.Ticker] = marketConfig
	}

	return invertedConfig
}

func (c *MarketConfig) ValidateBasic() error {
	if len(c.Name) == 0 {
		return fmt.Errorf("name cannot be empty")
	}

	for cpStr, marketConfig := range c.CurrencyPairToMarketConfigs {
		cp, err := oracletypes.CurrencyPairFromString(cpStr)
		if err != nil {
			return fmt.Errorf("currency pair is not formatted correctly %w", err)
		}

		if err := marketConfig.ValidateBasic(); err != nil {
			return fmt.Errorf("market config is not formatted correctly %w", err)
		}

		// Update the correctly formatted currency pair string.
		delete(c.CurrencyPairToMarketConfigs, cpStr)
		c.CurrencyPairToMarketConfigs[cp.ToString()] = marketConfig
	}

	return nil
}

func (c *CurrencyPairMarketConfig) ValidateBasic() error {
	if len(c.Ticker) == 0 {
		return fmt.Errorf("ticker cannot be empty")
	}

	return nil
}