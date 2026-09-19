# Documentação do Códice

Idioma: português brasileiro. Estes documentos foram elaborados em 18 e 19 de setembro de 2026, antes de se mexer no backend, e são a fonte de verdade para as próximas mudanças.

## Documentos

| Arquivo | Para que serve |
| --- | --- |
| [Codice_Especificacao_Mestre_v0.4.md](Codice_Especificacao_Mestre_v0.4.md) | Regras, decisões (DEC-001 a DEC-075), requisitos (RF/RNF/RN), modelo, arquitetura e questões em aberto. **Comece por aqui.** |
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

**Fase 2 (modelo de dados alvo): concluída na parte técnica**, no mesmo branch. Detalhes e decisões no plano (§4).

- Migrações versionadas com goose. Ao subir, a API aplica `00001_baseline` (o esquema antigo, idempotente) e `00002_model_target`. **Antes de rodar no seu banco de desenvolvimento, faça um backup** (`pg_dump`): a migração copia dados e não apaga nada, mas é o primeiro uso real.
- Obra com várias edições e vários arquivos (formato e idioma distintos), com hash único por arquivo; progresso por usuário e arquivo; autores múltiplos; notas que sobrevivem à obra e guardam título e autor; registro de auditoria.
- Retirada reversível no lugar da exclusão: o botão do frontend agora diz "Retire from Library". A exclusão física é um passo extra (`?purge=true`), só para obra já retirada.
- O worker continua escrevendo nas colunas antigas de `works`; gatilhos no banco projetam essas escritas no modelo novo. Isso sai na Fase 3.

**Fase 3 (jobs, ingestão e armazenamento): parcial.** As três pendências da Fase 2 foram resolvidas (edição completa de metadados com travas e proveniência, arquivos servidos só quando o catálogo os possui, auditoria somente de acréscimo), e entraram a fila de jobs no PostgreSQL e a ingestão endurecida. Detalhes e o que falta no plano (§5).

- **Jobs:** PostgreSQL é a fonte da verdade e o Redis só acorda o worker, então reiniciar ou esvaziar o Redis, ou subir sem ele, não perde nada. Erros temporários repetem até 3 vezes (30 s, 2 min, 10 min), os permanentes não repetem, e um job cujo worker sumiu é retomado por outro. `GET /admin/jobs`, `POST /admin/jobs/{id}/rerun` e `/cancel`, para owner e admin.
- **Upload e importação em lote:** limite real de tamanho (`CODICE_MAX_UPLOAD_MB`), validação do conteúdo, bytes idênticos recusados com a indicação do registro existente, e obra, hash e job numa só transação. O frontend agora explica a recusa.
- **Metadados:** o worker só preenche o que está vazio; o que os provedores externos encontram vira **sugestão** para aceitar ou rejeitar no modal de edição. Campos que você altera ficam confirmados e travados.
- **Worker:** rode o worker e a API. O worker sozinho já consome a fila pelo banco; o Redis é opcional.

**Fase 3, segunda etapa: organização em disco e modo referenciado, concluídos** (plano §5).

- **Layout:** cada arquivo gerenciado vai para `Autor/Obra/Idioma — Editora — Ano/Arquivo` assim que a análise termina; quadrinhos em série vão para `Série/NN - Título`. Corrigir metadados não move nada: uma reorganização é **explícita**, com prévia e confirmação do plano exato (`GET` e `POST /admin/storage/reorganize`).
- **Modo referenciado:** o owner autoriza diretórios (`/admin/storage/roots`), owner e admin varrem (`POST /admin/library/scan`) e os arquivos ficam onde estão, sem serem tocados. Os que somem ficam marcados como ausentes com notas e progresso preservados.
- **Mover para o gerenciado** (`POST /admin/library/move-to-managed`): copia, confere o hash e só então remove a origem, e só se ela não mudou. O que não puder ser removido fica em `GET /admin/storage/cleanups`.
- **Atenção:** a importação em lote continua **só copiando**; para movê-los é preciso pedir `removeOriginals: true`. A DEC-033 faz da remoção o padrão, e a decisão é sua.
- **Antes de rodar:** faça um `pg_dump`. São oito migrações. Depois da atualização, os arquivos já existentes continuam no caminho plano até você pedir uma reorganização.

**Pendências da Fase 3:** lixeira recuperável, candidatos a duplicidade por título/autor/ISBN, detecção de páginas sem texto para OCR, remoção dos gatilhos de compatibilidade e das colunas antigas, telas de jobs, raízes e reorganização, e limpeza de arquivos órfãos.

**Próximo passo:** abrir o PR do branch (fases 1 a 3) e, depois, a Fase 4 (contas e governança, com o LDAP). Os itens acima podem entrar antes ou depois.

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
- Login por um formulário único: o backend escolhe o provedor pela conta, sem tentar um após o outro. O owner autentica sempre por senha local. O LDAP vem primeiro; o OIDC depois, como botão separado. Se uma entrada do LDAP tem o mesmo nome de uma conta local, ela é vinculada automaticamente só se a pessoa provar as duas senhas (DEC-072 a 075).
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
