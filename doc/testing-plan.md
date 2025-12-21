# Comprehensive Testing Plan

This document enumerates the mandatory automated test coverage required for the poker tournament platform. It ties each suite back to the DER (Wallet ↔ TournamentRegistration ↔ Tournament ↔ Table ↔ TablePlayer ↔ Ranking/Prize/TransactionRecord) and to the runtime flows spanning poker logic, HTTP, crypto, and NATS/Unity messaging.

## 1. Tournament-level suites
- **Table orchestration**: Validate `Tournament.CreateTablesForTournament`, `planReshuffleTables`, `OnlyOneTableRemains`, and table lifecycle state transitions when balancing players across many tables.
- **Tournament states**: Simulate workflow checkpoints (registration, start, ongoing, finish) ensuring `Tournament` fields (`Start`, `Ongoing`, `PrizeList`, `IncrementBlind`) evolve consistently with DER expectations.
- **Leaderboard/ranking**: Cover ranking insertions and prize assignment hooks (`AssignChipsToWinners`, `HandlePlayerPrize`) with deterministic fixtures representing final-table finishes.
- **Workflow integration**: Unit-test `CheckLastTableInTables` logic by faking DB + NATS adapters to assert correct deletion, ranking insert, and JetStream publication when tournaments end.
- **Workflow state propagation**: Temporal `WorkflowTestSuite` scenario (`internal/workflow/workflow_state_test.go`) that drives `TournamentWorkflow` with scripted child results, proving the parent receives mutated table slices, reuses them for subsequent rounds, and terminates child workflows once tournament completion criteria are met.

## 2. Table-level suites
- **Dealing & stages**: Tests for `DealCards`, community cards, and stage transitions (pre-flop → showdown) verifying deck exhaustion, `Stage*` markers, and `CurrentTurn` decisions.
- **Betting flow**: Exercise `applyPlayerAction`, `SetTablePlayersCallAmount`, `ManageSidePots`, `AssignChipsToWinners`, and `SetEliminatePlayersWithNoChips` across edge cases (folds, all-ins, side pots, AFK).
- **Reshuffle & table end**: Ensure `MovePlayers`, `Reshuffle`, and `TableEnd` react to under-filled tables and trigger `TableEnds`, `Frozen`, and `LastTable` as mandated by DER.

## 3. Player-level suites
- **State transitions**: Validate `SetTablePlayerActions`, `ClearPlayerActions`, `AllPlayersExceptOneFold`, and `AllPlayersAllInExceptOneAndFolded` for diverse player distributions.
- **Chip accounting**: Confirm chip deductions/additions via raises, calls, all-ins, and payouts while keeping `TotalBet`, `CallAmount`, and `SidePots` consistent.
- **Unity messaging contract**: Mock JetStream to ensure `SendPlayerUpdateToNATS` publishes correct subjects/payloads (including AFK/all-in flags) per Unity client contract.

## 4. Crypto / on-chain suites
- **Transaction validation**: Refactor the controller helpers (`isValidTransactionBsc`, `isValidTransactionSepolia`) to accept an HTTP client so tests can replay deterministic BscScan/Sepolia JSON fixtures and cover success/failure branches, currency mismatches, and amount thresholds.
- **Transfer processing**: Unit-test `ProcessTransfers` with fake Ethereum/BSC clients to assert payouts per `PrizeList` result in `TransactionRecord` persistence and error logging.

## 5. HTTP endpoint suites
- **Controller handlers**: Using an in-memory SQLite GORM database plus httptest routers, cover key endpoints: tournament CRUD, registration flows (including wallet validation and duplicate prevention), ranking queries, prizes, and health checks.
- **Middleware interactions**: Add focused tests for rate limiting, session token, and wallet validation middleware using stub contexts to ensure protected routes enforce policy.

## 6. NATS / Unity messaging suites
- **JetStream lifecycle**: Spin up an embedded NATS server in tests to assert `nats.Connect` + `ConfigureStream` create the required subjects (`pokerServer.*`, `pokerClient.*`) and enforce `MaxAge`.
- **Unity action loop**: Fake JetStream contexts to verify consumer naming (`durable-consumer4-{table}-{player}`), ACK handling, and server side responses to player actions (fold timeout, AFK) align with Unity integration notes.
- **Unity player journey**: Simulate an end-to-end Unity client that (a) waits for tournament start, (b) posts blinds, (c) cycles through `call`, `raise`, `fold`, and `allin`, (d) handles reshuffle notifications by swapping JetStream subjects, and (e) reacts to elimination/final-table announcements. The server-side fake should publish `pokerServer.*` updates for each `pokerClient.*` action to validate stream stability across table switches.

## 7. Execution guidance
- Tests should reside near their domains (`internal/poker`, `internal/server/controllers`, `internal/nats`, etc.) and use mocks/fakes for DB, HTTP, Temporal, and NATS dependencies.
- Extend `go test ./...` coverage with targeted packages (e.g., `go test ./internal/server/... -run TestTournamentEndpoints`). Document any required test tags or environment variables.

This plan must be kept current with feature additions and referenced in each PR description to prove coverage across tournament, table, player, crypto, HTTP, and NATS workflows.
