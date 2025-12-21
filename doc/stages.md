# Table Stage Reference

This document summarizes the meaning of every table stage emitted to Unity/NATS so we can cross-check simulator behavior and Temporal workflows. All stages are defined in `internal/poker/table.go` and mirrored inside `internal/workflow/activities.go`.

| Stage | Trigger | Key effects | Exit conditions |
|-------|---------|-------------|-----------------|
| `initRound` | `HandleTableActivitie` increments `table.Round` and rotates blinds via `SMBBTurn`. | - Re-evaluates who is SB/BB<br>- Clears eliminated players<br>- Publishes first state snapshot | Once initial state is pushed and cards are dealt. |
| `preFlop` | After dealing two cards to every active player. | - `HandleTurn` runs betting loop starting with the seat after BB.<br>- `SetTablePlayerActions` recomputes available actions before each turn.<br>- `table.CurrentTurn` is populated so Unity can prompt the right wallet. | - Everyone folds except one (`AllFoldExceptOne`) → jump to `showDownAllFoldExceptOne`.<br>- Otherwise when betting closes, move to `flop`. |
| `flop` | Dealer reveals `table.FlopCards` (3 community cards). | - Broadcasts board cards.<br>- Runs another `HandleTurn` cycle with remaining players. | - `AllFoldExceptOne` → `showDownAllFoldExceptOne`.<br>- Otherwise continue to `turn`. |
| `turn` | Dealer reveals `table.TurnCard`. | Same as flop: broadcast + betting cycle. | - `AllFoldExceptOne` → `showDownAllFoldExceptOne`.<br>- Otherwise continue to `river`. |
| `river` | Dealer reveals `table.RiverCard`. | Final betting cycle. | - `AllFoldExceptOne` → `showDownAllFoldExceptOne`.<br>- Otherwise evaluate showdown (`showDown`). |
| `showDown` | At least two players remain. Happens after river betting or immediately if all remaining players are all-in. | - `EvaluateHand` ranks each player using riverboat evaluator.<br>- `AssignChipsToWinners` pays out total pot and side pots.<br>- Winners stay seated unless their stack hits 0. | - Clears player/table actions.<br>- Eliminates busted players and returns to `initRound` for the next hand (unless table/table tournament ends). |
| `showDownAllFoldExceptOne` | Early termination when `AllFoldExceptOne` becomes true at any betting street. No board cards or hand evaluation is needed. | - `UpdateTotalBetForFold` moves the entire pot to the last standing player.<br>- `AssignChipsToWinners` credits chips and winners are announced immediately. | - Clears player/table actions.<br>- Eliminates busted players and returns to `initRound`. |
| `finishTable` | Table has fewer than 2 active players after eliminations. | Signals Temporal to close the table and trigger reshuffle/prize handling. | Table remains in this stage until it’s removed. |
| `finishTournament` | `CheckLastTableInTables` detects only one table remains and it finished. | - Deletes table rows<br>- Inserts ranking entries<br>- Triggers payouts (`HandlePlayerPrize`). | Tournament controller workflow exits. |
| `switchingPlayer` | Reserved stage for seat changes/balance (not yet emitted in current workflow). | Intended to let Unity animate player transfers between tables. | Not used today. |

## Stage-specific helpers

- `AllFoldExceptOne`: recalculated inside `HandleTurn` after every action to detect early winners.
- `AllPlayersAllInExceptFolded` / `AllPlayersAllInExceptOneAndFolded`: short-circuit further betting streets when everyone is already committed. `HandleTurn` returns immediately and the activity proceeds to reveal the remaining community cards up to showdown.
- `PlayerActedInRound`, `LastToRaiserIndex`, and `SetTablePlayersCallAmount` control when a street can close and the stage moves forward.

Having this mapping lets us reason about logs like `[STAGE] Mesa X -> preFlop` or `[STAGE] Mesa X -> showDownAllFoldExceptOne` during simulator runs, and it matches the order enforced in `HandleTableActivitie` plus the regression tests inside `internal/workflow/activities_flow_test.go`.
