package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"server/config"
	"server/internal/codec"
	"server/internal/db"
	"server/internal/db/models"
	internalnats "server/internal/nats"
	"server/internal/poker"
	temporal "server/internal/workflow"

	"github.com/nats-io/nats.go"
	"go.temporal.io/sdk/client"
	"gorm.io/gorm"
)

const (
	defaultMaxTableSize  = 9
	defaultMinTableSize  = 2
	defaultStartingChips = 2500
	defaultBigBlind      = 50
)

func main() {
	totalPlayers := flag.Int("players", 27, "cantidad total de jugadores en el torneo de prueba")
	tablesWanted := flag.Int("tables", 3, "cantidad de mesas iniciales a crear")
	unityWallet := flag.String("unity-wallet", "unity-demo", "wallet que representará al jugador real")
	tournamentFlag := flag.String("tournament-id", "UNITY-DEMO-27", "identificador lógico del torneo de pruebas (puede ser texto, se convierte a número interno)")
	turnSeconds := flag.Int("turn-duration", 15, "duración del turno por jugador en segundos")
	seed := flag.Int64("seed", time.Now().UnixNano(), "semilla para reproducir las acciones aleatorias")
	mode := flag.String("mode", "sim", "sim=torneo simulado, client=consola tipo Unity")
	useTemporal := flag.Bool("temporal", true, "ejecutar los workflows reales de Temporal en lugar del bucle local")
	flag.Parse()

	if *totalPlayers < 2 {
		log.Fatalf("se requieren al menos 2 jugadores para iniciar el torneo de prueba")
	}
	if *tablesWanted < 1 {
		log.Fatalf("se requiere al menos una mesa para iniciar el torneo")
	}

	cpID, tournamentNumericID, err := resolveTournamentSubject(*tournamentFlag)
	if err != nil {
		log.Fatalf("tournament-id inválido: %v", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("no se pudo cargar la configuración: %v", err)
	}

	nc, js, err := internalnats.Connect(cfg)
	if err != nil {
		log.Fatalf("no se pudo conectar a NATS: %v", err)
	}
	defer nc.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	switch *mode {
	case "client":
		log.Printf("modo consola Unity apuntando a torneo %s (ID efectivo %s) / jugador %s", *tournamentFlag, cpID, *unityWallet)
		if err := runUnityConsoleClient(ctx, nc, js, cpID, *unityWallet); err != nil {
			log.Fatalf("cliente de consola finalizó con error: %v", err)
		}
	default:
		SIMPlayers := max(*totalPlayers, *tablesWanted*defaultMaxTableSize)
		dbConn, err := db.InitDB(&cfg.Database)
		if err != nil {
			log.Fatalf("no se pudo conectar a la base de datos: %v", err)
		}
		if err := db.Migrate(); err != nil {
			log.Fatalf("no se pudieron ejecutar las migraciones: %v", err)
		}
		temporal.SetJetStream(js)
		temporal.SetNATSConnection(nc)
		temporal.SetDBInstance(dbConn)

		builder := &workflowSimBuilder{
			cfg:               cfg,
			js:                js,
			nc:                nc,
			totalPlayers:      SIMPlayers,
			tables:            *tablesWanted,
			unityWallet:       *unityWallet,
			tournamentLabel:   *tournamentFlag,
			tournamentID:      tournamentNumericID,
			tournamentSubject: cpID,
			turnSeconds:       *turnSeconds,
			seed:              *seed,
			startChips:        defaultStartingChips,
			bigBlind:          defaultBigBlind,
			useTemporal:       *useTemporal,
		}

		sim, err := builder.Build(ctx)
		if err != nil {
			log.Fatalf("no se pudo preparar la simulación: %v", err)
		}

		log.Printf("torneo demo %s listo (ID efectivo %s). jugador unity: %s", *tournamentFlag, cpID, *unityWallet)
		log.Printf("suscríbete a pokerServer.%s.*.%s para recibir estados y publica acciones en pokerClient.%s.<table>.%s",
			cpID, *unityWallet, cpID, *unityWallet)
		log.Println("presiona CTRL+C para finalizar la simulación")

		if err := sim.Run(ctx); err != nil {
			log.Fatalf("el torneo simulado finalizó con error: %v", err)
		}
	}
}

type workflowSimBuilder struct {
	cfg               *config.Config
	js                nats.JetStreamContext
	nc                *nats.Conn
	totalPlayers      int
	tables            int
	unityWallet       string
	tournamentLabel   string
	tournamentID      uint
	tournamentSubject string
	turnSeconds       int
	seed              int64
	startChips        int
	bigBlind          int
	useTemporal       bool
}

func (b *workflowSimBuilder) Build(ctx context.Context) (*workflowSimulation, error) {
	if db.GetDB() == nil {
		return nil, errors.New("la base de datos no está inicializada")
	}
	if err := internalnats.ConfigureStream(b.js, &b.cfg.NATS.Stream); err != nil {
		return nil, fmt.Errorf("no se pudo configurar el stream de NATS: %w", err)
	}

	var temporalClient client.Client
	if b.useTemporal {
		dataConverter := codec.NewGzipDataConverter(10 * 1024)
		temporal.StartWorker(b.cfg, dataConverter)
		clientOptions := client.Options{
			HostPort:      b.cfg.Temporal.HostPort,
			DataConverter: dataConverter,
		}
		c, err := client.Dial(clientOptions)
		if err != nil {
			return nil, fmt.Errorf("no se pudo crear el cliente de Temporal: %w", err)
		}
		temporalClient = c
	}
	wallets := b.buildWalletList()
	seeder := &demoTournamentSeeder{
		cfg:            b.cfg,
		db:             db.GetDB(),
		tournamentID:   b.tournamentID,
		tournamentName: fmt.Sprintf("%s-%s", b.tournamentLabel, time.Now().Format("20060102150405")),
		wallets:        wallets,
		turnSeconds:    b.turnSeconds,
		startChips:     b.startChips,
		bigBlind:       b.bigBlind,
		unityWallet:    b.unityWallet,
		desiredTables:  b.tables,
	}

	tournament, err := seeder.Seed(ctx)
	if err != nil {
		return nil, err
	}

	return newWorkflowSimulation(b.cfg, b.js, b.nc, tournament, b.tournamentSubject, b.unityWallet, b.seed, b.useTemporal, temporalClient), nil
}

func (b *workflowSimBuilder) buildWalletList() []string {
	unique := map[string]struct{}{}
	result := make([]string, 0, b.totalPlayers)
	botIdx := 1
	for len(result) < b.totalPlayers-1 {
		candidate := fmt.Sprintf("unity-bot-%02d", botIdx)
		botIdx++
		if candidate == b.unityWallet {
			continue
		}
		if _, exists := unique[candidate]; exists {
			continue
		}
		unique[candidate] = struct{}{}
		result = append(result, candidate)
	}
	result = append(result, b.unityWallet)
	return result
}

type demoTournamentSeeder struct {
	cfg            *config.Config
	db             *gorm.DB
	tournamentID   uint
	tournamentName string
	wallets        []string
	turnSeconds    int
	startChips     int
	bigBlind       int
	unityWallet    string
	desiredTables  int
}

func (s *demoTournamentSeeder) Seed(ctx context.Context) (poker.Tournament, error) {
	if err := purgeTournamentData(s.db, s.tournamentID); err != nil {
		return poker.Tournament{}, err
	}

	model := models.Tournament{
		ID:                    s.tournamentID,
		Name:                  s.tournamentName,
		RegistrationStartDate: time.Now().Add(-10 * time.Minute),
		RegistrationEndDate:   time.Now().Add(-5 * time.Minute),
		StartDate:             time.Now().Add(-1 * time.Minute),
		EntryCost:             50,
		Currency:              "usdt",
		MinPlayers:            2,
		MaxPlayers:            len(s.wallets),
		TurnSeconds:           s.turnSeconds,
		StartChips:            s.startChips,
		BBValue:               s.bigBlind,
		IncrementBlind:        5,
		Ongoing:               true,
	}

	if err := s.db.Create(&model).Error; err != nil {
		return poker.Tournament{}, fmt.Errorf("no se pudo crear el torneo demo: %w", err)
	}

	for _, wallet := range s.wallets {
		walletID, err := ensureWalletExists(s.db, wallet)
		if err != nil {
			return poker.Tournament{}, err
		}
		if err := db.RegisterUserToTournament(model.ID, walletID); err != nil {
			return poker.Tournament{}, fmt.Errorf("no se pudo registrar %s en torneo demo: %w", wallet, err)
		}
	}

	pokerTournament := poker.Tournament{
		ID:             model.ID,
		Name:           model.Name,
		EntryCost:      model.EntryCost,
		Currency:       model.Currency,
		TurnSeconds:    model.TurnSeconds,
		StartChips:     model.StartChips,
		BBValue:        model.BBValue,
		IncrementBlind: model.IncrementBlind,
	}

	if _, err := temporal.CreatePrizePool(ctx, &pokerTournament, s.cfg); err != nil {
		return poker.Tournament{}, fmt.Errorf("no se pudo crear el prize pool demo: %w", err)
	}

	created, err := temporal.CreateTablesInTournament(ctx, &pokerTournament, s.cfg)
	if err != nil {
		return poker.Tournament{}, fmt.Errorf("no se pudieron crear las mesas demo: %w", err)
	}

	return *created, nil
}

func purgeTournamentData(dbConn *gorm.DB, tournamentID uint) error {
	if tournamentID == 0 {
		return nil
	}
	entities := []interface{}{
		&models.TablePlayer{},
		&models.Table{},
		&models.Ranking{},
		&models.Prize{},
		&models.TransactionRecord{},
		&models.TournamentRegistration{},
	}
	for _, entity := range entities {
		if err := dbConn.Where("tournament_id = ?", tournamentID).Delete(entity).Error; err != nil {
			return err
		}
	}
	return dbConn.Where("id = ?", tournamentID).Delete(&models.Tournament{}).Error
}

func ensureWalletExists(dbConn *gorm.DB, wallet string) (uint, error) {
	var record models.Wallet
	result := dbConn.Where("wallet = ?", wallet).First(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		record = models.Wallet{Wallet: wallet}
		if err := dbConn.Create(&record).Error; err != nil {
			return 0, fmt.Errorf("no se pudo crear wallet demo %s: %w", wallet, err)
		}
		return record.ID, nil
	}
	if result.Error != nil {
		return 0, result.Error
	}
	return record.ID, nil
}

type workflowSimulation struct {
	cfg               *config.Config
	js                nats.JetStreamContext
	nc                *nats.Conn
	tournament        poker.Tournament
	tournamentSubject string
	unityWallet       string
	tables            []poker.Table
	bots              []*botEngine
	rng               *rand.Rand
	handleTable       tableActivity
	checkTournament   tournamentCheck
	reshuffle         reshuffleFunc
	useTemporal       bool
	temporalClient    client.Client
}

type tableActivity func(ctx context.Context, table *poker.Table, cfg *config.Config) (*poker.Table, error)
type tournamentCheck func(tables []poker.Table, js nats.JetStreamContext) (bool, error)
type reshuffleFunc func(tables []poker.Table, updated poker.Table, js nats.JetStreamContext) []poker.Table

func newWorkflowSimulation(cfg *config.Config, js nats.JetStreamContext, nc *nats.Conn, tournament poker.Tournament, subject string, unityWallet string, seed int64, useTemporal bool, temporalClient client.Client) *workflowSimulation {
	rng := rand.New(rand.NewSource(seed))
	seen := make(map[string]struct{})
	bots := []*botEngine{}
	for _, table := range tournament.Tables {
		for _, player := range table.Players {
			if player.ID == unityWallet {
				continue
			}
			if _, ok := seen[player.ID]; ok {
				continue
			}
			seen[player.ID] = struct{}{}
			bots = append(bots, newBotEngine(nc, js, subject, player.ID, rng.Int63()))
		}
	}
	return &workflowSimulation{
		cfg:               cfg,
		js:                js,
		nc:                nc,
		tournament:        tournament,
		tournamentSubject: subject,
		unityWallet:       unityWallet,
		tables:            tournament.Tables,
		bots:              bots,
		rng:               rng,
		handleTable:       temporal.HandleTableActivitie,
		checkTournament:   poker.CheckLastTableInTables,
		reshuffle:         poker.Reshuffle,
		useTemporal:       useTemporal,
		temporalClient:    temporalClient,
	}
}

func (s *workflowSimulation) Run(ctx context.Context) error {
	if s.useTemporal {
		return s.runWithTemporal(ctx)
	}
	return s.runLocal(ctx)
}

func (s *workflowSimulation) runWithTemporal(ctx context.Context) error {
	if s.temporalClient == nil {
		return errors.New("Temporal client no inicializado; ejecuta con -temporal=false para modo local")
	}
	defer s.temporalClient.Close()

	botCtx, cancelBots := context.WithCancel(ctx)
	var botWG sync.WaitGroup
	for _, bot := range s.bots {
		botWG.Add(1)
		go bot.Run(botCtx, &botWG)
	}
	defer func() {
		cancelBots()
		botWG.Wait()
	}()

	stateCtx, cancelState := context.WithCancel(ctx)
	var stateWG sync.WaitGroup
	stateWG.Add(1)
	go s.watchTableStates(stateCtx, &stateWG)
	defer func() {
		cancelState()
		stateWG.Wait()
	}()

	workflowID := fmt.Sprintf("sim-tournament-%d-%d", s.tournament.ID, time.Now().UnixNano())
	log.Printf("[TEMPORAL] Ejecutando workflow %s", workflowID)
	we, err := s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "poker-task-queue",
	}, temporal.TournamentWorkflow, s.tournament.Tables, s.cfg)
	if err != nil {
		return fmt.Errorf("no se pudo iniciar TournamentWorkflow: %w", err)
	}

	var result []poker.Table
	if err := we.Get(ctx, &result); err != nil {
		return fmt.Errorf("el workflow %s falló: %w", workflowID, err)
	}
	log.Printf("[TEMPORAL] Workflow %s completado correctamente", workflowID)
	s.describeResultTables(result)
	return nil
}

