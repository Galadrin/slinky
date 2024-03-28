package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgCreateMarkets{}
	_ sdk.Msg = &MsgUpdateMarkets{}
	_ sdk.Msg = &MsgParams{}
	_ sdk.Msg = &MsgRemoveMarketAuthorities{}
)

// ValidateBasic determines whether the information in the message is formatted correctly, specifically
// whether the signer is a valid acc-address.
func (m *MsgCreateMarkets) ValidateBasic() error {
	// validate signer address
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return err
	}

	for _, market := range m.CreateMarkets {
		if err := market.ValidateBasic(); err != nil {
			return err
		}
	}

	return nil
}

// ValidateBasic determines whether the information in the message is formatted correctly, specifically
// whether the signer is a valid acc-address.
func (m *MsgUpdateMarkets) ValidateBasic() error {
	// validate signer address
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return err
	}

	for _, market := range m.UpdateMarkets {
		if err := market.ValidateBasic(); err != nil {
			return err
		}
	}

	return nil
}

// ValidateBasic determines whether the information in the message is formatted correctly, specifically
// whether the signer is a valid acc-address.
func (m *MsgParams) ValidateBasic() error {
	// validate signer address
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return err
	}

	return m.Params.ValidateBasic()
}

// ValidateBasic determines whether the information in the message is formatted correctly, specifically
// whether the signer is a valid acc-address.
func (m *MsgRemoveMarketAuthorities) ValidateBasic() error {
	// validate signer address
	if _, err := sdk.AccAddressFromBech32(m.Admin); err != nil {
		return err
	}

	if len(m.RemoveAddresses) == 0 {
		return fmt.Errorf("addresses to remove cannot be nil")
	}

	seenAuthorities := make(map[string]struct{}, len(m.RemoveAddresses))
	for _, authority := range m.RemoveAddresses {
		if _, seen := seenAuthorities[authority]; seen {
			return fmt.Errorf("duplicate address %s found", authority)
		}

		if _, err := sdk.AccAddressFromBech32(authority); err != nil {
			return fmt.Errorf("invalid market authority string: %w", err)
		}

		seenAuthorities[authority] = struct{}{}
	}

	return nil
}
