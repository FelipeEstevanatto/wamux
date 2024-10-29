# Evolution GO
## Pré-requisitos
- Go 1.22 ou superior
- PostgreSQL
- Arquivo .env configurado (exemplo fornecido abaixo)

## Configuração
1. Clone o repositório
2. Copie o arquivo .env.example para .env e configure as variáveis de ambiente necessárias
3. Certifique-se que o PostgreSQL está rodando e as databases foram criadas:
   - evogo_auth
   - evogo_users

## Variáveis de Ambiente
As seguintes variáveis devem ser configuradas no arquivo .env:
```
SERVER_PORT=4000
POSTGRES_AUTH_DB=postgresql://postgres:root@localhost:5432/evogo_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://postgres:root@localhost:5432/evogo_users?sslmode=disable
DATABASE_SAVE_MESSAGES=false
CLIENT_NAME=evolution
GLOBAL_API_KEY=REMOVED-SECRET
WADEBUG=DEBUG
LOGTYPE=console
```

## Como Executar
Para rodar a aplicação em modo desenvolvimento:
```bash
go run ./cmd/evolution-go/main.go -dev
```
Recursos Opcionais
A aplicação suporta as seguintes configurações opcionais que podem ser descomentas no arquivo .env:
- Conversor de áudio (API_AUDIO_CONVERTER)
- Mensageria AMQP (AMQP_URL)
- Webhook para notificações (WEBHOOK_URL)
Debug
Para habilitar logs de debug, configure as seguintes variáveis no .env:
```
WADEBUG=DEBUG
LOGTYPE=console
```