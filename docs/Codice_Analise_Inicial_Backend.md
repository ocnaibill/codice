# Códice
## Análise inicial do backend e do estado local

Data: 18 de setembro de 2026. Referência da inspeção: especificação mestre v0.2. Decisões posteriores consolidadas na v0.4 e na seção 8 deste relatório. Repositório: `C:/Users/ocnaibill/repos/codice`. HEAD observado: `f5ee384eb396517222206fbe8ed2b75bb81935ce`.

Esta é uma análise estática inicial do estado de trabalho, incluindo alterações sem commit. Não foram executados serviços, migrações ou testes. A presença de uma implementação não significa aprovação dos critérios de aceite. O repositório não foi modificado; arquivos locais de configuração com segredos não foram lidos.

## 1 Estado do Git

Não foram encontradas alterações em stage. Há 14 arquivos rastreados modificados ou removidos, além de arquivos e diretórios novos. As mudanças não se restringem ao frontend.

No backend, `cmd/api/main.go`, `internal/database/migrations.go` e `internal/handlers/library.go` estão modificados. Os handlers `favorites.go`, `notes.go` e `stats.go` são novos e não rastreados. Eles sustentam favoritos, notas/citações, estatísticas e tempo de leitura usados pela interface em desenvolvimento.

No frontend há alterações de autenticação, layout, leitores e consultas, remoção de Navbar e BookGrid antigos e novos componentes, páginas e hooks. Também existem `.claude/`, `Users/` e arquivos de uploads não rastreados; sua finalidade não foi presumida nem seu conteúdo incorporado à especificação.

Portanto, qualquer próxima revisão deve considerar o checkout completo e distinguir HEAD de mudanças locais. Fazer checkout, reset, limpeza de arquivos ou migração automática agora poderia interferir nesse trabalho.

## 2 Arquitetura observada

O backend já usa Go com Chi, PostgreSQL e Redis. O worker é Python, com extratores por formato e provedores de metadados. A ingestão usa Redis Streams; eventos de atualização seguem Redis PubSub e WebSocket. O frontend é React.

Esse arranjo já se aproxima da proposta de API modular com workers separados. PostgreSQL e Redis não são decisões a tomar do zero: são escolhas existentes a avaliar e preservar quando adequadas. A proposta de pgvector continua futura; não foi localizada integração vetorial no backend e worker inspecionados.

A licença presente em LICENSE é GNU AGPL versão 3. Isso resolve a dúvida sobre qual licença está atualmente registrada no repositório; não substitui a avaliação das licenças de dependências e modelos.

## 3 Componentes que merecem ser preservados

- Extratores separados por formato e registro de provedores bibliográficos: base útil para evolução sem reescrever o pipeline inteiro.
- Campos de bloqueio de título, autor, série e capa em `worker/analyzer.py`: já existe intenção de preservar edição humana durante reprocessamento.
- Progresso, notas e favoritos com `user_id`: direção correta para dados pessoais, embora autenticação e modelo de posição ainda precisem de ajustes.
- Estado de mídia e notificações de processamento: base para tornar falhas e etapas visíveis.
- Rotas de catálogo OPDS, entrega de mídia e páginas: implementação concreta para homologação, sem assumir compatibilidade integral apenas pelo README.
- Suporte de código a formatos além de PDF/EPUB/CBZ: manter como capacidade existente em avaliação; não remover para adequar artificialmente ao escopo inicial.

## 4 Achados prioritários

### 4.1 Autenticação Basic aceita sem validar senha

Em `backend/internal/middleware/auth.go:60`, qualquer cabeçalho que começa com `Basic ` passa adiante com uma identidade fixa, sem validação das credenciais. Esse ramo também existe em produção. Ele afeta as rotas protegidas pelo middleware comum, incluindo arquivos e operações do catálogo.

Em `backend/internal/handlers/opds.go:29`, a autenticação OPDS decodifica usuário e senha, mas verifica apenas a existência do usuário; a senha não é comparada com o hash. São dois caminhos distintos que precisam de correção. A presença de login JWT correto em outro fluxo não protege esses caminhos.

Prioridade: corrigir antes de considerar a instância segura para acesso multiusuário. A constatação vem da leitura direta do código; não foi realizado teste contra a instância.

### 4.2 Autenticação não impõe a permissão de administrador

As rotas de upload, importação em lote, alteração e exclusão de obras usam AuthMiddleware em `backend/cmd/api/main.go`, mas os handlers correspondentes não verificam papel de administrador. Um leitor autenticado pode alcançar operações que a especificação reserva à administração. A importação em lote também recebe um diretório do servidor sem uma lista explícita de raízes permitidas no handler inspecionado.

Isso diverge de DEC-003 e RF-007. Separar autenticação de autorização por ação é requisito anterior à expansão funcional.

### 4.3 Obra ainda funciona como unidade de arquivo