func (s *workflowSimulation) runLocal(ctx context.Context) error {
	botCtx, cancelBots := context.WithCancel(ctx)
	var botWG sync.WaitGroup
	for _, bot := range s.bots {
		botWG.Add(1)
		go bot.Run(botCtx, &botWG)
	}
	defer func() {
		cancelBots()
		botWG.Wait()
	}()

	for hand := 1; ; hand++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		finished, err := s.playHand(ctx, hand)
		if err != nil {
			return err
		}
		if finished {
			log.Printf("[TORNEO] Finalizado. %s", s.describeWinners())
			return nil
		}
	}
}

func (s *workflowSimulation) watchTableStates(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	subject := fmt.Sprintf("pokerServer.%s.>", s.tournamentSubject)
	msgCh := make(chan *nats.Msg, 256)
	sub, err := s.nc.ChanSubscribe(subject, msgCh)
	if err != nil {
		log.Printf("[STATE] no se pudo subscribir a %s: %v", subject, err)
		return
	}
	defer sub.Unsubscribe()

	stageByTable := make(map[string]string)
	lastStageAt := make(map[string]time.Time)
	stuckLogged := make(map[string]bool)
	store := newTableStateStore()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-msgCh:
			if msg == nil {
				continue
			}
			parts := strings.Split(msg.Subject, ".")
			if len(parts) != 3 {
				continue
			}
			var table poker.Table
			if err := json.Unmarshal(msg.Data, &table); err != nil {
				continue
			}
			stage := table.CurrentStage
			if stage != "" {
				if stage != stageByTable[table.ID] {
					log.Printf("[STAGE] Mesa %s -> %s (mano #%d)", table.ID, stage, table.Round)
					stageByTable[table.ID] = stage
					lastStageAt[table.ID] = time.Now()
					stuckLogged[table.ID] = false
				} else if ts, ok := lastStageAt[table.ID]; ok {
					turnWindow := time.Duration(max(table.TurnTime*2, 30)) * time.Second
					if turnWindow <= 0 {
						turnWindow = 30 * time.Second
					}
					if !stuckLogged[table.ID] && time.Since(ts) > turnWindow {
						log.Printf("[STUCK] Mesa %s lleva %.0fs en stage %s (turn=%ds)", table.ID, time.Since(ts).Seconds(), stage, table.TurnTime)
						stuckLogged[table.ID] = true
					}
				}
			}
			if changed, players, tables := store.update(table); changed {
				log.Printf("[STATE] Quedan %d jugadores distribuidos en %d mesas", players, tables)
			}
		}
	}
}

