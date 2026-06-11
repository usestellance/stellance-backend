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

	"github.com/The-True-Hooha/stellance-backend/internal/activitylog"
	"github.com/The-True-Hooha/stellance-backend/internal/notifications"
	"github.com/The-True-Hooha/stellance-backend/internal/transactions"
	"github.com/The-True-Hooha/stellance-backend/internal/user"
	"github.com/The-True-Hooha/stellance-backend/mail"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	hProtocol "github.com/stellar/go/protocols/horizon"
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

	activitylog.Log(ctx, ws.postgres, ws.log, userId, activitylog.ActionWalletCreated, activitylog.EntityWallet, wallet.ID, "")
	log.Info("wallet created successfully",
		"user_id", userId,
		"wallet_id", wallet.ID,
		"address", wallet.WalletAddress,
	)

	if ws.stage == "testnet" {
		go func() {
			ws.fundTestnetAccount(context.Background(), pair.Address(), wallet.ID, userId, pair)
			data := notifications.CreateNotificationDto{
				Title:  "New Transaction Update",
				UserId: userId,
				Body:   "A new wallet has been created for you, you can find your wallet in the wallet tab with your wallet address and private key. Kindly note that your private key is not visible to us, ensure to keep it safely secured on your end",
			}
			notifications.NewNotificationService().CreateNewNotification(context.Background(), data)

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
			balance, _ := ws.GetAccountBalance(ctx, wallet.WalletAddress, walletId)
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
	balance, err := ws.GetAccountBalance(ctx, wallet.WalletAddress, walletId)
	if err != nil {
		ws.log.Error("Failed to get account balance", "error", err, "wallet", walletId)
		balance = &StellarWalletBalance{USDC: 0, XLM: 0}
	}

	wallet.Balance = balance
	ws.cacheWalletInfo(ctx, &wallet)

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

func (ws *WalletService) GetAccountBalance(ctx context.Context, address, walletId string) (*StellarWalletBalance, error) {
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

	usdc := math.Round(usdcBalance*100) / 100
	xlm := math.Round(xlmBalance*100) / 100
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		ws.updateWalletBalance(ctx, walletId, usdc, xlm)
	}()

	return &StellarWalletBalance{
		USDC: usdc,
		XLM:  xlm,
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

func (ws *WalletService) ExportWalletKeys(ctx context.Context, walletID, userID, pin string, role user.UserRole) *utils.ApiResponse {
	if role != user.RoleAdmin {
		if err := ws.verifyPin(ctx, walletID, pin); err != nil {
			return &utils.ApiResponse{StatusCode: http.StatusForbidden, Message: err.Error()}
		}
	}
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
	activitylog.Log(ctx, ws.postgres, ws.log, userID, activitylog.ActionWalletExport, activitylog.EntityWallet, walletID, "")

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

// ── PIN management ────────────────────────────────────────────────────────────

func (ws *WalletService) SetPin(ctx context.Context, walletID, userID, pin string) *utils.ApiResponse {
	var ownerID string
	if err := ws.postgres.QueryRow(ctx,
		`SELECT user_id FROM wallets WHERE id = $1 AND is_active = TRUE`, walletID,
	).Scan(&ownerID); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "wallet not found"}
	}
	if ownerID != userID {
		return &utils.ApiResponse{StatusCode: http.StatusForbidden, Message: "access denied"}
	}
	hash, err := utils.HashPin(pin)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to set pin"}
	}
	ws.postgres.Exec(ctx,
		`UPDATE wallets SET pin_hash = $1, pin_set_at = NOW(), updated_at = NOW() WHERE id = $2`,
		hash, walletID,
	)
	activitylog.Log(ctx, ws.postgres, ws.log, userID, activitylog.ActionWalletPinSet, activitylog.EntityWallet, walletID, "")
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "transaction pin set successfully"}
}