Em `backend/internal/database/migrations.go:23`, `works` contém `file_path`. Em `:29`, `editions.work_id` é UNIQUE, permitindo no máximo uma edição por obra. Não há entidade File nesse esquema. Vários metadados editoriais também ficam em works.

Isso impede representar diretamente uma obra com múltiplas edições e arquivos como previsto em RF-005. O modelo alvo completo prevalece sobre o esquema atual. Após a inspeção, o mantenedor confirmou que os dados são de teste: uma migração conservadora do acervo não é obrigatória. Reinicialização de dados, se escolhida, deve ser explícita; não fundir registros automaticamente apenas por título.

### 4.4 Progresso pessoal ainda pertence à obra

Em `backend/internal/database/migrations.go:66`, a chave é `(user_id, work_id)`. O progresso é uma string, e as mudanças locais adicionam porcentagem, conclusão e segundos de leitura. Não há localização tipada/versionada por arquivo nem revisão de concorrência nesse contrato.

Se os dados de teste forem reaproveitados, interpretar cada valor conforme o leitor que o produziu; sua preservação não é requisito confirmado. Não converter formatos por porcentagem sem alinhamento. RF-015 está parcialmente encaminhado, mas ainda diverge do modelo pretendido.

### 4.5 Exclusão do catálogo também tenta apagar arquivos

`backend/internal/handlers/library.go:544` remove registros e, após commit, chama `os.Remove` para arquivo e capa em `:606` e `:611`. A API não separa nessa operação a remoção do catálogo da exclusão física. Relacionamentos com exclusão em cascata também afetam notas e progresso.

Isso diverge de RN-004 e requer uma política explícita antes de introduzir arquivos compartilhados entre edições, biblioteca referenciada ou lixeira.

### 4.6 Jobs pendentes não possuem recuperação visível

Em `worker/main.py:93`, o worker lê o consumer group com `streams={STREAM_NAME: '>'}`. Exceções genéricas deixam mensagens sem confirmação; não foi localizado reclaim/reprocessamento de pendências nesse loop. Reiniciar o processo não equivale a recuperar essas mensagens. O nome do consumidor também é fixo.

Manter Redis Streams é compatível com a especificação, mas exige política de pendências, identidade de consumidor, repetição limitada, idempotência e publicação consistente. Não é necessário trocar o broker para resolver essas lacunas.

### 4.7 Upload não publica arquivo e fila de forma consistente

Em `backend/internal/handlers/upload.go`, a operação grava arquivo, insere obra e depois publica no Redis. Se a publicação falha, remove o arquivo sem desfazer o registro inserido. O nome combina segundos do relógio com nome original e usa `os.Create`, permitindo colisão para uploads com mesmo nome no mesmo segundo.

A validação observada usa extensão; não há hash integral e deduplicação nessa rotina. O comentário sobre limite de 50 MB acompanha ParseMultipartForm, mas esse parâmetro controla memória do multipart e não implementa, por si só, limite total do corpo. Esses pontos precisam de testes direcionados de falha, concorrência e tamanho antes de declarar RF-008 atendido.

### 4.8 Inicialização administrativa não é atômica

`backend/internal/handlers/auth.go:148` verifica quantidade de usuários e depois insere o administrador, sem transação ou bloqueio entre as operações. Duas requisições concorrentes com nomes distintos podem atravessar a verificação. Isso não atende ao critério de inicialização única de RF-001.

## 5 Matriz preliminar por requisito funcional

“Não localizado” limita-se ao código e às rotas inspecionados. Nenhum item recebe aprovação funcional completa sem execução dos critérios de aceite.