func (s *workflowSimulation) describeResultTables(result []poker.Table) {
	if len(result) == 0 {
		log.Printf("[TORNEO] Finalizado sin mesas activas")
		return
	}
	s.tables = result
	log.Printf("[TORNEO] Finalizado. %s", s.describeWinners())
}

func (s *workflowSimulation) playHand(ctx context.Context, hand int) (bool, error) {
	active := make([]poker.Table, 0, len(s.tables))
	for _, table := range s.tables {
		if table.TableEnds || len(table.Players) == 0 {
			continue
		}
		log.Printf("[MESA %s] Comienza mano #%d (jugadores vivos: %d)", table.ID, table.Round+1, table.CountActivePlayers())
		active = append(active, table)
	}

	if len(active) == 0 {
		log.Printf("[TORNEO] No quedan mesas activas, la simulación terminó")
		return true, nil
	}

	log.Printf("[SIM] Ejecutando mano #%d en paralelo (%d mesas activas)", hand, len(active))

	type tableResult struct {
		table poker.Table
		err   error
	}

	results := make(chan tableResult, len(active))
	var wg sync.WaitGroup
	for _, tbl := range active {
		wg.Add(1)
		tableCopy := tbl
		go func(tableData poker.Table) {
			defer wg.Done()
			updated, err := s.handleTable(ctx, &tableData, s.cfg)
			if err != nil {
				results <- tableResult{table: tableData, err: err}
				return
			}
			results <- tableResult{table: *updated}
		}(tableCopy)
	}

	wg.Wait()
	close(results)

	for res := range results {
		if res.err != nil {
			return false, fmt.Errorf("mesa %s falló: %w", res.table.ID, res.err)
		}
		s.replaceTable(res.table)
		s.logTableResult(res.table)
		s.tables = s.reshuffle(s.tables, res.table, s.js)
		s.logTournamentState()
	}

	finished, err := s.checkTournament(s.tables, s.js)
	if err != nil {
		return false, err
	}
	return finished, nil
}

