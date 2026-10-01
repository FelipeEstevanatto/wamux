# Configuração

O WaMux é configurado por variáveis de ambiente (arquivo `.env` ou ambiente do
container).

A **lista completa e canônica** das variáveis está em
[`../referencia/environment-variables.md`](../referencia/environment-variables.md),
e o template comentado em
[`../../../docker/examples/.env.example`](../../../docker/examples/.env.example).

## Essenciais

| Variável | Descrição | Obrigatória |
|---|---|---|
| `GLOBAL_API_KEY` | Chave administrativa (`/instance/all`, `/server/stats`, Manager) | **sim** |
| `POSTGRES_AUTH_DB` / `POSTGRES_USERS_DB` | Connection strings dos bancos (criados no primeiro boot se ausentes) | sim |
| `SERVER_PORT` | Porta HTTP dentro do container | não (`8080`) |
| `CLIENT_NAME` / `OS_NAME` | Identificador e nome exibido do dispositivo no WhatsApp | não |
| `DATABASE_SAVE_MESSAGES` | Persistir mensagens (habilita histórico e contadores) | não |

## Onde configurar

- **Docker Compose** — edite o `.env` ao lado do `docker-compose.yml`.
- **Docker run** — use `-e VAR=valor` (ou `--env-file .env`).
- **Local** — crie um `.env` na raiz e rode `make dev`.

> O `/swagger` **não** é protegido por `GLOBAL_API_KEY`. Em um host público,
> defina `SWAGGER_ENABLED=false`.

Veja também: [Instalação](./installation.md) ·
[Variáveis de Ambiente](../referencia/environment-variables.md) ·
[Deploy com Docker](../deploy-producao/docker-deployment.md).
