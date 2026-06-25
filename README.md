# MVNO Dashboard

Sistema independente em Go + Gin + SQLite para gerenciar ICCIDs/SIM Cards, consultar dados da API Tip Brasil/Easy2Use, visualizar dashboard operacional e executar recargas controladas.

## Requisitos

- Go 1.22 ou superior
- Acesso a internet para baixar dependencias na primeira execucao
- Token da API Easy2Use/Tip Brasil

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
ENABLE_DEV_ROUTES=false
```

Nunca suba o arquivo `.env` para o GitHub.

## Rodar em desenvolvimento

Na raiz do projeto:

```bash
go run main.go
```

Ou:

```bash
go run ./cmd/api
```

Acesse:

```text
http://localhost:8080
```

Relatorios:

```text
http://localhost:8080/relatorios
```

Use no campo "Chave interna" o mesmo valor configurado em `ADMIN_KEY`.

## Gerar binario

Windows:

```bash
go build -o dist/mvno-dashboard.exe ./cmd/api
```

Linux/macOS:

```bash
go build -o dist/mvno-dashboard ./cmd/api
```

## Rodar em outro PC

1. Instale Go 1.22+.
2. Clone o repositorio novo.
3. Copie `.env.example` para `.env`.
4. Preencha `ADMIN_KEY`, `EASY2USE_USER_TOKEN` e `ALLOWED_CNPJS`.
5. Rode:

```bash
go mod download
go run main.go
```

O banco SQLite sera criado automaticamente em `./data/app.db`.

## Subir para um novo GitHub

Este projeto foi separado do repositorio antigo. Para conectar a um novo repositorio:

```bash
git remote add origin https://github.com/SEU_USUARIO/SEU_NOVO_REPOSITORIO.git
git add .
git commit -m "Inicializa MVNO Dashboard"
git push -u origin main
```

## Rotas principais

Todas as rotas abaixo, exceto `/health`, exigem o header:

```text
x-api-key: sua_ADMIN_KEY
```

Tambem e aceito:

```text
X-Admin-Key: sua_ADMIN_KEY
```

### Saude

```text
GET /health
```

### Sincronizar assinantes

Busca assinantes/contratos na API externa e salva ICCIDs localmente.

```text
POST /sync/assinantes
```

### Sincronizar estoque

Busca SIM Cards, status, operadora e eSIM na API externa.

```text
POST /sync/estoque
```

### Sincronizar ultima recarga

Consulta a ultima recarga de cada ICCID salvo e calcula a proxima janela de recarga.

```text
POST /sync/ultima-recarga
```

### Listar ICCIDs

```text
GET /iccids
```

### Recarga manual

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

Para permitir recarga real:

```text
ENABLE_REAL_RECHARGE=true
```

## Automacao

Simular rotina:

```text
POST /automation/check-recharges
```

Body:

```json
{
  "dry_run": true
}
```

Criar pendencias para aprovacao manual:

```json
{
  "create_approvals": true
}
```

## Regra preventiva

```text
next_recharge_due_at = ultima_recarga + 11 meses - 10 dias
```

Quando `hoje >= next_recharge_due_at`, a rotina pode adicionar saldo conforme configuracao e aprovacao.
