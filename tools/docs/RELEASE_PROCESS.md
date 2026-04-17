# Processo de Release — Evolution GO

> **Documento interno.** Este arquivo vive em `tools/docs/` porque o workflow
> `sync-releases.yml` limpa `tools/` antes do push para os repos públicos
> (`evolution-go` e `evolution-go-beta`). Nunca mover para `docs/`, raiz ou
> qualquer path copiado pelo workflow.

## Arquitetura de repositórios

Existem 3 repositórios envolvidos:

| Repo | Papel | Quem pusha |
|---|---|---|
| `EvolutionAPI/evolution-go-develop` | **Fonte de verdade** — onde o código é desenvolvido | Time (push manual) |
| `EvolutionAPI/evolution-go-beta` | Espelho da branch `develop`, com `pkg/core` ofuscado | Apenas o workflow |
| `EvolutionAPI/evolution-go` | **Espelho público oficial** da branch `main`, com `pkg/core` ofuscado | Apenas o workflow |

O fluxo é **unidirecional**: `develop-repo → público`. Os repos públicos são
mirrors — qualquer commit feito direto neles é **sobrescrito** no próximo sync.

## O que o workflow faz (`.github/workflows/sync-releases.yml`)

Dispara em `push` nas branches `main` ou `develop` deste repo (`evolution-go-develop`):

1. Determina o target:
   - `main` → `evolution-go` + versão do arquivo `VERSION`
   - `develop` → `evolution-go-beta` + `VERSION` com sufixo `-beta`
2. Clona o target e **apaga tudo** mantendo só `.git`
3. Constrói o `evolution-go-manager` (pnpm build)
4. Copia:
   - Arquivos raiz: `README.md`, `LICENSE`, `NOTICE`, `TRADEMARKS.md`,
     `CHANGELOG.md`, `COMMANDS.md`, `.env.example`, `.dockerignore`,
     `Dockerfile`, `Makefile`
   - Dirs: `docker/`, `docs/`, `public/`, `manager/dist/`, `cmd/`
   - `pkg/**/*.go` (exceto `pkg/core/`)
   - `tools/build-dist/templates/.github/` como `.github/` do target
5. **Ofusca** `pkg/core/` via `tools/build-dist/obfuscate.go` em um único
   arquivo `pkg/core/c0.go`
6. Registra `whatsmeow-lib` como submódulo apontando para
   `EvolutionAPI/whatsmeow`
7. Remove: `manager/dist/dist`, `tools/`, `scripts/`, `CLAUDE.md`
8. `go build ./cmd/evolution-go/` para validar
9. Commita no target, cria tag com a versão e `git push --tags`

## Implicações práticas

- Toda mudança que deve chegar nos repos públicos **precisa estar no develop**.
- `pkg/core/` é ofuscado — código ali vai virar um arquivo ilegível público.
- `CLAUDE.md`, `tools/`, `scripts/` ficam só no develop (são limpos no sync).
- Arquivos **não listados** (ex: este `.md`, `.claude/`, configs próprias)
  nunca chegam ao público — o workflow apaga tudo no target antes de copiar
  somente a whitelist.

## Como tratar PRs do repositório público

PRs da comunidade abrem no `EvolutionAPI/evolution-go` (público). Não podem
ser mergeados direto lá — seriam sobrescritos. O procedimento correto:

1. **Revisar** o diff do PR no público.
2. **Aplicar** as mudanças manualmente no `evolution-go-develop` (cherry-pick
   do branch do autor, ou edição manual quando o PR traz arquivos que não
   existem mais aqui — ex: `pkg/telemetry` removido).
3. Commitar no develop com mensagem creditando o autor
   (`Co-Authored-By: Nome <email>` ou referência `Closes EvolutionAPI/evolution-go#NN`).
4. Atualizar `CHANGELOG.md` mencionando o autor e o número do PR.
5. Push `develop` (dispara build beta).
6. Quando fizer release: merge `develop` → `main`, atualizar `VERSION`, push
   (dispara build na `evolution-go` pública com tag).
7. **Após** o sync completar com sucesso, fechar o PR no público com um
   comentário do tipo:

   > Integrated into develop in commit `<sha>` and released in v`<X.Y.Z>`.
   > Thank you for the contribution, @autor! Closing as already merged.

## Como tratar issues do repositório público

Issues são respondidas direto no público. Correções de código seguem o mesmo
fluxo de PRs: fix no develop, aguardar release, fechar a issue referenciando
o commit/versão.

## Checklist de release (v`X.Y.Z`)

1. [ ] Todos os fixes/features desejados estão commitados em `develop`
2. [ ] `go vet ./...` e `go build ./...` passam localmente
3. [ ] `VERSION` bumpado (segue semver)
4. [ ] `CHANGELOG.md` tem a seção da nova versão com créditos
5. [ ] `whatsmeow-lib` está no commit correto (submódulo)
6. [ ] Postman collection (`Evolution GO.postman_collection*.json`) reflete
      as rotas reais de `pkg/routes/routes.go`
7. [ ] `make swagger` rodado se houver anotações novas
8. [ ] Push `develop` → confirmar build `evolution-go-beta` verde
9. [ ] Merge `develop` → `main`
10. [ ] Push `main` → confirmar build `evolution-go` verde com tag criada
11. [ ] Criar Release no GitHub público usando o CHANGELOG da versão
12. [ ] Fechar PRs/issues do público que foram incorporados

## Troubleshooting

| Sintoma | Causa provável |
|---|---|
| Arquivo não aparece no público | Não está na whitelist do workflow — adicionar em `.github/workflows/sync-releases.yml` ou mover para um path copiado |
| Tag já existia e o push falhou | O workflow faz `tag -d` e `push :refs/tags/X` antes — se ainda falhar, ver permissão do `RELEASE_TOKEN` |
| Build falha em `Verify build` | Código que só compila no develop (depende de `pkg/core/` não-ofuscado) — refatorar |
| Manager não atualiza | Submódulo `evolution-go-manager` no develop precisa estar atualizado |
