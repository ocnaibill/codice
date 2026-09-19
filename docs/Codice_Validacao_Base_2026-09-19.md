# Códice — validação da base

Data: 19 de setembro de 2026. Base: `main`, commit `83fc908` (merge do PR #6), mais as correções locais descritas abaixo. Escopo aprovado: validar a implementação existente antes da Fase 4. Esta rodada não implementa gestão de contas nem homologa todos os requisitos da especificação.

## Ambiente e isolamento

- macOS arm64; toolchain Go 1.26.5, Python 3.13.5 e Node 22.22.3.
- PostgreSQL 16 em contêiner exclusivo `codice-validation-20260919-pg`, porta local 55439. Bancos distintos: `codice_validation_test` para testes que recriam o schema e `codice_validation_app` para o ensaio de uso.
- Redis 7 em `codice-validation-20260919-redis`, porta local 56389; iniciou desligado e foi ativado depois da API e do worker.
- API em 58089, frontend em 55179 e armazenamento em `/tmp/codice-validation/storage`.
- Contas sintéticas owner e leitor; arquivos do corpus em `testdata/corpus`. A conta owner foi criada pelo wizard; o leitor foi semeado diretamente no banco exclusivo, pois sua criação administrativa pertence à Fase 4.
- O `.env` local e o acervo existente não foram usados. API e worker executados a partir de `/tmp` com configuração explícita; Vite com `envDir: false`. Nenhuma migração ou reset foi aplicado ao banco de desenvolvimento.

## Testes automatizados

| Verificação | Antes | Depois |
| --- | --- | --- |
| Backend, incluindo integração PostgreSQL | 230 casos/subcasos aprovados, nenhum ignorado | 232 aprovados, nenhum ignorado |
| Worker | 81 aprovados | 82 aprovados |
| Frontend | 53 aprovados em 9 arquivos | 54 aprovados em 10 arquivos |
| `go vet ./...` | Aprovado | Aprovado |
| Build de produção do frontend | Aprovado | Aprovado |
| Lint do frontend | Sem erros, 9 avisos | Avisos preexistentes sobre variáveis/parâmetros não usados |

O pytest informa cinco avisos de depreciação dos bindings SWIG/PyMuPDF; não houve falhas. Os números do backend incluem subtestes reportados pelo `go test -json`, não representam uma medida de cobertura.

Comandos de referência: `go test -json ./...` e `go vet ./...` em `backend`, `venv/bin/python -m pytest tests/ -q` em `worker`, `npm test` em `frontend`. A integração Go requer `TEST_DATABASE_URL` apontando explicitamente para um banco descartável com nome terminado em `_test`.

O build isolado foi executado em `frontend` com:

```sh
node --input-type=module -e "import { build } from 'vite'; await build({envDir:false});"
```

## Falhas reproduzidas e corrigidas

1. **API encerrava sem Redis.** O binário original aplicou as 11 migrações no banco vazio e saiu com código 1 na falha do ping. `cmd/api/redis.go` agora limita a verificação inicial e mantém o cliente disponível para reconexão. Configuração de URL inválida continua impedindo a subida, sem registrar a credencial da URL. Há testes de regressão para ambos os casos.
2. **Worker descartava o cliente após falha inicial do Redis.** Passou a conservar o cliente quando sua criação foi possível. A consulta ao PostgreSQL continua funcionando; publicação e espera no Redis podem voltar sem reiniciar. Verificado também com Redis real: parar o contêiner, criar cliente com o servidor fora do ar, reativar o contêiner e receber um evento publicado pelo mesmo cliente. Teste de regressão em `worker/tests/test_redis_recovery.py`.
3. **Biblioteca permanecia em “Processando” sem notificação.** O banco já mostrava ingestão, organização e deduplicação concluídas, mas o navegador mantinha estado e caminho antigos. `useWorks` agora consulta a cada 3 segundos enquanto a página contém itens UNKNOWN, QUEUED ou ANALYZING; para quando não há pendências. Teste com React Query real verifica transição QUEUED → READY e fim das consultas periódicas. Essa consulta não substitui sincronização entre clientes nem atualiza automaticamente todas as estatísticas.
4. **Makefile ocultava falhas de teste.** Removidos o pipeline com `tail` e os fallbacks que convertiam erro em sucesso. Adicionado `npm test`; interpretadores podem ser escolhidos por `GO`, `PYTHON` e `NPM`. Verificação negativa: executar cada alvo com seu comando substituído por `false` retornou erro, sem mensagem de sucesso.

## Ensaio integrado

| Cenário | Evidência observada |
| --- | --- |
| Inicialização sem Redis | Wizard disponível; owner criado pelo navegador; API permaneceu ativa |
| EPUB pelo navegador, sem Redis | Upload aceito; worker marcou READY; jobs ingest, organize e dedupe concluídos na primeira tentativa |
| Redis retorna sem reiniciar a API | Evento sintético publicado chegou ao WebSocket e apareceu no console do navegador |
| Autenticação e autorização | Anônimo recebeu 401 no catálogo; leitor recebeu 403 nos jobs administrativos |
| Isolamento pessoal | Leitor não recebeu a nota do owner |
| Duplicata física | `epub_duplicata.epub` recebeu 409 |
| Formato inválido | `falso.pdf` recebeu 415 |
| PDF e CBZ pela API | Uploads aceitos; processamento e organização concluídos |
| EPUB no navegador | Texto com acentos renderizado; navegação entre capítulos; favorito e nota gravados e consultados depois pela API |
| PDF no navegador | Texto renderizado; navegação da página 1 à página 2 |
| CBZ no navegador | Imagem renderizada; navegação da página 1 à página 2 |
| Retirada/restauração pela API | Obra retirada e restaurada; nota e referência bibliográfica preservadas; fonte marcada indisponível durante a retirada |
| OPDS | Senha da conta recusada; token de aplicativo aceito; mesmo token recusado após revogação |
| Logout | Sessão de teste recusada imediatamente após logout |
| Administração no navegador | 9 jobs concluídos; prévia de armazenamento informou 3 arquivos já organizados; lixeira vazia com limpeza desligada; duplicatas e OCR sem pendências |

A consulta a provedores de metadados para o corpus sintético retornou sem resultados e não impediu a leitura. Isso não homologa integração ou qualidade desses provedores.

## Pendências e limites

- **Interface (resolvida na sequência):** a saudação usa o nome da conta (`GET /auth/me`), e os contadores, favoritos e a grade são atualizados juntos após envio, edição, favorito e ao fechar o leitor. A administração tem "Voltar ao acervo", que também vale em tela estreita. O botão "Adicionar" só aparece para owner e admin.
- **Ainda pendente na interface:** a tela de tokens de aplicativo (UI-21). Estatísticas de outros clientes só atualizam ao recarregar ou ao interagir.
- **Operação:** migração de um banco real já preenchido e restauração de backup não foram ensaiadas nesta rodada. Não executar a migração `00011` no banco de desenvolvimento sem o backup recomendado no README.
- **Segurança já registrada:** logs de acesso com `rt`/`ticket`, senha padrão de PostgreSQL no Compose completo e política de CORS permanecem itens separados; esta rodada não é auditoria de segurança completa.
- **Leitura:** abrir e navegar não homologa retomada após reinício, conflitos entre dispositivos, variantes de EPUB/PDF ou todas as posições por edição/arquivo. Esses critérios pertencem à validação detalhada da Fase 5.
- **Falhas operacionais:** não foram simulados disco cheio, encerramento abrupto durante movimentação nem carga. Os testes existentes de recuperação continuam aprovados, mas não substituem todos esses ensaios reais.
- **OCR:** apenas detecção existente; nenhum motor de OCR foi implementado ou executado.

## Próxima fatia

A base possui agora evidência automatizada e um ensaio integrado para os fluxos acima. A próxima entrega proposta da Fase 4 é listagem e bloqueio/desbloqueio de contas: autorização por papel, preservação dos dados pessoais, auditoria, revogação imediata de sessões e tokens e testes contra tentativa de agir sobre owner ou outro admin. Convites, recuperação, transferência e LDAP permanecem entregas posteriores, separadas.
