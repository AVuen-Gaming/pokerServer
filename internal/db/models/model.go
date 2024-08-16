package models

import (
	"time"
)

type User struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	Username  string    `gorm:"size:50;not null;unique"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	DeletedAt time.Time `gorm:"index"`
}

type Wallet struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	UserID        uint      `gorm:"not null;unique"`
	User          User      `gorm:"foreignKey:UserID"`
	WalletAddress string    `gorm:"size:100;not null;unique"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
	DeletedAt     time.Time `gorm:"index"`
}

type Tournament struct {
	ID                    uint      `gorm:"primaryKey;autoIncrement"`
	Name                  string    `gorm:"size:100;not null"`
	RegistrationStartDate time.Time `gorm:"not null"`
	RegistrationEndDate   time.Time `gorm:"not null"`
	StartDate             time.Time `gorm:"not null"`
	EndDate               time.Time
	Prize                 string
	Configuration         string
	Ongoing               bool      `gorm:"default:false"`
	MinPlayers            int       `gorm:"not null"`
	MaxPlayers            int       `gorm:"not null"`
	TurnSeconds           int       `gorm:"not null"`
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
