# Documentação do Códice

Idioma: português brasileiro. Estes documentos foram elaborados em 18 e 19 de setembro de 2026, antes de se mexer no backend, e são a fonte de verdade para as próximas mudanças.

## Documentos

| Arquivo | Para que serve |
| --- | --- |
| [Codice_Especificacao_Mestre_v0.4.md](Codice_Especificacao_Mestre_v0.4.md) | Regras, decisões (DEC-001 a DEC-075), requisitos (RF/RNF/RN), modelo, arquitetura e questões em aberto. **Comece por aqui.** |
| [Codice_Analise_Inicial_Backend.md](Codice_Analise_Inicial_Backend.md) | Análise do backend existente contra a especificação: achados de segurança e de modelo de dados (§4), matriz por requisito (§5) e verificação de código da Fase 0 (§9). |
| [Codice_Plano_Implementacao_v0.1.md](Codice_Plano_Implementacao_v0.1.md) | Plano em fases (0 a 6), com tarefas, testes e dependências. |
| [Codice_Validacao_Base_2026-09-19.md](Codice_Validacao_Base_2026-09-19.md) | Validação posterior ao merge do PR #6: testes executados, ensaio com navegador, correções e pendências. |

Os arquivos `codice_analysis_and_plan.md` e `codice_remaining_tasks.md`, na raiz do repositório, são planos anteriores (sprints de extratores e metadados). Não conhecem as decisões de governança da especificação atual.

## Convenções da especificação

- **C** confirmado pelo mantenedor, **P** proposto, **A** em aberto. Uma proposta não é decisão aprovada nem prova de implementação.
- Os IDs (RF, RNF, RN, DEC, QA, UI, FL) são estáveis: não reutilizar, marcar como substituído.
- O estado do código está em "Estado" abaixo e na análise.

## Estado em 19 de setembro de 2026

**Atualização após o merge:** o PR #6 já foi integrado à `main` (`83fc908`). A validação subsequente passou em 232 casos/subcasos Go (com PostgreSQL, nenhum ignorado), 82 testes Python e 54 testes frontend; build e `go vet` aprovados. API, worker e navegador foram exercitados com banco e arquivos sintéticos isolados. Correções locais: inicialização/reconexão sem Redis, atualização de obras pendentes sem WebSocket e propagação de falhas no Makefile. Consulte o relatório acima para o alcance e as pendências; esses resultados não homologam toda a especificação.

**Fase 0 (preparar o terreno): concluída.** Corpus sintético em `testdata/`, script `scripts/reset-dev-db.sh` (simulação por padrão) e trabalho local salvo em `chore/salvar-trabalho-local`.

