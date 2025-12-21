# NATS + JetStream en Unity (C#) — Análisis técnico

## 1. Requerimientos funcionales y de rendimiento
- Escenario actual: **3 torneos simultáneos × 12 mesas × ~90 jugadores** (≈270 jugadores activos). Cada jugador genera
  - 1 mensaje de acción por turno (2–4 s promedio)
  - 1 mensaje de estado recibido por cada cambio de mesa/etapa (~5–10 msgs/min)
- Necesitamos latencia <150 ms extremo a extremo, reintentos ante desconexiones y capacidad de persistir turnos durante 1 h.
- Unity debe correr en desktop (Windows/macOS) y eventualmente WebGL.

## 2. SDKs disponibles para C# / Unity
| SDK | Target frameworks | JetStream | Notas |
| --- | --- | --- | --- |
| `NATS.Client` (clásico) | .NET Standard 2.0 | ✅ (`IJetStream`, `IJetStreamManagement`) | Soporta IL2CPP; requiere hilo dedicado o `Task.Run` en Unity. |
| `NATS.Client.Core` (2024) | .NET 6 / Native AOT | ✅ | Mejor async/await, pero Unity 2022 LTS solo soporta hasta .NET Standard 2.1. |
| `nats.ws` | JS/WebSocket | ❌ JetStream directo, pero puede usarse con `nats.ws` gateway → JetStream server-side. |

**Conclusión**: para desktop/mobile usar `NATS.Client` estable; para WebGL exponer un gateway HTTP/WebSocket server-side que traduzca a JetStream.

## 3. Compatibilidad Unity
- **Mono/IL2CPP**: `NATS.Client` funciona siempre que se mantenga en threads en background (no en `Update`).
- **WebGL**: no hay sockets nativos, solo WebSockets seguros → usar `nats.ws` gateway expuesto (NATS ya corre con `websocket { listen: 9222 }`). Para JetStream se necesita un "bridge" server (Go/Node) que gestione los acks.
- Recomendación: crear un singleton `NatsClientBehaviour` que inicialice la conexión en `Awake`, mantenga colas thread-safe → `Update` drena eventos.

## 4. JetStream para el caso de uso
- Persistencia y replay: `MaxAge=1h` ya definido es suficiente para reconectar Unity y reprocesar turnos.
- Para acciones (Unity→server) usar **stream dedicado** o `pokerClient.*` dentro del mismo stream pero con `MaxAckPending=1` y `DeliverPolicy=New` para evitar repeticiones.
- Para estados (server→Unity) crear **durables por jugador** con `DeliverPolicy=LastPerSubject`; Unity hace `Fetch(1)` y ACK tras renderizar.
- Rendimiento: JetStream en modo in-memory + file backstore soporta >50k msgs/s en hardware modesto. Este caso (<5k msgs/min) es trivial.

### Ejemplo de Unity (desktop) para acciones
```csharp
using NATS.Client;
using NATS.Client.JetStream;

var opts = ConnectionFactory.GetDefaultOptions();
opts.Url = $"nats://{user}:{pass}@{host}:4222";
options.MaxReconnect = Options.ReconnectForever;
ICJetStream cjs;
IConnection conn = new ConnectionFactory().CreateConnection(opts);
IJetStream js = conn.CreateJetStreamContext();

// Suscripción a tabla/estado
var tableSubject = $"pokerServer.{tournamentId}.{tableId}";
var stateConsumer = $"unity-{playerWallet}-state";
var subOpts = PullSubscribeOptions.Builder()
    .WithDurable(stateConsumer)
    .WithStream("POKER_TOURNAMENT")
    .Build();
var pullSub = js.PullSubscribe(tableSubject, subOpts);

// Publicar acción
var action = new {
    LastAction = "raise",
    LastBet = 200,
    CallAmount = 0,
    IsAFK = false
};
var payload = Encoding.UTF8.GetBytes(JsonConvert.SerializeObject(action));
js.Publish($"pokerClient.{tournamentId}.{tableId}.{wallet}", payload);
```
> En Unity, envolver publicaciones en `Task.Run` y usar `SynchronizationContext` para retornar datos al hilo principal.

## 5. ¿Por qué JetStream cumple los requisitos?
- **Orden y persistencia**: asegura que Unity reciba cada cambio aun con reconexiones (crítico cuando un jugador cambia de mesa o se resuelve la mano mientras se desconecta).
- **Durable consumers por jugador** evitan ruido entre mesas y simplifican auditoría.
- **Backpressure**: `AckExplicit` y `MaxDeliver` protegen al servidor de Unitys colgados. Ajustar `MaxDeliver=3` para repetir acciones fallidas.
- **JetStream clustering** (opcional) permite escalar horizontalmente si se agregan más torneos.

## 6. Alternativas evaluadas
| Alternativa | Pros | Contras |
| --- | --- | --- |
| Photon Realtime / Quantum | Integración Unity nativa, matchmaking | Coste licencias, no encaja con backend actual (Temporal + Postgres), reescritura de lógica server-side. |
| Mirror + WebSocket custom | Control total, gRPC websockets | Requiere replicar persistencia/ack, mayor esfuerzo operativo. |
| Redis Streams | Simple, buen throughput | Falta topología publish-subscribe con wildcard, sin soporte directo WebSocket.

Dado que JetStream ya se usa en el servidor y satisface QoS requerido, **mantener NATS** es la opción óptima.

## 7. Plan de implementación recomendado
1. **SDK**: empaquetar `NATS.Client` en un asmdef y exponer `INatsService` inyectable.
2. **Conexión**: iniciar al cargar la escena (Awake) y reconectar automáticamente (`Options.MaxReconnect`, `ReconnectWait`).
3. **Suscripciones**:
   - Estados de mesa: `PullSubscribe` + coroutine que ejecuta `Fetch(1, timeout)`; despachar al hilo principal.
   - Estados de jugador: misma estrategia pero con subject específico.
4. **Acciones**: agrupar en `SendActionAsync(ActionPayload payload)` con validación local antes de publicar; encolar si no hay conexión y reintentar.
5. **WebGL bridge**: exponer microservicio Go que acepte WebSocket y reenvíe a JetStream para futuras builds web.
6. **Monitoring**: habilitar `nats-server` Prometheus + dashboards para detectar timeouts o backlog.

## 8. Riesgos y mitigaciones
| Riesgo | Mitigación |
| --- | --- |
| Unity bloquea hilo principal si JetStream opera en sync | Ejecutar en `Task.Run` y usar colas thread-safe para callbacks. |
| Reconexiones WebSocket no conservan durables | Registrar `DeliverPolicy=LastPerSubject` para rehidratar estado al reconectar. |
| WebGL sin JetStream | Bridge HTTP/WS + validar que no se pierdan ACKs; server hará publish en nombre del cliente. |
| Acciones duplicadas (doble tap) | Incluir `ActionNonce` en payload y deduplicar en servidor (TODO futuro). |

**Conclusión**: JetStream + `NATS.Client` es viable y recomendado para Unity C#; solo se requiere un wrapper limpio y, para WebGL, un pequeño gateway. Todos los puntos clave ya están alineados con la infraestructura actual. Documentación complementaria incluida en `.github/copilot-instructions.md`.