func (ws *WalletService) LookupWalletByEmail(ctx context.Context, email string) *utils.ApiResponse {
	var address, userID string
	err := ws.postgres.QueryRow(ctx, `
		SELECT w.address, w.user_id
		FROM wallets w
		JOIN users u ON u.id = w.user_id
		WHERE u.email = $1 AND w.is_primary = TRUE AND w.is_active = TRUE
	`, email).Scan(&address, &userID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "no wallet found for that email"}
	}
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       map[string]any{"wallet_address": address, "user_id": userID},
	}
}

const (
	pinMaxAttempts  = 5
	pinLockDuration = 15 * time.Minute
)

func pinLockKey(walletID string) string { return "pin:attempts:" + walletID }

func (ws *WalletService) verifyPin(ctx context.Context, walletID, pin string) error {
	lockKey := pinLockKey(walletID)

	attempts, _ := ws.redis.Get(ctx, lockKey).Int()
	if attempts >= pinMaxAttempts {
		ttl, _ := ws.redis.TTL(ctx, lockKey).Result()
		return fmt.Errorf("too many incorrect attempts — try again in %.0f minutes", ttl.Minutes())
	}

	var pinHash *string
	if err := ws.postgres.QueryRow(ctx,
		`SELECT pin_hash FROM wallets WHERE id = $1`, walletID,
	).Scan(&pinHash); err != nil {
		return fmt.Errorf("wallet not found")
	}
	if pinHash == nil {
		return fmt.Errorf("transaction pin not set — please set a pin first")
	}
	if !utils.VerifyPin(pin, *pinHash) {
		pipe := ws.redis.Pipeline()
		pipe.Incr(ctx, lockKey)
		pipe.Expire(ctx, lockKey, pinLockDuration)
		pipe.Exec(ctx)
		remaining := pinMaxAttempts - attempts - 1
		if remaining <= 0 {
			return fmt.Errorf("too many incorrect attempts — wallet locked for 15 minutes")
		}
		return fmt.Errorf("invalid transaction pin — %d attempt(s) remaining", remaining)
	}

	ws.redis.Del(ctx, lockKey)
	return nil
}

// ── Path payment helpers ──────────────────────────────────────────────────────

func (ws *WalletService) assetFromCode(code string) txnbuild.Asset {
	if code == "XLM" || code == "native" {
		return txnbuild.NativeAsset{}
	}
	return txnbuild.CreditAsset{Code: code, Issuer: ws.getUSDCIssuer()}
}

func (ws *WalletService) findPaths(ctx context.Context, sourceAddress, destAddress, destAssetCode, destAmount string) ([]txnbuild.Asset, string, error) {
	destAsset := ws.assetFromCode(destAssetCode)
	var destAssetType, destAssetCode_, destAssetIssuer string
	switch a := destAsset.(type) {
	case txnbuild.NativeAsset:
		destAssetType = "native"
	case txnbuild.CreditAsset:
		if len(a.Code) <= 4 {
			destAssetType = "credit_alphanum4"
		} else {
			destAssetType = "credit_alphanum12"
		}
		destAssetCode_ = a.Code
		destAssetIssuer = a.Issuer
	}
	_ = destAssetCode_

	_, _, client := ws.networkConfig(ctx)
	req := horizonclient.PathsRequest{
		DestinationAccount:     destAddress,
		DestinationAssetType:   horizonclient.AssetType(destAssetType),
		DestinationAssetCode:   destAssetCode_,
		DestinationAssetIssuer: destAssetIssuer,
		DestinationAmount:      destAmount,
		SourceAccount:          sourceAddress,
	}
	page, err := client.Paths(req)
	if err != nil {
		return nil, "", fmt.Errorf("no payment path found: %w", err)
	}
	if len(page.Embedded.Records) == 0 {
		return nil, "", fmt.Errorf("no payment path found between these assets")
	}

	best := page.Embedded.Records[0]
	var path []txnbuild.Asset
	for _, p := range best.Path {
		path = append(path, ws.hAssetToTxn(p))
	}
	return path, best.SourceAmount, nil
}