**Fase 1 (segurança e autorização): concluída na parte técnica**, no branch `feat/fase1-seguranca` (PR #6). Os cinco testes que abriam a fase passam, e cada um foi confirmado falhando quando a proteção correspondente é removida.

- Sem credencial: 401 em qualquer `APP_ENV`. O fallback que dava admin a requisições anônimas (HTTP e WebSocket) e o segredo JWT padrão foram removidos; a API não sobe sem `JWT_SECRET`.
- Papéis owner, admin e leitor, com política em `backend/internal/authz` (DEC-056, RN-022). Upload, importação em lote, edição e exclusão exigem owner ou admin. `PUT /users/{id}/role` (só owner) promove e rebaixa admins, lendo o papel do banco.
- Setup atômico: transação com trava, mais um índice único parcial que permite um único owner (RF-001). A conta criada no setup é owner; se já havia admins, a migração de papéis (dentro de `00001_baseline`) promove o mais antigo.
- Sessões no banco (DEC-070): o token carrega um id de sessão, e o servidor confere sessão, expiração e bloqueio a cada requisição, lendo o papel do banco. Existe logout, e `RevokeAllForUser` está pronto para o bloqueio e a redefinição de senha da Fase 4.
- Nada de token em URL: capas, arquivos, páginas e áudio usam um token de recurso de 15 minutos (`?rt=`, só GET/HEAD), e o WebSocket usa um ticket de 60 segundos.
- Tokens de aplicativo (DEC-071): `POST/GET/DELETE /auth/app-tokens`. OPDS e downloads por Basic usam o token como senha; **a senha da conta não vale mais ali**.
- Importação em lote confinada a raízes autorizadas (`<storage>/import` e `CODICE_IMPORT_ROOTS`), sem seguir symlinks; só arquivos regulares são importados.
- Login com uma única mensagem de falha; bcrypt com custo 12 e re-hash das contas antigas no login; `JWT_EXPIRATION_HOURS` agora define a duração da sessão.
- Compose: em `docker-compose.yml`, PostgreSQL e Redis presos a `127.0.0.1` e senha do Redis opcional; em `docker-compose.full.yml`, nenhum dos dois publica porta e `REDIS_PASSWORD` é obrigatória.

As pendências que restaram da Fase 1 estão na lista de pendências mais abaixo.

**Fase 2 (modelo de dados alvo): concluída.** Detalhes e decisões no plano (§4).

- Migrações versionadas com goose, aplicadas ao subir a API (`backend/internal/database/migrations`, `00001` a `00011`, cada uma com `Down`).
- Obra com várias edições e vários arquivos (formato e idioma distintos), com hash único por arquivo; progresso por usuário e arquivo; autores múltiplos; notas que sobrevivem à obra e guardam título e autor; registro de auditoria só de acréscimo.
- Retirada reversível no lugar da exclusão. A exclusão física é um passo extra (`?purge=true`), só para obra já retirada, e agora vai para a lixeira (abaixo).

**Fase 3 (jobs, ingestão, metadados, armazenamento): concluída.** PR: https://github.com/ocnaibill/codice/pull/6 (`feat/fase1-seguranca` → `main`). Detalhes no plano (§5).

- **Jobs:** PostgreSQL é a fonte da verdade e o Redis só acorda o worker. Erros temporários repetem até 3 vezes (30 s, 2 min, 10 min), os permanentes não repetem, e um job cujo worker sumiu é retomado por outro. O worker Python só pega `ingest`; o processo da API roda `organize`, `scan`, `transfer` e `dedupe`. `GET /admin/jobs`, `POST /admin/jobs/{id}/rerun` e `/cancel`.
- **Upload e importação em lote:** limite de tamanho (`CODICE_MAX_UPLOAD_MB`), validação do conteúdo, bytes idênticos recusados, e obra, hash e job numa só transação.
- **Metadados:** o worker só preenche o que está vazio; o que os provedores externos encontram vira **sugestão** para aceitar ou rejeitar. Campos que você altera ficam confirmados e travados. ISBN, editora, idioma e data ficam na edição principal; o autor, em `work_contributors`.
- **Armazenamento:** layout `Autor/Obra/Idioma — Editora — Ano/Arquivo` (quadrinhos em série: `Série/NN - Título`), reorganização **explícita** com prévia e confirmação do plano por hash (`/admin/storage/reorganize`), modo referenciado (o owner autoriza pastas em `/admin/storage/roots`, `POST /admin/library/scan`) e "mover para o gerenciado" (copia, confere o hash e só remove a origem se ela não mudou; o que sobrar fica em `/admin/storage/cleanups`).
- **Lixeira recuperável:** a exclusão física de uma obra retirada move os arquivos para `.trash/` (`/admin/trash`, com recuperar, apagar de vez e esvaziar, esses dois exigindo `confirm=true`). A limpeza automática vem **desligada**; o owner liga a regra (dias) em `/admin/trash/policy`, com prévia. Arquivos órfãos do armazenamento (`/admin/storage/orphans`) também vão para a lixeira.
- **Duplicatas (DEC-029):** depois de cada análise, um job `dedupe` sugere pares por ISBN igual, ou título normalizado igual com autor em comum (título sozinho não conta). `GET /admin/duplicates`; dispensar, ou unir escolhendo a obra que fica, com confirmação (a união não se desfaz). `POST /admin/duplicates/scan` compara o acervo todo.
- **OCR (RF-019), só a detecção:** o worker marca as páginas de PDF com menos de 20 caracteres úteis (`text_layers`); o detalhe da obra traz `needsOcr` e `pagesWithoutText`, e `GET /admin/ocr` lista os arquivos. **Rodar o OCR não está feito.**
- **Contração do modelo:** os gatilhos `works_project_*` e as colunas `file_path`, `format`, `language`, `publisher`, `publication_date`, `isbn` e `author_id` de `works` foram removidos (migração `00011`). Toda escrita grava edição, arquivo, local e colaboradores diretamente. O `Down` recria as colunas a partir das tabelas novas.
- **Telas de administração** (`frontend/src/features/admin`, botão "Administração" no cabeçalho, só para owner e admin, via `GET /auth/me`): trabalhos, armazenamento (pastas, varredura, importação de pasta, reorganização, remoções pendentes, órfãos), lixeira com a regra do owner, duplicatas e PDFs sem texto.
- **Importação de pasta pede confirmação sempre:** antes de copiar, a tela pergunta "Apagar os originais depois de copiar?" (manter, apagar ou cancelar) e nunca lembra a resposta. Sem a tela, a API só apaga com `removeOriginals: true`. Isso difere da DEC-033, que faz da remoção o padrão.

**Antes de rodar no seu banco de desenvolvimento, faça um backup.** A migração `00011` apaga colunas de `works` (o que faltar é copiado para as tabelas novas antes, mas é o primeiro uso real). O `pg_dump` **não foi feito nesta sessão** (o banco de desenvolvimento não estava no ar e a ferramenta recusou abrir o volume dos seus dados). Com o Compose no ar, e ajustando usuário e banco se o seu `.env` for diferente:

```bash
docker exec codice_db pg_dump -U codice_user -Fc codice_db > ~/codice-antes-da-00011-$(date +%F).dump
```

Para voltar: `docker exec -i codice_db pg_restore -U codice_user -d codice_db --clean --if-exists < arquivo.dump`. Os arquivos que já existem continuam no caminho plano até você pedir uma reorganização.

**Pendências:**
- Rodar o OCR de fato (a detecção já indica onde) e a fila `ocr`.
- Tela para listar arquivos referenciados e usar "mover para o gerenciado" (a API existe: `POST /admin/library/move-to-managed`).
- Tela para criar e revogar tokens de aplicativo (UI-21); sem ela o OPDS só funciona pela API. O botão de adicionar livros já fica escondido do leitor; o modal de edição ainda não é aberto por nenhuma tela.
- Ensaio básico da aplicação inteira realizado após o merge, inclusive com Redis ausente na inicialização. Permanecem os ensaios de falhas operacionais e os ajustes de interface descritos no relatório de validação.
- Da Fase 1: `rt` e `ticket` aparecem no log de acesso do chi (risco baixo); `POSTGRES_PASSWORD` ainda tem valor padrão em `docker-compose.full.yml`; CORS aceita uma única origem; não há MFA.

**Fase 4 (contas e governança): em andamento**, no branch `feat/fase4-contas`, em fatias pequenas.

- *Fatia 1, entregue:* `GET /users` (owner e admin) lista as contas sem nada secreto e diz, por conta, se quem pede pode bloqueá-la (`canBlock`), então a regra fica só no servidor. `POST /users/{id}/block` e `/unblock` seguem a política de RN-022: o admin só bloqueia leitores; ninguém bloqueia o dono nem a si mesmo; o papel vem do banco. Bloquear (DEC-060) marca a conta, revoga sessões e tokens de aplicativo **na mesma transação** e fecha os WebSockets abertos da conta; nada é apagado e o bloqueio se desfaz, mas o desbloqueio não ressuscita sessões nem tokens antigos. Repetir a ação é inofensivo e a auditoria (`user.block`, `user.unblock`) registra só a mudança. A administração ganhou a aba "Contas", com confirmação ao bloquear.
- *Fatia 2, entregue: convites (DEC-055, DEC-059, RF-048; migração `00012`).* `POST /invitations` cria um link de uso único, válido por 7 dias, com e-mail opcional (então só esse endereço resgata). O admin convida leitores; só o owner convida admins; nenhum convite cria owner (também garantido no banco). O segredo aparece **uma vez**, na criação: o banco guarda só o hash, e nem o log de auditoria nem a listagem o trazem. `GET /invitations` mostra o estado (pendente, usado, expirado, revogado) e `DELETE /invitations/{id}` revoga um pendente (o admin não revoga convite de admin). Público, com limite de requisições: `GET /auth/invitation?token=` e `POST /auth/redeem`; convite desconhecido, usado, expirado ou revogado responde igual (404). O resgate cria a conta e consome o convite na mesma transação, com a linha travada, e mesmo sem a trava só um resgate concorrente vence; se a criação falha (nome já usado, senha curta, e-mail de outro endereço), o convite continua valendo. Bloquear uma conta revoga os convites pendentes que ela emitiu. Na interface: seção "Convites" na aba Contas (o link é mostrado uma vez, com botão de copiar) e a página que o link abre (`/?invite=<segredo>`), que já entra na conta criada. **Decisão minha, a confirmar:** o resgate exige senha de pelo menos 8 caracteres (o setup do dono aceita 6 no navegador e nada no servidor).
- *Fatia 3, entregue: excluir conta e trocar a própria senha.* `DELETE /users/{id}` apaga de vez a conta e os dados pessoais dela (notas, favoritos, progresso, sessões, tokens, identidades ligadas), separado do bloqueio, que é reversível (DEC-060). Exige digitar o nome da conta; segue a mesma política (o admin exclui leitores, ninguém exclui o dono nem a si mesmo). O acervo e a trilha de auditoria ficam; a auditoria guarda o nome, o papel e só **contagens** (nunca o conteúdo das notas), e os convites pendentes que a conta emitiu são revogados. A listagem traz `canRemove`. `POST /auth/password` troca a própria senha: pede a senha atual, exige 8 ou mais caracteres, **encerra todas as outras sessões** (a usada continua; tokens de aplicativo são outra credencial e seguem valendo) e é auditada. Na interface, o avatar virou um menu ("Alterar senha", "Sair") e a aba Contas ganhou "Excluir" com o nome digitado.
- *Criação direta de conta pelo admin (RF-002), adiada de propósito:* o convite já cobre "criar ou convidar" (DEC-050) sem que o admin conheça a senha da pessoa. Criar com senha definida pelo admin só faz sentido junto da redefinição de senha (DEC-063), que precisa de uma credencial temporária que a pessoa é obrigada a trocar; as duas devem nascer juntas. Confirme se prefere assim.
- *Próximas fatias, em ordem sugerida:* redefinição de senha aprovada por owner ou admin, com a criação direta de conta (DEC-063), transferência de titularidade em duas etapas com comando local de recuperação (DEC-057, DEC-058), e por fim o LDAP no mesmo formulário de login (DEC-072 a 075).

## Como rodar

Suba PostgreSQL e Redis (`docker compose up -d`), defina `JWT_SECRET` (a API não sobe sem ele) e use um `.env` local, que nunca deve ser lido nem commitado por ferramentas.

```bash
cd backend && go run ./cmd/api          # aplica as migrações ao subir
cd worker && python -m venv venv && venv/bin/pip install -r requirements.txt && venv/bin/python main.py
cd frontend && npm install && npm run dev
```

O Redis é opcional para os jobs; sem ele o worker consulta o banco. O worker sozinho não organiza nem varre pastas: isso é da API.

## Como rodar os testes

Os testes do backend que usam PostgreSQL só rodam com `TEST_DATABASE_URL`, e recusam qualquer banco cujo nome não termine em `_test`, porque recriam o schema `public` a cada teste. Sem a variável eles são ignorados.

```bash
docker run -d --name codice-test-pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=codice_test -p 127.0.0.1:55432:5432 postgres:16-alpine
export TEST_DATABASE_URL='postgres://postgres:test@127.0.0.1:55432/codice_test?sslmode=disable'
cd backend && go vet ./... && go test ./...
```

O worker: `cd worker && venv/bin/python -m pytest` (82 testes). O frontend: `cd frontend && npm test` (85 testes). `make test` executa as três suítes e agora propaga falhas. Não use o banco de desenvolvimento como `TEST_DATABASE_URL`. Sem essa variável, os testes Go de integração continuam sendo ignorados: sucesso de `make test` sozinho não comprova integração.

Os testes semeiam obras com `testdb.AddWork` (`backend/internal/testdb`), que grava obra, edição, arquivo, local e autor como a aplicação faz.

## Decisões do mantenedor que valem lembrar

- Owner único, papel admin gerido só pelo owner, transferência em duas etapas, recuperação por comando local no servidor.
- Cadastro público desligado; convites de uso único e 7 dias; SMTP opcional; redefinição de senha sem SMTP aprovada por owner ou admin.
- Login por um formulário único: o backend escolhe o provedor pela conta, sem tentar um após o outro. O owner autentica sempre por senha local. O LDAP vem primeiro; o OIDC depois, como botão separado. Se uma entrada do LDAP tem o mesmo nome de uma conta local, ela é vinculada automaticamente só se a pessoa provar as duas senhas (DEC-072 a 075).
- Sem cotas por usuário. Backup diário via comando do Códice, com ferramentas externas a cargo do operador.
- PostgreSQL como fonte da verdade dos jobs; Redis só como entrega.
- Nomes no armazenamento: `Autor/Obra/Idioma — Editora — Ano/Arquivo`; a obra aparece sob cada autor no catálogo.
- Hardware mínimo não é único: será medido por perfil de uso (QA-016).

## Questões em aberto que não bloqueiam a Fase 4

Fronteira da pasta de série (quadrinhos e mangá versus livros), formato da auditoria administrativa, definição de "tempo de leitura" e "conclusão", padrão do papel do antigo owner na recuperação de emergência (proposto: leitor), QA-018 (clientes OPDS), QA-022 (licenças), QA-023 (variantes de EPUB/PDF).

## Ambiente

- O desenvolvimento passa a continuar no macOS, que tem Docker. A máquina Windows onde a Fase 0 foi feita não tinha Docker nem PostgreSQL.
- Este repositório tem `.env` local: nunca commitar nem ler o conteúdo dele em documentos ou logs.
- O script de reset do banco só simula sem `--yes`; não rodar com `--yes` sem decidir conscientemente recriar os dados de teste.
