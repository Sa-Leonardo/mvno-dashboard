# MVNO Dashboard

Painel analitico em Go + Gin para consultar dados MVNO na API Easy2Use/Tip Brasil e exibir relatorios de linhas, contratos, estoque, planos, status e estimativas financeiras.

O backend nao usa banco de dados. Ele funciona apenas como proxy seguro para a API externa, protegendo o token e mantendo um cache em memoria enquanto o processo esta aberto.

## Requisitos

- Go 1.22 ou superior
- Token da API Easy2Use/Tip Brasil

## Configuracao

Copie `.env.example` para `.env` e preencha:

```text
APP_ADDR=:8080
GIN_MODE=release
ADMIN_KEY=troque_esta_chave
EASY2USE_BASE_URL=https://mvno.tipbrasil.com.br/api/public
EASY2USE_USER_TOKEN=token_aqui
ALLOWED_CNPJS=58420964000179,15070244000118,58420964000330,58420964000500
RECHARGE_INTERVAL_DAYS=90
RECHARGE_SAFETY_WINDOW_DAYS=0
DEFAULT_RECHARGE_QUANTITY=1
PROVIDER_REQUEST_DELAY_MS=1200
```

Nunca suba o arquivo `.env` para o GitHub.

## Rodar

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

Use no campo "Chave interna" o mesmo valor configurado em `ADMIN_KEY`.

## Deploy

O projeto esta pronto para deploy em plataformas que suportam Go. A plataforma precisa receber estas variaveis de ambiente:

```text
ADMIN_KEY=sua_chave_interna_forte
EASY2USE_BASE_URL=https://mvno.tipbrasil.com.br/api/public
EASY2USE_USER_TOKEN=token_real
ALLOWED_CNPJS=58420964000179,15070244000118,58420964000330,58420964000500
RECHARGE_INTERVAL_DAYS=90
RECHARGE_SAFETY_WINDOW_DAYS=0
DEFAULT_RECHARGE_QUANTITY=1
PROVIDER_REQUEST_DELAY_MS=1200
GIN_MODE=release
```

Nao configure essas variaveis em arquivo `.env` no servidor de deploy. Use a area de secrets/environment variables da plataforma.

### Vercel

Funciona na Vercel usando o preset/runtime Go. O projeto inclui `vercel.json` com:

```json
{
  "version": 2,
  "framework": "go"
}
```

Passos:

1. Suba o repositorio no GitHub.
2. Importe o projeto na Vercel.
3. Em Environment Variables, configure as variaveis listadas acima.
4. Deploy.
5. Acesse a URL gerada e informe o valor de `ADMIN_KEY` no campo "Chave interna".

Observacoes importantes:

- A Vercel pode reiniciar ou escalar a funcao a qualquer momento. Como este projeto nao usa banco, o cache em memoria pode zerar e os dados serao buscados novamente da API.
- `POST /sync/ultima-recarga` consulta um ICCID por vez e pode demorar. Se a base for grande, pode bater no limite de duracao da funcao da Vercel. Nesse caso, prefira sincronizar apenas assinantes/estoque ou mover essa rotina para uma plataforma com processo persistente.
- O frontend esta embutido no binario Go, entao nao depende de arquivos estaticos soltos no ambiente da Vercel.

### Render, Railway ou similar

1. Conecte o repositorio GitHub.
2. Escolha deploy por Dockerfile, se disponivel.
3. Configure as variaveis de ambiente acima.
4. Use `/health` como health check, se a plataforma pedir.
5. Depois de publicar, acesse a URL gerada e informe o valor de `ADMIN_KEY` no campo "Chave interna".

### VPS com Docker

Build:

```bash
docker build -t mvno-dashboard .
```

Run:

```bash
docker run --rm -p 8080:8080 \
  -e ADMIN_KEY="sua_chave_interna_forte" \
  -e EASY2USE_BASE_URL="https://mvno.tipbrasil.com.br/api/public" \
  -e EASY2USE_USER_TOKEN="token_real" \
  -e ALLOWED_CNPJS="58420964000179,15070244000118,58420964000330,58420964000500" \
  -e RECHARGE_INTERVAL_DAYS="90" \
  -e RECHARGE_SAFETY_WINDOW_DAYS="0" \
  mvno-dashboard
```

Acesse `http://localhost:8080`.

## Como os dados funcionam

- `GET /iccids` carrega dados da API externa quando o cache em memoria esta vazio.
- `POST /sync/assinantes` busca assinantes/contratos e atualiza o cache em memoria.
- `POST /sync/estoque` busca estoque de SIM Cards e cruza com os contratos em memoria.
- `POST /sync/ultima-recarga` consulta a ultima recarga dos ICCIDs carregados e calcula a proxima data estimada usando `RECHARGE_INTERVAL_DAYS`.
- Ao reiniciar o servidor, o cache em memoria e zerado.

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

Retorna o modo atual:

```json
{
  "status": "ok",
  "mode": "report-only",
  "storage": "memory"
}
```

### Listar ICCIDs

```text
GET /iccids
```

### Resumo por CNPJ e status

```text
GET /iccids/summary
```

### Sincronizar assinantes

```text
POST /sync/assinantes
```

### Sincronizar estoque

```text
POST /sync/estoque
```

### Sincronizar ultima recarga

```text
POST /sync/ultima-recarga
```

Essa rota pode levar alguns minutos porque consulta a API externa para cada ICCID carregado.

### Proxima data estimada

```text
GET /automation/next-run
```

Mantido por compatibilidade com a interface. Ele nao executa recargas; apenas calcula datas a partir do cache em memoria.

## Modo relatorio

As rotas antigas de recarga, aprovacoes e operacoes foram desativadas. O sistema nao executa `saldo/adicionar`, nao cria pendencias e nao grava historico operacional.

O objetivo atual e responder perguntas como:

- Quantas linhas existem na base?
- Quantas estao ativas, bloqueadas, canceladas ou em estoque?
- Quantas sao eSIM ou fisicas?
- Qual a distribuicao por CNPJ, plano e operadora?
- Qual a receita mensal estimada por plano?
- Quais linhas nao possuem ultima recarga informada?