// networkConfig reads the current Stellar stage from DB (encrypted) and returns
// the appropriate Horizon URL and network passphrase. Falls back to startup config.
func (ws *WalletService) networkConfig(ctx context.Context) (string, string, *horizonclient.Client) {
	var encrypted string
	err := ws.postgres.QueryRow(ctx,
		`SELECT value FROM system_config WHERE key = 'stellar_network'`,
	).Scan(&encrypted)
	if err != nil {
		return ws.networkURL, ws.networkPass, ws.horizonClient
	}
	stage, err := utils.DecryptValue(encrypted)
	if err != nil {
		return ws.networkURL, ws.networkPass, ws.horizonClient
	}
	if stage == "mainnet" {
		url := os.Getenv("MAINNET_NETWORK_URL")
		return url, network.PublicNetworkPassphrase, &horizonclient.Client{HorizonURL: url}
	}
	url := os.Getenv("TESTNET_NETWORK_URL")
	return url, network.TestNetworkPassphrase, &horizonclient.Client{HorizonURL: url}
}

func (ws *WalletService) hAssetToTxn(a hProtocol.Asset) txnbuild.Asset {
	if a.Type == "native" {
		return txnbuild.NativeAsset{}
	}
	return txnbuild.CreditAsset{Code: a.Code, Issuer: a.Issuer}
}

func (ws *WalletService) applySlippage(amount string, pct float64) string {
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return amount
	}
	return strconv.FormatFloat(f*(1+pct), 'f', 7, 64)
}

func (ws *WalletService) buildAndSubmit(ctx context.Context, kp *keypair.Full, op txnbuild.Operation, memo string) (string, float64, error) {
	_, netPass, client := ws.networkConfig(ctx)
	account, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
	if err != nil {
		return "", 0, fmt.Errorf("failed to fetch account: %w", err)
	}
	sourceAccount := txnbuild.SimpleAccount{AccountID: kp.Address(), Sequence: account.Sequence}

	var txMemo txnbuild.Memo
	if memo != "" {
		// Stellar memo text max 28 bytes — truncate safely
		b := []byte(memo)
		if len(b) > 28 {
			b = b[:28]
		}
		txMemo = txnbuild.MemoText(string(b))
	}

	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &sourceAccount,
		IncrementSequenceNum: true,
		Operations:           []txnbuild.Operation{op},
		BaseFee:              txnbuild.MinBaseFee,
		Memo:                 txMemo,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
	})
	if err != nil {
		return "", 0, fmt.Errorf("failed to build transaction: %w", err)
	}
	tx, err = tx.Sign(netPass, kp)
	if err != nil {
		return "", 0, fmt.Errorf("failed to sign transaction: %w", err)
	}
	txe, err := tx.Base64()
	if err != nil {
		return "", 0, fmt.Errorf("failed to encode transaction: %w", err)
	}
	resp, err := client.SubmitTransactionXDR(txe)
	if err != nil {
		return "", 0, fmt.Errorf("failed to submit transaction: %w", err)
	}
	feeXLM := float64(resp.FeeCharged) / 1e7
	return resp.Hash, feeXLM, nil
}

// ── PayInvoice ────────────────────────────────────────────────────────────────

