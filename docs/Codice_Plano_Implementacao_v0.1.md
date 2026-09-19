# Códice
## Plano de implementação

**Versão:** 0.1 • **Data:** 19 de setembro de 2026 • **Idioma:** português brasileiro
**Base:** Especificação mestre v0.4 (DEC-001 a DEC-071) e análise inicial do backend
**Estado:** proposta para revisão. Nada aqui foi implementado, e o repositório não foi alterado.

Este plano ordena o trabalho pelo risco e pelas dependências que a análise apontou. Ele reaproveita a base existente (Go/Chi, PostgreSQL, Redis Streams, workers Python, React) e não propõe reescrita. As referências RF, RN, DEC e QA remetem à especificação. Estimativas de tamanho (P, M, G) são palpites relativos, sem prazo.

## 1 Princípios

1. **Segurança antes de funcionalidade.** Enquanto a autenticação Basic aceitar qualquer senha, a instância não é segura para mais de um usuário.
2. **Modelo de dados antes de ingestão, leitura e contas.** Quase tudo depende de Work/Edition/File, papéis e jobs.
3. **Fatias verticais pequenas, cada uma com testes.** Nenhum requisito é dado como atendido sem o critério de aceite executado (§25.2 da especificação).
4. **Nada de apagar ou reiniciar dados sem ação explícita.** O acervo é de teste (DEC-020), mas o reset do banco é um passo separado, pedido e confirmado por você.
5. **O frontend em andamento é preservado.** Ele se adapta a cada fase, sem descarte.

## 2 Fase 0: preparar o terreno

Tamanho: P. Sem decisões de produto pendentes.

- Organizar o trabalho local não commitado (14 arquivos modificados e vários novos, entre eles os handlers `favorites.go`, `notes.go` e `stats.go` e as páginas do frontend). Sugestão: criar um branch e commitar por tema, sem `reset`, `checkout` de arquivos nem limpeza. Você decide como agrupar os commits.
- Executar os testes existentes (`auth_test.go`, `pages_test.go`, `worker/tests`) e registrar o estado de partida. A análise foi estática e nada foi executado ainda.
- Montar o conjunto de arquivos de teste da §23.1: EPUB refluível com acentos, PDF digital, PDF escaneado, CBZ, arquivo corrompido, duplicata e arquivo alterado. Usar só material com origem permitida.
- Definir como o banco de testes é recriado (script de reset explícito, nunca automático).
- Verificar o que o código já faz e a análise não cobriu: hash de senha, forma do JWT e rotas de administração de usuários. **Feito em 19 de setembro de 2026**: resultados na seção 9 da análise do backend; geraram novas tarefas na Fase 1.
- **Estado da Fase 0 em 19 de setembro de 2026:** branch `chore/salvar-trabalho-local` criado com o trabalho local commitado; baseline registrado; corpus sintético gerado em `testdata/` (14 arquivos, determinístico); script `scripts/reset-dev-db.sh` escrito (simulação por padrão, backup antes, confirmação digitada, recusa em produção; nunca apaga arquivos). **Fase 0 concluída** na parte técnica; falta você decidir se o banco de desenvolvimento será recriado e onde ele roda, já que esta máquina não tem Docker nem PostgreSQL.

Saída: baseline de testes registrado, branch de trabalho limpo e corpus de teste montado.

## 3 Fase 1: segurança e autorização

Tamanho: M. Cobre os achados 4.1, 4.2 e 4.8, além de RN-015 e RN-022.

**Tarefas**
- Remover o ramo do middleware que aceita qualquer cabeçalho `Basic` (`backend/internal/middleware/auth.go`) e validar a senha no OPDS (`backend/internal/handlers/opds.go`).
- Introduzir papéis explícitos: owner, admin e leitor. Um índice único parcial no banco garante que só exista um owner (DEC-022).
- Middleware de autorização por ação. Upload, importação em lote, edição e exclusão do catálogo exigem admin ou owner.
- Regras de admin na API (DEC-056, RN-022): admin não promove, rebaixa, bloqueia nem remove outro admin, e ninguém altera o owner.
- Setup (`auth.go`): criar o owner dentro de uma transação, com bloqueio ou restrição de unicidade, para que duas requisições concorrentes não criem dois donos (RF-001).
- Importação em lote: aceitar só diretórios dentro de raízes autorizadas em configuração, ainda sem a interface do owner.
- **Achados da verificação de código (análise, §9):** remover o fallback que concede admin sem autenticação (também no WebSocket) e o segredo JWT padrão; ler o papel do banco a cada requisição, por meio das sessões de DEC-070, em vez de confiar no token de 7 dias; retirar o token da query string, usando tokens curtos e específicos para capas e arquivos; mensagem única de falha no login; não publicar as portas do PostgreSQL e do Redis no host (ou vincular a 127.0.0.1) e exigir senha no Redis; revisar o custo do bcrypt.

