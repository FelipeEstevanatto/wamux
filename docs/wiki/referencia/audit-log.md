# Audit Log

WaMux records administrative actions locally. Nothing is sent to a third party.

## Onde fica

A tabela `audit_logs` no banco de usuários (`POSTGRES_USERS_DB`), criada pela
migration `0016_audit_logs`. Sem banco configurado, as ações viram uma linha
estruturada `[AUDIT]` no log normal.

## O que é registrado

| Ação | Quando |
|---|---|
| `instance.create` | Uma instância é criada |
| `instance.delete` | Uma instância é removida |
| `instance.rename` | O nome de exibição muda |
| `instance.proxy.update` / `.delete` | Proxy por instância |
| `instance.s3.update` / `.delete` | S3 por instância (sem access/secret key) |
| `instance.hmac.update` / `.delete` | Chave HMAC de webhook (sem a chave) |
| `instance.settings.update` | Advanced settings |
| `instance.token.read` | `GET /instance/all` ou `/instance/info/:id` devolveu token |

Cada linha tem `actor` (`admin` ou o id da instância), `action`, `target`,
`detail` (JSON pequeno, sem segredos) e `created_at`.

## Consultar

```bash
curl -H "apikey: $GLOBAL_API_KEY" \
  "http://localhost:8081/server/audit?limit=100"
```

`limit` padrão 100, máximo 1000. A rota exige a chave global.

## Retenção

O audit é append-only e não é apagado automaticamente. Para manter o banco
pequeno, remova linhas antigas com SQL, por exemplo:

```sql
DELETE FROM audit_logs WHERE created_at < NOW() - INTERVAL '180 days';
```

Nunca coloque tokens, senhas ou payloads de mensagem em `detail`.
