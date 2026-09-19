# Documentação do Códice

Idioma: português brasileiro. Estes documentos foram elaborados em 18 e 19 de setembro de 2026, antes de se mexer no backend, e são a fonte de verdade para as próximas mudanças.

## Documentos

| Arquivo | Para que serve |
| --- | --- |
| [Codice_Especificacao_Mestre_v0.4.md](Codice_Especificacao_Mestre_v0.4.md) | Regras, decisões (DEC-001 a DEC-071), requisitos (RF/RNF/RN), modelo, arquitetura e questões em aberto. **Comece por aqui.** |
| [Codice_Analise_Inicial_Backend.md](Codice_Analise_Inicial_Backend.md) | Análise do backend existente contra a especificação: achados de segurança e de modelo de dados (§4), matriz por requisito (§5) e verificação de código da Fase 0 (§9). |
| [Codice_Plano_Implementacao_v0.1.md](Codice_Plano_Implementacao_v0.1.md) | Plano em fases (0 a 6), com tarefas, testes e dependências. |

Os arquivos `codice_analysis_and_plan.md` e `codice_remaining_tasks.md`, na raiz do repositório, são planos anteriores (sprints de extratores e metadados). Não conhecem as decisões de governança da especificação atual.

## Convenções da especificação

- **C** confirmado pelo mantenedor, **P** proposto, **A** em aberto. Uma proposta não é decisão aprovada nem prova de implementação.
- Os IDs (RF, RNF, RN, DEC, QA, UI, FL) são estáveis: não reutilizar, marcar como substituído.
- Nada foi implementado por causa da especificação. O estado do código está na análise.

## Estado em 19 de setembro de 2026

**Fase 0 (preparar o terreno): concluída.** Corpus sintético em `testdata/`, script `scripts/reset-dev-db.sh` (simulação por padrão) e trabalho local salvo em `chore/salvar-trabalho-local`.

**Fase 1 (segurança e autorização): concluída na parte técnica**, no branch `feat/fase1-seguranca`, ainda sem merge. Os cinco testes que abriam a fase passam, e cada um foi confirmado falhando quando a proteção correspondente é removida.

- Sem credencial: 401 em qualquer `APP_ENV`. O fallback que dava admin a requisições anônimas (HTTP e WebSocket) e o segredo JWT padrão foram removidos; a API não sobe sem `JWT_SECRET`.
- Papéis owner, admin e leitor, com política em `backend/internal/authz` (DEC-056, RN-022). Upload, importação em lote, edição e exclusão exigem owner ou admin. `PUT /users/{id}/role` (só owner) promove e rebaixa admins, lendo o papel do banco.
- Setup atômico: transação com trava, mais um índice único parcial que permite um único owner (RF-001). A conta criada no setup é owner; se já havia admins, a migração 012 promove o mais antigo.
- Sessões no banco (DEC-070): o token carrega um id de sessão, e o servidor confere sessão, expiração e bloqueio a cada requisição, lendo o papel do banco. Existe logout, e `RevokeAllForUser` está pronto para o bloqueio e a redefinição de senha da Fase 4.
- Nada de token em URL: capas, arquivos, páginas e áudio usam um token de recurso de 15 minutos (`?rt=`, só GET/HEAD), e o WebSocket usa um ticket de 60 segundos.
- Tokens de aplicativo (DEC-071): `POST/GET/DELETE /auth/app-tokens`. OPDS e downloads por Basic usam o token como senha; **a senha da conta não vale mais ali**.
- Importação em lote confinada a raízes autorizadas (`<storage>/import` e `CODICE_IMPORT_ROOTS`), sem seguir symlinks; só arquivos regulares são importados.
- Login com uma única mensagem de falha; bcrypt com custo 12 e re-hash das contas antigas no login; `JWT_EXPIRATION_HOURS` agora define a duração da sessão.
- Compose: em `docker-compose.yml`, PostgreSQL e Redis presos a `127.0.0.1` e senha do Redis opcional; em `docker-compose.full.yml`, nenhum dos dois publica porta e `REDIS_PASSWORD` é obrigatória.

**Pendências da Fase 1**, que não bloqueiam a Fase 2:
- Tela para criar e revogar tokens de aplicativo (UI-21). Até existir, use a API: sem ela o OPDS fica inutilizável.
- O frontend ainda mostra upload e edição ao leitor, que agora recebe 403. Esconder por papel é ajuste de interface.
- `rt` e `ticket` aparecem no log de acesso do chi. Como são curtos e de escopo restrito o risco é baixo; dá para censurá-los.
- Em `docker-compose.full.yml`, `POSTGRES_PASSWORD` ainda tem valor padrão. O banco deixou de ser exposto, mas convém exigir a variável.
- Bloquear, excluir e convidar contas, transferência de titularidade e redefinição de senha pertencem à Fase 4. Só a troca de papel existe.
- CORS aceita uma única origem, e MFA não existe. `worker/bulk_import.py` é uma ferramenta de linha de comando do operador e não foi alterada.

**Próximo passo: Fase 2, modelo de dados alvo** (Work/Edition/File, migrações versionadas, notas sem exclusão em cascata). Antes de começar, decidir a ferramenta de migração (goose ou golang-migrate) e o destino dos dados de teste, que só serão descartados por ação sua.

## Como rodar os testes

Os testes do backend que usam PostgreSQL só rodam com `TEST_DATABASE_URL`, e recusam qualquer banco cujo nome não termine em `_test`, porque recriam o schema `public` a cada teste. Sem a variável eles são ignorados.

```bash
docker run -d --name codice-test-pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=codice_test -p 127.0.0.1:55432:5432 postgres:16-alpine
export TEST_DATABASE_URL='postgres://postgres:test@127.0.0.1:55432/codice_test?sslmode=disable'
cd backend && go test ./...
```

No frontend, `cd frontend && npx vitest run`. Não use o banco de desenvolvimento como `TEST_DATABASE_URL`.

## Decisões do mantenedor que valem lembrar

- Owner único, papel admin gerido só pelo owner, transferência em duas etapas, recuperação por comando local no servidor.
- Cadastro público desligado; convites de uso único e 7 dias; SMTP opcional; redefinição de senha sem SMTP aprovada por owner ou admin.
- Sem cotas por usuário. Backup diário via comando do Códice, com ferramentas externas a cargo do operador.
- PostgreSQL como fonte da verdade dos jobs; Redis só como entrega.
- Nomes no armazenamento: `Autor/Obra/Idioma — Editora — Ano/Arquivo`; a obra aparece sob cada autor no catálogo.
- Hardware mínimo não é único: será medido por perfil de uso (QA-016).

## Pendências que não bloqueiam a Fase 1

Fronteira da pasta de série (quadrinhos e mangá versus livros), formato da auditoria administrativa, definição de "tempo de leitura" e "conclusão", padrão do papel do antigo owner na recuperação de emergência (proposto: leitor), QA-018 (clientes OPDS), QA-022 (licenças), QA-023 (variantes de EPUB/PDF).

## Ambiente

- O desenvolvimento passa a continuar no macOS, que tem Docker. A máquina Windows onde a Fase 0 foi feita não tinha Docker nem PostgreSQL.
- Este repositório tem `.env` local: nunca commitar nem ler o conteúdo dele em documentos ou logs.
- O script de reset do banco só simula sem `--yes`; não rodar com `--yes` sem decidir conscientemente recriar os dados de teste.
