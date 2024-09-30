package temporal

import (
	"server/config"
	"server/internal/poker"
	"time"

	"go.temporal.io/sdk/workflow"
)

func PlayerWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) (poker.Table, error) {
	SecTable := poker.Table{}
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 5,
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	err := workflow.ExecuteActivity(ctx, DealPreFlop, &table, config).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	err = workflow.ExecuteActivity(ctx, DealCardsActivity, &table, config).Get(ctx, &SecTable)
	if err != nil {
		return table, err
	}
	table.CurrentStage = StagePreFlop

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	if table.AllFoldExceptOne {
		err = workflow.ExecuteActivity(ctx, ShowDownAllFoldExecptOne, &table).Get(ctx, &table)
		if err != nil {
			return table, err
		}
		return table, nil //ver premios
	}

	table.FlopCards = SecTable.FlopCards

	err = workflow.ExecuteActivity(ctx, DealFlop, &table, config).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	if table.AllFoldExceptOne {
		err = workflow.ExecuteActivity(ctx, ShowDownAllFoldExecptOne, &table).Get(ctx, &table)
		if err != nil {
			return table, err
		}
		return table, nil //ver premios
	}

	table.TurnCard = SecTable.TurnCard

	err = workflow.ExecuteActivity(ctx, DealTurn, &table, config).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	if table.AllFoldExceptOne {
		err = workflow.ExecuteActivity(ctx, ShowDownAllFoldExecptOne, &table).Get(ctx, &table)
		if err != nil {
			return table, err
		}
		return table, nil //ver premios
	}

	table.RiverCard = SecTable.RiverCard

	err = workflow.ExecuteActivity(ctx, DealRiver, &table, config).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	if table.AllFoldExceptOne {
		err = workflow.ExecuteActivity(ctx, ShowDownAllFoldExecptOne, &table).Get(ctx, &table)
		if err != nil {
			return table, err
		}
		return table, nil //ver premios
	}

	table.AssignPlayerCardsFromSecTable(&SecTable)

	err = workflow.ExecuteActivity(ctx, ShowDown, &table).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	return table, nil
}

func RoundWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) (poker.Table, error) {
	table.Round++

	we1 := workflow.ExecuteChildWorkflow(ctx, PlayerWorkflow, table, config)

	err := we1.Get(ctx, &table)
	if err != nil {
		return table, err
	}

	return table, nil
}

func TableWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) (poker.Table, error) {

	we1 := workflow.ExecuteChildWorkflow(ctx, RoundWorkflow, table, config)

	err := we1.Get(ctx, &table)
	if err != nil {
		return table, err
	}

	return table, nil
}

func TournamentWorkflow(ctx workflow.Context, tables []poker.Table, config *config.Config) ([]poker.Table, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 5,
	}
	tournamentEnds := false
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	childWorkflows := make(map[string]workflow.Future)

	for i := 0; i < len(tables); i++ {
		we1 := workflow.ExecuteChildWorkflow(ctx, TableWorkflow, tables[i], config)
		childWorkflows[tables[i].ID] = we1
	}

	for len(childWorkflows) > 0 {
		selector := workflow.NewSelector(ctx)

		err := workflow.ExecuteActivity(ctx, CheckLastTable, &tables, config).Get(ctx, &tournamentEnds)
		if err != nil {
			return tables, err
		}

		if tournamentEnds {
			return tables, nil
		}

		for id, future := range childWorkflows {
			id := id

			selector.AddFuture(future, func(f workflow.Future) {
				var updatedTable poker.Table
				if err := f.Get(ctx, &updatedTable); err == nil {
					for j := 0; j < len(tables); j++ {
						if tables[j].ID == id {
							updatedTable = updateTableFromUpdatedTable(tables[j], updatedTable)
							tables[j] = updatedTable
							break
						}
					}

					err = workflow.ExecuteActivity(ctx, Reshuffle, &tables, updatedTable, config).Get(ctx, &tables)
					if err != nil {
						workflow.GetLogger(ctx).Error("Reshuffle activity failed", "error", err)
						return
					}

					updatedTable, foundTable := poker.GetTableByID(tables, updatedTable.ID)

					if _, ok := childWorkflows[id]; ok {
						if !tableExists(tables, id) {
							delete(childWorkflows, id)
						}
						if foundTable {
							childWorkflows[id] = workflow.ExecuteChildWorkflow(ctx, TableWorkflow, updatedTable, config)
						}
					}
				} else {
					workflow.GetLogger(ctx).Error("Child workflow failed", "error", err)
				}
			})
			if tables[0].Round == 4 {
				return tables, nil
			}
		}
		selector.Select(ctx)
	}

	return tables, nil
}
