package temporal

import (
	"server/config"
	"server/internal/poker"
	"time"

	"go.temporal.io/sdk/workflow"
)

func TableWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) error {
	SecTable := poker.Table{}
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 5,
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	err := workflow.ExecuteActivity(ctx, DealPreFlop, &table, config).Get(ctx, &table)
	if err != nil {
		return err
	}

	err = workflow.ExecuteActivity(ctx, DealCardsActivity, &table, config).Get(ctx, &SecTable)
	if err != nil {
		return err
	}

	table.CurrentStage = "preFlop"

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return err
	}

	if table.AllFold {
		return nil //ver premios
	}

	table.FlopCards = SecTable.FlopCards

	err = workflow.ExecuteActivity(ctx, DealFlop, &table, config).Get(ctx, &table)
	if err != nil {
		return err
	}

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return err
	}

	if table.AllFold {
		return nil //ver premios
	}

	table.TurnCard = SecTable.TurnCard

	err = workflow.ExecuteActivity(ctx, DealTurn, &table, config).Get(ctx, &table)
	if err != nil {
		return err
	}

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return err
	}

	if table.AllFold {
		return nil //ver premios
	}

	table.RiverCard = SecTable.RiverCard

	err = workflow.ExecuteActivity(ctx, DealRiver, &table, config).Get(ctx, &table)
	if err != nil {
		return err
	}

	err = workflow.ExecuteActivity(ctx, HandleTurns, &table).Get(ctx, &table)
	if err != nil {
		return err
	}

	if table.AllFold {
		return nil //ver premios
	}

	return nil
}
