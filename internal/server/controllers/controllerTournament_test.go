package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"server/config"
	"server/internal/db"
	"server/internal/db/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubResponse struct {
	statusCode int
	body       string
}

func (s stubResponse) toHTTPResponse() *http.Response {
	return &http.Response{
		StatusCode: s.statusCode,
		Body:       io.NopCloser(bytes.NewBufferString(s.body)),
		Header:     make(http.Header),
	}
}

func withHTTPStub(t *testing.T, mapping map[string]stubResponse) func() {
	t.Helper()
	original := httpGet
	httpGet = func(url string) (*http.Response, error) {
		for key, resp := range mapping {
			if strings.Contains(url, key) {
				return resp.toHTTPResponse(), nil
			}
		}
		t.Fatalf("unexpected url requested: %s", url)
		return nil, nil
	}
	return func() { httpGet = original }
}

func TestIsValidTransactionBscSuccess(t *testing.T) {
	nowHex := "0x" + strconv.FormatInt(time.Now().Unix(), 16)
	mapping := map[string]stubResponse{
		"action=tokentx": {
			statusCode: http.StatusOK,
			body:       `{"status":"1","message":"OK","result":[{"from":"0xSender","to":"0xReceiver","value":"2000000","contractAddress":"","blockNumber":"0x10"}]}`,
		},
		"bscscan.com/api?module=proxy&action=eth_getTransactionByHash": {
			statusCode: http.StatusOK,
			body:       `{"result":{"gas":"0x5208","gasPrice":"0x4a817c800"}}`,
		},
		"bscscan.com/api?module=proxy&action=eth_getBlockByNumber": {
			statusCode: http.StatusOK,
			body:       fmt.Sprintf(`{"result":{"timestamp":"%s"}}`, nowHex),
		},
	}
	restore := withHTTPStub(t, mapping)
	defer restore()

	cfg := &config.ServerConfig{Wallet: "0xReceiver", BcsApiKey: "test"}

	if !isValidTransactionBsc("0xhash", "0xSender", 1.0, cfg) {
		t.Fatalf("expected transaction to be considered valid")
	}
}

func TestIsValidTransactionBscRejectsMismatchedWallet(t *testing.T) {
	mapping := map[string]stubResponse{
		"action=tokentx": {
			statusCode: http.StatusOK,
			body:       `{"status":"1","message":"OK","result":[{"from":"0xSender","to":"0xSomeoneElse","value":"2000000","contractAddress":"","blockNumber":"0x10"}]}`,
		},
	}
	restore := withHTTPStub(t, mapping)
	defer restore()

	cfg := &config.ServerConfig{Wallet: "0xReceiver", BcsApiKey: "test"}

	if isValidTransactionBsc("0xhash", "0xSender", 1.0, cfg) {
		t.Fatalf("expected validation to fail when receiver wallet mismatches")
	}
}

func TestIsValidTransactionSepoliaSuccess(t *testing.T) {
	nowHex := "0x" + strconv.FormatInt(time.Now().Unix(), 16)
	mapping := map[string]stubResponse{
		"api-sepolia.etherscan.io/api?module=proxy&action=eth_getTransactionByHash": {
			statusCode: http.StatusOK,
			body:       `{"result":{"from":"0xSender","to":"0xReceiver","value":"0x38d7ea4c68000","gas":"0x5208","gasPrice":"0x77359400","blockNumber":"0x10"}}`,
		},
		"api-sepolia.etherscan.io/api?module=proxy&action=eth_getBlockByNumber": {
			statusCode: http.StatusOK,
			body:       fmt.Sprintf(`{"result":{"timestamp":"%s"}}`, nowHex),
		},
	}
	restore := withHTTPStub(t, mapping)
	defer restore()

	cfg := &config.ServerConfig{Wallet: "0xReceiver", SepApiKey: "test"}

	if !isValidTransactionSepolia("0xhash", "0xSender", 0.001, cfg) {
		t.Fatalf("expected sepolia transaction to validate")
	}
}

func TestGetTournamentsReturnsPersistedRows(t *testing.T) {
	dbInstance, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	db.DB = dbInstance
	if err := db.DB.AutoMigrate(&models.Tournament{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	sample := models.Tournament{Name: "Test Cup", EntryCost: 50, Currency: "usdt"}
	if err := db.DB.Create(&sample).Error; err != nil {
		t.Fatalf("failed to seed tournament: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tournaments", nil)
	rr := httptest.NewRecorder()

	GetTournaments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}

	var tournaments []models.Tournament
	if err := json.NewDecoder(rr.Body).Decode(&tournaments); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(tournaments) != 1 || tournaments[0].Name != "Test Cup" {
		t.Fatalf("unexpected tournaments payload: %+v", tournaments)
	}
}
