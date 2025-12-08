package poker

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math"
	"math/big"
	"server/internal/db"
	"server/internal/db/models"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
)

type Tournament struct {
	ID                    uint
	Name                  string
	RegistrationStartDate time.Time
	RegistrationEndDate   time.Time
	StartDate             time.Time
	EndDate               time.Time
	EntryCost             float32
	Currency              string
	Tables                []Table
	Players               []Player
	Prize                 string
	Configuration         string
	Ongoing               bool
	MinPlayers            int
	MaxPlayers            int
	TurnSeconds           int
	StartChips            int
	BBValue               int
	Start                 bool
	IncrementBlind        int
	PrizeList             []map[string]interface{}
}

func (tournament *Tournament) CreateTablesForTournament() {
	numPlayers := len(tournament.Players)
	numTables := numPlayers / tournament.MaxPlayers
	if numPlayers%tournament.MaxPlayers != 0 {
		numTables++
	}

	tables := make([]Table, numTables)

	playerIndex := 0

	for i := 0; i < numTables; i++ {
		table := Table{
			ID:      fmt.Sprintf("Table-%d", i+1),
			Players: []Player{},
		}

		for j := 0; j < tournament.MaxPlayers && playerIndex < numPlayers; j++ {
			table.Players = append(table.Players, tournament.Players[playerIndex])
			playerIndex++
		}

		tables[i] = table
	}

	tournament.Tables = tables
}

const (
	SepoliaRPC          = "https://rpc.sepolia.org"
	BscRPC              = "https://bsc-dataseed.binance.org/"
	USDTContractAddress = "0x55d398326f99059fF775485246999027B3197955"
)

type TransferInstruction struct {
	Currency      string  `json:"currency"`
	Position      int     `json:"position"`
	Prize         float64 `json:"prize"`
	WalletAddress string  `json:"wallet_address"`
}

type TransactionRecord struct {
	ID            uint           `gorm:"primaryKey;autoIncrement"`
	TournamentID  uint           `gorm:"not null"`
	Tournament    Tournament     `gorm:"foreignKey:TournamentID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WalletAddress string         `gorm:"size:100;not null"`
	TxHash        string         `gorm:"size:100;not null;unique"`
	Currency      string         `gorm:"size:50;not null"`
	Network       string         `gorm:"size:50;not null"`
	Amount        string         `gorm:"not null"`
	Success       bool           `gorm:"not null"`
	CreatedAt     time.Time      `gorm:"autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime"`
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func floatToBigInt(amount float64, decimals int) *big.Int {
	multiplier := math.Pow(10, float64(decimals))
	f := new(big.Float).SetFloat64(amount * multiplier)
	result := new(big.Int)
	f.Int(result)
	return result
}

func sendNativeSepolia(privateKeyStr, toAddress string, amount *big.Int) (string, error) {
	client, err := ethclient.Dial(SepoliaRPC)
	if err != nil {
		return "", fmt.Errorf("error conectando a Sepolia: %v", err)
	}
	defer client.Close()
	privateKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return "", fmt.Errorf("error con la clave privada: %v", err)
	}
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("error convirtiendo clave pública")
	}
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return "", fmt.Errorf("error obteniendo nonce: %v", err)
	}
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return "", fmt.Errorf("error obteniendo gas price: %v", err)
	}
	tx := types.NewTransaction(nonce, common.HexToAddress(toAddress), amount, 21000, gasPrice, nil)
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return "", fmt.Errorf("error obteniendo chainID: %v", err)
	}
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return "", fmt.Errorf("error firmando la transacción: %v", err)
	}
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return "", fmt.Errorf("error enviando la transacción: %v", err)
	}
	return signedTx.Hash().Hex(), nil
}

func sendERC20BSC(privateKeyStr, toAddress string, amount *big.Int) (string, error) {
	client, err := ethclient.Dial(BscRPC)
	if err != nil {
		return "", fmt.Errorf("error conectando a BSC: %v", err)
	}
	defer client.Close()
	privateKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return "", fmt.Errorf("error con la clave privada: %v", err)
	}
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("error convirtiendo clave pública")
	}
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return "", fmt.Errorf("error obteniendo nonce: %v", err)
	}
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return "", fmt.Errorf("error obteniendo gas price: %v", err)
	}
	tokenABI, err := abi.JSON(strings.NewReader(`[{"constant":false,"inputs":[{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"type":"function"}]`))
	if err != nil {
		return "", fmt.Errorf("error cargando el ABI: %v", err)
	}
	data, err := tokenABI.Pack("transfer", common.HexToAddress(toAddress), amount)
	if err != nil {
		return "", fmt.Errorf("error empaquetando parámetros: %v", err)
	}
	contractAddr := common.HexToAddress(USDTContractAddress)
	msg := ethereum.CallMsg{
		From: fromAddress,
		To:   &contractAddr,
		Data: data,
	}
	gasLimit, err := client.EstimateGas(context.Background(), msg)
	if err != nil {
		gasLimit = 60000
	}
	tx := types.NewTransaction(nonce, contractAddr, big.NewInt(0), gasLimit, gasPrice, data)
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return "", fmt.Errorf("error obteniendo chainID: %v", err)
	}
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return "", fmt.Errorf("error firmando la transacción: %v", err)
	}
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return "", fmt.Errorf("error enviando la transacción: %v", err)
	}
	return signedTx.Hash().Hex(), nil
}

func ProcessTransfers(instructions []TransferInstruction, sepoliaPrivateKey, bscPrivateKey string, tournamentID uint) {
	for _, instr := range instructions {
		if instr.WalletAddress == "" {
			log.Printf("Posición %d: Dirección de wallet vacía, omitiendo transferencia", instr.Position)
			continue
		}
		var txHash string
		var success bool
		var err error
		if instr.Currency == "sepolia" {
			amountWei := floatToBigInt(instr.Prize, 18)
			txHash, err = sendNativeSepolia(sepoliaPrivateKey, instr.WalletAddress, amountWei)
			if err != nil {
				log.Printf("Error en transferencia Sepolia posición %d: %v", instr.Position, err)
				success = false
				txHash = fmt.Sprintf("failed_%s", time.Now().Format(time.RFC3339Nano))
			} else {
				success = true
			}
			record := &models.TransactionRecord{
				TournamentID:  tournamentID,
				WalletAddress: instr.WalletAddress,
				TxHash:        txHash,
				Currency:      instr.Currency,
				Network:       "sepolia",
				Amount:        amountWei.String(),
				Success:       success,
			}
			if err := db.InsertTransactionRecord(record); err != nil {
				log.Printf("Error insertando registro de transacción: %v", err)
			}
		} else if instr.Currency == "usdt" {
			amountToken := floatToBigInt(instr.Prize, 6)
			txHash, err = sendERC20BSC(bscPrivateKey, instr.WalletAddress, amountToken)
			if err != nil {
				log.Printf("Error en transferencia BSC posición %d: %v", instr.Position, err)
				success = false
				txHash = fmt.Sprintf("failed_%s", time.Now().Format(time.RFC3339Nano))
			} else {
				success = true
			}
			record := &models.TransactionRecord{
				TournamentID:  tournamentID,
				WalletAddress: instr.WalletAddress,
				TxHash:        txHash,
				Currency:      instr.Currency,
				Network:       "bsc",
				Amount:        amountToken.String(),
				Success:       success,
			}
			if err := db.InsertTransactionRecord(record); err != nil {
				log.Printf("Error insertando registro de transacción: %v", err)
			}
		} else {
			log.Printf("Posición %d: Moneda %s no reconocida", instr.Position, instr.Currency)
		}
	}
}
