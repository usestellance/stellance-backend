package payments

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/stellar/go/amount"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon/operations"
)

const pageLimit = 200

type chainPage struct {
	Payments   []chainPayment
	NextCursor string
	Fetched    int
}

type chainClient interface {
	AccountPayments(ctx context.Context, account, cursor string) (chainPage, error)
	TransactionPayments(ctx context.Context, hash string) ([]chainPayment, error)
}

type horizonChain struct {
	client *horizonclient.Client
}

func (h horizonChain) AccountPayments(ctx context.Context, account, cursor string) (chainPage, error) {
	page, err := h.client.Operations(horizonclient.OperationRequest{
		ForAccount:    account,
		Cursor:        cursor,
		Order:         horizonclient.OrderAsc,
		Limit:         pageLimit,
		IncludeFailed: true,
		Join:          "transactions",
	})
	if err != nil {
		return chainPage{}, fmt.Errorf("horizon operations for %s: %w", account, err)
	}

	out := chainPage{NextCursor: cursor, Fetched: len(page.Embedded.Records)}
	for _, op := range page.Embedded.Records {
		out.NextCursor = op.PagingToken()
		p, ok := toChainPayment(op)
		if ok && p.To == account {
			out.Payments = append(out.Payments, p)
		}
	}
	return out, nil
}

func (h horizonChain) TransactionPayments(ctx context.Context, hash string) ([]chainPayment, error) {
	page, err := h.client.Operations(horizonclient.OperationRequest{
		ForTransaction: hash,
		Limit:          pageLimit,
		IncludeFailed:  true,
		Join:           "transactions",
	})
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return nil, ErrTransactionNotFound
		}
		return nil, fmt.Errorf("horizon operations for transaction %s: %w", hash, err)
	}

	var out []chainPayment
	for _, op := range page.Embedded.Records {
		if p, ok := toChainPayment(op); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// validMemo rejects memos Postgres cannot store as text: payers control memo bytes.
func validMemo(m string) bool {
	return utf8.ValidString(m) && !strings.ContainsRune(m, 0)
}

// toChainPayment returns false for operations that do not move an asset to a destination account.
func toChainPayment(op operations.Operation) (chainPayment, bool) {
	var pay operations.Payment
	switch v := op.(type) {
	case operations.Payment:
		pay = v
	case operations.PathPayment:
		pay = v.Payment
	case operations.PathPaymentStrictSend:
		pay = v.Payment
	default:
		return chainPayment{}, false
	}

	stroops, err := amount.ParseInt64(pay.Amount)
	if err != nil {
		return chainPayment{}, false
	}

	a := asset{Code: pay.Asset.Code, Issuer: pay.Asset.Issuer}
	if pay.Asset.Type == "native" {
		a = asset{Code: "XLM"}
	}

	p := chainPayment{
		OperationID: pay.ID,
		TxHash:      pay.TransactionHash,
		From:        pay.From,
		To:          pay.To,
		Asset:       a,
		Stroops:     stroops,
		Successful:  pay.TransactionSuccessful,
		ClosedAt:    pay.LedgerCloseTime,
	}
	if pay.Transaction != nil && pay.Transaction.MemoType == "text" && validMemo(pay.Transaction.Memo) {
		p.Memo = pay.Transaction.Memo
	}
	return p, true
}

// resolveNetwork reads the admin-selected network from system_config, falling back to the STELLAR_STAGE env.
func (s *PaymentService) resolveNetwork(ctx context.Context) (chainClient, string, error) {
	stage := utils.GetStellarStage()
	var encrypted string
	if err := s.postgres.QueryRow(ctx, `SELECT value FROM system_config WHERE key = 'stellar_network'`).Scan(&encrypted); err == nil {
		if v, derr := utils.DecryptValue(encrypted); derr == nil {
			stage = v
		}
	}

	url := os.Getenv("TESTNET_NETWORK_URL")
	if stage == "mainnet" {
		url = os.Getenv("MAINNET_NETWORK_URL")
	} else {
		stage = "testnet"
	}
	if url == "" {
		return nil, "", fmt.Errorf("horizon url not configured for %s", stage)
	}
	return horizonChain{client: &horizonclient.Client{HorizonURL: url}}, stage, nil
}

func expectedAsset(stage, currency string) asset {
	if strings.EqualFold(currency, "xlm") {
		return asset{Code: "XLM"}
	}
	return asset{Code: "USDC", Issuer: utils.USDCIssuer(stage)}
}

func explorerTxURL(stage, hash string) string {
	if stage == "mainnet" {
		return "https://stellar.expert/explorer/public/tx/" + hash
	}
	return "https://stellar.expert/explorer/testnet/tx/" + hash
}

// EnsureNetworkConfig seeds the admin network setting from the STELLAR_STAGE environment when it is unset.
func (s *PaymentService) EnsureNetworkConfig(ctx context.Context) error {
	enc, err := utils.EncryptValue(utils.GetStellarStage())
	if err != nil {
		return err
	}
	_, err = s.postgres.Exec(ctx, `INSERT INTO system_config (key, value) VALUES ('stellar_network', $1) ON CONFLICT (key) DO NOTHING`, enc)
	return err
}
