# Copilot Workspace Instructions

## 1. Dominio y servicios involucrados
- **Servidor Go (este repo)**: coordina torneos multi-mesa, valida registros on-chain, mantiene el estado de mesas en memoria, orquesta workflows de Temporal y publica/consume eventos en NATS JetStream.
- **Cliente Unity**: renderiza las mesas en 3D, muestra los estados recibidos por NATS (`pokerServer.*`) y publica las acciones del jugador (`pokerClient.*`). Corre C# sobre .NET Standard; puede conectarse vía TCP o WebSocket (puerto 4222/9222) usando el SDK oficial de NATS.
- **Front/backoffice web**: expone inscripción, lobby, ranking y panel admin sobre HTTP (ver rutas en `internal/server/routes/routes.go`). Usa los endpoints protegidos para leer/escribir contra Postgres.

## 2. DER (entidades clave y relaciones)
- `Wallet (1)` ↔ `TournamentRegistration (N)` ↔ `Tournament`.
- `Tournament (1)` ↔ `Table (N)` ↔ `TablePlayer` (cruce con `Wallet`).
- `Tournament` ↔ `Ranking`, `Prize`, `TournamentChip`, `TransactionRecord`, `RecordLog` (todas 1:N).
- `Prize.PrizeList` almacena JSON con `position`, `prize`, `currency`, `wallet_address`.
- Eliminaciones actualizan `TournamentRegistration.eliminated`, liberan `TablePlayer` y generan filas en `Ranking`.

## 3. Flujos críticos
1. **Creación de torneos**: `internal/server/server.go` dispara `createTournaments()` (o admin POST `/tournaments`). Se persiste en Postgres y se inicia `TournamentControllerWorkflow` en Temporal.
2. **Registro**: POST `/tournament/register` valida la transacción on-chain (BSC/USDT o Sepolia) antes de insertar en `TournamentRegistration`.
3. **Inicio de torneo** (`TournamentControllerWorkflow`): espera cierre de registro, genera `PrizeList`, crea mesas (`CreateTablesInTournament`), marca `start/ongoing` y lanza `TournamentWorkflow`.
4. **Ciclo de mesa** (`HandleTableActivitie`): por ronda -> reparte cartas, avanza etapas (pre-flop → flop → turn → river → showdown), publica el estado y espera acciones de Unity por JetStream.
5. **Acciones de jugador**: Unity publica JSON con últimos valores (`LastAction`, `LastBet`, flags). El servidor valida y actualiza side pots, turnos y winners.
6. **Reshuffle / balanceo**: `poker.Reshuffle` mueve jugadores entre mesas cuando se cierran; se notifica por NATS y se depuran tablas en DB.
7. **Pagos**: al finalizar `TournamentWorkflow`, `poker.ProcessTransfers` ejecuta payouts (Sepolia/BSC) y persiste `TransactionRecord`.

## 4. Mensajería NATS / JetStream
- **Config**: conexión `nats://<user>:<pass>@<host>:4222`; stream principal `POKER_TOURNAMENT` con subjects `pokerServer.>` y `pokerClient.>` (retención `LimitsPolicy`, `MaxAge=1h`). Adicionalmente se crea un stream configurable (`cfg.NATS.Stream`).
- **Publicaciones hacia Unity**
  - Tabla: `pokerServer.{tournamentID}.{tableID}` con payload `poker.Table` (campos: turnos, blind, cartas comunitarias, `Players[]`, `SidePots`, `CurrentStage`, etc.).
  - Jugador: `pokerServer.{tournamentID}.{tableID}.{wallet}` con payload `poker.Player` (fichas, cartas, flags AFK/allIn, `AvailableActions`, `CallAmount`, `HandStrength`, etc.).
- **Acciones desde Unity**
  - Subject `pokerClient.{tournamentID}.{tableID}.{wallet}`. El JSON esperado es el mismo esquema de `poker.Player`, pero solo se leen `LastAction`, `LastBet`, `CallAmount`, `IsAFK`. La respuesta debe ACK explicito (`nats.AckExplicit`).
  - Consumidor durable generado por el servidor: `durable-consumer4-{table}-{player}` se recrea en cada turno para garantizar backlog individual.
  - Campos relevantes del payload (ejemplo `{ "LastAction": "raise", "LastBet": 200, "CallAmount": 80, "IsAFK": false }`): `LastAction` acepta `raise|call|check|fold|allin`, `LastBet` se resta de `Chips`, `CallAmount` valida que el jugador cubra la apuesta pendiente y `IsAFK` adelanta el timeout. Cualquier otro campo es ignorado por seguridad.
  - El servidor espera ACK + publicación dentro de `TurnTime` segundos (2–10s según torneo); si no se recibe mensaje, `HandleTurn` marca `fold` (o `check` si `CallAmount=0`) y etiqueta al jugador como AFK.