func (s *workflowSimulation) replaceTable(updated poker.Table) {
	for i := range s.tables {
		if s.tables[i].ID == updated.ID {
			s.tables[i] = updated
			return
		}
	}
	s.tables = append(s.tables, updated)
}

func (s *workflowSimulation) logTournamentState() {
	alive := 0
	activeTables := 0
	for _, table := range s.tables {
		playersAlive := 0
		for _, player := range table.Players {
			if !player.IsEliminated {
				playersAlive++
			}
		}
		if playersAlive > 0 && !table.TableEnds {
			activeTables++
		}
		alive += playersAlive
	}
	log.Printf("[STATE] Quedan %d jugadores distribuidos en %d mesas", alive, activeTables)
}

func (s *workflowSimulation) logTableResult(table poker.Table) {
	if len(table.Winners) == 0 {
		log.Printf("[MESA %s] Mano #%d finalizada. Stage=%s", table.ID, table.Round, table.CurrentStage)
		return
	}
	winners := make([]string, 0, len(table.Winners))
	for _, winner := range table.Winners {
		label := winner.ID
		if winner.HandDescription != "" {
			label = fmt.Sprintf("%s (%s)", label, winner.HandDescription)
		}
		winners = append(winners, label)
	}
	log.Printf("[MESA %s] Mano #%d: ganadores %s", table.ID, table.Round, strings.Join(winners, ", "))
}