**Testes:** requisição sem credencial recebe 401 em qualquer `APP_ENV`; token de conta bloqueada ou de sessão revogada é recusado na hora; leitor recebe recusa em cada rota administrativa; senha errada no OPDS é recusada; dois setups concorrentes criam uma única conta owner; admin recebe recusa ao tentar promover outro admin.

**Decisões confirmadas em 19 de setembro de 2026:** sessões são registros no banco, revogáveis de imediato (DEC-070), e clientes OPDS usam tokens de aplicativo revogáveis, não a senha da conta (DEC-071). A Fase 1 inclui as tabelas de sessão e de tokens e a revogação por bloqueio ou redefinição de senha.

## 4 Fase 2: modelo de dados alvo

Tamanho: G. É o núcleo estrutural. Cobre os achados 4.3, 4.4 e 4.5 e as decisões DEC-013 a DEC-017, 023-025, 036, 038-041.

**Tarefas**
- Adotar migrações versionadas (ferramenta a escolher, como goose ou golang-migrate) no lugar da rotina atual em `migrations.go` (RNF-014).
- Esquema alvo: Work, Edition, File, StorageLocation, Contributor com vínculo N para N e papéis, Series com posição, Category em hierarquia sem ciclos, Tag global e pessoal, Collection privada, Progress por usuário e arquivo com Locator versionado, Note com referência bibliográfica persistente e sem exclusão em cascata, Favorite, AuditLog, Job, Invite, ResetRequest e Session.
- Restrições que garantem invariantes: um owner, hash único de arquivo, nota sobrevive à retirada da obra (RN-019), progresso pertence ao par usuário e arquivo.
- Migração de teste: como o acervo é de teste, a migração cria o esquema novo e você decide, no momento, se os dados atuais são descartados ou reimportados. Não fundir obras apenas por título.
- Adaptar as rotas existentes (biblioteca, notas, favoritos, estatísticas) ao esquema novo, mantendo os contratos que o frontend usa ou versionando-os.

**Testes:** obra com duas edições e três arquivos navegável sem duplicar a obra (RF-005); nota permanece consultável depois de retirar a obra (RF-039); progresso independente por arquivo; leitor não vê notas de outro.

**Risco principal:** o frontend depende dos contratos atuais. Mitigar mantendo os endpoints existentes como camada de compatibilidade até cada tela ser migrada.

## 5 Fase 3: jobs, ingestão e armazenamento

Tamanho: G. Cobre os achados 4.5, 4.6 e 4.7, RF-007 a 011, 019/020, 041, 044 e 045, e DEC-026 a 043, 065 a 069. Divide-se em duas partes.

**3a. Infraestrutura de jobs**
- Tabela de jobs no PostgreSQL como fonte da verdade, despachante para o Redis Streams e reconciliador (DEC-066, DEC-067).
- Worker com nome de consumidor único, lease e heartbeat, reivindicação de pendências, até 3 tentativas com espera crescente, erros permanentes sem repetição e reexecução manual (DEC-068).
- Prioridade para importações manuais e um job pesado por vez (DEC-069).
- Página de administração de jobs (RF-020).
- Testes de recuperação: reiniciar o worker, esvaziar o Redis, subir com Redis fora do ar e verificar que nada se perde nem duplica.

**3b. Ingestão e armazenamento**
- Upload com staging, nome único, hash integral, limite real de tamanho e validação de conteúdo, não só de extensão (RF-008). Hash igual leva ao registro existente, sem nova cópia (DEC-027).
- Armazenamento gerenciado no padrão de nomes de DEC-065, com sanitização, limite de caminho e desempate por ID.
- Modo referenciado com raízes configuradas pelo owner, sem alterar os arquivos (DEC-032, DEC-035).
- Fluxo "Mover para o armazenamento gerenciado" com verificação antes de remover a origem (DEC-033, RF-011).
- Retirada e restauração de obras, lixeira recuperável e limpeza opcional (DEC-038, 041 a 043).
- Metadados nativos com proveniência; provedores externos geram candidatos para aprovação (DEC-026, RF-009). Os locks de metadados existentes em `worker/analyzer.py` são preservados.
- Candidatos a duplicidade por título, autor ou ISBN vão para revisão administrativa (DEC-029).

**Testes:** arquivo corrompido é bloqueado; duplicata devolve o registro existente; falha no meio da transferência preserva uma cópia válida; retirar obra preserva arquivos e notas.

## 6 Fase 4: contas e governança

Tamanho: M a G. Cobre RF-002 a 004, 038, 047 a 049 e DEC-050 a 063. Depende das fases 1 e 2.

