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
PROVIDER_REQUEST_DELAY_MS=1200
ENABLE_REAL_RECHARGE=false
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

Tambem e aceito:

```text
x-api-key: sua_chave
```

Use o mesmo valor configurado em `ADMIN_KEY` no `.env`.

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

Por causa do rate limit da API externa, essa rotina espera `PROVIDER_REQUEST_DELAY_MS` entre consultas. O padrao recomendado e `1200`, que fica abaixo de 60 chamadas por minuto.

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
  "quantity": 1,
  "dry_run": true
}
```

Para chamar a API real de recarga, configure no `.env`:

```text
ENABLE_REAL_RECHARGE=true
```

Depois reinicie o servidor e envie:

```json
{
  "quantity": 1,
  "dry_run": false
}
```

Atencao: a API externa pode aplicar uma franquia diferente da quantidade enviada, conforme regra da operadora/plano. Em teste real, foi observado que `quantity: 1` pode resultar em credito maior no provedor.

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

Esse endpoint considera apenas ICCIDs acionaveis:

```text
auto_recharge_enabled = true
contract_status = EM USO
next_recharge_due_at >= hoje
```

Resposta inclui `next_recharge_iccids`, com os ICCIDs e CNPJs que pertencem a proxima data de recarga.

## Respostas da automacao

`POST /automation/check-recharges` retorna, em `results`, o ICCID, CNPJ, nome do assinante e dados da operacao para cada item avaliado ou recarregado.

## Regra preventiva

```text
next_recharge_due_at = ultima_recarga + 11 meses - 10 dias
```

Quando `hoje >= next_recharge_due_at`, a rotina automatica adiciona 1 GB.