- **Buenas prácticas NATS**
  - Mantener subjects estables al agregar nuevas mesas/tournaments.
  - Evitar payloads > 1MB (Temporal data converter limita a 64MB; NATS ideal < 1MB).
  - Unity debe usar `JetStream PullSubscribe` con `MaxAckPending=1` para no acumular acciones.

## 5. Flujo de Unity (cliente) resumido
1. Suscribirse a `pokerServer.*` (player + table) para renderizar estado actual.
2. Cuando sea su turno, mandar acción en `pokerClient.*` y esperar confirmación (ACK + nuevo estado).
3. Mantener heartbeat vía WebSocket si se usa WebGL; para desktop usar TCP directo.
4. Procesar cambios de mesa (`SwitchingTable`, `LastTable`, `TableEnds`) y actualizar lobby.

## 6. API HTTP expuesta
- **Admin**: `POST /tournaments` (requiere header `Authorization: Bearer <STATIC_TOKEN>`).
- **Público**: `GET /generate-token` emite JWT efímero.
- **Protegido** (JWT + rate limiting + validación de wallet): endpoints `GET /tournaments`, `/tournament/{wallet}`, `/tournaments/ongoing/{wallet}`, `/tournaments/registered/{wallet}`, `/tournaments/eliminated/{wallet}`, `/rankings/{tournamentID}`, `/ranking/{tournamentID}/{walletID}`, `/prizes/{tournamentID}`, `/table/{tournamentID}`, `/tablePlayer/{tournamentID}/{walletID}`, `/tournamentRegistration/{tournamentID}/{walletID}`, `/tournamentAvailableToStart/{tournamentID}/{walletID}`, `POST /tournament/register`, `POST /users`.

## 7. Temporal / workers
- Worker lanzado desde `temporal.StartWorker(cfg, dataConverter)` (ver `internal/workflow/worker.go`).
- DataConverter gzip (10KB chunk) para payloads de Workflow → Activity y JetStream context almacenado vía `config.WithJetStream`.
- `TournamentWorkflow` ejecuta múltiples `PlayerWorkflow` en paralelo; `selector.Select` re-lanza mesas activas y verifica condición de fin con `poker.CheckLastTableInTables`.

## 8. Infraestructura y configuración
- Docker compose trae Postgres 14, NATS (con JetStream + WebSocket 9222), Temporal server/UI. Ajustar `.env` para `DB_*`, `NATS_*`, `TEMPORAL_HOSTPORT`, llaves `SepoliaPrivateKey` y `BNBPrivateKey`.
- `config.LoadConfig()` mezcla `.env` + variables; `NATS_SUBJECTS` debe incluir `pokerServer.>,pokerClient.>` si se alimenta desde entorno.
- `nats-server.conf` ya habilita WebSocket sin TLS; planificar TLS para producción.

## 9. Lineamientos para Unity + NATS C#
- Usar `NATS.Client` (>= 1.0) o `NATS.Client.Core` para .NET Standard 2.0 (compatible con IL2CPP). JetStream se accede vía `IJetStream`/`IJetStreamPushSyncSubscribe`.
- Para WebGL, prox year: usar `nats.ws` + JS interop o Gateway HTTP.
- Config sugerida Unity (desktop):
  - Pool único de conexión y `Task.Run` para callbacks → sincronizar con hilo principal mediante colas thread-safe.
  - Crear consumer por jugador con `AckPolicy=Explicit`, `DeliverPolicy=LastPerSubject` para reanudar tras reconexión.
  - Limitar `MaxAckPending=1` para acciones, `MaxAckPending=50` para estados.
- JetStream cumple con requisitos de 3 torneos × 12 mesas × 90 jugadores (~270 players). Con latencia <10ms en LAN y retención 1h, es adecuado para reintentos y reconexiones. Ver análisis detallado en `doc/nats-unity-analysis.md`.

## 10. Principios de ingeniería / estilo
- **SOLID**: separar responsabilidades (p.ej. extraer validaciones blockchain de controllers, aislar lógica NATS en adapters). Favor composición sobre herencia.
- **Context + errores envolventes**: cada nueva goroutine debe aceptar `context.Context`; envolver errores con `%w`.
- **Inmutabilidad del estado de mesa**: no compartir punteros de `Player` entre mesas sin copia profunda; preferir slices nuevas al mutar.
- **Testing**: usar `internal/.../*_test.go` + fakes de NATS (ver `internal/nats/nats_test.go`) para nuevos módulos. Priorizar pruebas de ranking, payouts, balanceo de mesas.
- **Seguridad**: nunca exponer llaves privadas ni tokens en logs; usar `config.Server` para headers esperados.
- **Migrations**: `db.Migrate()` ejecuta `AutoMigrate` en arranque; coordinar cambios de esquema con downtime mínimo.
- **Mensajería**: toda publicación JetStream debe revisar `Ack` y manejar `nats.ErrTimeout`. Evitar spinner loops.
- **Observabilidad**: agregar logs estructurados (`log.Printf` → migrar a `zap`) y métricas (Temporal + NATS) cuando añadas features.