func (ws *WalletService) PayInvoice(ctx context.Context, walletID, userID string, dto PayInvoiceDTO) *utils.ApiResponse {
	if err := ws.verifyPin(ctx, walletID, dto.Pin); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusForbidden, Message: err.Error()}
	}

	// fetch payer wallet address
	var payerAddress string
	if err := ws.postgres.QueryRow(ctx,
		`SELECT address FROM wallets WHERE id = $1 AND user_id = $2 AND is_active = TRUE`,
		walletID, userID,
	).Scan(&payerAddress); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "wallet not found"}
	}

	// fetch invoice amount + vendor wallet address
	var invoiceTotal float64
	var vendorAddress string
	var invoiceStatus string
	err := ws.postgres.QueryRow(ctx, `
		SELECT i.total, i.status::text, w.address
		FROM invoice i
		JOIN wallets w ON w.user_id = i.created_by_id AND w.is_primary = TRUE AND w.is_active = TRUE
		WHERE i.id = $1
	`, dto.InvoiceID).Scan(&invoiceTotal, &invoiceStatus, &vendorAddress)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "invoice not found or vendor has no wallet"}
	}
	if invoiceStatus == "paid" {
		return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invoice is already paid"}
	}
	if invoiceStatus == "cancelled" {
		return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invoice is cancelled"}
	}

	destAmount := strconv.FormatFloat(invoiceTotal, 'f', 7, 64)
	destAssetCode := "USDC"

	kp, err := ws.getKeyPairForWallet(ctx, walletID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to load wallet keys"}
	}

	sourceAsset := ws.assetFromCode(dto.SourceAsset)
	var txHash string
	var feeXLM float64
	var sourceAmount string

	if dto.SourceAsset == destAssetCode {
		// direct USDC → USDC payment, no path needed
		op := txnbuild.Payment{
			Destination: vendorAddress,
			Amount:      destAmount,
			Asset:       ws.assetFromCode(destAssetCode),
		}
		txHash, feeXLM, err = ws.buildAndSubmit(ctx, kp, &op, dto.InvoiceID)
		sourceAmount = destAmount
	} else {
		path, srcAmt, findErr := ws.findPaths(ctx, payerAddress, vendorAddress, destAssetCode, destAmount)
		if findErr != nil {
			return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: findErr.Error()}
		}
		sourceAmount = srcAmt
		sendMax := ws.applySlippage(srcAmt, 0.02) // 2% slippage buffer
		op := txnbuild.PathPaymentStrictReceive{
			SendAsset:   sourceAsset,
			SendMax:     sendMax,
			Destination: vendorAddress,
			DestAsset:   ws.assetFromCode(destAssetCode),
			DestAmount:  destAmount,
			Path:        path,
		}
		txHash, feeXLM, err = ws.buildAndSubmit(ctx, kp, &op, dto.InvoiceID)
	}
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusBadGateway, Message: "payment failed: " + err.Error()}
	}

	// record transaction
	ws.postgres.Exec(ctx, `
		INSERT INTO transactions (invoice_id, wallet_id, transaction_hash, amount, currency, status, token_type, transaction_type, user_id, confirmed_at, source_asset, source_amount)
		VALUES ($1, $2, $3, $4, 'usdc', 'confirmed', 'usdc', 'path_payment', $5, NOW(), $6, $7)`,
		dto.InvoiceID, walletID, txHash, invoiceTotal, userID, dto.SourceAsset, sourceAmount,
	)

	// mark invoice paid
	ws.postgres.Exec(ctx, `
		UPDATE invoice SET status = 'paid', paid_at = NOW(), updated_at = NOW() WHERE id = $1`,
		dto.InvoiceID,
	)
	activitylog.Log(ctx, ws.postgres, ws.log, userID, activitylog.ActionWalletPayment, activitylog.EntityWallet, walletID, "")
	ws.redis.Del(ctx, ws.GetWalletCacheKey(walletID))
	go notifications.NewNotificationService().CreateNewNotification(context.Background(), notifications.CreateNotificationDto{
		Title:  "Payment Sent",
		UserId: userID,
		Body:   fmt.Sprintf("Your payment of %.2f USDC for invoice %s was successful. Tx: %s", invoiceTotal, dto.InvoiceID, txHash),
	})

	// notify invoice creator
	go func() {
		var creatorEmail, creatorName, invoiceNumber string
		var total float64
		err := ws.postgres.QueryRow(context.Background(), `
			SELECT u.email, COALESCE(u.first_name,''), i.invoice_number, i.total
			FROM invoice i JOIN users u ON u.id = i.created_by_id WHERE i.id = $1`,
			dto.InvoiceID,
		).Scan(&creatorEmail, &creatorName, &invoiceNumber, &total)
		if err != nil {
			ws.log.Warn("failed to fetch creator info for payment email", "error", err)
			return
		}
		var payerEmail string
		ws.postgres.QueryRow(context.Background(),
			`SELECT email FROM users WHERE id = $1`, userID,
		).Scan(&payerEmail)
		mail.NewMailer().SendPaymentConfirmedEmail(mail.PaymentConfirmedEmailData{
			CreatorEmail:  creatorEmail,
			CreatorName:   creatorName,
			PayerEmail:    payerEmail,
			InvoiceNumber: invoiceNumber,
			Total:         fmt.Sprintf("%.2f", total),
			Currency:      "USDC",
			TxHash:        txHash,
			DashboardURL:  "https://usestellance.com/dashboard",
		})
	}()

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "invoice paid successfully",
		Data: PathPaymentResult{
			TransactionHash: txHash,
			SourceAsset:     dto.SourceAsset,
			SourceAmount:    sourceAmount,
			DestAsset:       destAssetCode,
			DestAmount:      destAmount,
			Destination:     vendorAddress,
			Fee:             feeXLM,
		},
	}
}

