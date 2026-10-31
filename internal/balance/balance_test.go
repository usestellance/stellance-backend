package balance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
)

func horizon(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("STELLAR_STAGE", "dev")
	t.Setenv("TESTNET_NETWORK_URL", srv.URL)
}

func TestFetchReadsLiveBalances(t *testing.T) {
	issuer := utils.USDCIssuer("testnet")
	horizon(t, 200, `{"id":"GA","account_id":"GA","balances":[
		{"balance":"10002.1234567","asset_type":"native"},
		{"balance":"5.0300000","asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":"`+issuer+`"},
		{"balance":"99.0000000","asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":"GOTHER"}]}`)

	b, err := Fetch(context.Background(), "GA")
	if err != nil {
		t.Fatal(err)
	}
	if b.XLM != 10002.1234567 || b.USDC != 5.03 {
		t.Fatalf("balance = %+v, want full precision XLM and only the configured USDC issuer", b)
	}
}

func TestFetchUnknownAccountIsZero(t *testing.T) {
	horizon(t, 404, `{"type":"https://stellar.org/horizon-errors/not_found","title":"Resource Missing","status":404}`)
	b, err := Fetch(context.Background(), "GA")
	if err != nil || b.USDC != 0 || b.XLM != 0 {
		t.Fatalf("balance = %+v, err = %v", b, err)
	}
}

func TestFetchHorizonFailureIsAnError(t *testing.T) {
	horizon(t, 500, `{"title":"Internal Server Error","status":500}`)
	if b, err := Fetch(context.Background(), "GA"); err == nil {
		t.Fatalf("got balance %+v, want an error rather than zeros", b)
	}
}
