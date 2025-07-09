package wallet

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/transactions"
	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend.git/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
)

type WalletService struct {
	log           *slog.Logger
	postgres      *pgxpool.Pool
	redis         *redis.Client
	jwt           *jwt_.JwtTokenServiceConfig
	stage         string
	networkPass   string
	secureKey     []byte
	networkURL    string
	horizonClient *horizonclient.Client
}

func NewWalletService() *WalletService {
	var url string
	var networkPassPhrase string
	stage := utils.GetStellarStage()
	if stage == "testnet" {
		url = os.Getenv("TESTNET_NETWORK_URL")
		networkPassPhrase = network.TestNetworkPassphrase
	} else {
		url = os.Getenv("MAINNET_NETWORK_URL")
		networkPassPhrase = network.PublicNetworkPassphrase
	}
	keyBytes, err := base64.StdEncoding.DecodeString(os.Getenv("ENCRYPTION_KEY_BASE64"))
	if err != nil {
		log.Fatal("Invalid base64 encryption key", err)
	}

	if len(keyBytes) != 16 && len(keyBytes) != 24 && len(keyBytes) != 32 {
		log.Fatal("Invalid key size. Must be 16, 24, or 32 bytes")
	}
	return &WalletService{
		log:           config.GetAppContainer().Log,
		postgres:      config.GetAppContainer().Postgres,
		redis:         config.GetAppContainer().Redis,
		jwt:           jwt_.JwtTokenService(),
		stage:         stage,
		secureKey:     keyBytes,
		networkURL:    url,
		networkPass:   networkPassPhrase,
		horizonClient: &horizonclient.Client{HorizonURL: url},
	}
}