// ── Transfer ──────────────────────────────────────────────────────────────────

func (ws *WalletService) Transfer(ctx context.Context, walletID, userID string, dto TransferDTO) *utils.ApiResponse {
	if err := ws.verifyPin(ctx, walletID, dto.Pin); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusForbidden, Message: err.Error()}
	}

	var payerAddress string
	if err := ws.postgres.QueryRow(ctx,
		`SELECT address FROM wallets WHERE id = $1 AND user_id = $2 AND is_active = TRUE`,
		walletID, userID,
	).Scan(&payerAddress); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "wallet not found"}
	}

	kp, err := ws.getKeyPairForWallet(ctx, walletID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to load wallet keys"}
	}

	var txHash string
	var feeXLM float64
	var sourceAmount string

	if dto.SourceAsset == dto.DestAsset {
		op := txnbuild.Payment{
			Destination: dto.DestinationAddress,
			Amount:      dto.Amount,
			Asset:       ws.assetFromCode(dto.DestAsset),
		}
		txHash, feeXLM, err = ws.buildAndSubmit(ctx, kp, &op, "")
		sourceAmount = dto.Amount
	} else {
		path, srcAmt, findErr := ws.findPaths(ctx, payerAddress, dto.DestinationAddress, dto.DestAsset, dto.Amount)
		if findErr != nil {
			return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: findErr.Error()}
		}
		sourceAmount = srcAmt
		sendMax := ws.applySlippage(srcAmt, 0.02)
		op := txnbuild.PathPaymentStrictReceive{
			SendAsset:   ws.assetFromCode(dto.SourceAsset),
			SendMax:     sendMax,
			Destination: dto.DestinationAddress,
			DestAsset:   ws.assetFromCode(dto.DestAsset),
			DestAmount:  dto.Amount,
			Path:        path,
		}
		txHash, feeXLM, err = ws.buildAndSubmit(ctx, kp, &op, "")
	}
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusBadGateway, Message: "transfer failed: " + err.Error()}
	}

	currency := "usdc"
	if dto.DestAsset == "XLM" {
		currency = "xlm"
	}
	ws.postgres.Exec(ctx, `
		INSERT INTO transactions (wallet_id, transaction_hash, amount, currency, status, token_type, transaction_type, user_id, confirmed_at, source_asset, source_amount)
		VALUES ($1, $2, $3, $4, 'confirmed', $4, 'path_payment', $5, NOW(), $6, $7)`,
		walletID, txHash, dto.Amount, currency, userID, dto.SourceAsset, sourceAmount,
	)
	activitylog.Log(ctx, ws.postgres, ws.log, userID, activitylog.ActionWalletTransfer, activitylog.EntityWallet, walletID, "")
	ws.redis.Del(ctx, ws.GetWalletCacheKey(walletID))
	go notifications.NewNotificationService().CreateNewNotification(context.Background(), notifications.CreateNotificationDto{
		Title:  "Transfer Sent",
		UserId: userID,
		Body:   fmt.Sprintf("Your transfer of %s %s to %s was successful. Tx: %s", dto.Amount, dto.DestAsset, dto.DestinationAddress, txHash),
	})

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "transfer successful",
		Data: PathPaymentResult{
			TransactionHash: txHash,
			SourceAsset:     dto.SourceAsset,
			SourceAmount:    sourceAmount,
			DestAsset:       dto.DestAsset,
			DestAmount:      dto.Amount,
			Destination:     dto.DestinationAddress,
			Fee:             feeXLM,
		},
	}
}
