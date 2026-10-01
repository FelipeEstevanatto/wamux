# Deploy com Docker

Guia de deploy do WaMux usando Docker, Docker Compose, Swarm e Kubernetes.

## Índice

- [Visão Geral](#visão-geral)
- [Deploy com Docker Compose](#deploy-com-docker-compose)
- [Deploy com Docker Swarm](#deploy-com-docker-swarm)
- [Deploy com Kubernetes](#deploy-com-kubernetes)
- [Otimização e Gestão](#otimização-e-gestão)
- [Troubleshooting](#troubleshooting)

---

## Visão Geral

### Estratégias de Deploy

| Estratégia | Uso Recomendado | Complexidade | Escalabilidade |
|------------|-----------------|--------------|----------------|
| **Docker Compose** | Desenvolvimento, testes, deploys pequenos | Baixa | Limitada (single-host) |
| **Docker Swarm** | Produção pequena/média, HA | Média | Boa (multi-host) |
| **Kubernetes** | Produção enterprise, orquestração avançada | Alta | Excelente |

### Arquitetura de Componentes

```
┌─────────────────────────────────────────────────────────────────┐
│                     WAMUX STACK                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐ │
│  │ WaMux │◄────►│  PostgreSQL  │      │  RabbitMQ    │ │
│  │   (API)      │      │   (Auth DB)  │      │  (Events)    │ │
│  │  Port: 8080  │      │   (Users DB) │      │  Port: 5672  │ │
│  └──────┬───────┘      └──────────────┘      └──────────────┘ │
│         │                                                       │
│         │              ┌──────────────┐      ┌──────────────┐ │
│         └─────────────►│    MinIO     │      │     NATS     │ │
│                        │   (Media)    │      │  (Optional)  │ │
│                        │  Port: 9000  │      │  Port: 4222  │ │
│                        └──────────────┘      └──────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Imagem Docker

- **Registry**: `ghcr.io/felipeestevanatto/wamux`
- **Tags**: `latest`, `v1.x.x`
- **Base**: Alpine Linux 3.19.1
- **Tamanho**: ~50MB (compactada)
- **Arquiteturas**: amd64, arm64

---

## Deploy com Docker Compose

### Setup Básico

Configuração mínima com WaMux + PostgreSQL.

#### docker-compose.yml

```yaml
version: '3.8'

services:
  wamux:
    image: ghcr.io/felipeestevanatto/wamux:latest
    container_name: wamux
    restart: unless-stopped
    ports:
      - "8081:8080"
    environment:
      SERVER_PORT: 8080
      CLIENT_NAME: "wamux"
      GLOBAL_API_KEY: "SUBSTITUA-POR-UUID-FORTE"

      POSTGRES_AUTH_DB: "postgresql://postgres:postgres@postgres:5432/wamux_auth?sslmode=disable"
      POSTGRES_USERS_DB: "postgresql://postgres:postgres@postgres:5432/wamux_users?sslmode=disable"
      DATABASE_SAVE_MESSAGES: "false"

      WADEBUG: "INFO"
      LOGTYPE: "console"
      CONNECT_ON_STARTUP: "false"
      WEBHOOK_FILES: "true"

    volumes:
      - wamux_data:/app/dbdata
      - wamux_logs:/app/logs
    networks:
      - wamux_network
    depends_on:
      - postgres

  postgres:
    image: postgres:15-alpine
    container_name: postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: postgres
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./init-db.sql:/docker-entrypoint-initdb.d/init-db.sql
    networks:
      - wamux_network

volumes:
  wamux_data:
  wamux_logs:
  postgres_data:

networks:
  wamux_network:
    driver: bridge
```

#### init-db.sql

```sql
CREATE DATABASE wamux_auth;
CREATE DATABASE wamux_users;
SELECT 'Databases criados com sucesso!' as message;
```

#### Deploy

```bash
# Gerar API Key
uuidgen

# Editar docker-compose.yml e inserir API Key

# Iniciar
docker-compose up -d

# Verificar
docker-compose logs -f wamux
curl http://localhost:8081/server/ok
```

### Setup Completo

Incluindo RabbitMQ, MinIO e NATS.

```yaml
version: '3.8'

services:
  wamux:
    image: ghcr.io/felipeestevanatto/wamux:latest
    restart: unless-stopped
    ports:
      - "8081:8080"
    environment:
      SERVER_PORT: 8080
      GLOBAL_API_KEY: "SUA-CHAVE-AQUI"

      POSTGRES_AUTH_DB: "postgresql://postgres:senha@postgres:5432/wamux_auth?sslmode=disable"
      POSTGRES_USERS_DB: "postgresql://postgres:senha@postgres:5432/wamux_users?sslmode=disable"
      DATABASE_SAVE_MESSAGES: "true"

      AMQP_URL: "amqp://admin:admin@rabbitmq:5672/default"
      AMQP_GLOBAL_ENABLED: "true"
      AMQP_GLOBAL_EVENTS: "messages.upsert,messages.update,connection.update"
      
      MINIO_ENABLED: "true"
      MINIO_ENDPOINT: "minio:9000"
      MINIO_ACCESS_KEY: "minioadmin"
      MINIO_SECRET_KEY: "minioadmin"
      MINIO_BUCKET: "wamux-media"
      MINIO_USE_SSL: "false"

    volumes:
      - wamux_data:/app/dbdata
      - wamux_logs:/app/logs
    depends_on:
      - postgres
      - rabbitmq
      - minio

  postgres:
    image: postgres:15-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: senha
      POSTGRES_DB: postgres
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./init-db.sql:/docker-entrypoint-initdb.d/init-db.sql

  rabbitmq:
    image: rabbitmq:3-management-alpine
    restart: unless-stopped
    environment:
      RABBITMQ_DEFAULT_USER: admin
      RABBITMQ_DEFAULT_PASS: admin
      RABBITMQ_DEFAULT_VHOST: default
    ports:
      - "5672:5672"
      - "15672:15672"
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq

  minio:
    image: minio/minio:latest
    restart: unless-stopped
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio_data:/data

  nats:
    image: nats:2-alpine
    restart: unless-stopped
    ports:
      - "4222:4222"
      - "8222:8222"

volumes:
  wamux_data:
  wamux_logs:
  postgres_data:
  rabbitmq_data:
  minio_data:
```

**Acessos:**
- WaMux: http://localhost:8081
- Swagger: http://localhost:8081/swagger/index.html
- RabbitMQ: http://localhost:15672 (admin/admin)
- MinIO: http://localhost:9001 (minioadmin/minioadmin)

### Configurações Avançadas

#### Arquivo .env

```bash
# .env
WAMUX_VERSION=latest
POSTGRES_VERSION=15-alpine

# Portas
WAMUX_PORT=8081
POSTGRES_PORT=5432

# Credenciais
POSTGRES_USER=postgres
POSTGRES_PASSWORD=senha_forte
RABBITMQ_USER=admin
RABBITMQ_PASS=senha_forte

# WaMux
GLOBAL_API_KEY=df16caad-d0d2-41b2-bec5-75b90048a0db
CLIENT_NAME=wamux-prod
```

Referência no compose:
```yaml
services:
  wamux:
    image: ghcr.io/felipeestevanatto/wamux:${WAMUX_VERSION:-latest}
    ports:
      - "${WAMUX_PORT:-8081}:8080"
    environment:
      GLOBAL_API_KEY: "${GLOBAL_API_KEY}"
```

#### Health Checks

```yaml
services:
  wamux:
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/server/ok"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

  postgres:
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5

  rabbitmq:
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "ping"]
      interval: 30s
      timeout: 10s
      retries: 3
```

#### Resource Limits

```yaml
services:
  wamux:
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 512M

  postgres:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 1G
        reservations:
          cpus: '0.25'
          memory: 256M
```

---

## Deploy com Docker Swarm

### Inicialização

```bash
# Manager node
docker swarm init --advertise-addr 192.168.1.10

# Worker nodes
docker swarm join --token SWMTKN-1-xxxxx 192.168.1.10:2377

# Verificar cluster
docker node ls
```

### Preparação

```bash
# Volumes
docker volume create wamux_data
docker volume create wamux_logs

# Rede
docker network create --driver overlay network_public
```

### docker-compose.swarm.yml

```yaml
version: '3.8'

services:
  wamux:
    image: ghcr.io/felipeestevanatto/wamux:latest
    networks:
      - network_public
    environment:
      SERVER_PORT: 8080
      GLOBAL_API_KEY: "sua-chave-api"
      POSTGRES_AUTH_DB: "postgresql://user:pass@postgres:5432/wamux_auth"
      POSTGRES_USERS_DB: "postgresql://user:pass@postgres:5432/wamux_users"

    volumes:
      - wamux_data:/app/dbdata
      - wamux_logs:/app/logs

    deploy:
      replicas: 3
      placement:
        constraints:
          - node.role == worker
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
        window: 120s
      update_config:
        parallelism: 1
        delay: 10s
        failure_action: rollback
        order: start-first
      rollback_config:
        parallelism: 1
        delay: 5s
      labels:
        - traefik.enable=true
        - traefik.http.routers.wamux.rule=Host(`wamux.domain.com`)
        - traefik.http.routers.wamux.entrypoints=websecure
        - traefik.http.routers.wamux.tls.certresolver=letsencrypt
        - traefik.http.services.wamux.loadbalancer.server.port=8080

volumes:
  wamux_data:
    external: true
  wamux_logs:
    external: true

networks:
  network_public:
    external: true
```

### Deploy e Gerenciamento

```bash
# Deploy
docker stack deploy -c docker-compose.swarm.yml wamux

# Status
docker stack ls
docker service ls
docker service ps wamux_wamux

# Logs
docker service logs wamux_wamux -f

# Escalar
docker service scale wamux_wamux=5

# Atualizar (rolling update)
docker service update --image ghcr.io/felipeestevanatto/wamux:v1.2.0 wamux_wamux

# Remover
docker stack rm wamux
```

---

## Deploy com Kubernetes

### Manifests Básicos

#### Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
    name: wamux
```

#### ConfigMap

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: wamux-config
  namespace: wamux
data:
  SERVER_PORT: "8080"
  CLIENT_NAME: "wamux"
  WADEBUG: "INFO"
  LOGTYPE: "console"
  CONNECT_ON_STARTUP: "false"
  WEBHOOK_FILES: "true"
  DATABASE_SAVE_MESSAGES: "false"
```

#### Secrets

```bash
kubectl create secret generic wamux-secrets \
  --from-literal=GLOBAL_API_KEY=$(uuidgen) \
  --from-literal=POSTGRES_PASSWORD=$(openssl rand -base64 32) \
  --namespace=wamux
```

#### Deployment

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: wamux
  namespace: wamux
spec:
  replicas: 3
  selector:
    matchLabels:
      app: wamux
  template:
    metadata:
      labels:
        app: wamux
    spec:
      containers:
      - name: wamux
        image: ghcr.io/felipeestevanatto/wamux:latest
        ports:
        - containerPort: 8080
        env:
        - name: SERVER_PORT
          valueFrom:
            configMapKeyRef:
              name: wamux-config
              key: SERVER_PORT
        - name: GLOBAL_API_KEY
          valueFrom:
            secretKeyRef:
              name: wamux-secrets
              key: GLOBAL_API_KEY
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
        livenessProbe:
          httpGet:
            path: /server/ok
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /server/ok
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
        volumeMounts:
        - name: wamux-data
          mountPath: /app/dbdata
        - name: wamux-logs
          mountPath: /app/logs
      volumes:
      - name: wamux-data
        persistentVolumeClaim:
          claimName: wamux-data-pvc
      - name: wamux-logs
        persistentVolumeClaim:
          claimName: wamux-logs-pvc
```

#### Service

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: wamux-service
  namespace: wamux
spec:
  type: LoadBalancer
  selector:
    app: wamux
  ports:
  - port: 8080
    targetPort: 8080
```

#### Ingress

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: wamux-ingress
  namespace: wamux
  annotations:
    kubernetes.io/ingress.class: "nginx"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
spec:
  tls:
  - hosts:
    - wamux.domain.com
    secretName: wamux-tls
  rules:
  - host: wamux.domain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: wamux-service
            port:
              number: 8080
```

#### HorizontalPodAutoscaler

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: wamux-hpa
  namespace: wamux
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: wamux
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

### Deploy Kubernetes

```bash
# Aplicar manifests
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secrets.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
kubectl apply -f hpa.yaml

# Verificar
kubectl get all -n wamux
kubectl get pods -n wamux

# Logs
kubectl logs -f deployment/wamux -n wamux

# Escalar
kubectl scale deployment wamux --replicas=5 -n wamux

# Atualizar
kubectl set image deployment/wamux \
  wamux=ghcr.io/felipeestevanatto/wamux:v1.2.0 \
  -n wamux

# Rollback
kubectl rollout undo deployment/wamux -n wamux
```

---

## Otimização e Gestão

### Gestão de Volumes

#### Backup

```bash
# Backup volume
docker run --rm \
  -v wamux_data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/backup-$(date +%Y%m%d).tar.gz -C /data .

# Restaurar
docker run --rm \
  -v wamux_data:/data \
  -v $(pwd):/backup \
  alpine tar xzf /backup/backup-20250111.tar.gz -C /data
```

### Logging

```yaml
services:
  wamux:
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

### Boas Práticas

**Segurança:**
- Não executar containers como root
- Usar secrets para credenciais
- Habilitar HTTPS em produção
- Configurar resource limits
- Implementar health checks

**Performance:**
- Definir resource requests/limits
- Usar health checks
- Implementar HPA (Kubernetes)
- Configurar connection pooling

**Monitoramento:**
- Coletar logs centralizados
- Implementar métricas (Prometheus)
- Configurar alertas
- Dashboard de visualização (Grafana)

---

## Troubleshooting

### Container Reiniciando

```bash
# Ver logs
docker-compose logs wamux

# Verificar variáveis obrigatórias
# - GLOBAL_API_KEY
# - POSTGRES_*_DB
```

### Conectividade PostgreSQL

```bash
# Testar conexão
docker-compose exec wamux ping postgres

# Verificar porta
docker-compose exec wamux nc -zv postgres 5432

# Inspecionar rede
docker network inspect wamux_network
```

### Sem Espaço em Disco

```bash
# Ver uso
docker system df

# Limpar
docker container prune
docker image prune
docker volume prune  # CUIDADO: apaga volumes não utilizados
docker system prune -a
```

### OOM (Out of Memory)

```bash
# Ver eventos OOM
docker events --filter 'event=oom'

# Ver uso de memória
docker stats wamux

# Aumentar limite
deploy:
  resources:
    limits:
      memory: 4G
```

---

## Comandos Úteis

### Docker Compose

```bash
docker-compose up -d          # Iniciar
docker-compose ps             # Status
docker-compose logs -f        # Logs
docker-compose stop           # Parar
docker-compose restart        # Reiniciar
docker-compose down           # Remover
docker-compose pull           # Atualizar imagens
```

### Docker Swarm

```bash
docker stack deploy -c file.yml name    # Deploy
docker stack ls                          # Listar stacks
docker service ls                        # Listar serviços
docker service logs name -f              # Logs
docker service scale name=N              # Escalar
docker stack rm name                     # Remover
```

### Kubernetes

```bash
kubectl apply -f file.yaml              # Aplicar
kubectl get all -n namespace            # Listar recursos
kubectl logs -f deployment/name         # Logs
kubectl scale deployment name --replicas=N  # Escalar
kubectl delete -f file.yaml             # Deletar
```

---

**Documentação WaMux v1.0**
