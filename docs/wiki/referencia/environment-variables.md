# Variáveis de Ambiente

Referência rápida de variáveis de ambiente do WaMux.

Para documentação detalhada, consulte: [Configuração](../fundamentos/configuration.md)

---

## Obrigatórias

Só a conexão com o Postgres (`POSTGRES_AUTH_DB` e `POSTGRES_USERS_DB`), e o `docker-compose.yml` já define as duas.

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `GLOBAL_API_KEY` | Chave de administrador | Gerada no primeiro boot em `global-api-key`, na pasta acima de `LOG_DIRECTORY` |
| `DATABASE_SAVE_MESSAGES` | Salvar mensagens no banco | `true` |
| `DATABASE_SAVE_HISTORY_SYNC` | Salvar o histórico enviado pelo celular, até `MESSAGE_RETENTION_DAYS` | `true` |
| `TRUSTED_PROXIES` | Quem pode informar o IP do cliente: IPs/CIDRs ou `none` | Endereços privados, então um proxy no mesmo host, rede Docker ou LAN funciona sem configurar |

---

## Servidor

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `SERVER_PORT` | `8081` | Porta HTTP |
| `CLIENT_NAME` | `wamux` | Nome deste servidor. As instâncias pertencem ao nome com que foram criadas: ao trocá-lo, elas somem do manager e não reconectam até voltar ao nome antigo, sem perder dados. Servidores que compartilham um banco precisam de nomes diferentes. Também é o `NODE_ID` padrão |
| `OS_NAME` | `Linux` | Sistema operacional |
| `HTTP_COMPRESSION` | `true` | Compressão gzip das respostas (JSON e o painel). Defina `false` quando um proxy à frente (nginx/Traefik) já comprime, para não pagar o custo de CPU duas vezes. |

---

## Banco de Dados

| Variável | Descrição |
|----------|-----------|
| `POSTGRES_AUTH_DB` | Connection string banco de autenticação |
| `POSTGRES_USERS_DB` | Connection string banco de usuários |

**Formato:**
```env
POSTGRES_AUTH_DB=postgresql://user:pass@host:5432/wamux_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://user:pass@host:5432/wamux_users?sslmode=disable
```

---

## Retenção

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `MESSAGE_RETENTION_DAYS` | `365` | Dias de mensagens persistidas; `0` mantém para sempre |
| `MEDIA_RETENTION_DAYS` | = mensagens | Dias de mídia local; `0` mantém para sempre. Com retenção de mensagens, objetos do MinIO/S3 global são removidos quando nenhuma mensagem ainda referencia a URL |
| `SEND_OPERATION_RETENTION_DAYS` | `30` | Dias de operações idempotentes (`send_operations`) |
| `CHAT_READ_RETENTION_DAYS` | `0` | Dias de posições de leitura (`chat_reads`) |
| `CHAT_STATE_RETENTION_DAYS` | `0` | Dias de estado de chat (`chat_states`) |
| `TYPEBOT_SESSION_RETENTION_DAYS` | `0` | Dias de sessões do Typebot |
| `POLL_VOTE_RETENTION_DAYS` | `0` | Dias de votos de enquete |

O job de limpeza roda a cada 6 horas. `evo_cleanup_deleted_total{table}` no
`/metrics` mostra quantas linhas cada tabela perdeu.

---

## Logs