| RF | Avaliação inicial | Evidência ou lacuna |
| --- | --- | --- |
| 001 | Parcial | SetupMasterAdmin; concorrência não protegida |
| 002 | Parcial | Papéis e registro de leitores; gestão administrativa de usuários não localizada nas rotas |
| 003 | Parcial | Login local; OIDC/LDAP não localizados; Basic possui falhas |
| 004 | Não localizado | Vínculo de identidades e revogação de sessões não presentes nas rotas examinadas |
| 005 | Divergente | Uma edição por obra e caminho de arquivo em works |
| 006 | Parcial | Consulta, filtros, paginação e ficha presentes; sem múltiplas edições reais |
| 007 | Divergente | Upload e lote presentes sem exigência de administrador |
| 008 | Parcial | Lista de extensões; faltam validação de conteúdo e hash/deduplicação no upload |
| 009 | Parcial | Edição e locks; revisão de quarentena e candidatos não localizada |
| 010 | Parcial | Upload/lote copiam arquivos; modo referenciado não localizado |
| 011 | Não localizado | Adoção e reconciliação de fontes não identificadas |
| 012 | Não avaliado funcionalmente | Rotas de mídia e leitores existem; corpus não executado |
| 013 | Não avaliado funcionalmente | Mudanças locais nos viewers; comportamento completo não testado |
| 014 | Parcial | Persistência de posição; falta Locator versionado verificável |
| 015 | Divergente | Progresso por usuário e obra, sem separação por arquivo |
| 016 | Parcial | Notas pessoais novas; somente quote, sem Locator nem fluxo completo de edição |
| 017 | Não avaliado | Contrato backend de notas não demonstra Markdown estruturado, Wikilinks ou fórmulas |
| 018 | Parcial | library.go usa LIKE em título/autor; pesquisa integral e de marginalia não localizada |
| 019 | Parcial | Pipeline de extração de metadados; OCR/segmentação pesquisável não localizados |
| 020 | Parcial | Estados e eventos; administração de jobs e recuperação não completas |
| 021 | Parcial | Health e estatísticas locais; diagnóstico operacional integrado não demonstrado |
| 022 | Não localizado | Interruptor global e por área não encontrados no backend/worker |
| 023 | Parcial | Registro de provedores bibliográficos; provedores de modelos e perfis ainda não identificados |
| 024 | Não localizado | Embeddings e índice vetorial não encontrados |
| 025 | Divergente do fluxo proposto | Enriquecimento automático existe; candidato separado e aceite humano não identificados |
| 026 | Não localizado | Grafo explícito não identificado |
| 027 | Não localizado | Backend de trilhas/comparação não identificado |
| 028 | Não localizado | RAG e sínteses não identificados |
| 029 | Não localizado | Exportação PKM não identificada nas rotas |
| 030 | Parcial | Catálogo OPDS existe; autenticação e interoperabilidade exigem validação |
| 031 | Parcial | Atualização interna de progresso; adaptadores e conflitos não identificados |
| 032 | Parcial | Open Library, Google Books e ComicVine no pipeline; proveniência por campo incompleta |
| 033 | Não localizado | Telemetria ZFS/SMART não identificada no backend examinado |
| 034 | Não avaliado | Backup e restauração não executados; implantação merece revisão própria |

## 6 Ajustes que o código sugere para o documento mestre

Registrar uma seção de arquitetura existente: Go/Chi, PostgreSQL, Redis Streams/PubSub, Python e React. Manter separada da arquitetura alvo e das decisões aprovadas pelo mantenedor.

Atualizar QA-022 com a licença AGPLv3 existente, mantendo em aberto a política de modelos e dependências. Em QA-025, registrar Redis Streams como broker atual e avaliar suas lacunas antes de considerar substituição.

Adicionar favoritos e estatísticas pessoais como capacidades existentes em desenvolvimento, sujeitas à decisão de produto. A especificação não precisa descartá-las por não constarem da conversa inicial. Formalizar o significado de tempo de leitura e conclusão antes de consolidar esses indicadores.

Documentar locks de metadados como mecanismo já existente a preservar, evoluindo para proveniência por campo e candidatos revisáveis. Atualizar o inventário de formatos para distinguir aceitação de extensão, extração, leitura e homologação.

A especificação v0.2 permanece preservada como histórico. A v0.3 incorpora as decisões posteriores do mantenedor e identifica a base tecnológica existente, sem aprovar automaticamente o comportamento atual.

## 7 Sequência recomendada

1. Consolidar este inventário com uma revisão dos testes e da implantação, sem alterar dados reais.
2. Tratar autenticação Basic, autorização administrativa e inicialização concorrente como primeira frente de correção.
3. Implementar Work, Edition, File e Locator conforme o modelo alvo, incluindo bibliotecas compartilhadas e coleções privadas; definir explicitamente o destino dos dados de teste.
4. Corrigir publicação da ingestão, recuperação de jobs e política de exclusão.
5. Evoluir busca textual, OCR, integrações e conhecimento sobre essa base.

A primeira análise mostra uma base aproveitável. O trabalho principal é ajustar limites do domínio e garantias operacionais, sem uma reescrita indiscriminada do backend ou descarte do frontend em andamento.


## 8 Decisões posteriores do mantenedor

**Cadastro e provisionamento externo confirmados:** DEC-050 a 055 e RF-047/048 mantêm cadastro público desativado por padrão, com criação de contas ou convites por owner/admin. Convites são de uso único, válidos por sete dias e revogáveis; admin convida leitor, owner pode convidar admin e nenhum convite concede owner. Owner dispõe de controle por provedor OIDC/LDAP para criar conta como leitor no primeiro login externo válido, inclusive para identidades existentes no Authentik. Login posterior reutiliza o vínculo; controle inativo não cria contas desconhecidas. Se a identidade externa validada usar e-mail de conta local existente, o sistema oferece vínculo assistido mediante sessão local ou senha local válida; e-mail sozinho não basta. Recusa ou impossibilidade de confirmar encaminha o caso para análise de owner/admin. Isso não inclui importação antecipada de usuários, promoção por grupo externo nem fusão automática de duas contas Códice. Admin é atribuído somente dentro do Códice e somente pelo owner; owner somente por transferência explícita ou recuperação local de emergência. DEC-056 substitui DEC-002: promover, rebaixar, bloquear e remover admins é exclusivo do owner, fechando o desvio de convidar alguém como leitor e depois promovê-lo; admins seguem gerenciando leitores. O backend local observado não demonstra essa integração; sua implementação e testes permanecem necessários.