func (s *workflowSimulation) describeWinners() string {
	if len(s.tables) == 0 {
		return "Sin mesas registradas"
	}
	var winners []string
	for _, player := range s.tables[0].Players {
		if !player.IsEliminated {
			winners = append(winners, player.ID)
		}
	}
	if len(winners) == 0 {
		return "No se detectaron ganadores"
	}
	return fmt.Sprintf("Ganador(es): %s", strings.Join(winners, ", "))
}

type botEngine struct {
	wallet            string
	tournamentSubject string
	nc                *nats.Conn
	js                nats.JetStreamContext
	rng               *rand.Rand
}

func newBotEngine(nc *nats.Conn, js nats.JetStreamContext, subject, wallet string, seed int64) *botEngine {
	return &botEngine{
		wallet:            wallet,
		tournamentSubject: subject,
		nc:                nc,
		js:                js,
		rng:               rand.New(rand.NewSource(seed)),
	}
}

func (b *botEngine) Run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	subject := fmt.Sprintf("pokerServer.%s.*.%s", b.tournamentSubject, b.wallet)
	msgCh := make(chan *nats.Msg, 32)
	sub, err := b.nc.ChanSubscribe(subject, msgCh)
	if err != nil {
		log.Printf("[BOT %s] no se pudo suscribir a %s: %v", b.wallet, subject, err)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-msgCh:
			if msg == nil {
				continue
			}
			var player poker.Player
			if err := json.Unmarshal(msg.Data, &player); err != nil {
				continue
			}
			if !player.IsTurn || player.IsEliminated || player.ID != b.wallet {
				continue
			}
			action := b.randomAction(player)
			if action.LastAction == "" {
				continue
			}
			if err := publishUnityAction(b.js, b.tournamentSubject, player.CurrentTable, b.wallet, action); err != nil {
				log.Printf("[BOT %s] error enviando acción %s: %v", b.wallet, action.LastAction, err)
			} else {
				log.Printf("[BOT %s] %s (%d) en mesa %s", b.wallet, action.LastAction, action.LastBet, player.CurrentTable)
			}
		}
	}
}

