package temporal

import (
	"server/config"
	"server/internal/db"
	"server/internal/poker"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const MaxRoundsBeforeReset = 10

func PlayerWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) (poker.Table, error) {

	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 30,
	}

	if table.Frozen {
		err := workflow.Sleep(ctx, 15*time.Second)
		if err != nil {
			return table, err
		}
		return table, nil
	}

	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	err := workflow.ExecuteActivity(ctx, HandleTableActivitie, &table, config).Get(ctx, &table)
	if err != nil {
		return table, err
	}

	return table, nil
}

func RoundWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) (poker.Table, error) { //use in future?
	table.Round++

	we1 := workflow.ExecuteChildWorkflow(ctx, PlayerWorkflow, table, config)

	err := we1.Get(ctx, &table)
	if err != nil {
		return table, err
	}

	return table, nil
}

func TableWorkflow(ctx workflow.Context, table poker.Table, config *config.Config) (poker.Table, error) { //use in future?

	we1 := workflow.ExecuteChildWorkflow(ctx, RoundWorkflow, table, config)

	err := we1.Get(ctx, &table)
	if err != nil {
		return table, err
	}

	return table, nil
}

func TournamentWorkflow(ctx workflow.Context, tables []poker.Table, config *config.Config) ([]poker.Table, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 2000,
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	childWorkflows := make(map[string]workflow.Future)
	js := GetJetStream()

	for i := 0; i < len(tables); i++ {
		we1 := workflow.ExecuteChildWorkflow(ctx, PlayerWorkflow, tables[i], config)
		childWorkflows[tables[i].ID] = we1
	}

	for len(childWorkflows) > 0 {
		selector := workflow.NewSelector(ctx)

		tournamentEnds, err := poker.CheckLastTableInTables(tables, js)
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

					tables = poker.Reshuffle(tables, updatedTable, js)

					updatedTable, foundTable := poker.GetTableByID(tables, updatedTable.ID)

					if _, ok := childWorkflows[id]; ok {
						if !tableExists(tables, id) {
							delete(childWorkflows, id)
						}
						if foundTable {
							childWorkflows[id] = workflow.ExecuteChildWorkflow(ctx, PlayerWorkflow, updatedTable, config)
						}
					}
				} else {
					workflow.GetLogger(ctx).Error("Child workflow failed", "error", err)
				}
			})
		}
		selector.Select(ctx)
	}

	return tables, nil
}

func TournamentControllerWorkflow(ctx workflow.Context, tournament poker.Tournament, config *config.Config) (poker.Tournament, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 2000,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second * 5,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    1,
			BackoffCoefficient: 2.0,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	now := workflow.Now(ctx)
	waitDuration := tournament.RegistrationEndDate.Sub(now)

	if waitDuration > 0 {
		err := workflow.Sleep(ctx, waitDuration)
		if err != nil {
			ctx.Done()
			return tournament, err
		}
	}

	now = workflow.Now(ctx)
	err := workflow.ExecuteActivity(ctx, CreatePrizePool, &tournament, config).Get(ctx, &tournament)

	if err != nil {
		db.UpdateTournamentOngoing(tournament.ID, false)
		db.SetTournamentEndDate(tournament.ID)
		ctx.Done()
		return tournament, err
	}

	err = workflow.ExecuteActivity(ctx, CreateTablesInTournament, &tournament, config).Get(ctx, &tournament)
	if err != nil {
		db.UpdateTournamentOngoing(tournament.ID, false)
		db.SetTournamentEndDate(tournament.ID)
		ctx.Done()
		return tournament, err
	}

	waitDuration = tournament.StartDate.Sub(now)

	if waitDuration > 0 {
		err := workflow.Sleep(ctx, waitDuration)
		if err != nil {
			ctx.Done()
			return tournament, err
		}
	}

	err = db.UpdateTournamentOngoing(tournament.ID, true)
	if err != nil {
		db.UpdateTournamentOngoing(tournament.ID, false)
		db.SetTournamentEndDate(tournament.ID)
		ctx.Done()
		return tournament, err
	}

	err = db.UpdateTournamentStart(tournament.ID, true)
	if err != nil {
		db.UpdateTournamentOngoing(tournament.ID, false)
		db.SetTournamentEndDate(tournament.ID)
		ctx.Done()
		return tournament, err
	}

	we1 := workflow.ExecuteChildWorkflow(ctx, TournamentWorkflow, tournament.Tables, config)

	err = we1.Get(ctx, &tournament.Tables)
	if err != nil {
		db.UpdateTournamentOngoing(tournament.ID, false)
		db.SetTournamentEndDate(tournament.ID)
		ctx.Done()
		return tournament, err
	}
	db.SetTournamentEndDate(tournament.ID)

	err = db.UpdateTournamentOngoing(tournament.ID, false)
	if err != nil {
		ctx.Done()
		return tournament, err
	}
	ctx.Done()
	return tournament, nil
}