- Convites de uso único, 7 dias, revogáveis, com e-mail opcional (DEC-055, DEC-059, RF-048).
- Gestão de contas: criar, bloquear (revogação imediata) e excluir separadamente (DEC-060). O papel admin só muda por ação do owner.
- Transferência de titularidade em duas etapas e comando local de recuperação (DEC-057, DEC-058, RF-038).
- Pedidos de redefinição de senha aprovados por owner ou admin, sem SMTP (DEC-063, RF-049).
- Auditoria administrativa (formato ainda a definir, ver §9).
- **OIDC e LDAP:** depois do login local, como a especificação permite (§23). Inclui criação no primeiro login por provedor, vínculo assistido por e-mail coincidente e revalidação com teto de 24 horas (DEC-051, 053, 061).

**Testes:** convite expirado, usado ou revogado não cria conta; resgate concorrente cria uma só; conta bloqueada perde sessões e tokens na hora; comando de recuperação sem acesso local não tem efeito.

## 7 Fase 5: leitura, progresso e notas

Tamanho: M a G. Cobre RF-012 a 017, 041 a 043 e DEC-028 a 031.

- Ficha da obra com escolha de edição, idioma e arquivo antes de abrir o leitor (DEC-028).
- Locator versionado por formato (EPUB, PDF, CBZ) e progresso por arquivo, com retomada equivalente adiada (DEC-030).
- Notas privadas com referência bibliográfica que acompanha correções confirmadas e sobrevive à retirada da obra (DEC-039, DEC-040).
- Exportação das próprias notas (FL-09).
- Formalizar o significado de tempo de leitura e conclusão antes de consolidar as estatísticas (análise, §6).

**Testes:** troca de idioma ou formato retoma a posição própria do destino; nota exporta com a referência mesmo sem a fonte; dois usuários não enxergam o estado um do outro.

## 8 Fase 6: operação

Tamanho: M. Cobre RF-021, 034, RNF-010, 014 e DEC-064.

- Comando de backup do banco e da lista de arquivos, com verificação na restauração e criptografia opcional (DEC-064).
- Ensaio de restauração em instância limpa, medindo o tempo real (base do RTO), conforme FL-11.
- Saúde por componente, com falha isolada do Redis e de serviços opcionais (RF-021).
- Docker Compose com volumes persistentes e a ordem de inicialização de §10.4 da especificação.
- Documentação de implantação, incluindo o limite de privacidade (o operador do servidor acessa banco e discos) e o aviso de que o backup precisa ser agendado por quem opera a instância.

## 9 Depois do núcleo

Sem detalhamento agora, na ordem sugerida pela especificação: busca textual e OCR configurável (E2), OPDS e sincronização com matriz de clientes homologados e exportação PKM (E3), grafo, trilhas e embeddings (E4), e geração, RAG, áudio e novos formatos (E5). Perfis de hardware serão medidos nessa etapa (QA-016). Sobre IA local ou externa, já está decidido: local por padrão, com serviços externos só mediante autorização do owner por área, e IA sem influência no funcionamento do núcleo (DEC-044 a 047).

## 10 Decisões e verificações que ainda precisam de você

| Quando | Item | Proposta |
| --- | --- | --- |
| Fase 0 | Como agrupar os commits do trabalho local e em qual branch | Um branch por tema; você revisa antes do merge |
| Fase 1 | Sessões revogáveis e autenticação OPDS | Resolvido: DEC-070 e DEC-071 |
| Fase 2 | Ferramenta de migração | goose ou golang-migrate |
| Fase 2 | Destino dos dados de teste | Reset explícito, confirmado por você |
| Fase 3 | Fronteira da pasta de série (quadrinhos e mangá versus livros) | Como na especificação, §12.1 |
| Fase 4 | Formato da auditoria administrativa | Tabela de eventos com ator, ação, alvo e data, sem segredos |
| Fase 5 | Significado de tempo de leitura e conclusão | Definir antes de consolidar as estatísticas |

## 11 Riscos do plano

- **A análise foi estática.** Rotas e comportamentos podem ter detalhes que só a Fase 0 revela. O plano pode mudar depois dela.
- **A Fase 2 é grande e mexe em quase tudo.** Fatiar por entidade e manter camada de compatibilidade para o frontend.
- **Trabalho local não commitado.** Qualquer operação que sobrescreva arquivos precisa de sua confirmação antes.
- **Sem cobertura de testes de partida no backend além de auth e pages.** Cada fase cria os testes dos seus critérios de aceite.
- **Escopo de hobby.** As fases são reordenáveis; o plano só impõe as dependências (1 antes de 2; 2 antes de 3, 4 e 5).