func (b *botEngine) randomAction(state poker.Player) poker.Player {
	allowed := state.AvailableActions
	if len(allowed) == 0 {
		allowed = []string{"fold"}
	}
	choice := allowed[b.rng.Intn(len(allowed))]
	action := poker.Player{ID: state.ID, LastAction: choice}
	switch choice {
	case "fold":
		return action
	case "check":
		return action
	case "call":
		bet := state.CallAmount
		if bet <= 0 {
			bet = min(state.Chips, defaultBigBlind)
		}
		if bet > state.Chips {
			bet = state.Chips
		}
		action.LastBet = bet
		return action
	case "raise":
		minRaise := max(state.CallAmount+defaultBigBlind, defaultBigBlind)
		if minRaise > state.Chips {
			minRaise = state.Chips
		}
		maxRaise := state.Chips
		if maxRaise <= 0 {
			return poker.Player{ID: state.ID, LastAction: "call", LastBet: min(state.CallAmount, state.Chips)}
		}
		rangeSize := maxRaise - minRaise
		amount := minRaise
		if rangeSize > 0 {
			amount += b.rng.Intn(rangeSize + 1)
		}
		action.LastBet = amount
		return action
	case "allin":
		action.LastBet = state.Chips
		return action
	default:
		return poker.Player{ID: state.ID, LastAction: "fold"}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type tableStateStore struct {
	mu          sync.RWMutex
	tables      map[string]poker.Table
	lastPlayers int
	lastTables  int
}

func newTableStateStore() *tableStateStore {
	return &tableStateStore{tables: make(map[string]poker.Table)}
}

func (s *tableStateStore) update(table poker.Table) (bool, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tables[table.ID] = table
	players, tables := s.countLocked()
	changed := players != s.lastPlayers || tables != s.lastTables
	if changed {
		s.lastPlayers = players
		s.lastTables = tables
	}
	return changed, players, tables
}

func (s *tableStateStore) countLocked() (int, int) {
	totalPlayers := 0
	activeTables := 0
	for _, tbl := range s.tables {
		alive := 0
		for _, p := range tbl.Players {
			if !p.IsEliminated {
				alive++
			}
		}
		if alive > 0 {
			activeTables++
		}
		totalPlayers += alive
	}
	return totalPlayers, activeTables
}

func (s *tableStateStore) get(tableID string) (poker.Table, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tbl, ok := s.tables[tableID]
	return tbl, ok
}

func (s *tableStateStore) turnCountdown(tableID string) (time.Duration, bool) {
	table, ok := s.get(tableID)
	if !ok || table.EndTime == 0 {
		return 0, false
	}
	deadline := time.Unix(int64(table.EndTime), 0)
	return time.Until(deadline), true
}

func clampToSeconds(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int((d + time.Second/2) / time.Second)
}

func runUnityConsoleClient(ctx context.Context, nc *nats.Conn, js nats.JetStreamContext, tournamentID, wallet string) error {
	updates := make(chan poker.Player, 64)
	turns := make(chan poker.Player, 4)
	tableStore := newTableStateStore()
	var turnMu sync.Mutex
	activeTurnKey := ""
	subject := fmt.Sprintf("pokerServer.%s.>", tournamentID)

	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		parts := strings.Split(msg.Subject, ".")
		if len(parts) == 3 {
			var table poker.Table
			if err := json.Unmarshal(msg.Data, &table); err != nil {
				return
			}
			if changed, players, tables := tableStore.update(table); changed {
				log.Printf("[TORNEO] %d jugadores activos en %d mesas", players, tables)
			}
			return
		}

		player, ok := parsePlayerMessage(msg, tournamentID, wallet)
		if !ok {
			return
		}
		select {
		case updates <- player:
		default:
			log.Printf("cola de actualizaciones llena, se descarta mensaje")
		}

		turnMu.Lock()
		if player.ID == wallet && !player.IsTurn && activeTurnKey != "" {
			activeTurnKey = ""
		}
		if player.IsTurn {
			key := fmt.Sprintf("%s-%s-%d-%d", player.CurrentTable, player.ID, player.CallAmount, player.Chips)
			if activeTurnKey == "" {
				activeTurnKey = key
				turns <- player
			}
		}
		turnMu.Unlock()
	})
	if err != nil {
		return fmt.Errorf("no se pudo subscribir a actualizaciones: %w", err)
	}
	defer sub.Drain()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case player := <-updates:
				printPlayerUpdate(player, tableStore)
			}
		}
	}()

	reader := bufio.NewScanner(os.Stdin)
	for {
		select {
		case <-ctx.Done():
			return nil
		case turn := <-turns:
			if turn.CurrentTable == "" {
				log.Printf("turno recibido pero no se conoce mesa actual, ignorando")
				turnMu.Lock()
				activeTurnKey = ""
				turnMu.Unlock()
				continue
			}
			deadlineMsg := ""
			if remaining, ok := tableStore.turnCountdown(turn.CurrentTable); ok {
				secs := clampToSeconds(remaining)
				deadlineMsg = fmt.Sprintf(" | Tiempo restante aprox: %ds", secs)
			}
			fmt.Printf("\n==> TURNO | Mesa: %s | Call: %d | Stack: %d%s | Acciones: %v\n", turn.CurrentTable, turn.CallAmount, turn.Chips, deadlineMsg, turn.AvailableActions)
			fmt.Println("   Escribe un comando: fold | check | call <monto> | raise <monto> | allin [monto opcional]")
			for {
				fmt.Print("unity> ")
				if !reader.Scan() {
					return reader.Err()
				}
				line := strings.TrimSpace(reader.Text())
				if line == "" {
					continue
				}
				action, err := buildConsoleAction(line, turn)
				if err != nil {
					fmt.Printf("   ⚠ %v\n", err)
					continue
				}
				if err := publishUnityAction(js, tournamentID, turn.CurrentTable, wallet, action); err != nil {
					fmt.Printf("   ⚠ error enviando acción: %v\n", err)
					continue
				}
				fmt.Printf("   ✅ Acción %s enviada a %s\n", action.LastAction, turn.CurrentTable)
				turnMu.Lock()
				activeTurnKey = ""
				turnMu.Unlock()
				break
			}
		}
	}
}

