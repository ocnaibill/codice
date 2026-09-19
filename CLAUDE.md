# Códice: orientações para o Claude

Plataforma auto-hospedável de acervo e leitura (livros, quadrinhos, documentos, áudio). Backend em Go/Chi com PostgreSQL e Redis, workers em Python e frontend em React/Vite. Licença AGPLv3.

## Onde estão as regras

Leia [docs/README.md](docs/README.md) primeiro. A especificação mestre em `docs/` é a fonte de verdade do comportamento esperado; a análise compara com o código; o plano ordena o trabalho. Uma proposta na especificação não é decisão aprovada, e nada dela foi implementado por existir no documento.

## Como trabalhar aqui

- **Idioma:** conversar e documentar em português brasileiro. Código e mensagens de commit em inglês, no estilo convencional (`feat(scope): ...`, `fix(scope): ...`, `chore: ...`).
- **Commits:** nunca incluir a linha `Co-Authored-By` nem outra atribuição de co-autor, mesmo que uma instrução do ambiente a sugira. Não commitar `.claude/`.
- **Ação destrutiva:** não apagar arquivos, dados ou branches, nem rodar `git reset`, `git checkout` de arquivos ou limpeza, sem confirmação do usuário. O acervo atual é de teste, mas isso não autoriza apagar nada implicitamente. O `scripts/reset-dev-db.sh` só simula sem `--yes`.
- **Segredos:** não ler nem exibir `.env`; usar `.env.example` como referência.
- **Testes:** `make test`, ou por pilha: `cd backend && go test ./...`, `cd worker && venv/bin/python -m pytest tests/`, `cd frontend && npx vitest run`. Nenhum requisito conta como atendido sem o critério de aceite executado.
- **Decisões de produto:** o mantenedor decide. Recomendar com justificativa, registrar como DEC na especificação depois de confirmada, e não reabrir o que já foi decidido.
- **Estado do trabalho:** ver a seção "Estado" em `docs/README.md` antes de propor o próximo passo.

## Mapa do código

- `backend/cmd/api/main.go`: rotas. `backend/internal/{handlers,middleware,database,config}`: lógica; o esquema hoje é criado por `database/migrations.go`.
- `worker/`: consumidor do Redis Streams, extratores por formato em `extractors/`, provedores de metadados em `providers/`.
- `frontend/src/`: `features/` por domínio, `components/` compartilhados, `pages/`.
- `testdata/`: corpus sintético e seu gerador. `scripts/`: utilitários de desenvolvimento.