## 11. Pendientes / notas relevantes
- `createTournaments()` se ejecuta cada hora y pega al endpoint HTTP interno; mover a servicio scheduler o cron propio.
- Consumidores JetStream se recrean por turno; evaluar `PullSubscribe` para reducir churn.
- Falta validación fuerte en `/users` y `/wallet/*` (no rate limit por IP).
- No hay TLS para NATS/WebSocket; planificar `nats-server` con certificados reales antes de producción.
- Ver `doc/nats-unity-analysis.md` para cualquier cambio relacionado con Unity o JetStream.

## 12. Mandato de pruebas integrales
- Las nuevas features deben venir acompañadas de pruebas automatizadas en tres niveles: **torneo**, **mesa** y **jugador**. Cada nivel debe cubrir todas las reglas de poker vigentes: estados de mesa, flujo de apuestas, side pots, reshuffle, eliminaciones, premios y ranking.
- Además de la lógica de poker, se requieren pruebas que cubran **crypto/on-chain** (validación de transacciones y claves), **endpoints HTTP** (incluyendo rutas protegidas y validaciones de wallet), y **mensajería NATS/Unity** (publicaciones `pokerServer.*`, acciones `pokerClient.*`, reconexiones y ACKs JetStream).
- Las pruebas deben usar los módulos existentes (`internal/poker`, `internal/server`, `internal/nats`, `internal/workflow`, `internal/db`) y apoyarse en fakes/mocks para servicios externos cuando sea necesario.
- Documentar en cada PR qué capas fueron cubiertas y justificar cualquier área que quede pendiente para asegurar trazabilidad del plan de testeo continuo.

## 13. Simuladores y QA sobre el flujo original
- Cualquier herramienta de QA (ej. `cmd/unitytournament`) debe **reutilizar el mismo flujo de Temporal** que sirve producción (`TournamentWorkflow` + `HandleTableActivitie`). Evitar duplicar lógica de reparto o turnos fuera de `internal/workflow` y `internal/poker`.
- El simulador ahora arranca con `go run ./cmd/unitytournament -temporal=true` (valor por defecto) para sembrar un torneo demo, iniciar el worker real (`temporal.StartWorker`) con el data converter gzip y ejecutar `TournamentWorkflow` contra esas mesas. Usa `-temporal=false` solo para depuración local del antiguo bucle.
- Mientras el workflow corre, el simulador crea bots que publican acciones en `pokerClient.*` y un watcher que se suscribe a `pokerServer.<tournament>.>` para registrar cada transición `[STAGE] Mesa X -> flop/turn/river` y el conteo `[STATE]` de jugadores/mesas vivos. Estos logs son la referencia primaria para detectar stuck preflop.
- Si Temporal UI aparece vacía: valida que `temporal.StartWorker` se ejecute (flag `-temporal`), que Temporal apunte a `cfg.Temporal.HostPort`, y que el workflow esté usando un `workflowID` único (`sim-tournament-<id>-<ts>`). Repite `tctl workflow list` con ese ID antes de revisar la UI.
- Cuando se necesite simular bots o rellenar torneos, hacerlo sembrando datos reales en Postgres y dejando que las actividades (`CreateTablesInTournament`, `HandleTurn`, `poker.Reshuffle`) avancen el estado. Esto garantiza que Unity/QA reflejen cambios futuros sin mantenimiento doble.
- Las pruebas que cubran este flujo deben invocar los workflows originales (ver `internal/workflow/workflow_test.go`); si agregas nuevos escenarios, extiende esas suites en lugar de crear imitaciones parciales.
- Cualquier cambio en mensajes `pokerServer.*` / `pokerClient.*` debe verificarse tanto contra el servidor principal (`cmd/pokerServer`) como contra el simulador de Unity para asegurar paridad de subjects y timeouts.
- Para diagnosticar hands que no avanzan: habilita `-temporal=true` + `-mode sim`, revisa el stream `[STAGE]` para confirmar flop/turn/river; si faltan, instrumenta `HandleTableActivitie` y el watcher para verificar que `table.LastAction` se actualice tras cada broadcast. Nunca fuerces etapas manualmente desde el simulador, deja que el workflow las produzca.