func parsePlayerMessage(msg *nats.Msg, tournamentID, wallet string) (poker.Player, bool) {
	parts := strings.Split(msg.Subject, ".")
	if len(parts) < 4 {
		return poker.Player{}, false
	}
	if parts[0] != "pokerServer" || parts[1] != tournamentID {
		return poker.Player{}, false
	}
	if parts[len(parts)-1] != wallet {
		return poker.Player{}, false
	}
	var player poker.Player
	if err := json.Unmarshal(msg.Data, &player); err != nil {
		log.Printf("no se pudo decodificar mensaje: %v", err)
		return poker.Player{}, false
	}
	if player.CurrentTable == "" {
		player.CurrentTable = parts[len(parts)-2]
	}
	return player, true
}

func printPlayerUpdate(player poker.Player, tables *tableStateStore) {
	table, hasTable := tables.get(player.CurrentTable)
	if player.LastAction == "await_action" || player.IsTurn {
		deadlineInfo := ""
		if remaining, ok := tables.turnCountdown(player.CurrentTable); ok {
			deadlineInfo = fmt.Sprintf(" | Tiempo restante ~%ds", clampToSeconds(remaining))
		}
		stage := ""
		if hasTable && table.CurrentStage != "" {
			stage = fmt.Sprintf(" | Stage=%s", table.CurrentStage)
		}
		log.Printf("Mesa %s | Esperando acción%s%s | Call=%d | Stack=%d | Acciones=%v", player.CurrentTable, stage, deadlineInfo, player.CallAmount, player.Chips, player.AvailableActions)
		return
	}
	if player.IsAFK {
		log.Printf("Mesa %s | Tiempo agotado → acción automática %s", player.CurrentTable, player.LastAction)
		return
	}
	status := fmt.Sprintf("Mesa %s | Acción=%s | Stack=%d", player.CurrentTable, player.LastAction, player.Chips)
	if player.SwitchingTable {
		status += " | Cambiando de mesa"
	}
	if player.IsEliminated {
		status += " | ELIMINADO"
	}
	if hasTable && table.CurrentStage != "" {
		status += fmt.Sprintf(" | Stage=%s", table.CurrentStage)
	}
	log.Println(status)
}

