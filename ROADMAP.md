# Sentinel Roadmap

Plano de evolução organizado em marcos incrementais. Cada tarefa pode ser
executada de forma independente dentro do seu marco.

---

## Marco 1 — Dívida Técnica: Backend

Objetivo: eliminar code smells, melhorar manutenibilidade, corrigir gaps de
configuração.

### 1.1 Split `store/watchtower.go` (1,161 linhas → 6 arquivos) ✅

- [x] `store/wt_types.go` — Shared struct types
- [x] `store/wt_sessions.go` — Upsert/Get/List/Purge sessions + activity/inspector patch builders
- [x] `store/wt_window.go` — Upsert/List/Purge windows
- [x] `store/wt_panes.go` — Upsert/List/Purge panes + seen/unread marking
- [x] `store/wt_presence.go` — Presence CRUD + pruning
- [x] `store/wt_journal.go` — Journal insert/list/prune + runtime key-value
- [x] `store/helpers.go` — Shared utility functions

Critério: zero mudança de interface pública, todos os testes passam sem alteração.

### 1.2 Delta-based tickers para alerts e activity

- [ ] Adicionar `global_rev` tracking nas tabelas `ops_alerts` e `ops_timeline_events`
  (trigger ou campo atualizado no upsert)
- [ ] Refatorar `startAlertsTicker` para emitir apenas quando `global_rev` mudou desde
  o último broadcast
- [ ] Refatorar `startActivityTicker` com a mesma lógica de delta
- [ ] Remover broadcast integral — emitir apenas o delta (novos/alterados)

### 1.3 Configuração: migrar para TOML real

- [ ] Adicionar dependência `github.com/BurntSushi/toml` (ou `pelletier/go-toml/v2`)
- [ ] Substituir parser hand-rolled em `config.go` por decode TOML com sections
- [ ] Adicionar section `[alerts]` com thresholds (CPU, memory, disk) no template
- [ ] Adicionar section `[watchtower]` agrupando tick_interval, capture_lines, etc.
- [ ] Documentar todos os `SENTINEL_*` env vars no template gerado
- [ ] Manter backward-compat: env vars continuam tendo precedência

### 1.4 Limitar goroutines de runbook manual

- [ ] Adicionar semáforo (channel de capacidade N) em `api.Handler` para runs manuais,
  análogo ao que o scheduler já faz
- [ ] Configurar `N` via config (default: 5)
- [ ] Retornar 429 quando o semáforo estiver cheio

### 1.5 Adicionar índice faltante em `wt_presence` ✅

- [x] Nova migração: `CREATE INDEX IF NOT EXISTS idx_wt_presence_session ON wt_presence(session_name)`

### 1.6 Guardrail scope dinâmico

- [ ] Remover hardcode de `normalizeGuardrailScope` → `"action"` always
- [ ] Suportar scope `command` para inspeção de conteúdo de `SendKeys`
- [ ] Adicionar guardrail rules default para comandos destrutivos comuns:
  `rm -rf /`, `DROP DATABASE`, `shutdown`, `reboot`

---

## Marco 2 — Dívida Técnica: Frontend

Objetivo: melhorar code organization, acessibilidade, performance e cobertura
de testes.

### 2.1 Extrair sub-componentes de `services.tsx` (1,263 linhas) ✅

- [x] `ServiceBrowseRow.tsx` — row de serviço com action buttons
- [x] `ServiceStatusDialog.tsx` — dialog de propriedades/status do serviço
- [x] `ServiceLogsSheet.tsx` — sheet de logs com streaming, search, wrap toggle

Route file reduzido para ~808 linhas.

### 2.2 Extrair sub-componentes de `runbooks.tsx` (1,142 linhas) ✅

- [x] `RunbookDetailPanel.tsx` — header card + steps list + schedule section
- [x] `RunbookJobHistory.tsx` — lista de runs com output expandível
- [x] `useRunbooksPage.ts` — hook com state, queries, mutations, callbacks

Route file reduzido para ~186 linhas (84% reduction).

### 2.3 Testes para `useTmuxEventsSocket`

- [ ] Testar conexão/reconexão com WebSocket mock
- [ ] Testar event dispatching para cada tipo de mensagem
- [ ] Testar delta sync debounce e in-flight guard
- [ ] Testar adaptive fallback polling quando WS desconecta
- [ ] Testar exponential backoff com jitter

### 2.4 ARIA compliance em dialog tabs ✅

- [x] `GuardrailsDialog` — adicionar `role="tablist"`, `role="tab"`, `aria-selected`
- [x] `SettingsDialog` — mesmo tratamento
- [x] Alinhar com o padrão já correto do `MetricsPage`

### 2.5 Performance: memoizar sorts inline ✅

- [x] `PaneStrip` — envolver sort em `useMemo`
- [x] `WindowStrip` — idem

### 2.6 Unificar combobox pattern

- [ ] Avaliar se `CreateSessionDialog` pode usar o `Combobox` de `ui/combobox.tsx`
  em vez do combobox hand-rolled
- [ ] Se sim, migrar; se não (por questão de UX/autocomplete fs), documentar a decisão

