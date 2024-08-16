# pokersrv

install docker
install go 1.22.5

to run project
docker-compose -f .\docker-compose.yml up -d
go mod tidy
cd cmd pokerserver go run main.go

Struct
├── cmd
│   └── yourapp
│       └── main.go
├── config
│   └── config.go
├── internal
│   ├── db
│   │   ├── db.go
│   │   └── models
│   │       └── models.go
│   ├── nats
│   │   └── nats.go
│   ├── poker
│   │   ├── handlers.go
│   │   └── game_logic.go
│   ├── server
│   │   └── server.go
│   └── workflows
│       └── poker_workflow.go
├── pkg
│   └── utils
│       └── utils.go
└── go.mod
└── go.sum