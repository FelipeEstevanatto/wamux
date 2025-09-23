# Teste de Atualização de Instância

## Cenário de Teste

### 1. **Primeira Conexão** (Instância não existe):
```bash
POST /instance/connect
{
  "webhookUrl": "https://webhook1.example.com",
  "subscribe": ["MESSAGE", "RECEIPT"]
}
```
**Resultado Esperado**: 
- Nova instância criada
- Cliente iniciado
- Configurações aplicadas

### 2. **Segunda Conexão** (Instância já existe e está rodando):
```bash
POST /instance/connect
{
  "webhookUrl": "https://webhook2.example.com", 
  "subscribe": ["MESSAGE", "RECEIPT", "PRESENCE"]
}
```
**Resultado Esperado**:
- ✅ Instância **NÃO** reiniciada
- ✅ Configurações atualizadas no banco
- ✅ Configurações atualizadas na instância em execução
- ✅ Cache do userInfo atualizado
- ✅ Log: "Instance already running, settings updated without restarting client"

### 3. **Terceira Conexão** (Instância existe mas não está rodando):
```bash
POST /instance/connect
{
  "webhookUrl": "https://webhook3.example.com",
  "subscribe": ["ALL"]
}
```
**Resultado Esperado**:
- ✅ Nova instância iniciada
- ✅ Configurações aplicadas
- ✅ Log: "Starting new client instance"

## Logs Importantes

### Quando instância já está rodando:
```
[instanceId] Instance settings updated successfully in runtime
[instanceId] Instance already running, settings updated without restarting client
```

### Quando instância não está rodando:
```
[instanceId] Instance not in runtime yet, will be updated when connected
[instanceId] Starting new client instance
```

## Verificação

1. **Webhook URL** deve ser atualizada na instância em execução
2. **Eventos** devem ser atualizados sem reiniciar o cliente
3. **Cache** deve refletir as novas configurações
4. **Não deve haver** múltiplas instâncias rodando em paralelo