| Variável | Padrão | Valores | Descrição |
|----------|--------|---------|-----------|
| `DEBUG_ENABLED` | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR` | Nível de log |
| `LOG_TYPE` | `console` | `console`, `file` | Destino de saída |
| `LOG_DIRECTORY` | `/app/logs` | - | Diretório de arquivos de log |
| `LOG_MAX_SIZE` | `100` | - | Tamanho máximo por arquivo (MB) |
| `LOG_MAX_BACKUPS` | `5` | - | Arquivos de backup a manter |
| `LOG_MAX_AGE` | `30` | - | Retenção em dias |
| `LOG_COMPRESS` | `true` | `true`/`false` | Compressão de logs antigos |

---

## Conexão e Comportamento

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `CONNECT_ON_STARTUP` | `true` | Reconectar instâncias pareadas ao iniciar (desconexões explícitas via API permanecem paradas) |
| `WEBHOOK_FILES` | `true` | Enviar URLs de mídia em webhooks |
| `HISTORY_SYNC_DAYS` | `3650` | Janela de histórico pedida ao celular no pareamento (~10 anos) |
| `HISTORY_SYNC_MAX_MB` | `10240` | Tamanho máximo do histórico pedido ao celular (MB) |
| `HISTORY_STORAGE_QUOTA_MB` | `10240` | Cota de armazenamento declarada ao celular (MB) |
| `RECONNECT_CONFLICT_LIMIT` | `10` | Fechamentos 440 consecutivos antes de bloquear a reconexão automática; `0` = ilimitado. A sessão é mantida para um connect manual |
| `WATCHDOG_MODE` | `report` | `off`, `report` (só registra o que faria) ou `on` (reconstrói o socket travado sem logout) |
| `WATCHDOG_MAX_REBUILDS` | `5` | Reconstruções consecutivas sem socket saudável antes de desistir daquela instância |
| `INSTANCE_IDLE_EVICT_MINUTES` | `0` | Remove o cliente em memória de uma instância desconectada após N minutos; banco, JID e sessão são mantidos. `0` desliga |
| `QRCODE_TERMINAL` | `true` | Desenha o QR Code no log da instância e no terminal (`docker logs`). Com `LOG_TYPE=json`, só no log da instância |
| `QRCODE_MAX_COUNT` | `5` | Tentativas máximas de QR Code |
| `CHECK_USER_EXISTS` | `true` | Validar destinatário antes de enviar |

---

## Eventos

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `EVENT_IGNORE_GROUP` | `false` | Ignorar eventos de grupos |
| `EVENT_IGNORE_STATUS` | `true` | Ignorar eventos de status/stories |
| `WEBHOOK_URL` | - | URL para callbacks HTTP |
| `WEBHOOK_INCLUDE_TOKEN` | `true` | Inclui o token da instância no payload de eventos. `false` remove o token de webhooks, RabbitMQ, NATS e WebSocket; `instanceId`/`instanceName` continuam |

---

## RabbitMQ (AMQP)

| Variável | Descrição |
|----------|-----------|
| `AMQP_URL` | URL de conexão RabbitMQ |
| `AMQP_GLOBAL_ENABLED` | Habilitar filas globais |
| `AMQP_GLOBAL_EVENTS` | Eventos a publicar (separados por vírgula) |
| `AMQP_SPECIFIC_EVENTS` | Eventos específicos por instância |

**Exemplo:**
```env
AMQP_URL=amqp://user:pass@rabbitmq:5672/vhost
AMQP_GLOBAL_ENABLED=true
AMQP_GLOBAL_EVENTS=messages.upsert,messages.update,connection.update
```

---

## NATS

| Variável | Descrição |
|----------|-----------|
| `NATS_URL` | URL de conexão NATS |
| `NATS_GLOBAL_ENABLED` | Habilitar publicação global |
| `NATS_GLOBAL_EVENTS` | Eventos a publicar |

**Exemplo:**
```env
NATS_URL=nats://nats:4222
NATS_GLOBAL_ENABLED=true
NATS_GLOBAL_EVENTS=messages.upsert,connection.update
```

---

## MinIO/S3

| Variável | Descrição |
|----------|-----------|
| `MINIO_ENABLED` | Habilitar armazenamento S3-compatible |
| `MINIO_ENDPOINT` | Endpoint do servidor |
| `MINIO_ACCESS_KEY` | Access Key para autenticação |
| `MINIO_SECRET_KEY` | Secret Key para autenticação |
| `MINIO_BUCKET` | Nome do bucket |
| `MINIO_USE_SSL` | Utilizar HTTPS |
| `MINIO_REGION` | Região do bucket (AWS) |
| `S3_PUBLIC_READ` | Concede leitura pública (`s3:GetObject`) no bucket global. Padrão `false`: as URLs são presignadas e expiram |
| `INTEGRATIONS_ALLOW_PRIVATE` | Permite que webhooks, Typebot e S3 por instância alcancem endereços privados/loopback/metadata. Padrão `false`; habilite apenas para um n8n/MinIO interno |

**Exemplo:**
```env
MINIO_ENABLED=true
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=wamux-media
MINIO_USE_SSL=false
MINIO_REGION=us-east-1
```

---

## Proxy HTTP

| Variável | Descrição |
|----------|-----------|
| `PROXY_HOST` | Hostname do proxy |
| `PROXY_PORT` | Porta do proxy |
| `PROXY_USERNAME` | Usuário (opcional) |
| `PROXY_PASSWORD` | Senha (opcional) |

**Exemplo:**
```env
PROXY_HOST=proxy.empresa.com
PROXY_PORT=8080
PROXY_USERNAME=usuario
PROXY_PASSWORD=senha
```

---

## Recursos Adicionais

| Variável | Descrição |
|----------|-----------|
| `API_AUDIO_CONVERTER` | URL de serviço de conversão de áudio |
| `API_AUDIO_CONVERTER_KEY` | Chave de autenticação do conversor |

---

## Versão WhatsApp (Avançado)

| Variável | Descrição |
|----------|-----------|
| `WHATSAPP_VERSION_MAJOR` | Versão major do WhatsApp Web |
| `WHATSAPP_VERSION_MINOR` | Versão minor do WhatsApp Web |
| `WHATSAPP_VERSION_PATCH` | Versão patch do WhatsApp Web |

**⚠️ Atenção**: Modificar versão do WhatsApp pode resultar em bloqueio. Deixar vazio para usar versão automática.

---

## Exemplo Completo

```env
# Obrigatórias
GLOBAL_API_KEY=df16caad-d0d2-41b2-bec5-75b90048a0db
DATABASE_SAVE_MESSAGES=true