**Orçamento externo confirmado:** DEC-048/049, RF-046 e UI-23 estabelecem limite mensal global e limites opcionais por área, bloqueio de novas chamadas ao atingir limites e identificação de estimativas. Configuração e exibição financeira pertencem exclusivamente à página de gestão do owner, com proteção na API; não aparecem no hub, leitor ou administração comum. Mensagens de indisponibilidade operacional podem aparecer sem detalhes financeiros. Contabilização concorrente, moeda/período e tratamento de custo desconhecido são detalhes pendentes; não houve implementação dessa função nesta revisão.

**Provedores externos e privacidade confirmados:** DEC-045 a 047 estabelecem modelos locais por padrão e autorização externa pelo owner por área, sem fallback silencioso. Owner pode configurar provedor global com API key e provedores específicos por área; configuração específica prevalece, capacidades incompatíveis ficam indisponíveis e a credencial global não autoriza envios automaticamente. Notas privadas exigem autorização adicional do próprio leitor. RF-023 e seção 17.2 detalham o contrato; não se presume que um único serviço ofereça OCR, embeddings e geração nem que esse gerenciamento já exista no backend.

**Controle de IA e OCR confirmado:** DEC-044 separa OCR do interruptor global de IA. Desligar IA suspende busca semântica e sugestões com modelos, sínteses e assistente; preserva notas e resultados aceitos. Texto nativo, busca textual e heurísticas sem modelos continuam disponíveis. OCR pode continuar se habilitado em seu controle próprio, inclusive com motor baseado em modelos; etapas semânticas posteriores não herdam essa exceção. Essa distinção é requisito alvo, não funcionalidade constatada no backend.

**Lixeira e limpeza confirmadas:** DEC-041 a 043 e RF-045 definem lixeira recuperável para arquivos gerenciados, sem duplicação de conteúdo, com restauração/esvaziamento administrativo e exibição do espaço ocupado. Limpeza automática é desativada por padrão; ao habilitar, sugerir 30 dias, ajustáveis pelo owner, contados desde a entrada na lixeira. Alterações valem para novos itens; aplicação aos existentes exige ação explícita do owner com prévia, inclusive dos já vencidos. Após vencimento, itens elegíveis são excluídos definitivamente. Notas e referências bibliográficas permanecem; arquivos externos referenciados não entram nessa limpeza. Nenhum arquivo foi excluído durante esta atualização documental.

**Atualização da referência das notas confirmada:** DEC-040 determina que correções confirmadas de título e autor sejam refletidas na referência bibliográfica das notas enquanto a obra estiver disponível. Ao retirar a obra, conservar os últimos dados confirmados. A atualização não modifica o texto pessoal nem a citação salva e não dá aos administradores acesso às notas privadas. A persistência e a exportação devem refletir essa política mesmo depois da indisponibilidade da fonte.

**Retirada e memória bibliográfica confirmadas:** DEC-038/039 e RF-039 distinguem retirada reversível do catálogo, com arquivos preservados, de exclusão física administrativa adicional e explicitamente confirmada. Retirar item referenciado nunca apaga a fonte externa. Notas sobrevivem a ambas as ações e guardam ao menos título da obra e autor, exibindo fonte indisponível. O esquema atual com vínculo sujeito a cascata e a listagem de notas por JOIN com works precisam evoluir: não basta impedir a exclusão da nota se sua consulta deixar de retorná-la sem a obra ativa. Persistência da referência bibliográfica e consulta/exportação independente do catálogo ativo são critérios da implementação futura.

**Organização física confirmada:** DEC-036/037 e RF-044 definem pastas legíveis com IDs estáveis de obra, edição e arquivo no banco. Corrigir metadados não renomeia ou move arquivos automaticamente. Owner/admin solicita reorganização, examina prévia e executa dentro das raízes autorizadas. Notas e progresso permanecem ligados aos IDs. Essa decisão substitui a proposta anterior de caminhos físicos por ID/hash; detalhes de nomes, colisões e recuperação ainda precisam de implementação.

**Armazenamento confirmado na revisão de 19 de setembro:** modo referenciado aceita vários diretórios e não altera os arquivos. Owner configura raízes autorizadas e destino gerenciado; admins operam dentro desses limites. Importação gerenciada e mudança de referenciado para gerenciado usam o mesmo fluxo de transferência, mantendo somente o exemplar final após integridade e publicação confirmadas. Cópia temporária é permitida; remover a origem acessível ao servidor somente após verificação. Falhas preservam uma cópia válida e indicam limpeza pendente quando necessário. Upload não permite apagar o original no computador do remetente. DEC-032 a 035 e RF-011 substituem a proposta anterior de manter permanentemente origem e cópia gerenciada. Não houve movimentação ou remoção de arquivos nesta revisão documental.