func (ws *WalletService) encryptPrivateKey(seed string) (string, error) {
	block, err := aes.NewCipher(ws.secureKey)
	if err != nil {
		log.Error("error", err)
		return "", err
	}

	fmt.Println("checking....")
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", nil
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := gcm.Seal(nonce, nonce, []byte(seed), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func (ws *WalletService) decryptPrivateKey(encryptedData string) (string, error) {
	cipherText, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(ws.secureKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(cipherText) < nonceSize {
		return "", fmt.Errorf("cipher text is too short")
	}

	nonce, ciphertext := cipherText[:nonceSize], cipherText[nonceSize:]
	plainData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plainData), nil
}

func (ws *WalletService) CreateWallet(ctx context.Context, userId string) *utils.ApiResponse {
	log := ws.log

	rateLimitKey := fmt.Sprintf("wallet:create:rate:%s", userId)
	count, err := ws.redis.Incr(ctx, rateLimitKey).Result()
	if err == nil && count == 1 {
		ws.redis.Expire(ctx, rateLimitKey, 24*time.Hour)
	}

	tx, err := ws.postgres.Begin(ctx)
	if err != nil {
		log.Error("failed to start postgres transaction")
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Sever is currently unavailable, please contact support",
		}
	}
	defer tx.Rollback(ctx)

	var existingWalletCount int
	const checkWalletQ string = `
		SELECT COUNT(*)
		FROM wallets
		WHERE user_id = $1
	`
	err = tx.QueryRow(ctx, checkWalletQ, userId).Scan(&existingWalletCount)
	if err != nil {
		log.Error("failed to query existing wallet count")
		existingWalletCount = 0
	}

	if existingWalletCount >= 1 {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "You have reached your wallet creation limit, try back again",
		}
	}

	pair, err := keypair.Random()
	if err != nil {
		log.Error("failed to generate key pair")
		return &utils.ApiResponse{
			Message:    "Sever currently unavailable",
			StatusCode: http.StatusInternalServerError,
		}
	}

	encryptSeed, err := ws.encryptPrivateKey(pair.Seed())
	if err != nil {
		log.Error("failed to encrypt private key")
		return &utils.ApiResponse{
			Message:    "Server currently unavailable",
			StatusCode: http.StatusInternalServerError,
			Error:      err.Error(),
		}
	}

	if existingWalletCount == 5 {
		return &utils.ApiResponse{
			Message:    "Kindly contact support to create more wallet",
			StatusCode: http.StatusForbidden,
		}
	}

	var isPrimary bool
	if existingWalletCount == 0 {
		isPrimary = true
	}

	var wallet WalletResponseDto

	const createWalletQ string = `
	INSERT INTO wallets
	(user_id, address, private_key, tag, chain, currency, is_primary, is_active)
	VALUES
	($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING
	id, user_id, address, tag, chain, is_primary, is_active, created_at
`

	walletTag := fmt.Sprintf("wallet_%d", existingWalletCount+1)
	err = tx.QueryRow(ctx, createWalletQ,
		userId,
		pair.Address(),
		encryptSeed,
		walletTag,
		"stellar",
		"usdc",
		isPrimary,
		true,
	).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.WalletAddress,
		&wallet.Tag,
		&wallet.Chain,
		&wallet.IsPrimary,
		&wallet.IsActive,
		&wallet.CreatedAt,
	)

	if err != nil {
		log.Error("failed to create new wallet, postgres error", "error", err)
		return &utils.ApiResponse{
			Message:    "Failed to create wallet",
			StatusCode: http.StatusInternalServerError,
		}
	}

	if err = tx.Commit(ctx); err != nil {
		log.Error("failed to commit new wallet creation details", "error", err)
		return &utils.ApiResponse{
			Message:    "failed to create new wallet, kindly contact support",
			StatusCode: http.StatusInternalServerError,
		}
	}

	ws.cacheWalletInfo(ctx, &wallet)

	log.Info("wallet created successfully",
		"user_id", userId,
		"wallet_id", wallet.ID,
		"address", wallet.WalletAddress,
	)

	if ws.stage == "testnet" {
		go func() {
			ws.fundTestnetAccount(context.Background(), pair.Address(), wallet.ID, userId, pair)
		}()
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusCreated,
		Message:    "wallet created successfully",
		Data: WalletResponseDto{
			ID:            wallet.ID,
			UserID:        userId,
			WalletAddress: wallet.WalletAddress,
			PrivateKey:    pair.Seed(),
			Tag:           wallet.Tag,
			Chain:         wallet.Chain,
			IsPrimary:     wallet.IsPrimary,
			IsActive:      wallet.IsActive,
			CreatedAt:     wallet.CreatedAt,
		},
	}
}

func (ws *WalletService) GetUserWallet(ctx context.Context, userId, walletId string, role user.UserRole) *utils.ApiResponse {
	cachedKey := ws.GetWalletCacheKey(walletId)
	cached, err := ws.redis.Get(ctx, cachedKey).Result()
	if err == nil {
		var wallet WalletResponseDto
		if err := json.Unmarshal([]byte(cached), &wallet); err == nil {
			if wallet.UserID != userId && role != user.RoleAdmin {
				return &utils.ApiResponse{
					StatusCode: http.StatusForbidden,
					Message:    "Access denied",
				}
			}
			balance, _ := ws.getAccountBalance(wallet.WalletAddress)
			wallet.Balance = balance

			return &utils.ApiResponse{
				StatusCode: http.StatusOK,
				Message:    "successful",
				Data:       wallet,
			}
		}
	}

	var query string
	var args []interface{}

	if role == user.RoleAdmin {
		query = `SELECT id, user_id, address, tag, chain, is_primary, is_active, created_at 
				 FROM wallets WHERE id = $1`
		args = []interface{}{walletId}
	} else {
		query = `SELECT id, user_id, address, tag, chain, is_primary, is_active, created_at 
				 FROM wallets WHERE id = $1 AND user_id = $2`
		args = []interface{}{walletId, userId}
	}

	var wallet WalletResponseDto
	err = ws.postgres.QueryRow(ctx, query, args...).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.WalletAddress,
		&wallet.Tag,
		&wallet.Chain,
		&wallet.IsPrimary,
		&wallet.IsActive,
		&wallet.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Wallet not found",
			}
		}
		ws.log.Error("Failed to fetch wallet", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch wallet",
		}
	}
	balance, err := ws.getAccountBalance(wallet.WalletAddress)
	if err != nil {
		ws.log.Error("Failed to get account balance", "error", err, "wallet", walletId)
		balance = &StellarWalletBalance{USDC: 0, XLM: 0}
	}

	wallet.Balance = balance
	ws.cacheWalletInfo(ctx, &wallet)
	if err == nil {
		go ws.updateWalletBalance(context.Background(), walletId, balance.USDC, balance.XLM)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       wallet,
	}
}

