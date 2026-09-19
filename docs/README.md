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

**Fase 0 (preparar o terreno): concluída na parte técnica.**
- Trabalho local salvo em commits no branch `chore/salvar-trabalho-local` (favoritos, notas, estatísticas, novo layout, telas de login).
- Baseline: `go build`, `go vet` e `go test` passam; 5 testes do frontend e 22 do worker passam; o frontend compila. Esses testes **não** cobrem autorização nem OPDS.
- Corpus sintético em `testdata/` (14 arquivos, gerados de forma determinística por `testdata/generate_corpus.py`).
- Script `scripts/reset-dev-db.sh` (simulação por padrão; ver o cabeçalho do arquivo).

**Próximo passo: Fase 1, segurança e autorização.** Começar escrevendo os testes que hoje falham, sem precisar de banco (mocks):
1. Requisição sem credencial recebe 401 em qualquer `APP_ENV` (hoje, fora de produção, vira admin).
2. Senha errada no OPDS é recusada (hoje só se confere se o usuário existe) e um cabeçalho `Basic` qualquer não autentica no middleware comum.
3. Leitor é recusado nas rotas de upload, importação em lote, edição e exclusão.
4. Dois setups concorrentes criam uma única conta owner.
5. Admin não promove, rebaixa, bloqueia nem remove outro admin (regra RN-022).

Depois: papéis owner/admin/leitor, sessões no banco (DEC-070), tokens de aplicativo para OPDS (DEC-071), remoção do fallback anônimo e do segredo JWT padrão, e as demais tarefas da Fase 1 do plano.

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

- O desenvolvimento passa a continuar no macOS. A máquina Windows onde a Fase 0 foi feita não tinha Docker nem PostgreSQL.
- Este repositório tem `.env` local: nunca commitar nem ler o conteúdo dele em documentos ou logs.
- O script de reset do banco só simula sem `--yes`; não rodar com `--yes` sem decidir conscientemente recriar os dados de teste.
