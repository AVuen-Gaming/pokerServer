package models

import (
	"time"
)

type Wallet struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	Wallet    string    `gorm:"size:100;not null;unique"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	DeletedAt time.Time `gorm:"index"`
}

type Tournament struct {
	ID                    uint       `gorm:"primaryKey;autoIncrement"`
	Name                  string     `gorm:"size:100;not null"`
	RegistrationStartDate time.Time  `gorm:"not null"`
	RegistrationEndDate   time.Time  `gorm:"not null"`
	StartDate             time.Time  `gorm:"not null"`
	Start                 bool       `gorm:"default:false"`
	EndDate               *time.Time `gorm:"default:null"`
	Prize                 string
	EntryCost             float32 `gorm:"not null"`
	Currency              string  `gorm:"not null"`
	Configuration         string
	Ongoing               bool `gorm:"default:false"`
	MinPlayers            int  `gorm:"not null"`
	MaxPlayers            int  `gorm:"not null"`
	TurnSeconds           int  `gorm:"not null"`
	StartChips            int
	BBValue               int
	CreatedAt             time.Time `gorm:"autoCreateTime"`
	UpdatedAt             time.Time `gorm:"autoUpdateTime"`
	DeletedAt             time.Time `gorm:"index"`
}

type TournamentChip struct {
	ID           uint       `gorm:"primaryKey;autoIncrement"`
	TournamentID uint       `gorm:"not null"`
	Tournament   Tournament `gorm:"foreignKey:TournamentID"`
	WalletID     uint       `gorm:"not null"`
	Wallet       Wallet     `gorm:"foreignKey:WalletID"`
	Chips        int        `gorm:"not null"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	DeletedAt    time.Time  `gorm:"index"`
}

type TournamentRegistration struct {
	ID           uint       `gorm:"primaryKey;autoIncrement"`
	TournamentID uint       `gorm:"not null"`
	Tournament   Tournament `gorm:"foreignKey:TournamentID"`
	WalletID     uint       `gorm:"not null"`
	Wallet       Wallet     `gorm:"foreignKey:WalletID"`
	Eliminated   bool       `gorm:"default:false"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	DeletedAt    time.Time  `gorm:"index"`
}

type Table struct {
	ID           uint       `gorm:"primaryKey;autoIncrement"`
	TournamentID uint       `gorm:"not null"`
	Tournament   Tournament `gorm:"foreignKey:TournamentID"`
	TableNumber  int        `gorm:"not null"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	DeletedAt    time.Time  `gorm:"index"`
}

type TablePlayer struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	TableID   uint      `gorm:"not null"`
	Table     Table     `gorm:"foreignKey:TableID"`
	WalletID  uint      `gorm:"not null"`
	Wallet    Wallet    `gorm:"foreignKey:WalletID"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	DeletedAt time.Time `gorm:"index"`
}

type Ranking struct {
	ID           uint       `gorm:"primaryKey;autoIncrement"`
	TournamentID uint       `gorm:"not null"`
	Tournament   Tournament `gorm:"foreignKey:TournamentID"`
	WalletID     uint       `gorm:"not null"`
	Wallet       Wallet     `gorm:"foreignKey:WalletID"`
	Position     int        `gorm:"not null"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	DeletedAt    time.Time  `gorm:"index"`
}

type RecordLog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	TableName string    `gorm:"size:100;not null"`
	Action    string    `gorm:"size:100;not null"`
	RecordID  uint      `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	DeletedAt time.Time `gorm:"index"`
}
