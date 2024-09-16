package temporal

import (
	"server/config"
	"server/internal/poker"
	"time"

	"go.temporal.io/sdk/workflow"
)

func TableWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) (poker.Table, error) {
	SecTable := poker.Table{}
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 5,
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	err := workflow.ExecuteActivity(ctx, DealPreFlop, &table, config).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	if len(table.Players) < 2 { //cambiar por min players de table
		return table, nil
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