Esta seção atualiza as recomendações anteriores sem alterar os achados da inspeção estática.

- O modelo completo da especificação prevalece sobre as limitações do código existente. Work, Edition e File continuam separados.
- Owner é papel distinto e essencial. Não será substituído pela alternativa de proteger somente o último administrador. Há exatamente um owner, com transferência explícita. Admins não podem removê-lo ou rebaixá-lo; configurar autenticação e transferir a instância são poderes exclusivos do owner. Recuperação de acesso e papel do antigo owner após transferência precisam de detalhe técnico.
- Bibliotecas são administradas por owner/admin e compartilhadas entre todos os usuários autenticados. “Compartilhadas” não significa acesso anônimo.
- Leitores organizam coleções próprias e privadas, com referências às obras; não ingerem arquivos nem removem obras do catálogo. Notas, favoritos e progresso são privados.
- Retirada do catálogo é administrativa. Exclusão de uma coleção ou de dados pessoais pelo próprio leitor não constitui exclusão de acervo.
- Importação gerenciada por padrão, catalogação referenciada e enriquecimento como sugestões foram aceitos. Notas devem ser preservadas após retirada administrativa da obra, mantendo conteúdo e privacidade e exibindo fonte indisponível. Exclusão em cascata das notas deve ser removida do desenho alvo.
- A base Go, Python e Redis será mantida nesta etapa; substituição exige benefício demonstrado. Go é preferência explícita do mantenedor.
- O acervo é inteiramente de teste. Isso simplifica o redesenho do banco, mas não é autorização implícita para apagar arquivos ou alterações de trabalho.

### Separação das próximas frentes

**Correções verificáveis sem novas decisões de produto:** autenticação Basic, exigência de papel administrativo na ingestão/alteração/exclusão, atomicidade do setup e testes de regressão. O setup deve criar owner conforme a decisão atual.

**Estrutura confirmada a implementar:** identidades distintas de obra/edição/arquivo, biblioteca compartilhada, coleção privada e papel owner. A fronteira entre admin e owner está confirmada: autenticação, transferência da instância e gestão do papel admin são exclusivas do owner; admin não pode removê-lo ou rebaixá-lo nem alterar o papel ou o estado de outros admins.

**Recuperação e transferência do owner confirmadas:** DEC-057 e DEC-058 definem a recuperação como comando local no servidor (redefinição de acesso do owner ou transferência quando ele está indisponível), sem variável de ambiente nem arquivo-gatilho, sempre auditada e sem rota remota. Na transferência normal o owner escolhe se o antigo owner vira admin ou leitor. Transferência em duas etapas com aceite do destino e link de redefinição de uso único de 1 hora, exibido só no terminal, foram confirmados; o padrão “leitor” quando o operador omite o papel na recuperação continua proposta. Não haverá cotas por usuário nesta etapa. Nada disso foi localizado no backend observado; implementação e testes permanecem necessários.

**Convites, revogação e contas duplicadas confirmados:** DEC-059 a 062. Convite é link avulso por padrão, com vínculo a e-mail opcional; SMTP é opcional e desativado por padrão. Bloquear é reversível, distinto de excluir, e revoga sessões e tokens imediatamente. Identidades OIDC/LDAP são revalidadas com teto de 24 horas ajustável pelo owner, além do bloqueio manual. Fusão de contas nunca é automática e exige confirmação manual de um admin, que vê só contagens, com confirmação das pessoas quando possível e auditoria com reversão ou backup; a ferramenta é futura. Sem SMTP, a redefinição de senha local é pedida pela pessoa e aprovada por owner ou admin, que repassa o link (DEC-063, RF-049). Nenhuma dessas funções foi localizada no backend observado.

**Backup confirmado:** DEC-064 define um comando mínimo do Códice (pacote consistente do banco e lista de arquivos, restauração verificada), com documentação para ferramentas externas. Índices e derivados ficam fora por padrão. RPO de 24 horas, RTO de algumas horas e retenção sugerida de 7 diários, 4 semanais e 3 mensais; criptografia opcional com chave fora do servidor. Não há ferramenta de backup em uso hoje; agendamento e ferramentas ficam com o operador. Nenhum comando desse tipo foi localizado no backend observado.

**Nomes no armazenamento confirmados:** DEC-065 fixa `Autor/Obra/Idioma — Editora — Ano/Arquivo` e pasta de série para séries. O caminho reflete um único autor; a autoria completa fica no catálogo e a obra aparece na navegação sob cada autor creditado, sem criar “Vários autores” como autor. O modelo de autoria do esquema atual precisa aceitar vários autores por obra. Sanitização, corte de 3 autores, limite de cerca de 200 caracteres e sufixo de desempate por ID são propostas. Nada disso foi localizado no backend observado.