func (ws *WalletService) fundTestnetAccount(ctx context.Context, address, walletID, userId string, pair *keypair.Full) {
	ws.log.Info("proceeding to fund test wallet")
	resp, err := ws.horizonClient.Fund(address)
	if err != nil {
		ws.log.Error("failed to fund testnet account", "error", err, "address", address)
		return
	}
	ws.log.Info("testnet account funded", "address", address, "tx_hash", resp.Hash)

	time.Sleep(5 * time.Second)
	if err := ws.setupUSDCTrustline(ctx, pair); err != nil {
		ws.log.Error("Failed to set up USDC trustline", "error", err, "stage", ws.stage)
	}

	now := time.Now()
	fee := float64(resp.FeeCharged)
	txService := transactions.NewTransactionService()
	transactionData := transactions.TransactionDto{
		WalletID:          &walletID,
		TransactionHash:   resp.Hash,
		Amount:            10000,
		Token:             "xlm",
		TransactionStatus: "confirmed",
		ConfirmedAt:       &now,
		TransactionType:   "funding",
		NetworkFee:        &fee,
	}

	_, err = txService.CreateNewTransaction(ctx, userId, transactionData, "xlm")
	if err != nil {
		ws.log.Error("failed to record transaction record on wallet test funding")
	}
}

func (ws *WalletService) getAccountBalance(address string) (*StellarWalletBalance, error) {
	account, err := ws.horizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: address,
	})

	if err != nil {
		if hErr, ok := err.(*horizonclient.Error); ok && hErr.Response.StatusCode == 404 {
			return &StellarWalletBalance{
				USDC: 0,
				XLM:  0,
			}, nil
		}
		ws.log.Error("failed to get wallet balance", "error", err)
		return nil, err
	}

	var usdcBalance, xlmBalance float64
	for _, balance := range account.Balances {
		switch {
		case balance.Asset.Type == "native":
			xlmBalance, _ = strconv.ParseFloat(balance.Balance, 64)
		case balance.Asset.Code == "USDC" && balance.Asset.Issuer == ws.getUSDCIssuer():
			usdcBalance, _ = strconv.ParseFloat(balance.Balance, 64)
		}
	}

	return &StellarWalletBalance{
		USDC: math.Round(usdcBalance*100) / 100,
		XLM:  math.Round(xlmBalance*100) / 100,
	}, nil
}

func (ws *WalletService) getUSDCIssuer() string {
	if ws.stage == "testnet" {
		return "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
	}
	return "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
}

func (ws *WalletService) updateWalletBalance(ctx context.Context, walletID string, usdc_balance, xlm_balance float64) {
	const query string = `UPDATE wallets SET usdc_balance = $2, xlm_balance = $3, updated_at = NOW() WHERE id = $1`
	_, err := ws.postgres.Exec(ctx, query, walletID, usdc_balance, xlm_balance)
	if err != nil {
		ws.log.Error("failed to update wallet balance", "error", err, "wallet_id", walletID)
	}
	ws.log.Info("successfully updated wallet balance on wallet Id ===> " + walletID)
}

func (ws *WalletService) cacheWalletInfo(ctx context.Context, wallet *WalletResponseDto) {
	data, err := json.Marshal(wallet)
	if err != nil {
		return
	}

	cacheKey := ws.GetWalletCacheKey(wallet.ID)
	ws.redis.Set(ctx, cacheKey, data, 5*time.Minute)
}

