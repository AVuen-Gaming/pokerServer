package db

import (
	"fmt"
	"server/internal/db/models"
)

func InsertTransactionRecord(tr *models.TransactionRecord) error {
	if err := DB.Create(tr).Error; err != nil {
		return fmt.Errorf("error insertando el registro de transacción: %v", err)
	}
	return nil
}