**Fila, jobs e upload confirmados:** DEC-066 a 069 respondem aos achados 4.6 e 4.7. O estado dos jobs passa a viver no PostgreSQL, com Redis Streams só como entrega; upload e importação gravam obra e job na mesma transação, com despacho posterior e espera no banco se o Redis estiver fora do ar; erros temporários têm até 3 tentativas com espera crescente, erros permanentes não se repetem e jobs falhos aguardam reexecução manual; importações manuais têm prioridade e o padrão é um job pesado por vez. A §10.4 da especificação propõe a implantação recuperável. O código atual não faz reclaim de pendências, usa nome de consumidor fixo e publica após inserir a obra; esses pontos exigem implementação e testes de recuperação.

**Questões ainda abertas:** detalhamento técnico da recuperação local; heurísticas de duplicidade e obtenção do título mínimo na ingestão. Recuperação local sem segundo owner foi confirmada em DEC-054; política de lixeira, prazos e exclusão definitiva está confirmada em DEC-041 a 043.

**Ingestão confirmada:** arquivo válido com título mínimo pode entrar no acervo e ser lido sem aguardar aprovação de enriquecimento, salvo suspeita de duplicidade pendente. Metadados nativos preenchem campos vazios com proveniência; provedores externos, OCR e IA geram sugestões para aprovação administrativa. Inválidos ficam bloqueados; possíveis duplicatas aguardam revisão; campos confirmados manualmente não são sobrescritos automaticamente. O enriquecimento externo automático observado em worker/main.py deve evoluir para esse fluxo, sem bloquear leitura por sugestões bibliográficas pendentes.

**Duplicidade e versões confirmadas:** hash idêntico não gera outra cópia armazenada; o usuário recebe indicação do registro existente. Formatos, edições e idiomas distintos podem coexistir sob a mesma obra. A ficha permite selecionar edição, idioma e arquivo/formato antes da leitura. Semelhança de título, autor ou ISBN exige revisão administrativa, sem fusão automática. DEC-027 a DEC-029 e RF-041 formalizam a decisão; o atual acoplamento entre obra e arquivo precisa ser substituído para atendê-la.

**Retomada entre versões confirmada:** progresso independente por arquivo permanece o padrão. O usuário poderá solicitar uma posição equivalente em outro idioma, edição ou formato, com análise progressiva de capítulos, palavras e contexto, apoiada opcionalmente por OCR e IA. DEC-030 e RF-042 registram a capacidade futura; não foi observada sua implementação nesta análise. A seção 18.2 propõe apresentação de candidatos com precisão e evidências, sem sobrescrever progresso por mera sugestão. Esse recurso depende de File, Locator versionado e conteúdo recuperável; não exige sincronização contínua nem transferência de notas.

**Evolução para áudio confirmada:** DEC-031 e RF-043 incluem retomada assistida entre livro e audiobook, em ambos os sentidos, como funcionalidade futura posterior à retomada textual. O método de transcrição/alinhamento temporal ainda será definido. A funcionalidade não bloqueia o progresso independente nem a entrega inicial de RF-042.

**Organização confirmada em 19 de setembro de 2026:** há um único acervo, apresentado como Biblioteca, com categorias temáticas sobrepostas. Uma obra pode pertencer a várias categorias sem duplicação de arquivos. Coleções privadas podem reunir obras de todo o acervo. Isso substitui a hipótese anterior de múltiplas bibliotecas administrativas.

**Classificação confirmada na mesma revisão:** categorias admitem subcategorias e organizam a navegação principal; tags descrevem assuntos transversais. Owner/admin gerenciam categorias e tags globais. Leitores podem gerenciar tags pessoais privadas sem modificar a classificação compartilhada. DEC-025 e RF-040 registram essa decisão; a tabela atual de tags globais não deve ser presumida suficiente para o escopo pessoal.

**Validação ainda necessária:** executar testes em ambiente isolado e completar inspeção de implantação e funcionalidades marcadas como não localizadas. Não converter ausência de evidência nesta revisão inicial em prova de ausência.


### Confirmação final de governança e notas

O mantenedor aprovou DEC-022 e DEC-023, incorporadas na especificação v0.4. Esses pontos deixam de ser propostas: owner único com transferência explícita e proteção contra remoção/rebaixamento por admins; configuração de autenticação e transferência exclusivas do owner; preservação das notas com indicação de fonte indisponível após retirada da obra. Nenhuma alteração de código ou dados decorreu desta atualização documental.


## 9 Verificação de código da Fase 0 (19 de setembro de 2026)

