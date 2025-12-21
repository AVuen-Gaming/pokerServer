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

to stop all in compose
docker-compose down --volumes --remove-orphans

## Unity tournament demo

The helper command now drives the same poker workflow that `cmd/pokerServer` executes: it seeds Postgres with a 3-table tournament, lets Temporal activities run the real dealing/betting logic, and stands up JetStream consumers so Unity can play a seat while bots drive the other chairs.

> 🔁 Always keep the simulator running in one terminal/container and the Unity console client in another. They must share the exact same `-tournament-id` value (any string is accepted; both sides hash it to the same numeric subject automatically).

### Start the simulator (bots + workflow)

**PowerShell**

```
go run ./cmd/unitytournament `
	-mode sim `
	-players 27 `
	-tables 3 `
	-unity-wallet unity-demo `
	-tournament-id UNITY-DEMO-27 `
	-turn-duration 15
```

**Bash**

```
go run ./cmd/unitytournament \
	-mode sim \
	-players 27 \
	-tables 3 \
	-unity-wallet unity-demo \
	-tournament-id UNITY-DEMO-27 \
	-turn-duration 15
```

The simulator:

- Builds 3 tables with 27 seats (Unity keeps one seat while bots fill the rest).
- Lets the existing `HandleTableActivitie` activity deal cards, advance stages, and reshuffle tables, so any future gameplay change automatically applies here.
- Publishes real table snapshots (`pokerServer.<TournamentID>.<TableID>`) and per-player updates (`pokerServer.<TournamentID>.<TableID>.<WalletID>`), including timers and tournament state logs.
- Forces AFK defaults when Unity runs out of time, identical to production.
- Reuses the JetStream stream defined by `NATS_STREAM_NAME`. If the stream already exists, the simulator now merges any missing subjects instead of failing with "subjects overlap"; run `nats stream rm <name>` only if you explicitly want to wipe it.

### Unity wiring / console client

Use a second terminal to run the console client so you can act for the Unity wallet:

**PowerShell**

```
go run ./cmd/unitytournament `
	-mode client `
	-unity-wallet unity-demo `
	-tournament-id UNITY-DEMO-27
```

**Bash**

```
go run ./cmd/unitytournament \
	-mode client \
	-unity-wallet unity-demo \
	-tournament-id UNITY-DEMO-27
```

The console UI subscribes to `pokerServer.<TournamentID>.*.<WalletID>`, displays the remaining turn time based on the table’s countdown, prints the overall tournament state (players left vs. tables), and only prompts for input while it is truly your turn. Type `fold`, `check`, `call 200`, `raise 300`, or `allin` to publish on `pokerClient.<TournamentID>.<TableID>.<WalletID>`.