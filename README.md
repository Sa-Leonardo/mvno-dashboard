# Automacao chip-MOV

MVP em Go + Gin para adicionar 1 GB preventivo em ICCIDs vinculados a 2 CNPJs, usando a API Tip Brasil/Easy2Use.

## Configuracao

Copie `.env.example` para `.env` e preencha:

```text
APP_ADDR=:8080
GIN_MODE=release
ADMIN_KEY=troque_esta_chave
EASY2USE_BASE_URL=https://mvno.tipbrasil.com.br/api/public
EASY2USE_USER_TOKEN=token_aqui
ALLOWED_CNPJS=00000000000100,11111111111100
DATABASE_PATH=./data/app.db
RECHARGE_INTERVAL_MONTHS=11
RECHARGE_SAFETY_WINDOW_DAYS=10
DEFAULT_RECHARGE_QUANTITY=1
```

## Rodar

```bash
go run ./cmd/api
```

Ou gerar binario:

```bash
go build -o dist/chipmov-api.exe ./cmd/api
```

## Endpoints

Todos os endpoints abaixo, exceto `/health`, exigem:

```text
X-Admin-Key: sua_chave
```

### Saude

```text
GET /health
```

### Sincronizar assinantes

Busca assinantes na API externa, filtra somente os CNPJs permitidos e salva os ICCIDs.

```text
POST /sync/assinantes
```

### Sincronizar ultima recarga

Consulta a ultima recarga de cada ICCID salvo e calcula `next_recharge_due_at`.

```text
POST /sync/ultima-recarga
```

### Listar ICCIDs

```text
GET /iccids
```

### Adicionar saldo manual

```text
POST /iccids/{iccid}/saldo
```

Body:

```json
{
  "quantity": 1
}
```

### Rotina para n8n

Teste seguro:

```text
POST /automation/check-recharges
```

Body:

```json
{
  "dry_run": true
}
```

Execucao real:

```json
{
  "dry_run": false
}
```

### Proxima execucao util

```text
GET /automation/next-run
```

## Regra preventiva

```text
next_recharge_due_at = ultima_recarga + 11 meses - 10 dias
```

Quando `hoje >= next_recharge_due_at`, a rotina automatica adiciona 1 GB.