Leitura direta de `middleware/auth.go`, `handlers/auth.go`, `handlers/opds.go`, `handlers/ws.go`, `cmd/api/main.go` e dos arquivos Compose, mais a execução dos testes existentes e do corpus sintético contra os extratores do worker. Baseline: `go build`, `go vet` e `go test` passam; 5 testes do frontend e 22 do worker passam; o frontend compila. Esses testes não cobrem autorização nem OPDS.

**Achados novos, além dos da seção 4:**

1. **Fallback de desenvolvimento concede admin sem autenticação.** Sem cabeçalho `Authorization` e com `APP_ENV` diferente de `production`, `AuthMiddleware` injeta um usuário admin fixo (`DefaultDevUserID`). O WebSocket também aceita conexão sem token fora de produção. O `docker-compose.yml` de desenvolvimento não define `APP_ENV`. Uma instância iniciada assim e acessível na rede é administrável por qualquer requisição anônima.
2. **Segredo JWT padrão embutido.** Fora de produção, se `JWT_SECRET` não existir, o código usa uma chave fixa pública no repositório; tokens podem ser forjados. O `docker-compose.full.yml` exige `JWT_SECRET` e assume `production`, o que mitiga esse caso.
3. **O papel vem do token, não do banco, e o token vale 7 dias sem revogação.** Bloquear, rebaixar ou trocar a senha de uma conta não tem efeito até o token expirar. Isso contradiz DEC-060 e DEC-070.
4. **Token na URL.** O middleware e o WebSocket aceitam `?token=` na query string. O `chiMiddleware.Logger` está ativo, e o log padrão inclui a URI, o que provavelmente grava tokens em log. Verificar na execução e preferir tokens curtos e específicos para recursos.
5. **Não há rotas de gestão de usuários** (criar, listar, bloquear, promover, convidar): RF-002 está ausente, não parcial. O papel de setup é `admin`, não há `owner`, e o registro público cria `reader`.
6. **Mensagens de login distinguem usuário inexistente de senha incorreta**, o que permite enumerar usuários. O limite de 10 tentativas por minuto por IP em `/auth/*` atenua, mas não elimina.
7. **PostgreSQL e Redis publicam portas no host** (5432 e 6379) em ambos os Compose, e o Redis não tem senha. Em rede local, qualquer máquina alcança a fila e o banco.
8. **CORS aceita uma única origem** (`CORS_ALLOWED_ORIGINS`, padrão `http://localhost:5173`). Adequado para o desenvolvimento; requer atenção na implantação.
9. **Hash de senha:** bcrypt com custo 10. Adequado; revisar o custo na Fase 1.

**Comportamento do worker observado com o corpus sintético** (`testdata/corpus`):
- EPUB truncado (zip inválido) é aceito pelo extrator como sucesso: título vem do nome do arquivo e o autor fica "Unknown Author", com 0 páginas. O CBZ segue o mesmo padrão. Só o PDF falso (texto com extensão `.pdf`) é rejeitado, porque o PyMuPDF levanta erro. Isso confirma que RF-008 exige validação de conteúdo antes da extração.
- PDF escaneado, sem camada de texto, é extraído sem sinalizar a ausência de texto: o detector de páginas sem texto útil para OCR (RF-019) ainda não existe.
- Os extratores usam `print` com emojis. Executados no Windows com o console em `cp1252`, a extração inteira falha com `UnicodeEncodeError`. Em Docker/Linux não ocorre; vale trocar por `logging`.
- EPUB, CBZ (LTR e RTL, com `ComicInfo.xml`) e CBZ de imagem longa extraem título, autor, idioma, série e ISBN corretamente. O extrator não lê a direção de leitura nem o layout fixo do EPUB.


## 10 Situação dos achados após a Fase 1 (19 de setembro de 2026)

Resultado das correções no branch `feat/fase1-seguranca`, cada uma com testes que falham quando a proteção é removida. Os achados 4.3 a 4.7 (modelo de dados, progresso, exclusão, jobs e upload) não fazem parte da Fase 1 e seguem abertos.

| Achado | Situação | Como |
| --- | --- | --- |
| 4.1 Basic sem validar senha | Corrigido | O middleware comum recusa Basic. Onde Basic ainda é aceito (OPDS, capas, arquivos), o segredo é um token de aplicativo verificado por hash (DEC-071); a senha da conta não vale mais |
| 4.2 Sem exigência de admin | Corrigido | `RequireStaff` em upload, lote, edição e exclusão; a importação em lote só lê raízes autorizadas e não segue symlinks |
| 4.8 Setup não atômico | Corrigido | Transação com trava e índice único parcial do owner; teste com 12 setups simultâneos |
| §9.1 Fallback de admin anônimo | Corrigido | Removido do HTTP e do WebSocket |
| §9.2 Segredo JWT padrão | Corrigido | A API não sobe sem `JWT_SECRET` |
| §9.3 Papel no token, 7 dias sem revogação | Corrigido | Sessões no banco (DEC-070); o papel é lido do banco a cada requisição |
| §9.4 Token na URL | Corrigido, com ressalva | Tokens de recurso de 15 minutos e ticket de 60 segundos; ainda aparecem no log de acesso |
| §9.5 Sem gestão de usuários | Parcial | Existe o papel owner e a troca de papel por owner; convites, bloqueio e exclusão ficam para a Fase 4 |
| §9.6 Mensagens de login | Corrigido | Mensagem única e mesmo custo de bcrypt para usuário inexistente |
| §9.7 Portas do PostgreSQL e Redis | Corrigido | Loopback no Compose de desenvolvimento; sem porta publicada no completo; senha do Redis obrigatória no completo |
| §9.8 CORS de origem única | Mantido | Adequado ao desenvolvimento; revisar na implantação |
| §9.9 Custo do bcrypt | Corrigido | Custo 12, com re-hash no login |