func (ws *WalletService) GetWalletCacheKey(walletId string) string {
	return fmt.Sprintf("wallet:%s", walletId)
}

func (ws *WalletService) ExportWalletKeys(ctx context.Context, walletID, userID string, role user.UserRole) *utils.ApiResponse {
	var encryptedSeed string
	var walletAddress string
	var dbUserID string
	var walletId string

	query := `SELECT id, private_key, address, user_id FROM wallets WHERE id = $1`
	args := []interface{}{walletID}

	if role != user.RoleAdmin {
		query += ` AND user_id = $2`
		args = append(args, userID)
	}

	err := ws.postgres.QueryRow(ctx, query, args...).Scan(
		&walletId,
		&encryptedSeed,
		&walletAddress,
		&dbUserID,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Wallet not found or access denied",
			}
		}
		ws.log.Error("Failed to fetch wallet", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to retrieve wallet details",
		}
	}

	seed, err := ws.decryptPrivateKey(encryptedSeed)
	if err != nil {
		ws.log.Error("Failed to decrypt private key", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process wallet keys",
		}
	}
	ws.log.Info("Wallet keys exported",
		"walletId", walletID,
		"exportedBy", userID,
		"role", role,
		"walletOwner", dbUserID,
	)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]interface{}{
			"private_key":    seed,
			"wallet_address": walletAddress,
		},
	}
}

func (ws *WalletService) setupUSDCTrustline(ctx context.Context, userKeypair *keypair.Full) error {
	ws.log.Info("Setting up USDC trustline",
		"stage", ws.stage,
		"issuer", ws.getUSDCIssuer(),
		"address", userKeypair.Address())

	_, err := ws.horizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: userKeypair.Address(),
	})
	if err != nil {
		if hErr, ok := err.(*horizonclient.Error); ok && hErr.Response.StatusCode == 404 {
			ws.log.Info("Account needs funding before trust line can be created", "address", userKeypair.Address())
			return nil
		}
		return err
	}

	client := ws.horizonClient
	account, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: userKeypair.Address(),
	})
	if err != nil {
		return err
	}

	for _, balance := range account.Balances {
		if balance.Asset.Code == "USDC" && balance.Asset.Issuer == ws.getUSDCIssuer() {
			return nil
		}
	}
	asset, err := txnbuild.CreditAsset{
		Code:   "USDC",
		Issuer: ws.getUSDCIssuer(),
	}.ToChangeTrustAsset()

	if err != nil {
		return err
	}

	changeTrustOp := &txnbuild.ChangeTrust{
		Line:  asset,
		Limit: "1000000",
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &account,
			IncrementSequenceNum: true,
			Operations:           []txnbuild.Operation{changeTrustOp},
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{
				TimeBounds: txnbuild.NewTimeout(300),
			},
		},
	)
	if err != nil {
		ws.log.Error("Failed to build trustline transaction", "error", err)
		return err
	}

	tx, err = tx.Sign(ws.networkPass, userKeypair)
	if err != nil {
		ws.log.Error("Failed to sign trustline transaction", "error", err)
		return err
	}

	txe, err := tx.Base64()
	if err != nil {
		ws.log.Error("Failed to encode transaction", "error", err)
		return err
	}

	resp, err := ws.horizonClient.SubmitTransactionXDR(txe)
	if err != nil {
		ws.log.Error("Failed to submit trustline transaction", "error", err)
		return err
	}

	ws.log.Info("USDC trustline created successfully",
		"address", userKeypair.Address(),
		"txHash", resp.Hash)

	return nil
}

func (ws *WalletService) getKeyPairForWallet(ctx context.Context, walletId string) (*keypair.Full, error) {
	var encryptedSeed string
	err := ws.postgres.QueryRow(ctx,
		"SELECT private_key FROM wallets WHERE id = $1",
		walletId).Scan(&encryptedSeed)

	if err != nil {
		return nil, err
	}

	seed, err := ws.decryptPrivateKey(encryptedSeed)
	if err != nil {
		return nil, err
	}

	return keypair.ParseFull(seed)
}