func buildConsoleAction(input string, turn poker.Player) (poker.Player, error) {
	fields := strings.Fields(strings.ToLower(input))
	if len(fields) == 0 {
		return poker.Player{}, fmt.Errorf("comando vacío")
	}
	cmd := fields[0]
	normalized := canonicalAction(cmd)
	if len(turn.AvailableActions) > 0 && !containsAction(turn.AvailableActions, normalized) {
		return poker.Player{}, fmt.Errorf("acción %s no permitida. Disponibles: %v", normalized, turn.AvailableActions)
	}
	amount := 0
	if len(fields) > 1 {
		val, err := strconv.Atoi(fields[1])
		if err != nil {
			return poker.Player{}, fmt.Errorf("monto inválido: %v", err)
		}
		amount = val
	}
	action := poker.Player{ID: turn.ID, LastAction: normalized}
	switch normalized {
	case "fold":
		return action, nil
	case "check", "pass":
		action.LastAction = "check"
		if turn.CallAmount > 0 {
			return poker.Player{}, fmt.Errorf("no puedes hacer check, debes pagar %d", turn.CallAmount)
		}
		return action, nil
	case "call":
		bet := turn.CallAmount
		if amount > 0 {
			bet = amount
		}
		action.LastBet = bet
		return action, nil
	case "raise":
		if amount <= 0 {
			return poker.Player{}, fmt.Errorf("indica el monto del raise")
		}
		action.LastBet = amount
		return action, nil
	case "allin":
		bet := turn.Chips
		if amount > 0 && amount <= turn.Chips {
			bet = amount
		}
		action.LastAction = "allin"
		action.LastBet = bet
		return action, nil
	default:
		return poker.Player{}, fmt.Errorf("acción desconocida: %s", cmd)
	}
}

func publishUnityAction(js nats.JetStreamContext, tournamentID, tableID, wallet string, action poker.Player) error {
	subject := fmt.Sprintf("pokerClient.%s.%s.%s", tournamentID, tableID, wallet)
	data, err := json.Marshal(action)
	if err != nil {
		return fmt.Errorf("no se pudo codificar acción: %w", err)
	}
	if _, err := js.Publish(subject, data); err != nil {
		return fmt.Errorf("no se pudo publicar acción en %s: %w", subject, err)
	}
	return nil
}

func containsAction(actions []string, target string) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func canonicalAction(cmd string) string {
	switch cmd {
	case "pass":
		return "check"
	default:
		return cmd
	}
}

func resolveTournamentSubject(raw string) (string, uint, error) {
	val := strings.TrimSpace(raw)
	if val == "" {
		val = "1001"
	}
	if num, err := strconv.Atoi(val); err == nil {
		if num <= 0 {
			return "", 0, fmt.Errorf("el tournament-id debe ser positivo")
		}
		return strconv.Itoa(num), uint(num), nil
	}
	hash := crc32.ChecksumIEEE([]byte(val))
	mapped := int(hash%900000) + 100000
	return strconv.Itoa(mapped), uint(mapped), nil
}