## 11 Situação dos achados 4.3, 4.4 e 4.5 após a Fase 2 (19 de setembro de 2026)

| Achado | Situação | Como |
| --- | --- | --- |
| 4.3 Obra como unidade de arquivo | Modelo criado, migração em curso | `editions` deixou de ser 1 para 1, e existem `files`, `storage_locations` e `work_contributors`; uma obra com várias edições e arquivos aparece uma vez no catálogo. As colunas antigas de `works` continuam como entrada do worker, projetadas por gatilhos, até a Fase 3 |
| 4.4 Progresso pertence à obra | Corrigido | `reading_progress` por usuário e arquivo, com Locator versionado (ainda vazio: cada leitor o adotará na Fase 5) e contador de revisão. O progresso antigo foi copiado para o arquivo primário |
| 4.5 Exclusão apaga arquivos e notas | Corrigido em parte | Retirar é reversível e preserva arquivos e notas; a exclusão física é um passo separado, restrito a obra retirada, e só apaga arquivos gerenciados. A lixeira recuperável e a limpeza automática (DEC-041 a 043) são da Fase 3 |
| Notas dependiam de JOIN com a obra | Corrigido | A nota guarda título e autor e sobrevive à retirada e à exclusão da obra (RF-039) |

## 12 Situação dos achados 4.5, 4.6 e 4.7 após a Fase 3 (19 de setembro de 2026)

| Achado | Situação | Como |
| --- | --- | --- |
| 4.6 Jobs sem recuperação | Corrigido | Fila no PostgreSQL com lease, batimento e retomada de worker que sumiu, até 3 tentativas com espera crescente, erro permanente sem repetição e reexecução manual. O nome do consumidor é único por processo. Verificado com a pilha real, inclusive com o Redis parado |
| 4.7 Upload sem consistência | Corrigido | Streaming para staging com nome aleatório, hash SHA-256, limite real do corpo, validação do conteúdo e obra, hash e job numa única transação; se algo falha, o arquivo é removido. Bytes idênticos, também em envios simultâneos, devolvem o registro existente |
| 4.5 Exclusão apaga arquivos | Corrigido em parte | Além da retirada reversível da Fase 2, arquivos e capas só são servidos quando o catálogo os possui e obras retiradas ficam ocultas a leitores. Falta a lixeira recuperável (DEC-041 a 043) |
| RF-008 Validar formato real | Corrigido no envio | O conteúdo é conferido antes de aceitar. Os extratores continuam lenientes; por isso o worker agora falha (permanente) quando o arquivo não existe |
| Enriquecimento sobrescrevia dados | Corrigido | O worker preenche só campos vazios ou vindos do arquivo e registra a origem; provedores externos geram sugestões que o admin aceita ou rejeita (DEC-019, DEC-026) |

## 13 Organização em disco e modo referenciado (19 de setembro de 2026)

| Item da especificação | Situação | Como |
| --- | --- | --- |
| DEC-036 e DEC-065 (pastas legíveis, identidade no banco) | Atendido | O caminho é só localização: o arquivo mantém seu id ao mudar de lugar, então notas e progresso não dependem do nome físico |
| DEC-037, RF-044 (reorganizar de forma explícita) | Atendido | Prévia com hash do plano, execução só do plano exato, recusa se a biblioteca mudou |
| DEC-032, DEC-035 (referenciado, raízes do owner) | Atendido | Raízes só do owner, varredura sem alterar arquivos, arquivo servido por id dentro da raiz |
| RF-011, DEC-033 e DEC-034 (mover para o gerenciado) | Atendido, com uma divergência | O fluxo verifica hash e só remove a origem se ela não mudou. A remoção é opt-in na importação em lote (a DEC-033 a faz padrão) |
| RNF-008 (consistência e recuperação) | Atendido para movimentos | Destino registrado antes do movimento e recuperação na subida |
| RF-041 (várias edições e formatos) | Base pronta | O catálogo já modela edições e arquivos; falta a criação de edições pela interface (Fase 5) |
| DEC-041 a 043 (lixeira) | Aberto | Sem lixeira recuperável ainda |