# Servidor
SERVER_PORT=8081
CLIENT_NAME=wamux
OS_NAME=Linux

# Banco de Dados
POSTGRES_AUTH_DB=postgresql://postgres:senha@postgres:5432/wamux_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://postgres:senha@postgres:5432/wamux_users?sslmode=disable

# Logs
DEBUG_ENABLED=INFO
LOG_TYPE=console

# Comportamento
CONNECT_ON_STARTUP=true
WEBHOOK_FILES=true
CHECK_USER_EXISTS=true
EVENT_IGNORE_STATUS=true

# Webhook
WEBHOOK_URL=https://seu-servidor.com/webhook

# RabbitMQ (opcional)
AMQP_URL=amqp://admin:admin@rabbitmq:5672/default
AMQP_GLOBAL_ENABLED=true
AMQP_GLOBAL_EVENTS=messages.upsert,connection.update

# MinIO (opcional)
MINIO_ENABLED=true
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=wamux-media
MINIO_USE_SSL=false
```

---

## Recursos

- **[Configuração Detalhada](../fundamentos/configuration.md)** - Documentação completa de cada variável
- **[.env.example](https://github.com/FelipeEstevanatto/wamux/blob/main/docker/examples/.env.example)** - Arquivo de exemplo com todas as variáveis

---

**Documentação WaMux v1.0**

---

## WhatsApp Cloud API (WABA)

O canal oficial da Meta é opcional e desabilitado por padrão. O `DATA_ENCRYPTION_KEY`
(ou a chave derivada de `GLOBAL_ENCRYPTION_KEY`/`GLOBAL_API_KEY`) é obrigatório
para armazenar o token de acesso criptografado.

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `CLOUD_API_ENABLED` | `false` | Habilita criação de instâncias e as rotas do Cloud API |
| `CLOUD_API_GRAPH_BASE_URL` | `https://graph.facebook.com` | Base do Graph; use um servidor de teste apenas em desenvolvimento |
| `CLOUD_API_GRAPH_VERSION` | `v27.0` | Versão do Graph; confirme a versão suportada no deploy |
| `CLOUD_API_APP_ID` | vazio | App Meta inicial (seed de `cloud_apps`) |
| `CLOUD_API_APP_SECRET` | vazio | App secret inicial; criptografado no banco |
| `CLOUD_API_VERIFY_TOKEN` | vazio | Verify token inicial; o valor é salvo como hash |
| `CLOUD_API_HTTP_TIMEOUT_SECONDS` | `30` | Timeout de requisições JSON |
| `CLOUD_API_RESPONSE_HEADER_TIMEOUT_SECONDS` | `60` | Timeout para o primeiro header |
| `CLOUD_API_MEDIA_TIMEOUT_SECONDS` | `120` | Timeout de transferência de mídia |
| `CLOUD_API_MAX_WEBHOOK_BYTES` | `4194304` | Limite do corpo do webhook (a Meta documenta 3 MB) |
| `CLOUD_API_MAX_RESPONSE_BYTES` | `10485760` | Limite de resposta JSON do Graph |
| `CLOUD_API_MAX_RESPONSE_HEADER_BYTES` | `1048576` | Limite de headers de resposta |
| `CLOUD_API_MAX_MEDIA_BYTES` | `104857600` | Limite de mídia de entrada |
| `CLOUD_API_MEDIA_WORKERS` | `2` | Workers de download de mídia |
| `CLOUD_API_WINDOW_HOURS` | `24` | Janela de atendimento |
| `CLOUD_API_ENFORCE_WINDOW` | `true` | Bloqueia mensagens livres fora da janela |
| `CLOUD_API_RPS` | `8` | Piso do limitador adaptativo |
| `CLOUD_API_RETRY_MAX` | `3` | Tentativas para requisições idempotentes |

> Envios `POST /messages` nunca são repetidos automaticamente. Um timeout é
> reportado como resultado incerto; o chamador deve consultar a operação em vez
> de reenviar às cegas.

O webhook público é `GET/POST /webhook/cloud` (ou `/webhook/cloud/:appId` para
múltiplos apps) e exige `X-Hub-Signature-256`. O callback precisa ser HTTPS e
público; `localhost` não é acessível pela Meta.