---

## Marco 3 — Activity Events Completos

Objetivo: toda ação operacional significativa deve ser rastreada na timeline.

### 3.1 Backend: expandir fontes de activity events

- [ ] Verificar que service actions (start/stop/restart) já gravam via `RecordServiceAction`
  — o assessment indica que sim; validar nos testes
- [ ] Adicionar events para: `service.registered`, `service.unregistered` (track/untrack)
- [ ] Adicionar events para: `alert.created`, `alert.acked`, `alert.resolved`, `alert.deleted`
- [ ] Adicionar events para: `guardrail.blocked`, `guardrail.overridden`
- [ ] Adicionar events para: `schedule.created`, `schedule.triggered`, `schedule.disabled`
- [ ] Adicionar events para: `config.updated` (timezone, locale changes)

### 3.2 Frontend: enricher a activity page

- [ ] Adicionar ícones por source/eventType (runbook, service, alert, guardrail, schedule)
- [ ] Adicionar filtro por source na toolbar
- [ ] Considerar virtualização se a lista crescer além de 200 items

---

## Marco 4 — Alert Webhooks

Objetivo: notificação externa quando alertas são criados ou resolvidos.

### 4.1 Backend

- [ ] Adicionar campo `webhook_url` e `webhook_events` no config TOML
  (events: `alert.created`, `alert.resolved`, `alert.acked`)
- [ ] Criar `internal/notify/webhook.go` com POST assíncrono:
  - Payload JSON: `{event, alert, host, timestamp}`
  - Retry com backoff (3 tentativas, como o runbook webhook)
  - Timeout de 10s
- [ ] Integrar no `health.go`: após `UpsertAlert` e `ResolveAlert`, chamar webhook
- [ ] Integrar no `handler_ops.go`: após ack, chamar webhook
- [ ] Adicionar testes para o webhook delivery

### 4.2 Frontend

- [ ] Adicionar section "Notifications" no `SettingsDialog`
- [ ] Campo para webhook URL + checkboxes para eventos
- [ ] Botão "Test webhook" que envia um payload de teste

---

## Marco 5 — Runbook Parameters

Objetivo: transformar runbooks estáticos em ferramentas parametrizáveis.

### 5.1 Modelo de dados

- [ ] Adicionar campo `parameters` (JSON) na tabela `ops_runbooks`:
  ```json
  [{"name": "ENV", "label": "Environment", "type": "string", "default": "staging", "required": true}]
  ```
- [ ] Migração SQL para o novo campo
- [ ] Atualizar tipos Go: `OpsRunbook.Parameters []RunbookParameter`
- [ ] Atualizar tipos TS: `OpsRunbook.parameters`

### 5.2 Backend: substituição de variáveis

- [ ] No `runner.go`, antes de executar cada step, substituir `{{PARAM_NAME}}`
  no command string pelo valor fornecido
- [ ] Validar que todos os parâmetros `required` foram fornecidos antes de iniciar
- [ ] Sanitizar valores: escapar para shell (`shellescape` ou single-quote wrapping)
- [ ] Armazenar parâmetros usados no metadata do run (para auditoria)

### 5.3 Frontend: UI de parâmetros

- [ ] No editor de runbook: seção "Parameters" com add/remove/edit de parâmetros
- [ ] No dialog de "Run": form dinâmico com os parâmetros do runbook,
  defaults pré-preenchidos, validação de required
- [ ] No job history: mostrar parâmetros usados em cada run

---

## Marco 6 — Terminal Insights

Objetivo: extrair valor inteligente do output dos terminais.

### 6.1 Marker detection configurável

- [ ] Mover keywords de detecção de markers de hardcoded para config/store
- [ ] API para CRUD de marker patterns (regex + severity + label)
- [ ] Seeds padrão: panic, fatal, error, OOM, segfault, connection refused
- [ ] UI no SettingsDialog para gerenciar patterns

### 6.2 Linkagem runbook ↔ marker

- [ ] Quando um marker de erro é detectado, buscar runbooks que mencionem o serviço/pattern
  no title ou description
- [ ] Mostrar "Suggested runbooks" no timeline event detail
- [ ] Quick-action: "Run this runbook" direto do timeline

### 6.3 Scheduled health reports

- [ ] Criar runbook-like report template: métricas, alertas, timeline summary
- [ ] Gerar relatório JSON/HTML com snapshot do estado do host
- [ ] Entregar via webhook (mesmo mecanismo do Marco 4)
- [ ] Frequência configurável: daily, weekly

---

## Ordem de execução sugerida

```
Marco 1 (Backend debt)  ──┐
                          ├──► Marco 3 (Activity events)
Marco 2 (Frontend debt) ──┘        │
                                    ▼
                            Marco 4 (Alert webhooks)
                                    │
                                    ▼
                            Marco 5 (Runbook params)
                                    │
                                    ▼
                            Marco 6 (Terminal insights)
```

Marcos 1 e 2 podem ser atacados em paralelo. Marco 3 depende da base limpa.
Marcos 4–6 são sequenciais pelo valor incremental que cada um entrega.
