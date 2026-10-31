// Package balance reads wallet balances straight from Horizon. Balances are never stored.
package balance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/stellar/go/clients/horizonclient"
)

const requestTimeout = 8 * time.Second

type Balance struct {
	USDC float64 `json:"usdc" example:"5.03"`
	XLM  float64 `json:"xlm" example:"10002"`
}

// Fetch returns the live balance of a Stellar account. An account that does not exist on chain has a zero balance.
func Fetch(ctx context.Context, address string) (*Balance, error) {
	stage := utils.GetStellarStage()
	url := os.Getenv("TESTNET_NETWORK_URL")
	if stage == "mainnet" {
		url = os.Getenv("MAINNET_NETWORK_URL")
	}
	if url == "" {
		return nil, errors.New("horizon url is not configured")
	}

	client := &horizonclient.Client{HorizonURL: url, HTTP: &http.Client{Timeout: requestTimeout}}
	account, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: address})
	if err != nil {
		var hErr *horizonclient.Error
		if errors.As(err, &hErr) && hErr.Response.StatusCode == http.StatusNotFound {
			return &Balance{}, nil
		}
		return nil, fmt.Errorf("horizon account %s: %w", address, err)
	}

	issuer := utils.USDCIssuer(stage)
	var b Balance
	for _, bal := range account.Balances {
		v, perr := strconv.ParseFloat(bal.Balance, 64)
		if perr != nil {
			return nil, fmt.Errorf("horizon balance %q: %w", bal.Balance, perr)
		}
		switch {
		case bal.Asset.Type == "native":
			b.XLM = v
		case bal.Asset.Code == "USDC" && bal.Asset.Issuer == issuer:
			b.USDC = v
		}
	}
	return &b, nil
}
