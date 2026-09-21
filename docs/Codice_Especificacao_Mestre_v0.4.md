# Códice
## Especificação mestre do projeto

**Versão:** 0.4 • **Data:** 18 de setembro de 2026 • **Idioma:** português brasileiro  
**Responsável pelas decisões:** @ocnaibill  
**Estado:** consolidação para revisão e confronto com o backend existente

**Revisão temática de 19 de setembro de 2026:** DEC-024 define acervo único e categorias temáticas. Referências anteriores a bibliotecas no plural devem ser interpretadas conforme essa decisão.

O Códice é um gerenciador de acervo e leitura, aberto e auto-hospedável, voltado a livros, quadrinhos, documentos e, progressivamente, áudio. Reúne catálogo, leitor, marginalia e organização de conhecimento, com processamento documental e recursos de IA opcionais conforme a capacidade do servidor.

Este documento define o comportamento esperado do produto, suas regras e a arquitetura conceitual de referência. Serve como fonte de verdade para discutir mudanças e analisar o backend existente. Uma proposta aqui registrada não constitui decisão aprovada nem afirmação de que uma funcionalidade já está implementada.

## Sumário

1. Controle e interpretação do documento
2. Visão e limites do produto
3. Decisões confirmadas
4. Atores e permissões
5. Modelo conceitual
6. Requisitos funcionais do núcleo
7. Requisitos funcionais de conhecimento e integrações
8. Regras de negócio
9. Requisitos não funcionais
10. Arquitetura inicial
11. Autenticação e identidade
12. Storage e integridade
13. Ingestão e quarentena
14. Workers e filas
15. Extração documental e OCR
16. Busca textual e embeddings
17. IA generativa e governança
18. Leitura e sincronização
19. Grafo e gestão de conhecimento
20. Integrações
21. Páginas planejadas
22. Fluxos principais
23. Evolução e critérios de entrega
24. Decisões propostas e questões em aberto
25. Análise do backend existente
26. Glossário e referências

## 1 Controle e interpretação do documento

### 1.1 Estados normativos

**Confirmado (C):** orientação expressa pelo mantenedor na conversa de origem. **Proposto (P):** detalhamento técnico ou funcional sujeito a revisão. **Em aberto (A):** escolha ainda necessária. O estado aplica-se ao item inteiro; critérios de aceite são propostas de verificação, mesmo quando o requisito é confirmado.

As expressões “deve” e “não deve” descrevem o contrato pretendido do item. Em itens propostos, esse contrato só passa a integrar a base aprovada após validação. **Núcleo** indica a primeira base utilizável; **Evolução** indica capacidade posterior, sem eliminar seu lugar na visão do projeto. Prioridade e sequência não constituem prazo de entrega.

Os identificadores RF, RNF, RN, DEC, QA, UI e FL são estáveis nesta consolidação. Os códigos RF desta versão pertencem a um catálogo novo e não devem ser tratados como equivalentes automáticos aos códigos da proposta v0.1 da conversa. As referências QA-001 a QA-011 preservam a numeração do debate original.

### 1.2 Manutenção da fonte de verdade

Alterações devem registrar versão, data, motivo, itens afetados e responsável pela aprovação. Não reutilizar identificadores excluídos: marcar como substituídos e apontar os sucessores. Registrar decisões relevantes com contexto, alternativas, consequências e evidências.

Ao analisar o código, usar os estados “não avaliado”, “atendido”, “parcial”, “ausente”, “divergente” e “não aplicável”. A implementação pode motivar revisão da especificação, desde que a alteração seja explícita. Nenhum requisito deste documento recebe status de implementação antes da análise do repositório.

| Versão | Registro | Efeito |
| --- | --- | --- |
| 0.1 | Proposta de requisitos na conversa de origem | Base histórica sujeita às respostas posteriores |
| 0.2 | Consolidação em 18 de setembro de 2026 | Multiusuário, storage duplo, IA modular, critérios de aceite e roteiro de análise |
| 0.3 | Decisões do mantenedor após análise inicial | Owner distinto, modelo completo, bibliotecas compartilhadas e coleções privadas |
| 0.4 | Confirmação de governança e preservação | Owner único, transferência explícita e notas preservadas após retirada de obra |
| 0.4 (rev. 19/09) | Cadastro, convites e papéis | DEC-050 a 062: convites, provisionamento externo, papel admin gerido só pelo owner, recuperação local, papel do antigo owner, convite avulso com SMTP opcional, bloqueio imediato, revalidação externa de 24 horas, fusão manual e redefinição de senha sem SMTP (DEC-063, RF-049) política de backup (DEC-064), padrão de nomes (DEC-065) e recuperação de filas (DEC-066 a 069); DEC-002 substituída; sem cotas por usuário |
| 0.4 (rev. 19/09, login) | Login unificado e LDAP | DEC-072 a 075: formulário único com roteamento determinístico pela conta, owner sempre local, LDAP antes do OIDC e vínculo por coincidência de nome com prova das duas pontas |
| Próxima | Validação e implementação | Implementar decisões e verificar critérios de aceite |

## 2 Visão e limites do produto

### 2.1 Objetivos

O Códice deve permitir que o usuário mantenha seu acervo e suas notas sob seu controle, encontre e retome leituras e estabeleça relações entre passagens. Sua utilidade básica deve permanecer disponível sem GPU, sem assinatura externa e sem execução de modelos generativos.

O projeto é um hobby de evolução contínua. A organização por etapas ajuda a consolidar dependências; não limita o produto a um MVP comercial. A ambição de funcionar em diversos ambientes de self-hosting exige perfis de consumo e medições, e não uma promessa de suporte a qualquer hardware sem requisitos mínimos.

### 2.2 Escopo por capacidade

| Área | Base inicial | Evolução prevista |
| --- | --- | --- |
| Formatos | PDF, EPUB e CBZ | CBR, M4B/áudio, Markdown e outros após validação |
| Biblioteca | Obras, edições, arquivos, autores, séries e metadados | Coleções avançadas e enriquecimento assistido |
| Leitura | Leitor web, posição, marcadores e marginalia | Áudio, comparação e adaptadores externos |
| Pesquisa | Metadados, conteúdo textual e notas autorizadas | Busca semântica e híbrida |
| Conhecimento | Notas e relações manuais | Grafo, trilhas, sínteses e RAG |
| Operação | Storage local, filas, backup e administração | Telemetria ZFS/SMART e provedores adicionais |

CBZ é um contêiner, não uma classificação editorial. HQ, mangá e webtoon influenciam preferências de leitura, mas não devem determinar de forma irrevogável como o arquivo será exibido. O suporte a EPUB refluível e fixed-layout precisa ser discriminado na matriz de compatibilidade.

### 2.3 Limites propostos

Comercialização de livros, rede social pública, marketplace, quebra de DRM e hospedagem SaaS obrigatória ficam fora do escopo. O Códice pode interoperar com ferramentas de PKM, sem prometer substituir todas as suas funções. Compatibilidade com e-readers deve ser demonstrada por cliente, protocolo e versão; a presença de OPDS não garante sincronização universal.

## 3 Decisões confirmadas

| ID | Decisão | Origem e consequência |
| --- | --- | --- |
| DEC-001 | Multiusuário desde o início | QA-001; o wizard cria o owner inicial, conforme DEC-014 |
| DEC-002 | ~~Administradores gerenciam contas e promovem outros administradores~~ **Substituída por DEC-056** | QA-001; leitor comum possui papel distinto. Admins continuam gerenciando contas de leitores; atribuição e retirada do papel admin passam a ser exclusivas do owner |
| DEC-003 | Ingestão inicialmente restrita a administradores | QA-001; aplicar autorização na API e nos jobs |
| DEC-004 | Autenticação local como padrão e suporte a OIDC e LDAP | QA-002; Authentik é cenário desejado de integração |
| DEC-005 | Aceitar estruturas existentes e organizar novas importações por padrão | QA-003; suportar biblioteca referenciada e gerenciada |
| DEC-006 | PDF, EPUB e CBZ são os formatos iniciais | QA-004; outros formatos entram progressivamente |
| DEC-007 | Apresentar progresso como conceito comum a leitura e áudio | QA-005; modelo de posição específico ainda é proposta |
| DEC-008 | O leitor escolhe o modo de visualização | QA-006; contemplar HQ, mangá e webtoon |
| DEC-009 | IA terá controle global e por área | Complemento à QA-010; OCR possui controle separado, conforme DEC-044 |
| DEC-010 | Favorecer modelos leves e diversos ambientes de self-hosting | Complemento à QA-010; núcleo independente de hardware potente |
| DEC-011 | Separar internamente catálogo e sincronização, com experiência unificada quando útil | QA-011; cada protocolo recebe seu adaptador |
| DEC-012 | Consolidar o documento antes da próxima rodada de alterações no backend | Pedido de documentação; analisar documento e código nos dois sentidos |

| DEC-013 | Work, Edition e File permanecem entidades distintas, conforme a visão do documento | Confirmado após análise; o esquema atual não limita o modelo alvo |
| DEC-014 | Owner é papel essencial e distinto de administrador | Confirmado; o wizard cria o owner inicial |
| DEC-015 | Bibliotecas são compartilhadas entre todos os usuários autenticados e geridas por administradores | Não significa acesso anônimo ou publicação na internet |
| DEC-016 | Leitores podem criar coleções próprias e privadas | Coleções organizam referências ao acervo; não concedem ingestão |
| DEC-017 | Notas, favoritos e progresso são privados por usuário | Owner/admin não recebem acesso funcional automático a dados pessoais alheios |
| DEC-018 | Retirada de obras do catálogo é operação administrativa | Leitor pode gerenciar seus próprios dados, mas não excluir obras ou arquivos do acervo |
| DEC-019 | Enriquecimento externo gera sugestões e preserva campos confirmados | Importações gerenciadas por padrão; estruturas existentes podem ser referenciadas |
| DEC-020 | O acervo atual é inteiramente de teste | Não exige migração conservadora de dados de produção; não autoriza apagamento implícito |
| DEC-021 | Manter a base Go, Python e Redis nesta etapa | Go é preferência explícita; mudanças de stack exigem benefício demonstrado |

| DEC-022 | Existe exatamente um owner após a inicialização, com transferência explícita | Admins não podem removê-lo ou rebaixá-lo; configuração de autenticação e transferência da instância são exclusivas do owner |
| DEC-023 | Notas permanecem após retirada administrativa da obra | Exibir fonte indisponível e preservar privacidade e conteúdo pessoal |
| DEC-024 | Uma única biblioteca contém todo o acervo da instância | Categorias temáticas podem se sobrepor; uma obra pode pertencer a várias sem duplicar arquivos. Coleções pessoais permanecem privadas e podem reunir qualquer obra do acervo |
| DEC-025 | Categorias hierárquicas organizam a navegação principal; tags descrevem assuntos transversais | Owner/admin gerenciam categorias, subcategorias e tags globais. Leitores gerenciam tags pessoais privadas, sem alterar a classificação compartilhada |
| DEC-026 | Disponibilidade para leitura é independente da aprovação de enriquecimento | Arquivo válido com título mínimo pode entrar no acervo; metadados nativos preenchem campos vazios com proveniência. Provedores externos, OCR e IA geram sugestões para aprovação administrativa; arquivos inválidos ficam bloqueados e possíveis duplicatas aguardam revisão. Campos confirmados manualmente não são sobrescritos automaticamente |
| DEC-027 | Arquivos idênticos por hash não geram nova cópia armazenada | Informar duplicidade e oferecer acesso ao registro existente; não fundir identidades bibliográficas automaticamente |
| DEC-028 | Uma obra reúne formatos, edições e idiomas distintos | Leitor escolhe edição, idioma e arquivo/formato disponíveis na ficha da obra antes de abrir o leitor; cada opção mantém sua posição própria |
| DEC-029 | Semelhança de título, autor ou ISBN gera candidato para revisão | Owner/admin decide o vínculo; não fundir registros automaticamente com base nesses sinais |
| DEC-030 | Progresso independente por arquivo é o padrão; retomada equivalente entre versões é opcional | Ao trocar idioma, edição ou formato, retomar a posição própria do destino ou iniciar do começo. Usuário pode solicitar análise para continuar em posição equivalente, com heurísticas estruturais e textuais e, opcionalmente, OCR e IA |
| DEC-031 | Retomada assistida entre livro e audiobook integra o escopo futuro | A primeira implementação de RF-042 abrange versões textuais, incluindo conteúdo recuperado por OCR. Alinhamento entre texto e tempo de áudio será uma evolução posterior, opcional, preservando progresso independente |
| DEC-032 | Modo referenciado aceita vários diretórios e não altera seus arquivos | Owner define raízes permitidas; admins catalogam dentro delas |
| DEC-033 | Modo gerenciado mantém somente o exemplar final após transferência concluída | Preservar original significa preservar os bytes recebidos. Cópia temporária pode ser usada; remover origem acessível ao servidor somente após integridade verificada e publicação confirmada. Upload pelo navegador não permite apagar a fonte no computador de quem enviou |
| DEC-034 | Adotar é mudar do modo referenciado para gerenciado pelo mesmo fluxo de incorporação | Ação explícita “Mover para o armazenamento gerenciado”; não é terceiro modo de armazenamento |
| DEC-035 | Configurar raízes permitidas e destino gerenciado é exclusivo do owner | Admins importam e transferem arquivos somente dentro das raízes autorizadas |
| DEC-036 | Armazenamento gerenciado usa pastas legíveis e identidades estáveis no banco | Caminho não é identidade: obra, edição e arquivo têm IDs próprios; notas e progresso permanecem vinculados a esses IDs |
| DEC-037 | Correção de metadados não reorganiza automaticamente arquivos existentes | Owner/admin solicita “Reorganizar armazenamento”, confere a prévia e executa dentro das raízes autorizadas |
| DEC-038 | Retirada do catálogo é reversível e distinta de exclusão física | Owner/admin pode ocultar e restaurar a obra, preservando arquivos e notas. Excluir arquivos gerenciados exige ação adicional e confirmação explícita; retirar item referenciado nunca apaga a fonte externa |
| DEC-039 | Notas preservam título da obra e autor mesmo sem fonte disponível | Referência bibliográfica persistente deve sobreviver à retirada da obra e à exclusão de seus arquivos, sem depender de consulta ao catálogo ativo |
| DEC-040 | Referência bibliográfica das notas acompanha correções confirmadas enquanto a obra está disponível | Correções de título e autor atualizam a referência de apoio, sem modificar o texto pessoal da nota. Ao retirar a obra, preservar os últimos dados confirmados |
| DEC-041 | Exclusão de arquivo gerenciado passa por lixeira recuperável, sem duplicar conteúdo | Owner/admin pode restaurar ou esvaziar explicitamente; interface mostra espaço ocupado. Retirar do catálogo continua sendo ação distinta, sem enviar automaticamente à lixeira |
| DEC-042 | Limpeza automática da lixeira é desativada por padrão e configurável pelo owner | Quando habilitada, itens são excluídos definitivamente após o prazo configurado. Notas e referências bibliográficas são preservadas; arquivos externos referenciados não participam da limpeza |
| DEC-043 | Ao habilitar limpeza automática, sugerir retenção de 30 dias, ajustável pelo owner | Contar desde a entrada na lixeira. Alterações de prazo valem para novos itens; aplicação aos existentes exige ação explícita do owner com prévia |
| DEC-044 | OCR tem controle próprio, separado do interruptor global de IA | IA desligada suspende busca semântica, sugestões com modelos, sínteses e assistente. Extração nativa, busca textual e heurísticas sem modelos permanecem disponíveis; OCR pode continuar se habilitado em sua configuração. Notas e resultados aceitos são preservados |
| DEC-045 | Execução de modelos é somente local por padrão; owner autoriza serviços externos por área | Informar dados transmitidos; falta de recursos ou falha local nunca provoca envio externo silencioso |
| DEC-046 | Enviar notas privadas a provedores externos exige autorização do próprio leitor | Autorização da instância não substitui consentimento individual; recusa ou revogação bloqueia novos envios de suas notas |
| DEC-047 | Owner pode configurar provedor externo global com credencial compartilhada entre capacidades ou provedores específicos por área | Configuração específica prevalece sobre o padrão global. Usar apenas capacidades suportadas; credencial global não habilita áreas nem dispensa autorizações de transmissão |
| DEC-048 | Controlar orçamento externo com limite mensal global e limites opcionais por área | Interromper novas chamadas ao atingir limite configurado; custos sem apuração precisa devem ser identificados como estimativas |
| DEC-049 | Gestão e visualização de orçamento ficam exclusivamente na página de gestão do owner | Valores, limites, consumo financeiro e relatórios não aparecem em dashboards gerais, leitores ou administração comum; autorização também é aplicada na API |
| DEC-050 | Cadastro público fica desativado por padrão; owner/admin cria contas ou convites | Configuração de autenticação e eventual alteração da política pertencem ao owner |
| DEC-051 | Owner pode permitir criação de conta no primeiro login por provedor OIDC/LDAP | Controle por provedor permite que identidade externa válida, ainda sem conta Códice, seja cadastrada automaticamente como leitor. Isso não implica sincronização antecipada de todos os usuários do diretório |
| DEC-052 | Papéis administrativos são atribuídos exclusivamente dentro do Códice | OIDC/LDAP autenticam e podem provisionar leitores, mas não promovem por grupo externo. Somente owner promove leitores a admin, conforme DEC-056; owner só é atribuído pela transferência explícita da titularidade |
| DEC-053 | E-mail coincidente entre identidade externa e conta local inicia vínculo assistido, não automático | Após autenticação externa, o usuário pode confirmar que é a mesma pessoa por sessão local ativa ou senha local válida. Com confirmação, vincular a identidade à conta existente; sem confirmação, encaminhar para análise de owner/admin |
| DEC-054 | Recuperação de owner perdido exige procedimento local de emergência no servidor | Não existe segundo owner de recuperação. O procedimento deve ser documentado, requerer acesso local à instância e não poder ser disparado remotamente por admin comum |
| DEC-055 | Convites são de uso único, válidos por sete dias e revogáveis antes do uso | Admin pode convidar leitor; somente owner pode convidar admin. Convite não pode conceder owner |
| DEC-056 | Atribuir e retirar o papel admin é exclusivo do owner | Substitui DEC-002. Fecha o desvio “convidar como leitor e depois promover”. Admins criam, convidam, bloqueiam e gerenciam leitores, mas não promovem, rebaixam, bloqueiam ou removem outros admins. Uma conta admin comprometida não consegue criar novos admins para persistir |
| DEC-057 | Recuperação local do owner é um comando executado no servidor, sem variável de ambiente nem arquivo-gatilho | Cobre dois casos: (1) owner perdeu senha ou segundo fator: gera link de redefinição de uso único exibido apenas no terminal e encerra as sessões do owner; (2) owner indisponível: transfere a titularidade a uma conta existente em uma única transação. Exige acesso local à instância, não existe por API remota e gera auditoria destacada ao owner no próximo login. Link de redefinição de uso único, válido por 1 hora e exibido só no terminal; transferência normal em duas etapas com aceite da conta de destino |
| DEC-058 | Ao transferir a titularidade, o owner escolhe o papel do antigo owner: admin ou leitor | A escolha é feita no momento da transferência, sem padrão automático. A transferência nunca deixa zero ou dois owners |
| DEC-059 | Convite é um link avulso por padrão, com vínculo a e-mail opcional; SMTP é opcional e desativado por padrão | O link é entregue fora do Códice (mensagem, pessoalmente). Quem emite pode informar um e-mail, e então só esse endereço resgata o convite; o Códice compara, mas não envia nada sem SMTP. O owner pode configurar SMTP para envio automático, mas nenhuma função essencial depende dele |
| DEC-060 | Bloquear conta é distinto de excluí-la, e o bloqueio tem efeito imediato | Bloqueio é reversível: impede login, revoga na hora sessões e tokens de clientes e preserva notas e progresso. Excluir conta e dados pessoais é ação separada, com confirmação explícita. Admin bloqueia leitores; somente owner bloqueia admins (DEC-056) |
| DEC-061 | Identidades externas são revalidadas com teto de 24 horas, ajustável pelo owner, além do bloqueio manual | A sessão de conta OIDC/LDAP só se mantém enquanto o Códice consegue confirmar a identidade no provedor dentro do teto. Conta desativada no provedor perde acesso no máximo após esse prazo; bloqueio manual atua de imediato |
| DEC-062 | Fusão de duas contas Códice nunca é automática e exige confirmação manual de um admin | Ferramenta futura e explícita; vínculo assistido por e-mail (DEC-053) continua sendo o caminho normal. Salvaguardas confirmadas: o admin vê apenas contagens, nunca o conteúdo das notas; sempre que possível as duas pessoas também confirmam; a fusão é auditada e reversível ou precedida de backup verificado |
| DEC-063 | Sem SMTP, a redefinição de senha local é pedida pela pessoa e aprovada por owner ou admin, que repassa a informação de acesso | Nenhum fluxo essencial depende de e-mail (DEC-059). Detalhes técnicos em RF-049 |
| DEC-064 | Backup: o Códice oferece um comando mínimo e a documentação para ferramentas externas; agendamento e ferramentas ficam com o operador | O comando gera um pacote consistente do banco e a lista de arquivos, e a restauração o verifica. Índices e derivados ficam fora do backup por padrão. Metas: backup diário (RPO de 24 horas para o banco) e restauração em algumas horas (RTO), com ensaio documentado. Retenção sugerida: 7 diários, 4 semanais e 3 mensais, ajustável. Criptografia opcional com chave mantida fora do servidor. Originais e agendamento podem usar ferramentas externas (snapshots ZFS, restic, Borg), escolhidas por quem opera a instância |
| DEC-065 | Nomes no armazenamento gerenciado seguem `Autor/Obra/Idioma — Editora — Ano/Arquivo`, com pasta de série para séries e a autoria completa mantida no catálogo | O caminho é só localização; a autoria vive no banco. Uma obra com vários autores aparece na navegação por autor sob cada autor creditado, e “Vários autores” nunca é criado como autor. Uso da pasta de série (`Série/NN - Título`) aceito pelo mantenedor; detalhes de sanitização, limite de caminho e desempate são propostas em §12.1 |
| DEC-066 | O PostgreSQL é a fonte da verdade dos jobs; o Redis Streams é apenas o meio de entrega | Estado, tentativas e resultado ficam no banco. Se o Redis reiniciar ou perder dados, um reconciliador recoloca na fila os jobs pendentes ou com lease vencido. Redis fica fora do backup (DEC-064) |
| DEC-067 | Upload e importação gravam obra e job na mesma transação, e um despachante publica na fila | Arquivo entra em staging com nome único; se o Redis estiver indisponível, o job espera no banco e é despachado quando voltar. A interface mostra “recebido, aguardando processamento”, sem perder o envio |
| DEC-068 | Falhas: erro temporário tem até 3 tentativas com espera crescente; erro permanente não é repetido; depois disso o job fica como falhou até reexecução manual | Espera sugerida de 30 segundos, 2 minutos e 10 minutos, sem medição prévia. Admin vê o motivo em texto claro e pode reexecutar ou cancelar. Falha de etapa opcional não bloqueia a leitura (RN-012) |
| DEC-069 | Importações manuais têm prioridade sobre varreduras em lote, OCR e embeddings; o padrão é um job pesado por vez | Concorrência e limites ajustáveis pelo owner; pausar durante a leitura fica como opção futura, sem promessa |
| DEC-070 | Sessões são registros no banco, revogáveis de imediato | Suporta DEC-060: bloquear conta, redefinir senha ou encerrar sessão invalida o acesso na hora, o que um JWT puro sem verificação no servidor não garante |
| DEC-071 | Clientes OPDS e similares autenticam com tokens de aplicativo revogáveis, não com a senha da conta | Token vinculado ao usuário, com nome e escopo, revogável individualmente e invalidado por bloqueio ou redefinição de senha. Confirma a linha “Executar clientes e sincronização” da §4.1 |
| DEC-072 | O login usa um formulário único, e o backend escolhe o provedor de forma determinística pela conta | Inspirado no plugin LDAP do Jellyfin. Conta local confere a senha local; conta vinculada a uma identidade externa confere no provedor; nome desconhecido só chega ao provedor se o owner habilitou a criação no primeiro login (DEC-051). Não há tentativa em cascata, para que uma senha local errada não seja enviada ao diretório. Toda falha de credencial tem a mesma resposta pública (RNF-006) |
| DEC-073 | O owner autentica sempre por senha local | Nenhuma identidade externa é vinculada à conta do owner, e uma queda do LDAP ou do OIDC nunca o deixa sem acesso. Complementa DEC-054 e DEC-057. Proposta: o destino de uma transferência de titularidade precisa ter senha local, que define ao aceitar se ainda não tiver |
| DEC-074 | O LDAP é o primeiro provedor externo entregue, no mesmo formulário; o OIDC segue confirmado (DEC-004) e entra depois como botão separado | OIDC é por redirecionamento e não cabe atrás de usuário e senha. Define a ordem de entrega pendente em QA-002 |
| DEC-075 | Se a entrada do LDAP tem o mesmo nome de uma conta local, o vínculo é automático somente com a prova das duas pontas | A pessoa autentica no LDAP e informa também a senha local; se as duas conferem, a identidade externa é adicionada à conta local existente, sem passar por admin. É o vínculo assistido da DEC-053 estendido à coincidência de nome e não uma fusão de duas contas Códice, que continua manual (DEC-062). Nome ou e-mail iguais, sozinhos, nunca vinculam; sem a senha local correta nada é vinculado e o caso vai para owner/admin |
| DEC-076 | Tempo de leitura é tempo ativo: só conta enquanto há interação recente (virar página, rolar, teclar ou tocar nos últimos 90 segundos), e o áudio conta enquanto toca | Uma aba aberta e parada não acumula horas. O tempo é registrado por arquivo e somado por obra nas estatísticas. Sessões de leitura (`ReadingSession`) continuam fora até haver decisão de coleta e retenção |
| DEC-077 | Concluído é um fato do arquivo: marcado ao chegar ao fim (última página, 95% ou fim do áudio) ou por ação explícita, e não é desfeito por navegar para trás | Reabrir é uma ação explícita ("reler"). A data da conclusão é a primeira, até ser reaberta. Uma obra está concluída quando o arquivo lido por último está concluído; "em andamento" e as estatísticas seguem essa mesma regra |
| DEC-078 | O modo de exibição dos quadrinhos (LTR, RTL, webtoon, página dupla) é lembrado entre quadrinhos, neste dispositivo e por conta (duas pessoas no mesmo navegador não compartilham); EPUB e PDF não guardam preferências | Quem lê mangá não escolhe RTL a cada volume, mas o tamanho da fonte ou o zoom de um livro não deve seguir outro livro. Uma preferência por publicação, ou por usuário entre dispositivos, fica para quando houver necessidade |
| DEC-079 | A versão de uma obra que conta para a pessoa é a última que ela abriu **depois de tê-la começado**; é essa a que "Continuar lendo", o cartão e as estatísticas mostram | Abrir sem avançar não começa (uma posição, um percentual ou uma conclusão é o que começa), então quem abre o PDF por dois segundos e sai continua vendo o EPUB em 42%. Uma versão não concluída vem antes de uma concluída. A porcentagem de uma versão não se transfere para outra: cada arquivo guarda a sua (DEC-030), e a interface mostra a da versão que conta. A pergunta "continuar de onde parou na outra versão?" e o salto de posição dependem da retomada equivalente (RF-042) e ficam para ela; até lá a ficha só diz onde a pessoa está ("você está em 42% no PDF") e deixa abrir do começo ou na posição própria |
| DEC-080 | Concluir tem histórico: cada conclusão de um arquivo é registrada e a ficha diz "terminada N vezes: X em EPUB, Y em PDF"; concluir uma versão a tira de "Continuar lendo" e deixa só a outra em andamento; a pessoa pode marcar a **obra toda** como finalizada; reler é uma nova leitura | Complementa a DEC-077. Ao concluir uma versão havendo outra em andamento, o leitor pergunta se a obra toda está finalizada: sim tira todas as versões de "Continuar lendo" sem dar a outra como lida; não a mantém. A marca sai quando a pessoa volta a ler. "Reler" reabre o arquivo do zero (posição e percentual), e concluí-lo de novo conta outra vez. As estatísticas "concluídos no mês" contam a obra uma vez, em quantos formatos e vezes ela tenha sido concluída |
| DEC-081 | O botão "Ler" de um cartão abre a versão que conta quando a obra está em andamento; na primeira vez, ou depois de concluída, abre a ficha para a pessoa escolher formato, idioma e edição; uma obra de um arquivo só abre esse arquivo | Mantém a DEC-028 (a escolha acontece antes de abrir o leitor) onde há o que escolher, sem custar um clique a quem só tem um arquivo, e faz o "Ler" concordar com o "Continuar". A capa abre sempre a ficha |
| DEC-082 | O texto dos arquivos é guardado como segmentos com locator, por geração publicada de uma vez; o texto extraído é guardado como saiu do arquivo, e a busca ignora caixa e acentos sem stemming | Uma geração nova é escrita sem ser visível e uma função SQL a publica e apaga as anteriores, então a busca vê o texto velho ou o novo, nunca metade, e um worker que morre no meio não deixa nada pesquisável (RNF-008). O trecho mostrado é sempre o texto original, com acento; só o índice o normaliza. A extração tem versão: mudar uma regra ou um limite reprocessa tudo. Stemming por idioma ("correr" achar "corrida") fica para depois, como outra coluna, sem mexer no que está guardado |
| DEC-083 | O texto extraído é dado derivado: fica fora do backup e é extraído de novo depois de uma restauração | É grande, refaz-se dos arquivos e não vale nada sem eles. Sem ele o backup e o tempo de restauração seguem o tamanho do banco de antes; a busca por conteúdo volta quando o job termina, atrás do que as pessoas pedirem. Complementa DEC-064 |
| DEC-084 | Lê-se o texto depois de analisar o arquivo, num job próprio e atrás do trabalho que as pessoas pediram; a leitura de uma obra nunca depende dele e ele não muda o estado da obra | Prioridade menor que a ingestão e a organização, e mesmo limite de um job pesado por vez (DEC-069). Um arquivo que não lê fica registrado como falho e os outros da obra seguem; um arquivo que não está onde devia interrompe o job para tentar de novo. Quem pode pedir de novo é um administrador. Quadrinhos e áudio não têm texto até o OCR; PDF sem camada de texto fica "vazio" à espera do OCR; MOBI ainda não é lido |
| DEC-085 | O texto de um livro é do acervo: qualquer pessoa autenticada o pesquisa, como pode ler os arquivos; as notas de cada pessoa continuam só dela | A busca só considera texto publicado, de arquivos que ainda abrem e de obras que não foram retiradas: um resultado nunca leva a uma fonte que não abre (§16.1) |
| DEC-086 | A retomada equivalente (RF-042) usa heurísticas sobre o texto já indexado (sequências de palavras, nomes e números, título ou número do capítulo, contagem de capítulos), sem modelo; a sugestão é um salto único, nunca sincronização contínua, e o aceite é só histórico | Um candidato só aponta para conteúdo realmente encontrado no destino; sem evidência, nenhuma sugestão, e duas igualmente prováveis são ambíguas para a pessoa escolher, nunca um palpite. Depois do salto cada arquivo segue com o seu próprio progresso (DEC-030); a interface mostra a versão que conta (DEC-079). Os limiares são um primeiro palpite a medir (QA-028), não um limite aprovado |
| DEC-087 | (confirmada pelo mantenedor em 21/09/2026) Todo arquivo com sumário ganha uma **estrutura padrão** (partes, capítulos, e o que é frente, corpo ou fim do livro), e a retomada equivalente (RF-042) procura **capítulo primeiro, depois a passagem, depois confere o caminho de volta** | O worker lê o sumário do EPUB (NCX ou documento de navegação, com as âncoras que apontam para o meio de um arquivo) e os marcadores do PDF; cada trecho sabe o nó em que está e nunca cruza dois; cada nó é frente, corpo ou fim pelo que se chama e pelo lugar em que está (a palavra só vale quando abre o título, para "O fim do mundo" continuar sendo um capítulo). O motor corta a história de cada arquivo na profundidade em que os dois têm o mesmo número de divisões ("3 livros" com "3 livros", "28 capítulos" com "28"), confere os números dos títulos quando existem e trata a numeração que recomeça como outro livro no mesmo arquivo; a busca das palavras e dos nomes fica **dentro do capítulo alinhado** quando os números confirmam, corpo só procura corpo (um apêndice que cita todo mundo nunca disputa com um capítulo), e o que foi achado é conferido no caminho de volta (descartado, se apoiado só em nomes, ou rebaixado). Sem sumário nos dois lados vale o que valia (o arquivo como capítulo), com confiança limitada a média. Uma divisão grande não vira posição de retomada apenas por estar estruturalmente alinhada. **Medido** (ensaio de 21/09/2026, sem modelo): no Verne (domínio público, três edições, verdade pelos capítulos), o capítulo certo em 214 de 219 ofertas entre idiomas (as 5 restantes são fronteira da régua; antes, 53% com metade das ofertas), e 6 de 224 tentativas sem resposta; no Duna, PT-PT com PT-BR passou de 5 para 19 achados e as âncoras ficaram todas na posição esperada. Limites: o teste só mede o capítulo, não o parágrafo; livros com divisões diferentes entre as edições ou sem sumário (PDF sem marcadores) continuam com o método antigo; os limiares seguem a ser medidos no conjunto de avaliação (#32, QA-028) |
| DEC-088 | (confirmada pelo mantenedor em 21/09/2026) Quando os dois arquivos têm capítulos alinhados e nenhuma passagem foi achada, a retomada equivalente pode oferecer **uma posição aproximada dentro do capítulo** (a mesma fração do caminho, por caracteres), rotulada como aproximada | O texto de um capítulo traduzido tem comprimento proporcional em todas as suas partes, então a mesma fração do capítulo do outro arquivo cai perto. A estimativa só existe quando o alinhamento por contagem é confirmado pelos números dos títulos e o capítulo ocupa no máximo 10% do corpo; a precisão é `approximate` (a interface diz "posição aproximada no mesmo capítulo"), a confiança é média, e uma passagem achada sempre a substitui. Sem essas garantias o motor não oferece nem uma aproximação nem o começo da divisão apenas pela estrutura. Isso muda a promessa de "nunca inventa" da DEC-086 num ponto: é uma estimativa declarada, não uma correspondência. **Medido:** em 118 pares de trechos do Verne, a posição proporcional dentro do capítulo cai a até 1 trecho em 98% dos casos (o começo do capítulo, 25%); ponta a ponta na API, 96% a até 1 trecho (159 tentativas). No Duna, estender a aproximação a livros inteiros acertou até 1 trecho em 78,9% dos casos, mas produziu erros de até 63 trechos e variou de 41% a 100% conforme a direção; esse resultado motivou a abstinência em unidades grandes. O ganho nelas fica para o modelo opcional (#31) |

O esquema físico do storage, pgvector e os motores de OCR permanecem propostas. O mecanismo da recuperação local está confirmado em DEC-057 e o papel do antigo owner em DEC-058; duas etapas na transferência e link de 1 hora foram confirmados; resta como proposta apenas o papel padrão do antigo owner na recuperação de emergência.

## 4 Atores e permissões

O **owner** é o administrador principal, criado no wizard, com papel distinto e responsabilidade pela governança da instância. Configuração de autenticação e transferência da instância são exclusivas do owner, conforme DEC-022. O **administrador** gerencia catálogo, usuários, ingestão, filas e integrações, respeitando as ações reservadas ao owner. O **leitor** consulta o acervo permitido e mantém seu estado de leitura e conhecimento pessoal. Um **cliente externo** opera com identidade e escopo próprios por meio de um adaptador. Um **worker** executa tarefas autorizadas e não representa um usuário com acesso irrestrito.

### 4.1 Matriz proposta de autorização

| Ação | Leitor | Administrador | Observação |
| --- | --- | --- | --- |
| Ler e pesquisar acervo autorizado | Sim | Sim | Bibliotecas compartilhadas entre usuários autenticados |
| Alterar progresso e notas pessoais | Próprios | Próprios | Administração não concede acesso funcional às notas alheias |
| Ingerir e editar catálogo global | Não | Sim | Confirmado para ingestão |
| Criar, convidar, bloquear e gerenciar leitores | Não | Sim | Owner também pode |
| Promover a admin, rebaixar, bloquear ou remover admins | Não | Não | Exclusivo do owner (DEC-056); inclui convite de admin (DEC-055) |
| Configurar processamento local e integrações gerais | Não | Sim | Autenticação, raízes e provedores externos de modelos são exclusivos do owner |
| Configurar provedores externos de modelos e autorizar transmissão por área | Não | Não | Exclusivo do owner; envio de notas privadas ainda exige autorização do leitor |
| Configurar raízes permitidas e destino gerenciado | Não | Não | Exclusivo do owner; admins operam dentro das raízes autorizadas |
| Configurar autenticação e transferir titularidade | Não | Não | Exclusivo do owner |
| Retirar obras do catálogo | Não | Sim | Owner também pode; preservar notas pessoais |
| Exportar notas | Próprias | Próprias | Compartilhamento exige regra futura explícita |
| Executar clientes e sincronização | Próprios | Próprios | Tokens vinculados ao usuário e revogáveis |

Owner possui as capacidades administrativas comuns e é o único autorizado a configurar autenticação e transferir a titularidade da instância. Após inicialização, existe exatamente um owner. Admins não podem removê-lo nem rebaixá-lo. Também não alteram o papel ou o estado de outros admins: promover, rebaixar, bloquear e remover admins é exclusivo do owner, conforme DEC-056. A transferência deve ser explícita e substituir a titularidade sem criar estado com zero ou dois owners.

Leitor pode criar, renomear, organizar e excluir suas coleções privadas, notas e favoritos. Remover uma obra de uma coleção altera somente o vínculo pessoal. A retirada do catálogo e a exclusão física são operações distintas, ambas vedadas ao leitor. Notas são preservadas após retirada administrativa da obra e exibem a fonte como indisponível. A retirada não altera sua autoria ou privacidade.

A privacidade funcional não impede o operador do servidor de acessar banco e discos. A documentação de implantação deve explicar esse limite. O conceito de “destaques da comunidade” presente nas telas fica dependente de uma política explícita de compartilhamento, ainda não aprovada.

## 5 Modelo conceitual

### 5.1 Identidade bibliográfica e arquivos

**Work (Obra)** representa a criação intelectual. **Edition (Edição)** identifica uma publicação com idioma, editora, data, tradução e identificadores. **File (Arquivo)** representa uma manifestação digital específica e sua versão imutável. Uma obra tem várias edições e cada edição pode ter vários arquivos. Um ISBN não deve ser a chave primária da obra nem prova suficiente de identidade física.

**Author/Contributor** registra pessoas ou organizações com papéis de autoria, tradução, narração ou edição. **Series** organiza obras e posições; o vínculo deve aceitar ordem fracionária ou explícita. **Tag** distingue vocabulário global de classificação pessoal.

| Entidade | Campos conceituais mínimos | Relações e invariantes propostas |
| --- | --- | --- |
| Work | id, título, descrição, tipo | N edições; N contribuições; identidade independente de caminho |
| Edition | id, work_id, idioma, editora, data, identificadores | Pertence a uma obra; não agrega traduções distintas sem revisão |
| File | id, edition_id, formato, hash, tamanho, versão, disponibilidade | Uma versão de conteúdo; mantém relação com localização e derivados |
| StorageLocation | id, modo, raiz, chave/caminho, estado | Gerenciada ou referenciada; caminho não é identidade bibliográfica |
| Contributor | id, nome, identificadores | Vínculo com papel e entidade de destino |
| MetadataCandidate | alvo, campo, valor, fonte, evidência, estado | Separa sugestão de valor canônico aceito |
| DerivedArtifact | file_id, tipo, versão do processamento, chave, estado | Recriável; original preservado |

Separar bytes físicos de vínculos de catálogo é uma opção de implementação para deduplicação, não uma obrigação de tabelas nesta versão. Uma deduplicação física não pode fundir obras ou edições silenciosamente.

### 5.2 Biblioteca categorias e coleções

**Library (Biblioteca)** representa o único acervo da instância, disponibilizado pelos administradores a todos os usuários autenticados. Leitor não adiciona arquivos à biblioteca nem retira obras do catálogo. A organização lógica não precisa corresponder às pastas físicas de armazenamento.

**Category (Categoria)** organiza a navegação temática do acervo e admite subcategorias, como Ciência da Computação → Inteligência Artificial. Ficção Científica, Biografias, Literatura Fantástica e Clássicos Atemporais são outros exemplos, sem impor taxonomia fixa. Uma obra pode pertencer a várias categorias, e cada categoria reúne várias obras. Esses vínculos não duplicam edições ou arquivos. Owner/admin gerenciam a hierarquia e a classificação compartilhada.

**Tag (Etiqueta)** descreve assuntos transversais às categorias, como ecologia, distopia e aprendizado de máquina. Tags globais e suas atribuições são administradas por owner/admin; tags pessoais pertencem ao usuário e são privadas. Criar ou atribuir uma tag pessoal não modifica o vocabulário nem a classificação global. Regras de atribuição automática continuam pendentes de detalhamento.

**Collection (Coleção)** é uma organização pessoal e privada, pertencente a um usuário. **CollectionItem** referencia uma obra do catálogo, com ordem opcional. Criar ou excluir coleção não copia nem apaga arquivos. A API deve filtrar por proprietário em listagem, consulta, alteração, exclusão e busca. Compartilhamento de coleções fica fora do escopo inicial.

### 5.3 Texto e localização

**DocumentSegment** é uma unidade textual recuperável, derivada de conteúdo nativo, OCR ou nota do usuário. Deve conter id, origem, texto, idioma, sequência, referência à versão da fonte, Locator, versão da extração e proprietário/visibilidade quando aplicável. Os vínculos com obra, edição e arquivo são obrigatórios conforme a origem; uma nota independente não precisa inventar um arquivo.

**Locator** é um endereço tipado e versionado dentro da publicação. A posição deve continuar interpretável após reinício e permitir detectar quando a fonte mudou. Não se deve considerar um número de página renderizada de EPUB equivalente a uma página fixa de PDF.

| Formato | Localização proposta | Precisão e limites |
| --- | --- | --- |
| EPUB | recurso/href, CFI quando disponível, trecho de apoio | Posição ligada à versão; fallback textual pode ser ambíguo |
| PDF | índice de página e região normalizada opcional | Separar índice interno de rótulo impresso da página |
| CBZ | identificador do item e ordem, região opcional | Ordem natural estável; substituição exige nova versão |
| Áudio | faixa/capítulo e tempo em milissegundos | Sem equivalência automática com posição textual |
| Webtoon | imagem/segmento e deslocamento normalizado | Preservar posição apesar de mudança no tamanho da tela |

### 5.4 Dados pessoais e conhecimento

| Entidade | Responsabilidade | Relações principais |
| --- | --- | --- |
| User e ExternalIdentity | Conta e formas de login | N identidades por conta; papéis independentes do provedor |
| ReadingProgress | Última posição por usuário e arquivo/versão | Locator, progressão opcional, revisão, dispositivo e datas |
| ReadingSession | Intervalo de atividade | Usuário, arquivo, início, fim; coleta e retenção configuráveis |
| Bookmark | Posição salva | Usuário e Locator |
| Highlight | Seleção textual ou região | Texto citado, seletor, cor e origem |
| Annotation | Nota pessoal | Markdown, tags, vínculo opcional com destaque, obra ou Locator; referência bibliográfica persistente com título da obra e autor para notas vinculadas a fontes |
| Preference | Preferências de exibição | Padrão por usuário e sobreposição por publicação |
| Concept e Relation | Nós conceituais e ligações | Escopo, tipo, origem, evidências e confirmação |
| Trail e TrailItem | Sequência de estudo | Itens ordenados, obras/passagens/notas e progresso próprio |
| SegmentEmbedding | Representação vetorial derivada | Segmento, modelo/revisão, dimensão e hash do conteúdo |
| GeneratedArtifact | Resultado de modelo generativo | Fontes, modelo, configuração, versão do prompt e estado |
| Job e JobAttempt | Trabalho assíncrono e tentativas | Tipo, alvo, versão, prioridade, lease e erro sanitizado |

Cardinalidades e agregados são uma base de discussão, não um esquema SQL final. A escolha entre enumerações, tabelas de papéis e entidades associativas será confrontada com o backend.

## 6 Requisitos funcionais do núcleo

Cada item inclui estado, horizonte e um critério de aceite observável. O detalhamento técnico de segurança, consistência e persistência está nas seções seguintes.

### 6.1 Instalação e contas

**RF-001 — Inicializar a instância [C · Núcleo].** O wizard deve criar o primeiro owner e conduzir a configuração inicial. Aceite: em base vazia, a conclusão cria uma única configuração ativa; duas inicializações concorrentes não produzem dois proprietários iniciais por acidente. A transação e o bloqueio são detalhamento proposto.

**RF-002 — Gerenciar usuários [C · Núcleo].** Owner e administradores devem criar, convidar, bloquear e gerenciar contas de leitores. Somente o owner cria administradores, promove leitores a admin e rebaixa, bloqueia ou remove admins (DEC-056). Aceite: leitor não executa essas ações pela interface nem pela API; admin que tenta promover, rebaixar, bloquear ou remover outro admin recebe recusa na API; alterações de papel tornam-se efetivas em sessões segundo política documentada e ficam registradas em auditoria.

**RF-003 — Autenticar por múltiplos mecanismos [C · Núcleo e evolução].** Login local é o padrão; OIDC e LDAP devem ser suportados. Aceite: cenários de login válido, credencial inválida, conta desativada e provedor indisponível são distinguíveis sem expor segredos. A ordem de entrega dos provedores externos está em aberto.

**RF-004 — Gerenciar sessões e identidades [C no vínculo assistido · Núcleo].** Usuário deve encerrar sessões e vincular identidades externas por fluxo autenticado. Quando uma identidade OIDC/LDAP validada tiver o mesmo e-mail de uma conta local, o sistema oferece vínculo assistido: a pessoa confirma por sessão local ativa ou senha local válida e então a identidade é vinculada à conta existente. Aceite proposto: coincidência de e-mail sozinha não concede acesso à conta; confirmação válida não cria segunda conta; recusa, falha ou ausência de senha local encaminha o caso para análise de owner/admin; revogação impede nova utilização do token. Fusão de duas contas Códice independentes nunca é automática: permanece ação futura que exige confirmação manual de um admin (DEC-062). Salvaguardas confirmadas em 19 de setembro de 2026, porque notas, favoritos e progresso são privados (DEC-017): a ferramenta mostra ao admin apenas contagens, sem o conteúdo das notas; sempre que possível, obtém também a confirmação das pessoas titulares; a fusão é auditada e reversível ou precedida de backup verificado.

### 6.2 Catálogo e ingestão

**RF-005 — Catalogar obras, edições e arquivos [C · Núcleo].** Administrador deve cadastrar e corrigir os três níveis separadamente, com autores, séries e tags. Aceite: uma obra com duas traduções e três arquivos permanece navegável sem duplicar indevidamente a obra.

**RF-006 — Consultar biblioteca e ficha [P · Núcleo].** Leitor deve filtrar e ordenar por título, autor, série, formato, idioma, tags, disponibilidade e estado pessoal. Aceite: a ficha apresenta edições e arquivos disponíveis e diferencia dados bibliográficos de progresso pessoal. Uma obra com vários autores aparece na listagem de cada um deles, e o rótulo de pasta “Vários autores” nunca é exibido como autor.

**RF-007 — Ingerir arquivos [C quanto à permissão · Núcleo].** Somente administradores devem iniciar ingestão. Upload e diretório monitorado são meios propostos. Aceite: uma tentativa não autorizada não cria arquivo permanente nem job; uma importação autorizada recebe identificador rastreável.

**RF-008 — Validar e detectar duplicidade [C quanto à política de duplicidade · Núcleo].** O pipeline deve verificar formato real, integridade básica, tamanho e hash. Arquivo com hash idêntico não gera nova cópia armazenada; a interface informa duplicidade e oferece acesso ao registro existente. Formatos, edições e idiomas distintos podem pertencer à mesma obra, com vínculo revisado quando necessário. Semelhança de título, autor ou ISBN produz candidato para revisão administrativa, sem fusão automática. Aceite: reimportar bytes idênticos não duplica armazenamento; EPUB e PDF da mesma obra podem coexistir; tradução em outro idioma pode ser vinculada à obra sem substituir a edição anterior; ISBN coincidente não provoca fusão automática.

**RF-009 — Revisar quarentena e metadados [C · Núcleo].** Owner/admin deve aceitar, corrigir, rejeitar ou reprocessar itens ambíguos. Arquivo válido com título mínimo pode ser incorporado e liberado para leitura sem aprovação prévia de todo o enriquecimento, exceto quando houver suspeita de duplicidade pendente. Metadados nativos preenchem apenas campos vazios e mantêm sua origem; metadados de provedores externos, OCR ou IA aguardam aprovação administrativa. Aceite: sugestão de ano ou editora não bloqueia leitura; possível duplicata aguarda revisão; arquivo inválido permanece bloqueado; campo confirmado manualmente não é sobrescrito automaticamente; rejeitar sugestão não apaga a fonte externa.

**RF-010 — Operar dois modos de biblioteca [C · Núcleo].** O sistema deve catalogar estruturas existentes e organizar novas importações no padrão gerenciado. Aceite: a indexação referenciada não move arquivos; a gerenciada publica uma cópia íntegra segundo padrão configurado.

**RF-011 — Mover para armazenamento gerenciado e reconciliar arquivos [C no fluxo de transferência · Evolução].** Owner/admin deve poder transferir explicitamente um arquivo referenciado para o modo gerenciado, usando o mesmo fluxo de incorporação e respeitando raízes autorizadas pelo owner. A origem acessível ao servidor é removida somente depois da verificação de integridade e confirmação do destino e do vínculo. Aceite proposto: transferência bem-sucedida mantém somente o exemplar final; falha não causa perda da única cópia válida; origem alterada durante a transferência não é apagada; falha ao remover origem aparece como limpeza pendente, não como conclusão integral. Reconciliação de fontes movidas ou ausentes preserva notas e histórico.

### 6.3 Leitor e marginalia

**RF-012 — Ler formatos iniciais [C · Núcleo].** O leitor deve abrir PDF, EPUB e CBZ suportados no navegador. Aceite: exemplares do corpus de teste abrem, navegam e retomam posição; variante não suportada recebe mensagem clara e preserva o original.

**RF-013 — Configurar visualização [C · Núcleo].** Usuário deve escolher direção, modo paginado/contínuo, ajuste e distribuição de páginas conforme o formato. Aceite: mangá pode usar RTL e webtoon rolagem vertical; preferências persistem sem afetar outros usuários.

**RF-014 — Navegar e retomar [P · Núcleo].** Disponibilizar sumário quando existente, salto para posição, marcadores e modo imersivo. Aceite: retornar de uma busca ou nota reabre o Locator correspondente; falha de localização oferece contexto e não simula precisão.

**RF-015 — Registrar progresso unificado [C no conceito · Núcleo e evolução].** Exibir continuidade de leitura e áudio no mesmo produto, preservando posição específica por arquivo. Aceite: progresso de áudio não altera automaticamente o EPUB; a ficha identifica a origem da posição apresentada.

**RF-016 — Criar marginalia [P · Núcleo].** Usuário deve criar, editar e excluir marcadores, destaques e notas próprias, com tags pessoais. Aceite: nota pode retornar à fonte; outro usuário não a obtém por URL, pesquisa, exportação ou grafo sem compartilhamento permitido.

**RF-017 — Editar notas estruturadas [P · Núcleo e evolução].** Markdown é o formato base proposto; Wikilinks e fórmulas são evolução. Aceite: conteúdo é sanitizado, exportável e não executa scripts; links a conceitos inexistentes são tratados explicitamente.

### 6.4 Pesquisa e processamento

**RF-018 — Pesquisar metadados, texto e notas [P · Núcleo].** Oferecer pesquisa global e por obra, filtros e trechos com localização. Aceite: consulta em conteúdo indexado retorna a fonte correta; resultados pessoais respeitam usuário e permissões.

**RF-019 — Extrair texto e executar OCR [P · Núcleo incremental].** Priorizar texto nativo utilizável e executar OCR assíncrono apenas quando necessário ou solicitado. Aceite: PDF misto pode processar somente páginas sem texto útil; falha de OCR não torna ilegível um arquivo válido.

**RF-020 — Acompanhar e controlar jobs [C quanto a estado, falhas e prioridade (DEC-066 a 069) · Núcleo].** Administração deve listar etapas, progresso, falhas com motivo em texto claro, cancelamentos e reexecuções. Aceite proposto: tarefa interrompida por reinício pode ser recuperada sem publicação duplicada; cancelar impede publicar saída incompleta; reiniciar ou esvaziar o Redis não perde jobs pendentes; upload com Redis indisponível é aceito e processado depois; erro temporário esgota 3 tentativas antes de marcar “falhou”; erro permanente não é repetido; job falho pode ser reexecutado ou cancelado pelo admin.

**RF-021 — Administrar configuração e saúde [P · Núcleo].** Exibir disponibilidade de storage, filas e integrações, com diagnóstico útil. Aceite: indisponibilidade de serviço opcional aparece isoladamente, sem marcar todo o acervo como indisponível.

## 7 Requisitos funcionais de conhecimento e integrações

**RF-022 — Controlar IA por área [C · Evolução].** Disponibilizar interruptor global e controles de busca semântica, metadados, tags, grafo, trilhas, comparação, sínteses e assistente. Aceite: desligar o global impede novas execuções dessas áreas; as preferências individuais são preservadas para reativação.

**RF-023 — Configurar provedores e perfis [C na política de provedores · Evolução].** Separar capacidades de OCR, embeddings e geração. Owner pode configurar provedor externo global com API key e substituições por área; cada área pode usar o padrão global, um provedor específico ou execução local. Validar compatibilidade de capacidade e modelo; manter limites de consumo. Aceite proposto: configuração específica prevalece; capacidade ausente é informada sem trocar de serviço silenciosamente; API key global não habilita envio por si só; provedor indisponível não aciona envio externo não autorizado; notas privadas só são transmitidas com autorização vigente do leitor.

**RF-024 — Gerar embeddings e busca semântica [P · Evolução].** Indexar segmentos com modelo versionado e retornar passagens relevantes com fontes. Aceite: consulta usa o mesmo espaço vetorial do índice; troca de modelo inicia índice separado, sem misturar vetores incompatíveis.

**RF-025 — Sugerir enriquecimento [P · Evolução].** Sugerir metadados, tags e conceitos com evidências. Aceite: usuário autorizado pode aceitar, editar ou rejeitar; rejeição não reaparece continuamente sem nova evidência ou pedido de reprocessamento.

**RF-026 — Manter grafo explícito [P · Evolução].** Criar e visualizar relações manuais entre obras, conceitos e marginalia, mesmo com IA desativada. Aceite: relação manual persiste após apagar índices vetoriais; relações sugeridas são identificadas separadamente.

**RF-027 — Criar trilhas e comparar passagens [P · Evolução].** Ordenar obras/passagens e abrir leitura comparada, com notas da relação. Aceite: trilha manual funciona sem modelo; cada passagem preserva seu Locator e sua fonte.

**RF-028 — Gerar sínteses e respostas sobre o acervo [P · Evolução].** Recuperar fontes autorizadas e produzir resultado identificado como geração automática. Aceite: citações abrem fontes reais; ausência de evidência é declarada; texto recuperado não recebe autoridade para alterar configuração ou executar ações.

**RF-029 — Exportar conhecimento [P · Evolução].** Oferecer Markdown e JSON, com opções de YAML frontmatter, Wikilinks, callouts e, para grafos, Canvas. Aceite: prévia corresponde ao arquivo gerado; exportação contém somente dados autorizados e links estáveis às fontes.

**RF-030 — Servir catálogo OPDS [P · Evolução].** Publicar catálogo autenticado e acesso autorizado aos arquivos. Aceite: cliente da matriz de compatibilidade descobre e obtém publicação; obter o livro não muda seu progresso.

**RF-031 — Sincronizar estado de leitura [C na separação de responsabilidades · Evolução].** Adaptadores externos devem converter posições para o estado canônico interno. Aceite: atualização concorrente não vence somente por ter maior porcentagem; conflitos são detectados e ficam recuperáveis.

**RF-032 — Consultar metadados externos [P · Evolução].** Provedores configuráveis devem retornar candidatos e origem por campo. Aceite: timeout ou limite de API permite cadastro manual e não bloqueia a leitura.

**RF-033 — Integrar telemetria opcional [P · Evolução].** Informações de ZFS e SMART podem complementar métricas básicas de armazenamento. Aceite: instalação em outro sistema de arquivos mantém catálogo, leitura e backup operantes.

**RF-034 — Exportar e restaurar dados operacionais [C quanto à política de backup (DEC-064) · Núcleo].** O Códice deve oferecer um comando local que gere um pacote consistente do banco e a lista de arquivos, e um procedimento de restauração que o verifique. Deve documentar como proteger banco, originais, configuração e dados pessoais com ferramentas externas, sem prometer agendamento próprio. Índices e derivados ficam fora do backup por padrão, por serem recriáveis. Critérios de aceite propostos: restauração em instância limpa recupera notas e abre arquivos; índices derivados podem ser reconstruídos; pacote corrompido ou incompatível é recusado antes de sobrescrever a instância; a criptografia opcional exige chave fornecida pelo operador; a interface de armazenamento e recuperação (UI-19) exibe a data do último backup registrado, quando houver.

**RF-035 — Administrar biblioteca compartilhada [C · Núcleo].** Owner e administradores gerenciam o acervo único e sua organização temática; todos os usuários autenticados podem consultá-lo. Aceite: uma obra aparece em várias categorias sem duplicação de arquivos; leitor pode abrir obra disponível, mas recebe recusa ao tentar ingerir ou retirar obra por API. Coleções pessoais não alteram a classificação global.

**RF-036 — Organizar coleções privadas [C · Núcleo].** Usuário deve criar e gerenciar coleções próprias com referências às obras. Aceite: outro usuário não obtém coleção por ID, listagem ou busca; excluir coleção ou item não altera obra, arquivo ou coleção de terceiros.

**RF-037 — Manter favoritos privados [C · Núcleo].** Usuário pode marcar e desmarcar obras como favoritas. Aceite: favoritos pertencem à conta e não alteram classificação global da biblioteca.

**RF-038 — Transferir ou recuperar titularidade [C · Núcleo].** Somente o owner pode transferir explicitamente a titularidade da instância a outra conta enquanto autenticado. Se todas as formas de acesso do owner forem perdidas, a recuperação exige procedimento local de emergência no servidor. Aceite proposto: transferência é atômica; requisições concorrentes não geram múltiplos owners; tentativa remota de admin é recusada; falha preserva o owner anterior; recuperação não é exposta por API remota nem pela interface de admin comum e gera registro de auditoria. Conforme DEC-057 e DEC-058: a recuperação é um comando executado no servidor, em dois casos (redefinição de acesso do owner e transferência quando o owner está indisponível), sem variável de ambiente nem arquivo-gatilho, e sempre auditada. Na transferência normal, o owner escolhe se o antigo owner passa a admin ou leitor.

Confirmado em 19 de setembro de 2026: a transferência normal ocorre em duas etapas. O owner a inicia e confirma com a senha; a conta de destino, ativa, aceita reautenticando-se; até a aceitação nada muda e o owner pode cancelar. O link de redefinição da recuperação local é de uso único, válido por 1 hora e exibido apenas no terminal.

Proposta técnica, sujeita a revisão: na transferência por recuperação local não há owner presente para escolher. O operador informa o papel do antigo owner ao executar o comando e, se omitido, a conta passa a leitor, por ser a opção mais conservadora quando o acesso pode ter sido comprometido. O novo owner pode ajustar o papel depois. Aceite adicional: a redefinição encerra todas as sessões do owner; comando sem acesso local ao servidor não produz efeito.

**RF-039 — Preservar notas de fontes retiradas [C · Núcleo].** Retirar uma obra do catálogo ou excluir seus arquivos não deve excluir as notas dos usuários. A nota deve manter ao menos título da obra e autor como referência bibliográfica persistente, mesmo quando não for possível consultar a fonte. Aceite: texto, autoria pessoal e privacidade permanecem; consulta e exportação exibem título, autor e indicação de fonte indisponível; não há exclusão em cascata das notas nem perda da referência bibliográfica por remoção de vínculos. Para fonte originalmente sem autor identificado, exibir essa ausência explicitamente, sem inventar autoria.

**RF-040 — Organizar categorias e tags [C · Núcleo].** Owner/admin devem gerenciar categorias com subcategorias e tags globais. Usuários devem poder gerenciar tags pessoais privadas sem alterar a classificação compartilhada. Aceite proposto: uma obra aparece em múltiplas categorias e recebe tags transversais; leitor não altera categorias ou tags globais pela API; tags pessoais não aparecem em buscas ou respostas de outro usuário. A hierarquia deve rejeitar ciclos.

**RF-041 — Escolher versão da obra para leitura [C · Núcleo].** A ficha da obra deve apresentar edições, idiomas e arquivos/formatos disponíveis e permitir a escolha antes de abrir o leitor. Idioma pertence à edição; formato pertence ao arquivo. Aceite proposto: escolher uma edição em português em PDF abre esse arquivo; escolher uma edição em inglês em EPUB abre a outra versão, sem substituir a posição da primeira. A interface deve distinguir edições do mesmo idioma por seus metadados disponíveis, como editora, ano ou tradução. Critérios de seleção automática ao retomar leitura serão definidos separadamente.

**RF-042 — Retomar em posição equivalente entre versões [C na capacidade · Evolução].** Usuário deve poder solicitar continuidade entre arquivos, formatos, edições ou idiomas da mesma obra, com análise da posição de origem e do conteúdo de destino. Isso é opcional e não substitui o progresso independente por arquivo. Critérios de aceite propostos: sem solicitação, abrir a posição própria do destino ou seu início; apresentar correspondência com sua precisão e contexto; ausência ou ambiguidade não altera o progresso; aceitar uma sugestão permite abrir o destino sem modificar a posição de origem. Heurística de capítulo deve funcionar sem modelos; OCR e IA respeitam controles de processamento, disponibilidade e política local/remota.

**RF-043 — Retomar entre livro e audiobook [C na capacidade · Evolução futura].** Em etapa posterior à retomada entre versões textuais, usuário deve poder solicitar continuidade do livro para o audiobook e do audiobook para o livro. O sistema deve relacionar um Locator textual a uma posição temporal real no áudio, apresentando a correspondência e sua precisão para confirmação. Critérios de aceite propostos: capítulos, introduções ou versões abreviadas não autorizam conversão direta por porcentagem; ausência de correspondência preserva ambas as posições; aceitar um candidato abre o destino e mantém o progresso de origem. Transcrição e método de alinhamento permanecem decisões técnicas futuras, respeitando os controles de modelos e consumo do host.

**RF-044 — Reorganizar armazenamento gerenciado [C · Núcleo].** Owner/admin pode solicitar reorganização dos caminhos conforme os metadados atuais, revisar uma prévia e executar explicitamente. Critérios de aceite propostos: editar título ou autor não move arquivos; reorganizar preserva IDs, bytes, notas e progresso; conflitos não sobrescrevem destinos; caminhos permanecem nas raízes autorizadas; falha deixa estado recuperável. O modo referenciado não é reorganizado por essa operação.

**RF-045 — Gerenciar lixeira e exclusão definitiva [C · Núcleo].** Owner/admin deve poder enviar arquivos gerenciados para a lixeira, restaurar itens e esvaziá-la explicitamente. Somente owner configura a limpeza automática, desativada por padrão. Quando habilitada, o vencimento do prazo leva à exclusão definitiva dos itens elegíveis. Critérios de aceite propostos: lixeira não duplica conteúdo; interface mostra espaço ocupado; item restaurado não é apagado por job pendente; notas, título da obra e autor permanecem após limpeza; arquivo externo referenciado não é removido; falha de remoção não é reportada como espaço liberado.

**RF-046 — Controlar orçamento de provedores externos [C · Evolução].** Owner deve poder configurar limite mensal global e limites opcionais por área, acompanhar consumo e distinguir custos apurados de estimados. Ao atingir um limite aplicável, impedir novas chamadas externas cobertas por ele. Gestão e visualização financeira devem existir somente na página exclusiva do owner. Critérios de aceite propostos: leitor e admin não acessam dados financeiros pela interface nem pela API; alcançar limite global bloqueia novas chamadas em todas as áreas cobertas; limite de área bloqueia essa área; estimativas são rotuladas; telas de uso informam apenas indisponibilidade operacional, sem valores ou relatórios financeiros.

**RF-047 — Controlar cadastro e provisionamento no login [C · Núcleo e evolução].** Cadastro público deve permanecer desativado por padrão; owner/admin pode criar contas ou emitir convites. Owner controla por provedor OIDC/LDAP se uma identidade autenticada ainda desconhecida pode criar conta automaticamente como leitor. Critérios de aceite propostos: com o controle ativo, primeiro login externo válido cria uma conta e seu vínculo; logins seguintes reutilizam essa conta; com o controle inativo, identidade desconhecida não cria conta, e contas já vinculadas seguem a política de acesso vigente. Cadastro local público desativado não impede provisionamento externo expressamente habilitado. Nenhum primeiro login concede admin ou owner automaticamente. Grupos externos não promovem contas: promoção para admin ocorre somente por ação do owner dentro do Códice, e owner somente por transferência explícita.

**RF-048 — Emitir e revogar convites [C · Núcleo].** Owner/admin deve poder emitir convites de uso único para leitores, com validade de sete dias, e revogá-los antes do resgate. Somente owner pode emitir convite que cria conta admin; nenhum convite cria owner. Critérios de aceite propostos: convite expirado, usado ou revogado não cria conta; resgate concorrente cria no máximo uma conta; convite de admin emitido por admin comum é recusado; auditoria registra emissor, papel pretendido, validade, revogação e uso, sem registrar token em texto aberto.

**RF-049 — Redefinir senha local sem SMTP [C no fluxo · Núcleo].** Sem SMTP configurado, quem esqueceu a senha local envia um pedido de redefinição pela tela de login. O pedido chega a owner e admins, e um deles o aprova e repassa por fora do Códice a informação de acesso à pessoa que solicitou (DEC-063). Propostas técnicas, sujeitas a revisão: a aprovação gera um link de redefinição de uso único, válido por 1 hora, como no fluxo de recuperação do owner; o pedido responde de forma idêntica exista ou não a conta, para não revelar quais contas existem; há limite de pedidos por origem e por conta e pedidos repetidos se consolidam; pedidos de contas admin são vistos e aprovados somente pelo owner, coerente com DEC-056; a redefinição encerra as sessões e os tokens de clientes da conta; o aprovador confirma a identidade da pessoa por seus próprios meios antes de repassar o link. Aceite proposto: link usado, expirado ou pedido rejeitado não altera a senha; a auditoria registra solicitante, aprovador, decisão e uso, sem registrar o link em texto aberto. Com SMTP opcional habilitado pelo owner, o envio automático do link ao e-mail da conta é evolução possível; sem SMTP o fluxo acima permanece completo. Contas exclusivamente OIDC/LDAP redefinem a senha no provedor.

## 8 Regras de negócio



Todas as regras abaixo são propostas de formalização, exceto as restrições expressamente cobertas por DEC confirmadas.

| ID | Regra | Consequência observável |
| --- | --- | --- |
| RN-001 | Preservar o original | OCR, capas e conversões geram derivados separados |
| RN-002 | Obra, edição e arquivo possuem identidades distintas | Correção de título não muda identidade nem perde notas |
| RN-003 | Deduplicação física não decide equivalência bibliográfica | Hash idêntico não gera nova cópia; título, autor ou ISBN semelhantes exigem revisão de vínculo; formatos, edições e idiomas distintos podem coexistir na mesma obra |
| RN-004 | Retirada reversível do catálogo e exclusão física são ações distintas | Owner/admin pode retirar e restaurar; exclusão de arquivos gerenciados exige ação adicional confirmada e checagem de referências; retirada de item referenciado não apaga a fonte externa |
| RN-005 | Fonte referenciada permanece sob controle externo | Scanner não move nem apaga o arquivo |
| RN-020 | Transferência explícita para gerenciado pode remover a origem verificada | Exceção ao modo referenciado somente na mudança de modo autorizada; destino íntegro e vínculo confirmado antes da remoção. Upload não apaga fonte do cliente |
| RN-021 | Raízes de armazenamento são configuradas pelo owner | Admins não escolhem caminhos fora das raízes permitidas |
| RN-006 | Nota, progresso, preferência e grafo pessoal pertencem ao usuário | Autorização acompanha consultas, jobs e exportações |
| RN-007 | Ingestão inicial exige papel administrador | Confirmada por DEC-003 |
| RN-008 | Metadado confirmado não é sobrescrito silenciosamente | Manter candidato, origem e histórico da resolução |
| RN-009 | Similaridade e geração são sugestões | Pontuação não equivale a verdade ou probabilidade de correção |
| RN-010 | Localização pertence a arquivo e versão | Mudança de arquivo exige reconciliação e preservação do histórico |
| RN-011 | Progresso pode diminuir legitimamente | Voltar capítulos não é erro nem perde para maior porcentagem |
| RN-012 | Falhas opcionais não bloqueiam o núcleo | Ler não depende de OCR, vetor ou LLM concluído |
| RN-013 | Dados derivados são versionados e reconstruíveis | Invalidar derivados por versão de fonte e configuração |
| RN-014 | Envio externo exige configuração explícita | Nunca realizar fallback silencioso para nuvem |
| RN-015 | Exatamente um owner após inicialização | Transferência explícita; admin não remove nem rebaixa owner |
| RN-022 | Papel admin só muda por ação do owner | Admin não promove, rebaixa, bloqueia nem remove outro admin; verificação aplicada na API |
| RN-019 | Retirada de obra preserva notas pessoais | Fonte indisponível identificada; conteúdo e privacidade preservados |
| RN-016 | Aceitar sugestão cria registro de autoria e proveniência | Preservar quem aceitou e quais evidências sustentaram a ação |
| RN-017 | Desligar IA não apaga dados existentes | Novos jobs param; retenção e exclusão são ações separadas |
| RN-018 | Obra disponível pode ter processamento parcial | UI diferencia legibilidade, indexação textual e enriquecimento |

## 9 Requisitos não funcionais

Os critérios a seguir são propostas verificáveis; valores de capacidade e desempenho ainda dependem de benchmark. Não representam medições do backend.

**RNF-001 — Auto-hospedagem [C].** Núcleo deve executar em infraestrutura do usuário. Verificação: instalação documentada a partir de ambiente limpo, sem conta de serviço externo obrigatória.

**RNF-002 — Independência de acelerador [C como direção].** Biblioteca, leitura, anotações e busca convencional não devem exigir GPU. Verificação: executar esses fluxos em perfil apenas CPU, com IA desligada; registrar RAM e CPU consumidas.

**RNF-003 — Consumo controlado [P].** Workers devem aceitar concorrência, tamanho de lote, timeout e limites de recursos. Verificação: ingestão em lote mantém API responsiva e respeita limites declarados; excesso de memória interrompe o job de forma recuperável.

**RNF-004 — Desempenho mensurável [P].** Definir corpus, número de usuários, hardware e estado dos caches antes de fixar metas. Hipótese inicial para validação: p95 de catálogo até 1 s e busca textual até 2 s, em LAN, com 10 mil obras e cinco usuários concorrentes. Abertura do leitor e tempo de OCR terão metas por formato e hardware. QA-016 bloqueia tratá-las como compromissos.

**RNF-005 — Isolamento multiusuário [P].** Aplicar autorização no servidor a arquivos, notas, resultados, embeddings, grafos e caches. Verificação: suíte com dois usuários tenta acessar recursos privados do outro em cada superfície.

**RNF-006 — Segurança de credenciais [P].** Senhas locais recebem hash adaptativo adequado; segredos externos não devem aparecer em logs ou respostas. Verificação: revisão da configuração de sessão, proteção contra abuso de login e revogação. Algoritmo e parâmetros pertencem à decisão técnica de implementação.

**RNF-007 — Processamento de arquivos não confiáveis [P].** Extratores operam com privilégios mínimos, diretório temporário isolado e limites de expansão, páginas e tempo. Verificação: arquivo inválido, caminho malicioso em arquivo compactado e conteúdo que excede quota não escapam da área de trabalho.

**RNF-008 — Consistência e recuperação [P].** Publicação de arquivos e resultados deve resistir a falhas entre escrita e commit. Verificação: reinício em cada etapa deixa estado recuperável, sem apontar resultado concluído para arquivo parcial.

**RNF-009 — Observabilidade [P].** Logs estruturados devem correlacionar importação, job e tentativa, com métricas de filas, erros e armazenamento. Verificação: administrador encontra causa operacional sem exposição de texto de notas, tokens ou conteúdo integral por padrão.

**RNF-010 — Backup verificável [P].** Incluir banco, arquivos originais, configuração e dados pessoais; proteger segredos no backup. Verificação: ensaio de restauração com checagem de hashes e retomada de leitura. Metas de RPO, RTO e retenção estão em DEC-064: backup diário, restauração em algumas horas e retenção sugerida de 7 diários, 4 semanais e 3 mensais. Nenhuma ferramenta de backup externa é exigida.

**RNF-011 — Portabilidade [P].** Oferecer exportação de dados pessoais e configuração documentada de volumes. Verificação: migração de diretório/host conserva identificadores e links lógicos. Lista de arquiteturas de CPU e sistemas suportados depende de homologação.

**RNF-012 — Acessibilidade e responsividade [P].** Interface deve permitir teclado, foco visível, contraste adequado, rótulos acessíveis e operação móvel. Verificação: fluxos essenciais completos sem mouse; preferência editorial de cor não deve inviabilizar acessibilidade.

**RNF-013 — Modularidade [P].** Extratores e provedores devem ter contratos substituíveis. Verificação: trocar provedor de embedding não exige alterar entidades de nota ou progresso.

**RNF-014 — Atualização e migrações [P].** Mudanças de esquema e processamento devem ser versionadas, documentadas e acompanhadas de caminho de recuperação. Verificação: migração em cópia do banco preserva conteúdo; recuperação por backup é ensaiada quando rollback direto não é seguro.

**RNF-015 — Operação local [P].** Núcleo deve funcionar sem acesso à internet após instalação das dependências necessárias. Verificação: bloquear saída de rede e realizar leitura, notas, busca textual e administração local.

**RNF-016 — Licenciamento e distribuição [P].** Código, pesos de modelos, fontes e dependências devem possuir inventário e condições compatíveis com a forma de distribuição escolhida. Verificação: revisar versões efetivamente distribuídas; o repositório contém AGPLv3; política de dependências e modelos permanece em QA-022.

## 10 Arquitetura inicial

### 10.1 Organização proposta

Adotar inicialmente um **monólito modular com workers separados por processo**. Essa organização permite reduzir serviços obrigatórios em homelabs e, ao mesmo tempo, isolar tarefas intensivas. É uma hipótese a confrontar com o backend; linguagem, framework e broker não estão escolhidos neste documento.

Fluxo principal: **interface web e clientes → API autenticada → serviços de domínio → banco e storage**. Processamento pesado segue **registro da tarefa → fila persistente → worker → publicação de resultado versionado**. O frontend consulta o estado do processamento sem manter a requisição original aberta.

| Módulo | Responsabilidade | Limite de dependência |
| --- | --- | --- |
| Identity | Contas, papéis, identidades e sessões | Provedores autenticam; domínio autoriza |
| Library | Obras, edições, arquivos, metadados e disponibilidade | Não depende de resultados de IA |
| Ingestion | Validação, revisão e publicação | Coordena; extratores trabalham por contrato |
| Reader | Posição, preferências e acesso ao conteúdo | Consome Locators versionados |
| Marginalia | Notas, destaques e marcadores | Escopo pessoal aplicado em todas as saídas |
| Processing | Extração, OCR, capas e segmentação | Produz derivados rastreáveis |
| Search | Metadados, texto e semântica | Aplica autorização antes de retornar conteúdo |
| Knowledge | Grafo, trilhas e comparação | Relações manuais independentes de modelos |
| Integrations | OPDS, sync, PKM e metadados externos | Adaptadores não redefinem o estado canônico |
| Operations | Jobs, configuração, auditoria e saúde | Privilégios administrativos explícitos |

PostgreSQL com busca textual é candidato para reduzir dependências; pgvector é candidato para busca vetorial no mesmo ambiente. A documentação desses projetos confirma as capacidades gerais, mas a escolha para o Códice depende de avaliação de carga e do código existente [T1, T2]. Não há obrigação inicial de banco de grafos ou serviço vetorial separado.

### 10.2 Base tecnológica existente

A análise inicial identificou Go/Chi na API, PostgreSQL, Redis Streams para ingestão, PubSub para eventos e workers Python. React compõe o frontend. Manter essa base nesta etapa, priorizando contratos, autorização e recuperação de jobs. Redis não substitui persistência canônica de estado e decisões no banco.

Não há evidência medida de que trocar a stack resolveria melhor as lacunas atuais. Reavaliar apenas diante de problema concreto de consumo, confiabilidade ou manutenção, com comparação reproduzível. A arquitetura detalhada da implantação permanece pendente.

### 10.3 Contratos entre componentes

O domínio publica fatos como “arquivo incorporado”, “texto extraído” e “fonte removida”. Consumidores devem tolerar repetição e mensagens atrasadas. Uma transação de banco não deve pressupor que armazenamento e broker participam da mesma transação; usar registro durável de trabalho ou padrão de outbox conforme a implementação escolhida.

Requisições longas retornam identificador da operação. Downloads respeitam autorização e suporte de entrega adequado ao formato. Todos os adaptadores passam pela mesma camada de autorização. IDs internos estáveis devem evitar acoplamento com nomes físicos ou IDs de provedores externos.

### 10.4 Implantação recuperável

Propostas técnicas, sujeitas a revisão, para a implantação com Docker Compose ou equivalente:
- **Volumes persistentes:** banco, armazenamento gerenciado (originais) e configuração com segredos. Redis não é fonte de dados e pode reiniciar sem volume; se usar persistência própria, ela é apenas otimização.
- **Workers sem estado local relevante:** reiniciar ou substituir um worker não perde trabalho, pois leases vencidos são recuperados (DEC-066).
- **Ordem de inicialização:** migrações versionadas concluídas antes de aceitar novos jobs (RNF-014); o reconciliador roda depois das migrações.
- **Saúde:** verificações de disponibilidade de banco, Redis, storage e workers aparecem isoladamente na administração (RF-021); indisponibilidade do Redis não impede leitura nem login.
- **Temporários:** staging e temporários ficam em volume próprio, com limpeza segura de órfãos, sem apagar originais.
- **Restauração:** após restaurar o banco a partir de backup, o reconciliador recria a fila a partir dos jobs registrados, sem depender do estado anterior do Redis (FL-11).

## 11 Autenticação e identidade

### 11.1 Conta canônica

Uma conta Códice pode possuir identidade local, OIDC e/ou LDAP. Proposta: identificar OIDC por emissor e sujeito estáveis; LDAP por provedor e identificador estável do diretório, com política para mudança de nome. Vinculação exige sessão autenticada e confirmação de domínio da identidade. E-mail coincidente não basta.

### 11.2 Fluxos e recuperação

OIDC é candidato prioritário para login web; LDAP atende diretórios existentes. Authentik é o cenário de integração citado pelo mantenedor, não uma dependência obrigatória. Testar disponibilidade, logout, expiração, vinculação e desativação usando a configuração real adotada.

Existe uma via local de recuperação administrativa, confirmada em DEC-054. Ela deve ter procedimento documentado, exigir acesso local à instância e não constituir credencial fixa de fábrica nem endpoint remoto. O administrador principal é owner, um papel próprio confirmado. Quantidade, poderes exclusivos e transferência explícita estão confirmados em DEC-022. O mecanismo de recuperação e o papel do antigo owner estão confirmados em DEC-057 e DEC-058 e detalhados em RF-038. Não substituir esse papel por simples proteção do último admin.

Cadastro público fica desativado por padrão. Owner/admin pode criar contas ou emitir convites. Convites têm uso único, validade de sete dias e podem ser revogados antes do resgate. Admin emite convites para leitores; somente owner emite convite para administrador. Convite não pode criar owner. Autenticação e políticas de cadastro continuam sob configuração exclusiva do owner.

Proposta técnica: convite armazena somente segredo verificável, emissor, papel pretendido, expiração, estado e auditoria. O resgate cria conta de forma atômica e consome o convite; requisições concorrentes não criam múltiplas contas. Conforme DEC-059, o padrão é um link avulso entregue fora do Códice; quem emite pode restringir o resgate a um e-mail informado. Sem SMTP configurado, a interface exibe o link para ser copiado; com SMTP opcional, o owner pode habilitar o envio, sem que nenhum fluxo essencial dependa dele. Uso único, validade de sete dias e revogação valem nos dois casos. Convite com e-mail informado recusa o resgate por outro endereço.

Para cada provedor OIDC/LDAP, oferecer ao owner “Permitir criar conta no primeiro login”. Quando habilitado, autenticação externa válida de identidade ainda desconhecida cria uma conta Códice com papel leitor e registra o vínculo estável. Isso atende ao cenário em que alguém já possui conta no Authentik e ainda não possui conta no Códice. A criação ocorre ao entrar no Códice; sincronização antecipada de todo o diretório ou provisionamento iniciado pelo provedor exigiriam integração adicional e não estão implícitos nesta decisão.

Proposta técnica: manter o controle desativado até configuração explícita; criar conta e vínculo atomicamente, com unicidade da identidade para resistir a logins concorrentes. Quando a identidade externa validada tiver e-mail igual ao de uma conta local, apresentar vínculo assistido antes de criar outra conta. A pessoa confirma que é titular da conta por sessão local ativa ou senha local válida; nesse caso, vincular a identidade externa à conta existente. O e-mail sozinho não é prova suficiente.

Se a pessoa recusar, não puder comprovar a conta local ou o método local não estiver disponível, criar um caso para análise de owner/admin, sem vincular ou fundir contas automaticamente. A análise pode aprovar vínculo, orientar criação de conta separada ou resolver uma duplicidade existente. “Fusão” nesse cenário normalmente significa adicionar a identidade externa à conta local já existente, preservando seus dados; duas contas Códice independentes exigem ferramenta administrativa explícita e futura para unir dados pessoais de modo auditável. Com o controle de criação automática desativado, identidade desconhecida sem coincidência de e-mail informa necessidade de convite ou análise, sem criar conta. Desativar a opção não apaga contas existentes nem substitui sua política de bloqueio.

Mapeamento de grupos externos para papéis está fora do escopo: o provisionamento automático cria somente leitores e elevação para administrador ocorre somente dentro do Códice, por ação do owner. Owner é atribuído somente pela transferência explícita da instância. Grupos externos podem futuramente auxiliar em diagnóstico ou seleção de acesso, sem autoridade para atribuir papéis. MFA e política de expiração continuam em aberto.

Login por formulário único (DEC-072 a 075). Propostas técnicas, sujeitas a revisão. O backend decide nesta ordem: (1) conta local sem vínculo externo confere a senha local; (2) conta vinculada a uma identidade LDAP confere no diretório; (3) nome desconhecido, com o provedor habilitado para criar contas no primeiro login, tenta o diretório e, se confere, cria um leitor; (4) qualquer outra falha responde igual. A conta do owner nunca segue os passos 2 e 3.

Para a coincidência de nome (DEC-075), o Códice consulta o diretório com a conta de serviço, sem enviar a senha digitada, para saber se existe entrada com aquele nome. Só então tenta o bind com a senha informada. Se o bind confere e existe conta local sem vínculo, nenhuma sessão é criada: o servidor devolve um pedido de vínculo com um ticket de uso único e vida curta, o cliente pede a senha local, e somente quando ela confere a identidade é gravada e a sessão aberta. As tentativas são limitadas por conta e por origem. A identidade externa é o par provedor e identificador estável do diretório (por exemplo, `entryUUID`), nunca o nome de usuário, de modo que renomear no diretório não cria outra conta. O LDAP exige LDAPS ou StartTLS, timeouts, escape dos valores usados em filtros e uma conta de serviço com leitura mínima.

Clientes OPDS de contas LDAP também usam tokens de aplicativo (DEC-071), de modo que a senha do diretório só trafega no formulário de login. Provedor indisponível é um estado operacional distinto de credencial inválida. Conta bloqueada e credencial inválida têm a mesma resposta pública, com o motivo na auditoria: isso afina o critério de aceite de RF-003, que pedia distinguir “conta desativada”.

Revogação de acesso: bloquear uma conta é reversível, separado da exclusão, e revoga imediatamente sessões e tokens de clientes, preservando notas e progresso (DEC-060). Para contas OIDC/LDAP, o Códice revalida a identidade no provedor com teto de 24 horas, ajustável pelo owner: conta desativada no provedor perde o acesso no máximo após esse prazo, e o bloqueio manual atua de imediato (DEC-061). Proposta técnica: quando o provedor estiver indisponível na revalidação, tratar como falha de renovação e encerrar a sessão apenas após o teto, sem apagar dados. Aceite proposto: conta bloqueada não obtém nem renova token; provedor que reporta conta desativada encerra a sessão local na próxima revalidação.

## 12 Storage e integridade

### 12.1 Biblioteca gerenciada

Novas importações devem seguir o padrão gerenciado, conforme DEC-005, com pastas legíveis e identificadores estáveis no banco, conforme DEC-036. Exemplo ilustrativo: `Frank Herbert/Duna/Português — Aleph — 2017/Duna.epub`. Obra, edição e arquivo possuem IDs próprios; o caminho é uma localização mutável. Notas, progresso e referências internas usam essas identidades e não dependem do nome físico.

Corrigir metadados não move nem renomeia automaticamente arquivos já incorporados. Owner/admin pode solicitar “Reorganizar armazenamento”, conferir uma prévia com caminhos atuais, destinos e conflitos e executar a operação dentro das raízes permitidas. A estrutura inicial é aplicada na incorporação; mudanças posteriores dependem dessa ação explícita. Reorganizar somente nomes e caminhos não cria nova versão do conteúdo.

Detalhamento técnico proposto: tratar caracteres incompatíveis, limites de caminho, autoria múltipla, metadados ausentes e colisões de nomes sem sobrescrever arquivos. Revalidar a prévia antes da execução, atualizar localização e registrar recuperação em caso de falha entre movimentação física e confirmação no banco. Alterações externas devem ser reconciliadas e não presumidas como exclusão definitiva. Padrão confirmado em DEC-065: `Autor/Obra/Idioma — Editora — Ano/Arquivo`, com pasta de série para séries. A pasta reflete um único autor por limitação física: um arquivo não pode estar em várias pastas sem duplicar bytes, o que DEC-027 e DEC-033 vedam. A autoria completa fica no catálogo: a navegação e os filtros por autor listam a obra sob cada autor creditado. “Vários autores” é apenas rótulo de pasta e nunca vira registro de autor, filtro ou seção da biblioteca.

Propostas técnicas, sujeitas a revisão:
- **Pasta de autor.** Usar o primeiro autor creditado. Obra com mais de 3 autores, como uma antologia, usa a pasta `Vários autores`; todos os autores continuam vinculados no banco.
- **Pasta de série.** Obras de quadrinhos e mangá com posição em série usam `Série/NN - Título`; livros de uma série permanecem no caminho por autor e a série fica apenas no catálogo. Confirmar essa fronteira na implementação.
- **Campos ausentes.** Autor ausente usa `Autor desconhecido`; idioma, editora ou ano ausentes são omitidos do nome da pasta, sem placeholders. O título mínimo continua obrigatório (DEC-026).
- **Caracteres.** Substituir ou remover `\ / : * ? " < > |`, normalizar Unicode preservando acentos e cortar espaços e pontos finais.
- **Tamanho.** Limitar o caminho relativo a cerca de 200 caracteres, truncando o título e mantendo o sufixo de desempate. O valor é conservador e independe do acesso do mantenedor, que usa SFTP/FTP; protege quem acessa por SMB ou baixa arquivos em Windows.
- **Colisões.** Nunca sobrescrever. Caminhos iguais recebem um sufixo curto e estável derivado do ID do arquivo, por exemplo `Duna [a3f9].epub`, sem depender da ordem de chegada.

O owner configura o destino gerenciado e as raízes de origem permitidas. Admins realizam importações dentro desses limites. A organização gerenciada conserva os bytes originais, sem exigir uma segunda cópia permanente da origem. Conversões, OCR e outros derivados continuam separados e não substituem esses bytes.

Em importações de diretórios acessíveis ao servidor, o fluxo transfere o arquivo, verifica integridade, publica no destino e confirma o vínculo antes de remover a origem. Cruzar volumes pode exigir cópia temporária e verificação; no mesmo sistema de arquivos, movimentação pode evitar duplicação temporária, com recuperação registrada para falhas entre movimentação e confirmação no banco. A operação deve sempre conservar ao menos uma cópia válida e recuperável.

Antes de remover uma origem copiada, verificar que ela ainda corresponde ao conteúdo transferido; se tiver mudado, preservar a fonte e sinalizar conflito. Falta de permissão para removê-la mantém o destino íntegro e registra limpeza pendente. Não anunciar transferência integralmente concluída enquanto a origem permanecer por falha de limpeza. Repetições do job não devem apagar caminhos reutilizados por outro arquivo.

No upload pelo navegador, o servidor só controla o arquivo recebido e seus temporários; não pode remover a fonte no computador do remetente. A política de exemplar final único aplica-se ao armazenamento sob gestão do servidor, sem prometer eliminar cópias externas ou backups. Detecção de hash idêntico, por si só, não autoriza apagar uma fonte fora de uma transferência gerenciada explícita.

### 12.2 Biblioteca referenciada

O scanner registra arquivos existentes em vários diretórios permitidos pelo owner, sem reorganizá-los. Permissões de leitura bastam para catalogação. Detectar alteração, remoção e reaparecimento; metadados de data/tamanho auxiliam o scanner, mas a validação de conteúdo depende de hash quando necessário.

Se um arquivo mudar, criar nova versão ou estado de reconciliação; não atribuir as antigas notas à nova versão sem avaliação. Oferecer relocalização e a ação explícita “Mover para o armazenamento gerenciado”, reutilizando o fluxo de incorporação da seção 12.1. Não existe modo “adotado” separado: após transferência concluída, o arquivo é gerenciado e a origem foi removida de forma verificada.

### 12.3 Ciclo de vida dos dados

Separar originais, staging/quarentena, derivados, temporários, exportações e backups. Não haverá cotas por usuário nesta etapa (mantenedor, 19 de setembro de 2026): leitores não ingerem arquivos e o uso de espaço é visível ao owner/admin. Limites técnicos de tamanho e expansão de arquivos são tratados em RNF-007 e QA-023, e a limpeza de temporários deve ser segura. Coleta de derivados órfãos deve verificar referências e jobs ativos. Anotações humanas e decisões de revisão não são cache descartável.

Retirar do catálogo oculta a obra da biblioteca e permite restaurá-la, preservando arquivos e notas. Exclusão física de arquivos gerenciados é ação administrativa adicional, com confirmação explícita e checagem de referências para não apagar bytes ainda utilizados por outro vínculo. Retirada de item referenciado nunca apaga sua fonte externa. A transferência explícita de referenciado para gerenciado continua seguindo seu fluxo próprio, definido em DEC-034.

Notas mantêm título da obra e autor mesmo após a fonte tornar-se indisponível. Proposta técnica: persistir uma referência bibliográfica de apoio na nota ou em registro durável separado, sem exclusão em cascata, preenchida ao vincular a fonte. A leitura e exportação da nota não podem depender exclusivamente de JOIN com uma obra ativa. Preservar também o identificador da fonte quando possível, para futura reconciliação; não vincular automaticamente outra obra apenas por coincidência de título e autor.

Conforme DEC-040, enquanto a obra estiver disponível, correções confirmadas de título e autor atualizam a referência bibliográfica das notas vinculadas. Essa atualização não altera o texto pessoal, a citação salva ou a autoria da nota e não concede acesso administrativo ao conteúdo privado. Ao retirar a obra, preservar os últimos valores confirmados; sugestões ainda não aprovadas não os substituem. Critérios de aceite propostos: corrigir autoria no catálogo reflete-se na referência da nota e em sua exportação; retirada posterior mantém os valores corrigidos; correção concorrente com retirada não deixa a referência incompleta ou dependente da obra ativa.

Excluir arquivo gerenciado envia-o para uma lixeira recuperável do Códice, sem criar outra cópia de conteúdo. Owner/admin pode restaurar itens ou esvaziar a lixeira explicitamente. A interface informa espaço ocupado e, quando houver limpeza agendada, a previsão de exclusão definitiva. Retirar obra do catálogo não envia seus arquivos automaticamente para a lixeira.

A limpeza automática fica desativada por padrão. O owner pode habilitá-la e configurar o prazo de retenção; vencido esse prazo, os itens elegíveis são excluídos definitivamente pelo serviço de limpeza. A exclusão definitiva impede restauração pela lixeira do Códice, sem prometer remover backups ou snapshots externos. Nenhuma limpeza apaga notas ou sua referência bibliográfica, nem arquivos externos referenciados.

Ao habilitar a limpeza automática, sugerir 30 dias de retenção, ajustáveis pelo owner, sem habilitação implícita. O prazo é contado desde a entrada do arquivo na lixeira. Alterações de prazo valem apenas para novos itens; aplicar uma política a itens existentes exige ação explícita do owner, com prévia dos itens afetados e daqueles que já estarão vencidos. Itens recebidos enquanto a limpeza estiver desativada não ganham vencimento retroativo só por sua habilitação; sua inclusão exige essa mesma ação explícita.

Detalhamento técnico proposto: registrar entrada na lixeira e prazo aplicável por item, revalidar estado e referências antes de apagar, impedir disputa entre restauração e limpeza e registrar sucesso ou falha. Item restaurado não pode ser apagado por um job antigo. Falha de remoção mantém estado pendente e contabilização do espaço até confirmação. Critérios de aceite propostos: reduzir prazo não muda o vencimento de itens antigos; prévia de aplicação retroativa identifica os já vencidos; contagem usa a entrada original na lixeira, não a data de alteração da configuração. Metas e retenção de backup permanecem em QA-020. ZFS não é requisito para a biblioteca funcionar.

## 13 Ingestão e quarentena

### 13.1 Estados propostos

**Recebido → validando → aguardando revisão ou pronto para incorporação → incorporado**. Rejeitado e falhou são estados separados. Processamentos posteriores mantêm estado próprio: pendente, executando, concluído, parcial, falhou, cancelado ou ignorado por configuração.

Disponibilidade para leitura não deve depender da conclusão de OCR ou embeddings. O resultado da ingestão informa quais capacidades estão prontas. Quarentena editorial por metadado ambíguo deve ser diferenciada de bloqueio técnico por arquivo inválido.

### 13.2 Etapas

1. Receber arquivo com autoria da operação e limites de tamanho.
2. Validar caminho, formato detectado, estrutura e segurança básica.
3. Calcular hash e localizar duplicatas físicas e candidatos bibliográficos.
4. Extrair metadados internos e, se permitido, consultar fontes externas.
5. Apresentar divergências relevantes para revisão administrativa.
6. Incorporar obra/edição/arquivo e confirmar localização íntegra.
7. Enfileirar capas, extração textual e etapas opcionais habilitadas.
8. Atualizar disponibilidade e registrar falhas recuperáveis por etapa.

Arquivo inválido não avança para leitores ou extratores comuns. Arquivo válido com título mínimo pode ser incorporado sem revisão manual de toda importação, desde que não haja suspeita de duplicidade pendente. Possíveis duplicatas aguardam revisão administrativa antes de nova incorporação; uma obra já disponível não é bloqueada por outra importação suspeita.

Metadados extraídos diretamente do arquivo preenchem campos ainda vazios, com proveniência registrada. Valores de provedores externos, OCR ou IA são candidatos sujeitos a aprovação administrativa. Campos confirmados manualmente nunca são sobrescritos automaticamente. A ausência de aprovação de editora, ano ou outro enriquecimento não impede a leitura do arquivo incorporado. Essa aprovação trata de metadados bibliográficos; a execução de OCR e a indexação textual seguem os controles próprios de processamento.

QA-015 está resolvida quanto ao fluxo. DEC-027 a DEC-029 distinguem duplicata física, versões legítimas e suspeita bibliográfica. Hash idêntico encerra a tentativa de armazenar nova cópia e aponta ao registro existente; não se exige nova incorporação só para informar a duplicidade. Semelhança de título, autor ou ISBN encaminha o vínculo para revisão administrativa, sem fusão automática. Heurísticas de suspeita e a forma de obter o título mínimo ainda exigem detalhamento técnico, sem alterar essa política.

## 14 Workers e filas

### 14.1 Inventário proposto

| Worker | Entrada | Saída | Dependência e falha |
| --- | --- | --- | --- |
| Scanner | Raiz referenciada ou monitorada | Candidatos de ingestão/alteração | Falta de permissão é diagnosticada sem apagar catálogo |
| Validação e hash | Arquivo recebido | Formato, hash, relatório | Limites e arquivo corrompido interrompem incorporação |
| Metadados e capa | Arquivo e provedores permitidos | Candidatos e miniaturas | Falha externa permite revisão manual |
| Extração textual | Arquivo válido | Texto nativo e estrutura | Processar por unidade; registrar lacunas |
| OCR | Páginas/regiões selecionadas | Texto, coordenadas e confiança quando disponíveis | Falha preserva original e permite retomada |
| Segmentação e índice textual | Texto versionado | Segmentos e índice | Publicar nova geração sem misturar versões |
| Embeddings | Segmentos e modelo | Vetores versionados | Opcional; não bloqueia busca textual |
| Sugestões e geração | Fontes autorizadas e configuração | Candidatos, sínteses ou respostas | Revalidar permissões antes de publicar |
| Exportação e sincronização | Pedido/evento autenticado | Pacote ou estado conciliado | Idempotência e conflitos preservados |
| Reconciliação e limpeza | Estado de storage e jobs | Correções e remoção de temporários elegíveis | Não apagar originais referenciados |

### 14.2 Contrato de execução

Cada job deve ter id, tipo, alvo, versão da fonte, hash de configuração, autor/escopo, prioridade, estado, tentativas e datas. A chave de idempotência combina tarefa, fonte/versão e configuração relevante. Evitar depender de entrega “exatamente uma vez”; o desenho proposto tolera entrega repetida e publica cada resultado lógico uma vez.

Worker adquire lease, envia heartbeat e confirma conclusão somente após persistir o resultado. Lease vencido permite recuperação. Tentativas usam espera crescente e limite; erros permanentes vão para revisão, sem repetição infinita. Cancelamento é cooperativo, com descarte seguro de saída incompleta.

Confirmado em DEC-066 a 069: o estado dos jobs vive no PostgreSQL e o Redis Streams só entrega mensagens; a criação da obra e do job ocorre na mesma transação, com despacho posterior à fila; erros temporários têm até 3 tentativas com espera crescente e erros permanentes seguem direto para revisão; jobs falhos aguardam reexecução manual do admin; importações manuais têm prioridade e o padrão é um job pesado por vez. Propostas técnicas, sujeitas a revisão: um reconciliador roda na inicialização e periodicamente, recolocando na fila jobs pendentes ou com lease vencido; cada worker usa nome de consumidor único por instância, e mensagens pendentes de consumidores extintos são reivindicadas após o vencimento do lease; a fila de mensagens pendentes tem limite de entregas, e o excedente vira “falhou” no banco; o upload calcula hash integral, aplica limite real de tamanho do corpo e nunca reutiliza nome de arquivo em staging.

Tarefas devem ser divididas por página, capítulo ou lote quando útil. Checkpoints permitem retomada, mas publicação de uma geração precisa indicar se é completa ou parcial. Um job atrasado não pode sobrescrever uma versão mais nova.

### 14.3 Consumo e administração

Separar classes leves, OCR e modelos permite controlar concorrência. Importações manuais têm prioridade sobre varreduras em lote, OCR e embeddings, e o padrão é um job pesado por vez, conforme DEC-069. Começar conservadoramente em CPU e permitir limites por perfil. Pausar processamento durante leitura é uma opção proposta, não requisito de detecção automática já fechado. Capacidade da fila, backpressure, prioridades e política de starvation precisam ser testadas.

## 15 Extração documental e OCR

### 15.1 Finalidade

OCR converte conteúdo visual em texto utilizável por busca, citações e processamento posterior. Não estabelece sozinho autoria, título ou relações verdadeiras. EPUB e PDF com texto nativo utilizável seguem diretamente para segmentação; PDFs mistos podem usar OCR apenas nas páginas necessárias.

Saída conceitual: texto, idioma, página/região, coordenadas, ordem de leitura, motor e versão, confiança quando fornecida e ligação com a fonte. Ausência de confiança ou layout deve ser representada como ausência, nunca como valor fabricado. Escalas de confiança de motores distintos não são diretamente comparáveis.

### 15.2 Provedores candidatos

OCRmyPDF com Tesseract é candidato inicial para PDFs escaneados; OCRmyPDF adiciona camada textual pesquisável ao PDF [T3]. O produto deve armazenar esse PDF como derivado. Extração de regiões e coordenadas pode exigir saída específica do motor ou etapa complementar.

PaddleOCR e manga-ocr foram citados como candidatos de evolução, respectivamente para cenários de OCR/layout e mangá. Não se fixa nesta versão modelo, licença, benchmark ou garantia de consumo. Avaliar cada candidato com corpus real, idiomas, orientação de texto, RAM, latência e licença dos pesos antes de adotá-lo.

### 15.3 Qualidade e reprocessamento

Texto existente pode estar ilegível ou com ordem incorreta; “tem texto” não basta como critério. Definir heurísticas e revisão para camadas ruins. Usuário autorizado pode solicitar reprocessamento, mantendo versão anterior até a nova publicação.

O corpus de validação deve conter português com acentos, páginas de duas colunas, PDF misto, páginas rotacionadas, mangá com texto vertical e imagens longas. Medir precisão em transcrição amostrada, qualidade de ordem de leitura e utilidade para busca. Nenhum limiar universal foi aprovado.

A separação de OCR do interruptor global de IA está confirmada em DEC-044. OCR é processamento documental com controle próprio de habilitação e consumo e pode continuar com IA desligada, mesmo quando seu motor emprega modelos internamente. Essa exceção vale para reconhecimento textual e processamento documental; sugestões de metadados ou análises semânticas posteriores continuam sujeitas aos controles das respectivas áreas. A interface deve explicitar essa distinção.

## 16 Busca textual e embeddings

### 16.1 Busca convencional

Unificar a entrada de pesquisa e distinguir resultados de catálogo, conteúdo e marginalia. Exibir obra, edição/arquivo, trecho, origem e Locator. Índices devem acompanhar alterações, exclusões e permissões; resultados obsoletos não podem abrir uma fonte errada.

PostgreSQL Full Text Search é candidato para busca linguística e ranking [T1]. Pesquisa literal, por substring e por ISBN pode exigir mecanismos distintos: a busca linguística não deve ser apresentada como garantia de correspondência exata. Configuração por idioma, acentos, frases e operadores será definida com testes em português e demais idiomas do acervo.

### 16.2 Segmentação

Usar estrutura editorial: capítulo/seção/parágrafo no EPUB; página/blocos no PDF; página/região no OCR; nota como unidade pessoal. Chunks para embedding podem agrupar segmentos, preservando todos os Locators. Tamanho, sobreposição e limites por modelo são parâmetros versionados.

Conservar texto original extraído e transformação aplicada. Normalização para indexação não deve alterar a citação exibida como se fosse transcrição fiel. Fonte removida ou alterada invalida derivados; a nota humana permanece, com estado de vínculo quebrado quando necessário.

### 16.3 Busca semântica e híbrida

Um embedding representa conteúdo em um espaço vetorial. Registrar provedor, modelo, revisão, dimensão, normalização e versão do pré-processamento. Mesmo número de dimensões não torna dois modelos compatíveis.

pgvector é candidato para armazenar e pesquisar vetores [T2]. Índice exato ou aproximado será escolhido por benchmark. Busca híbrida pode combinar resultados textuais e semânticos com método de fusão documentado; pontuações brutas de naturezas distintas não devem ser somadas sem critério.

Na troca de modelo, construir nova geração de índice, medir qualidade e só então ativá-la. Busca textual continua disponível. Aplicar filtro de autorização à recuperação, reranking, cache e conteúdo enviado ao gerador. Vetores de notas pessoais mantêm o mesmo escopo de acesso da nota.

Modelos multilíngues pequenos, como a família MiniLM citada no debate, são candidatos de avaliação. Não há modelo padrão aprovado. O conjunto de avaliação deve incluir consultas conhecidas, relevância julgada pelo usuário, latência, memória e recuperação de passagens corretas.

## 17 IA generativa e governança

### 17.1 Controles e dependências

A execução efetiva exige: interruptor global ligado, área habilitada, provedor configurado, política de transmissão compatível e recurso computacional disponível. Preferências por área não devem ser apagadas ao desligar o global. Desligamento impede novos jobs e solicita cancelamento dos ativos quando tecnicamente possível; uma requisição já transmitida não pode ser desfeita.

Com IA desligada, busca semântica fica suspensa mesmo quando já existem índices vetoriais; não apagar esses índices apenas por desligar o controle. Sugestões que usam modelos, sínteses e assistente também ficam suspensos. Dados gerados existentes permanecem identificados; notas, resultados aceitos e grafo manual continuam disponíveis. Extração de texto nativo, busca textual e heurísticas sem modelos, inclusive as da retomada entre versões, permanecem disponíveis. OCR depende exclusivamente de sua configuração própria de processamento e das políticas de execução/transmissão aplicáveis. A suspensão de IA não deve apagar as preferências por área.

### 17.2 Execução local e externa

Política confirmada: somente local por padrão. Owner autoriza provedores externos explicitamente por área e tipo de dado. “Preferir local” não autoriza fallback externo implicitamente; falha ou insuficiência do host não pode provocar envio silencioso. Informar quais arquivos, páginas, trechos ou notas cada área transmite e permitir revogação. A configuração de OCR separada do toggle de IA não dispensa essa política.

O owner pode configurar um provedor global com uma API key para as capacidades disponíveis nesse serviço, ou selecionar provedores específicos por área. Cada área indica execução local, herança do provedor global ou configuração específica; a configuração específica prevalece. Um único provedor pode exigir modelos diferentes para geração, embeddings ou processamento de imagens. Capacidade não suportada deve ser indicada como indisponível até configuração compatível, sem prometer que uma única API atende a todas as tarefas.

Credencial global simplifica configuração, mas não concede autorização geral de transmissão nem habilita recursos desligados. O provedor efetivo, modelo e dados enviados devem ficar visíveis na configuração da área. Proposta técnica: armazenar credenciais em referência protegida e reutilizável, sem expor API keys aos leitores, aos prompts ou aos logs.

Notas privadas exigem também autorização do leitor proprietário, mesmo quando o owner já permite o provedor externo. Sem consentimento, excluir essas notas e conteúdo pessoal derivado delas dos pedidos externos, ou informar que a operação depende de autorização. A recusa não bloqueia funções locais disponíveis. Consentimento deve identificar destinos e finalidades; mudança de provedor ou ampliação de finalidade não herda silenciosamente a autorização anterior. Revalidar autorização antes do envio de jobs pendentes; revogação impede novos envios, mas não desfaz transmissões já realizadas. Esses mecanismos são detalhamento proposto para cumprir DEC-046.

Provedores generativos devem oferecer operações como síntese, classificação e extração estruturada sem amarrar o domínio a um modelo. Escolher modelos por tarefa, idioma, qualidade e orçamento. Um modelo pequeno pode ser adequado para classificação e insuficiente para síntese complexa; avaliar, não prometer equivalência.

### 17.3 Orçamento e página exclusiva do owner

O controle de gastos de provedores externos inclui limite mensal global e limites opcionais por área. A configuração, os valores e os relatórios ficam somente na página de gestão exclusiva do owner, protegidos no servidor e na interface. Não incluir cartões de custo ou orçamento no hub, no leitor ou nas páginas administrativas comuns. A restrição financeira não oculta do leitor as informações necessárias para consentir com o envio de suas notas.

Ao alcançar um limite aplicável, novas chamadas externas são bloqueadas. Uma operação afetada pode informar “Recurso externo temporariamente indisponível”, sem revelar valores, limites ou relatórios. O tratamento operacional de tarefas bloqueadas deve permitir acompanhamento sem expor dados financeiros a outros papéis.

Detalhamento técnico proposto: contabilizar por provedor e área sem duplicar gastos de uma credencial global; reservar orçamento estimado antes de iniciar chamadas concorrentes e reconciliar após a resposta. Repetições e reprocessamentos também consomem orçamento. Registrar período, moeda, tabela de preços e uso medido quando disponível; distinguir gasto estimado de cobrança apurada. Custo desconhecido não deve ser tratado como zero. Política para serviços sem estimativa confiável, moeda e fechamento mensal ainda precisam de especificação.

O limite é um controle sobre chamadas iniciadas pelo Códice, não uma garantia absoluta sobre a fatura externa: chamadas já em andamento, diferenças de preço e uso da mesma chave fora da aplicação podem não estar integralmente refletidos. Explicar esse alcance na própria página do owner. Bloqueio financeiro não autoriza trocar silenciosamente de provedor ou executar outra tarefa com custo.

### 17.4 Proveniência e limites

GeneratedArtifact deve registrar fontes versionadas, usuário/escopo, modelo, provedor, parâmetros relevantes, versão do prompt, momento e confirmação humana. Respostas RAG devem citar trechos recuperados e reconhecer ausência de evidência. Fontes são dados não confiáveis, inclusive quando contêm instruções dirigidas a assistentes.

Sugestões não alteram catálogo, grafo ou notas pessoais sem regra explícita de aceitação. Não conceder ao gerador acesso direto a shell, segredos, exclusão de arquivos ou configuração administrativa. Exclusão da fonte precisa invalidar ou sinalizar artefatos dependentes conforme política de retenção.

## 18 Leitura e sincronização

### 18.1 Estado interno

ReadingProgress deve identificar usuário, arquivo/versão, Locator, progressão quando calculável, revisão e dispositivo. Registrar horário de recebimento no servidor e horário informado pelo cliente separadamente. Porcentagem desconhecida pode ser nula; não inventar precisão.

A tela “Continuar lendo e ouvindo” pode agrupar por obra, apresentando arquivo/edição ativa. Política proposta: mostrar a atividade válida mais recente, com seletor para outros formatos. Não usar a maior porcentagem como posição comum nem converter EPUB em áudio sem alinhamento validado.

### 18.2 Retomada equivalente entre versões

Por padrão, cada arquivo conserva sua posição; trocar de versão retoma a posição própria do destino ou inicia do começo. Como recurso opcional confirmado em DEC-030, o usuário pode pedir “Continuar nesta versão de onde parei na outra”. A origem deve ser identificada explicitamente quando houver várias posições possíveis. Essa ação busca uma correspondência; não estabelece sincronização contínua entre os arquivos.

Estratégia proposta, da análise mais simples para a mais custosa:

1. Comparar estrutura editorial, títulos de capítulos e seções. Numeração ou nome coincidente fornece candidato aproximado, não prova de equivalência.
2. Comparar um trecho ao redor da posição de origem com texto do destino, usando palavras, sequências e contexto. Priorizar texto nativo; recorrer a OCR conforme configuração quando o destino ou a origem forem imagens.
3. Quando necessário e habilitado, usar análise semântica multilíngue ou modelos para sugerir candidatos entre traduções e versões mais divergentes. Cada candidato deve apontar para conteúdo realmente localizado no arquivo de destino, nunca para posição inventada pelo modelo.

Proposta de apresentação: mostrar trecho de origem, trecho de destino e precisão disponível, como “capítulo correspondente” ou “passagem provável”. Se houver vários candidatos, permitir escolha; sem evidência suficiente, informar que não foi possível localizar e manter a navegação manual. Capítulos reorganizados, traduções, prefácios, versões abreviadas e erros de OCR devem integrar o corpus de avaliação. Semelhança não garante equivalência exata.

Proposta de persistência: registrar Locators e versões das duas fontes, método, versão do processamento, evidências e aceite do usuário. Uma sugestão não sobrescreve progresso existente. Ao aceitar, abrir a posição escolhida e seguir a política normal de gravação no destino, preservando a origem. Invalidar correspondências quando qualquer fonte mudar. Notas e destaques não são transferidos automaticamente por esse recurso.

Análises custosas seguem a fila e os limites do host. A desativação de IA mantém heurísticas sem modelos disponíveis; OCR conserva seu controle próprio. Limiares, custo máximo e cancelamento precisam de detalhamento antes da implementação. Não é necessário executar modelos em toda troca de versão.

A primeira implementação cobre versões textuais, incluindo texto recuperado por OCR. Retomada entre livro e audiobook está confirmada como evolução posterior em DEC-031 e RF-043: exige correspondência entre passagem e tempo real da gravação, em ambos os sentidos. Transcrição, alinhamento, disponibilidade de capítulos e tratamento de versões abreviadas serão avaliados nessa etapa. Esse recurso não bloqueia a entrega da retomada textual nem do leitor de áudio com progresso próprio.

### 18.3 Conflitos

Proposta: aceitar atualização condicionada à revisão conhecida pelo cliente. Em conflito, manter histórico e solicitar escolha ou aplicar política documentada para o adaptador. Horário de dispositivo sozinho não é autoridade devido a relógios incorretos. Cliente offline pode reenviar evento idempotente, sem apagar estado mais recente silenciosamente.

### 18.4 Catálogo e protocolos externos

OPDS organiza descoberta e distribuição de publicações [T4]. OPDS Progression aparece como especificação separada em draft [T5]; suporte depende de interoperabilidade comprovada e não deve ser apresentado como capacidade universal de clientes OPDS. KOReader é candidato a adaptador independente, com protocolo e versão a validar.

A página unificada pode dizer “Dispositivos e sincronização”, exibindo por cliente: descoberta/download, envio de progresso, recebimento de progresso e estado de conexão. Não exibir “sincronizado” apenas porque um arquivo foi baixado.

## 19 Grafo e gestão de conhecimento

Grafo manual conecta obra, autor, conceito, tag, nota e citação com relações tipadas. Uma relação possui origem manual ou sugerida, escopo pessoal ou compartilhado, evidências e estado de revisão. Similaridade é sinal de proximidade textual; não prova concordância, oposição ou causalidade.

Trilhas ordenam obras, capítulos e passagens com comentários e progresso próprio. A recomendação pode partir do grafo e de embeddings; trilhas criadas pelo leitor continuam utilizáveis sem IA. Comparação abre duas fontes com contextos independentes, inclusive em layout sequencial no celular.

Exportação para PKM deve preservar IDs, fonte, Locator, datas e tags em formato legível. Markdown simples é base; frontmatter, Wikilinks e callouts são opções. Escrita em vault requer destino autorizado, proteção contra caminhos inválidos e política de conflito; não sobrescrever nota externa modificada silenciosamente.

Deep links HTTPS são proposta de base interoperável. O esquema `codice://` depende de manipulador instalado e permanece opcional. Links não devem carregar credenciais e precisam revalidar autorização ao abrir.

## 20 Integrações

| Integração | Objetivo | Estado e contrato mínimo |
| --- | --- | --- |
| Local, OIDC e LDAP | Autenticação | Confirmado o suporte; políticas detalhadas em aberto |
| Authentik | Cenário de provedor externo | Validar com configuração real; sem dependência obrigatória |
| Open Library | Candidatos bibliográficos | Proposto; cache, timeout, origem e resolução de divergência |
| Goodreads | Menção histórica em telas | Sem compromisso de implementação; avaliar acesso autorizado antes de planejar |
| OPDS | Descoberta e aquisição | Proposto; versão e clientes homologados em QA-018 |
| KOReader e OPDS Progression | Estado de leitura | Adaptadores candidatos; não presumir suporte pelo catálogo |
| Obsidian e Markdown | Portabilidade de notas | Exportação desacoplada; escrita direta exige resolver conflitos |
| Obsidian Canvas | Exportação visual de relações | Evolução; preservar nós, rótulos e referências |
| OCR, embeddings e geração | Processamento substituível | Interfaces independentes e saídas versionadas |
| ZFS e SMART | Telemetria adicional | Opcional, com privilégios mínimos e isolamento |

Toda integração deve declarar dados enviados, autenticação, timeout, política de repetição, limites de uso, versões testadas e comportamento na indisponibilidade. Expor segredos somente ao componente executor. Uma falha externa não remove dados locais.

## 21 Páginas planejadas

### 21.1 Mapeamento das telas de origem

O inventário textual identifica treze grupos de fluxos com códigos SCREEN. Eles são referências de design; não afirmam implementação. Temas e variações móveis não representam funcionalidades distintas.

| ID | Página ou fluxo e referência | Responsabilidade e requisitos |
| --- | --- | --- |
| UI-01 | Hub — 44, 6, 5; mobile 3 | Catálogo, continuar leitura, disponibilidade; RF-006, 015, 021 |
| UI-02 | Ficha da obra — 43; mobile 41 | Edições, arquivos, metadados e acesso ao leitor; RF-005, 006, 012 |
| UI-03 | Leitor imersivo — 23; mobile 40 | Aparência, seleção, navegação e posição; RF-012 a 017 |
| UI-04 | Sumário e topografia — 22; mobile 38 | Capítulos, marcadores e retomada; RF-014, 015 |
| UI-05 | Marginalia da obra — 20; mobile 37 | Notas por capítulo, tags e retorno à fonte; RF-016, 018 |
| UI-06 | Adicionar glosa — 19; mobile 35 | Destaque, nota, tags e vínculos; RF-016, 017, 026 |
| UI-07 | Busca avançada e tags — 16; mobile 33 | Filtros de marginalia e exportação em lote; RF-018, 029 |
| UI-08 | Busca global — 14; mobile 32 | Acervo e fontes; modo textual e semântico; RF-018, 024 |
| UI-09 | Grafo — 12; mobile 31 | Nós, relações, sugestões e detalhes; RF-025, 026 |
| UI-10 | Trilha — 11; mobile 29 | Sequência, passagens e progresso próprio; RF-027 |
| UI-11 | Leitor contextual da trilha — 9; mobile 27 | Passagem ativa e notas da relação; RF-014, 027 |
| UI-12 | Comparação — 7; mobile 25 | Duas fontes e síntese opcional; RF-027, 028 |
| UI-13 | Exportação PKM — 18; mobile 34 | Formatos, prévia e destinos; RF-029 |

### 21.2 Páginas complementares propostas

| ID | Página | Conteúdo e estados necessários |
| --- | --- | --- |
| UI-14 | Wizard e login | Primeiro admin, storage, perfil; falha, retomada e recuperação |
| UI-15 | Usuários e identidades | Convite/criação, papéis, provedores e sessões |
| UI-16 | Ingestão e quarentena | Upload, fila, duplicatas, divergências e revisão |
| UI-17 | Edição do catálogo | Obra/edição/arquivo, candidatos e proveniência |
| UI-18 | Jobs e diagnóstico | Progresso por etapa, erro, cancelamento e reexecução |
| UI-19 | Storage e recuperação | Raízes, espaço, arquivos ausentes e backup |
| UI-20 | Processamento e IA | OCR, toggle global, áreas e consumo computacional; sem dados financeiros |
| UI-21 | Dispositivos e integrações | OPDS, sincronização, tokens e compatibilidade |
| UI-22 | Preferências e privacidade | Aparência, exportação pessoal e política de transmissão |
| UI-23 | Gestão de provedores externos e orçamento | Exclusiva do owner: provedor global/específicos, credenciais, permissões de transmissão, limite mensal global, limites por área, consumo financeiro e indicação de estimativas |

Toda página deve prever carregamento, vazio, erro, permissão insuficiente e processamento parcial quando pertinentes. Mobile deve preservar tarefas essenciais; ações comparativas podem usar alternância/sequência em vez de reproduzir duas colunas estreitas.

### 21.3 Direção visual

Preservar a intenção editorial: papel aquecido (`#f4ede3`, `#faf6ef`), carvão (`#141311`, `#1f1d1a`), terracota (`#c86d3b`) e sálvia (`#4a7060`). Newsreader e Plus Jakarta Sans são referências de tipografia; disponibilização e licença devem ser verificadas na implementação. Opções acessíveis, modo e-ink e preferência do leitor prevalecem quando necessário.

Telemetria avançada é administrativa, sem sobrecarregar a página inicial do leitor. Indicadores de similaridade devem explicar o significado; evitar apresentar “coerência semântica” como precisão científica sem métrica definida.

## 22 Fluxos principais

**FL-01 — Primeira inicialização.** Pré-condição: instância sem configuração concluída. Mantenedor abre wizard, cria owner, define storage e perfil. Sistema valida escrita/leitura e conclui atomicamente. Em falha, informa etapa e permite retomar; nunca expõe criação administrativa irrestrita depois de inicializado. Cobre RF-001 a 003, 010 e RN-015.

**FL-02 — Importação gerenciada.** Administrador envia PDF/EPUB/CBZ. Sistema valida, calcula hash e apresenta conflitos. Após revisão necessária, publica cópia verificada e catálogo, libera leitura e inicia derivados. Duplicata ou falha de armazenamento não produz incorporação duplicada. Cobre RF-007 a 010, 019 e 020.

**FL-03 — Catálogo de pastas existentes.** Administrador escolhe um ou mais diretórios nas raízes permitidas pelo owner, em modo referenciado. Scanner detecta arquivos e propõe vínculos; catálogo preserva caminhos originais. Arquivo removido vira indisponível, mantendo notas. Ação posterior de mover para gerenciado usa o fluxo comum: transferir, verificar, confirmar destino/vínculo e remover a origem acessível ao servidor. Falha preserva cópia válida e estado recuperável. Cobre RF-010 e 011.

**FL-04 — Ler e anotar.** Leitor abre arquivo autorizado e retoma Locator. Seleciona texto/região, cria destaque ou nota e salva. Ao retornar pela marginalia, o sistema resolve a versão original. Se fonte mudou, exibe vínculo pendente de reconciliação. Cobre RF-012 a 017.

**FL-05 — Pesquisar e abrir resultado.** Leitor consulta acervo; servidor aplica permissões e recupera resultados de catálogo, texto e notas. Resultado informa fonte e localização. Busca semântica só participa quando habilitada e disponível; ausência desse módulo mantém modo textual. Cobre RF-018 e 024.

**FL-06 — Processar OCR.** Detector identifica páginas sem texto útil. Worker processa lote limitado, registra proveniência e publica segmentos. Busca passa a localizar trechos; erro em uma página é visível e reprocessável. Original e leitura continuam disponíveis. Cobre RF-019, 020 e RN-001.

**FL-07 — Aceitar relação sugerida.** Sistema apresenta relação candidata com passagens de apoio. Leitor examina, altera, aceita ou rejeita. Aceite cria relação identificada com responsável; desligar IA não remove essa decisão humana. Cobre RF-025 a 027.

**FL-08 — Sincronizar dois dispositivos.** Cliente envia posição com versão conhecida e identificador de evento. Servidor valida identidade e arquivo, aceita ou registra conflito. UI permite retomar a posição escolhida; porcentagem maior não recebe preferência automática. Cobre RF-031 e RN-011.

**FL-09 — Exportar notas.** Leitor seleciona itens próprios, formato e destino, revisa prévia e confirma. Sistema gera pacote com fontes e links. Escrita em vault detecta conflito; download continua alternativa. Cobre RF-029 e RNF-011.

**FL-10 — Desligar IA e recuperar operação.** Administrador desliga global. Sistema impede novos jobs inteligentes, solicita cancelamento e mantém notas e artefatos identificados. Catálogo, leitura, busca textual e relações manuais continuam. Em reinício, tarefas pendentes respeitam a configuração vigente. Cobre RF-022, RN-012 e RN-017.

**FL-11 — Restaurar a instância.** Operador restaura banco, originais e configuração compatíveis, verifica integridade e reconstitui credenciais de forma segura. Jobs e derivados são reconciliados antes de reativar processamento. Usuário acessa nota e retoma leitura. Cobre RF-034 e RNF-010.

## 23 Evolução e critérios de entrega

| Etapa proposta | Entrega | Condição de saída |
| --- | --- | --- |
| E0 — Consolidar | Revisão de entidades, permissões e decisões | Questões estruturais identificadas e análise do backend concluída |
| E1 — Acervo utilizável | Login local, multiusuário, storage, ingestão, PDF/EPUB/CBZ, leitura e notas | FL-01 a 04 e restauração básica verificadas |
| E2 — Pesquisa e operação | Extração, OCR configurável, busca textual, jobs e provedores de identidade | FL-05/06; isolamento e consumo medidos |
| E3 — Interoperabilidade | Exportação PKM, OPDS e sincronização homologada | FL-08/09; matriz real de clientes publicada |
| E4 — Conhecimento | Grafo/trilhas manuais, embeddings e sugestões | Proveniência, avaliação de recuperação e FL-07/10 |
| E5 — Recursos adicionais | Geração, RAG, áudio e formatos adicionais | Qualidade e recursos medidos por capacidade |

As etapas são reordenáveis. OIDC/LDAP são compromisso de escopo confirmado, mesmo se a entrega ocorrer depois do login local. Componentes já bons no backend podem antecipar uma etapa. Nenhuma fase exige apagar ou reescrever código sem avaliação.

### 23.1 Conjunto mínimo de validação

Preparar arquivos de teste com origem permitida: EPUB refluível com acentos, EPUB fixed-layout para avaliar escopo, PDF digital, PDF escaneado, PDF misto, CBZ em LTR/RTL, imagem longa, arquivo corrompido, duplicata e conteúdo alterado. Acrescentar áudio quando entrar no escopo de entrega.

Cobrir dois usuários, administrador, conta externa, leitura offline no cliente quando suportada, reinício de worker, disco cheio, provedor indisponível e conflito de progresso. Para cada teste registrar requisito, cenário, resultado, versão e evidência. Métricas sem contexto de hardware não aprovam requisito de desempenho.

## 24 Decisões propostas e questões em aberto

### 24.1 Registro de decisões propostas

| ID | Proposta e motivo | Alternativa ou custo | Estado |
| --- | --- | --- | --- |
| DEC-P01 | Monólito modular com workers para reduzir operação | Serviços separados podem isolar mais, com maior custo operacional | P |
| DEC-P02 | PostgreSQL FTS e pgvector para reduzir serviços | Avaliar solução existente ou motor especializado por carga | P |
| DEC-P03 | DocumentSegment e Locator como contratos centrais | Mais modelagem inicial; melhora rastreabilidade | P |
| DEC-P04 | Substituída por DEC-036 e DEC-037 | Pastas legíveis; IDs estáveis no banco; reorganização administrativa com prévia | Substituída |
| DEC-P05 | Original imutável e derivados versionados | Aumenta uso de disco e exige coleta de resíduos | P |
| DEC-P06 | Confirmada por DEC-044 | OCR com controle próprio; IA global governa os recursos inteligentes opcionais | Confirmada |
| DEC-P07 | Confirmada por DEC-045 a 047 | Local por padrão; autorização do owner por área e consentimento individual para notas; provedor global ou específico | Confirmada |
| DEC-P08 | Revisões para conflitos de progresso | Mais recente por relógio é simples, mas sensível a relógios e offline | P |
| DEC-P09 | Substituída por DEC-014 e DEC-022 | Owner único, poderes exclusivos e transferência explícita confirmados | Substituída |

### 24.2 Registro das perguntas originais

| ID original | Situação nesta versão | Pendência residual |
| --- | --- | --- |
| QA-001 | Multiusuário, administração, owner único, recuperação local e papel do antigo owner confirmados | Detalhar apenas os artefatos técnicos da recuperação (QA-012) |
| QA-002 | Local, OIDC e LDAP confirmados | Ordem de entrega resolvida em DEC-074 (LDAP primeiro, OIDC depois como botão) e vínculo por nome em DEC-075. Pendentes: grupos, MFA e atributos do diretório usados como nome e identificador |
| QA-003 | Dois modos de storage confirmados | Organização física, adoção e reconciliação |
| QA-004 | PDF, EPUB e CBZ confirmados | Variantes aceitas e sequência de formatos adicionais |
| QA-005 | Progresso comum na experiência confirmado | Regra de agregação por obra e conflitos |
| QA-006 | Visualização escolhida pelo usuário confirmada | Defaults por publicação e dispositivos |
| QA-007 | Finalidade do OCR e controle separado confirmados | Motores, benchmark e critérios de qualidade ainda precisam de validação |
| QA-008 | Busca textual discutida | Backend de busca, idiomas e consulta literal |
| QA-009 | Embeddings discutidos | Modelo, segmentação, avaliação e armazenamento |
| QA-010 | Áreas de IA, toggles, transmissão e orçamento confirmados | DEC-044 a 049; detalhar modelos compatíveis, contabilização, moeda/período, custo desconhecido e implementação do consentimento |
| QA-011 | Separação interna aceita | Protocolos, clientes e resolução de conflitos |

### 24.3 Perguntas para fechar as próximas decisões

| ID | Pergunta | Encaminhamento proposto e impacto |
| --- | --- | --- |
| QA-012 | Resolvida quanto ao mecanismo | DEC-054, DEC-057 e DEC-058: comando local, dois casos, escolha do papel do antigo owner na transferência. Duas etapas e link de 1 hora confirmados. Pendente apenas o padrão “leitor” quando o operador omite o papel na recuperação, ainda proposto, e os testes |
| QA-013 | Resolvida para o escopo inicial | Bibliotecas compartilhadas entre usuários autenticados; coleções pessoais privadas; sem bibliotecas restritas por usuário |
| QA-014 | Resolvida quanto ao padrão. Quotas descartadas | Pastas legíveis com IDs no banco e reorganização explícita (DEC-036/037); padrão de nomes e autoria completa no catálogo em DEC-065; sem cotas por usuário nesta etapa. Pendente confirmar na implementação a fronteira da pasta de série (quadrinhos/mangá versus livros), o corte de 3 autores e o limite de 200 caracteres |
| QA-015 | Resolvida quanto ao fluxo de aprovação | DEC-026: liberar arquivo válido com título mínimo, salvo possível duplicata; bloquear inválidos; enriquecimento externo/OCR/IA exige aprovação. Detalhar heurísticas de duplicidade e obtenção do título mínimo |
| QA-016 | Encaminhada: perfis de consumo, sem mínimo único | O consumo depende de quais recursos a instalação usa (IA desligada, OCR, modelos locais ou serviço externo, tamanho do acervo). Em vez de um hardware mínimo único, medir perfis por conjunto de recursos e registrar RAM, CPU e tempos por perfil depois que o núcleo existir (RNF-003, RNF-004). Sem SLOs antes dessas medições |
| QA-017 | Alcance do interruptor resolvido em DEC-044 | Suspender busca semântica e recursos com modelos; preservar notas e resultados aceitos; OCR separado, texto nativo, busca textual e heurísticas sem modelos disponíveis. Detalhamento de cancelamento de jobs continua técnico |
| QA-018 | Quais versões OPDS e clientes serão homologados? | Definir matriz separada para catálogo e progresso |
| QA-019 | Como tratar conflitos e progresso por obra? | Validar revisões, posição mais recente e escolha manual |
| QA-020 | Quais metas e retenção de backup? | Política da lixeira resolvida em DEC-041 a 043: limpeza opcional, sugestão de 30 dias desde a entrada, alterações prospectivas e aplicação retroativa explícita com prévia. Backup resolvido em DEC-064: comando mínimo do Códice com documentação para ferramentas externas, RPO de 24 horas, RTO de algumas horas e retenção sugerida de 7/4/3. Ferramentas e agendamento ficam com o operador; medir o tempo real de restauração com o acervo real |
| QA-021 | Como resolver contas duplicadas e revogar acesso? | Cadastro/convites, criação de leitores, promoção local e vínculo assistido por e-mail coincidente confirmados em DEC-050 a 055; gestão do papel admin restrita ao owner em DEC-056; convite avulso com e-mail opcional e SMTP opcional em DEC-059; bloqueio imediato em DEC-060; revalidação externa de 24 horas em DEC-061; fusão manual com salvaguardas em DEC-062; redefinição de senha sem SMTP em DEC-063. Pendente apenas o formato da auditoria administrativa |
| QA-022 | Qual política para dependências e modelos? | AGPLv3 já presente no repositório; avaliar fontes, dependências e pesos |
| QA-023 | Quais variantes de EPUB/PDF serão aceitas? | Definir fixed-layout, arquivos protegidos e limites de tamanho |
| QA-024 | Como escrever em vault sem sobrescrever edições? | Começar por download e definir reconciliação bidirecional |
| QA-025 | Resolvida quanto à política | Redis mantido nesta etapa como meio de entrega, com estado no PostgreSQL (DEC-066), envio e job na mesma transação (DEC-067), 3 tentativas e reexecução manual (DEC-068) e prioridade com um job pesado por vez (DEC-069). Pendente: testes de recuperação e a implantação em §10.4 |
| QA-026 | Como obter os protótipos completos e reconciliar sua contagem? | Inventário textual enumera 13 fluxos; validar declaração de 16 conceitos |
| QA-027 | Como aprovar e desfazer sugestões em lote? | Definir trilha de auditoria, confirmação e conflitos de edição |
| QA-028 | Como validar retomada equivalente entre versões? | Capacidades confirmadas em RF-042 e RF-043: texto primeiro, livro e audiobook em evolução futura. Definir precisão, limiares, custo, corpus multilíngue e, na etapa de áudio, método de alinhamento temporal |

QA-012, QA-014, QA-020, QA-021 e QA-025 estão resolvidas quanto à política e QA-016 foi encaminhada para medição posterior por perfil. As demais questões não bloqueiam o início da implementação antes de estabilizar persistência e permissões. Questões sobre modelos específicos podem aguardar o núcleo funcional.

## 25 Análise do backend existente

### 25.1 Objetivo e método

A revisão deverá produzir um diagnóstico em duas direções: requisitos para código e código para requisitos. Primeiro levantar linguagem, framework, banco, migrações, entidades, endpoints, autenticação, storage, jobs, testes e implantação. Não inferir ausência apenas por nomes diferentes.

Para cada requisito, registrar evidência com caminho de arquivo, símbolo/rota e teste ou comportamento observado. Classificar atendimento, risco e ação sugerida. Uma divergência pode resultar em ajuste do código, revisão da especificação ou decisão pendente. Manter justificativa, sem assumir que a proposta deste documento é superior ao que já foi implementado.

### 25.2 Matriz inicial de rastreabilidade

| Área e requisitos | Evidência a procurar | Estado inicial |
| --- | --- | --- |
| Identity — RF-001 a 004, RN-015 | Modelos de conta, middleware, sessões, bootstrap e testes de permissão | Não avaliado |
| Library — RF-005 a 006, RN-002/003 | Obra/edição/arquivo, identificadores, metadados e consultas | Não avaliado |
| Ingestion — RF-007 a 011 | Upload, scanner, hash, quarentena, publicação e duplicidade | Não avaliado |
| Reader — RF-012 a 017 | Contratos de formato, Locator, progresso e isolamento das notas | Não avaliado |
| Processing — RF-019/020 | Fila, lease, retries, checkpoints, versionamento e limites | Não avaliado |
| Search — RF-018/024 | Segmentos, índice, idioma, vetores e filtros de autorização | Não avaliado |
| Knowledge — RF-025 a 029 | Relações, autoria, fontes, exportação e portabilidade | Não avaliado |
| Integrations — RF-030 a 033 | Adaptadores, credenciais, timeout e matriz de clientes | Não avaliado |
| Operations — RF-021/034 e RNF | Volumes, migrações, logs, backup, restore e carga | Não avaliado |

Cada linha acima deve ser expandida por ID individual durante a análise. Modelo de registro: **ID; evidência; estado; lacuna; impacto; decisão recomendada; teste; responsável**. Estado “atendido” exige evidência de comportamento, não apenas existência de classe ou endpoint.

### 25.3 Entregáveis da revisão

1. Inventário do backend e diagrama das dependências reais.
2. Matriz por requisito, com evidências e lacunas.
3. Lista de decisões já implementadas que merecem ser preservadas.
4. Registro de divergências e propostas de alteração do documento.
5. Plano incremental de implementação e migração, priorizado por dependência e risco.

O mantenedor confirmou que o acervo atual é inteiramente de teste. Podemos redesenhar o esquema sem preservar compatibilidade com dados de produção; eventual reinicialização será uma operação explícita, separada das alterações de código e dos experimentos locais de frontend.

Antes de alterar código estrutural, resolver divergências que afetem identidade de obra/arquivo, escopo pessoal, preservação dos originais e formato de Locator. Mudanças locais reversíveis podem avançar após o diagnóstico correspondente, sem exigir que todas as questões futuras estejam encerradas.

## 26 Glossário e referências

### 26.1 Glossário

**Marginalia:** conjunto de notas, destaques e marcadores associados à leitura. **PKM:** gestão pessoal de conhecimento. **OCR:** reconhecimento de texto em imagens. **Embedding:** representação numérica usada para recuperar conteúdo semelhante. **RAG:** geração de resposta com recuperação prévia de fontes. **Locator:** posição tipada em uma versão de publicação. **Worker:** processo que executa trabalho assíncrono. **Quarentena:** estado de revisão ou isolamento de importações. **Proveniência:** registro da origem e transformação de um dado. **Lease:** direito temporário de um worker executar um job. **RPO:** perda de dados temporal máxima aceitável após incidente. **RTO:** tempo máximo pretendido para restaurar operação. **SLO:** objetivo mensurável de nível de serviço.

### 26.2 Fontes de requisitos

**S1 — Conversa Especificação De Requisitos.** Identificador `6aad30c4-9138-83e9-846e-07282d1972f8`. Inclui levantamento textual de telas, proposta v0.1, respostas QA-001 a QA-011, complemento sobre OCR/IA leve e solicitação de consolidação. Link: [abrir conversa de origem](chatgpt-conversation://6aad30c4-9138-83e9-846e-07282d1972f8).

**S2 — Solicitação da versão 0.2.** Documento mestre editável abrangendo requisitos, regras, modelo, arquitetura, workers, OCR/IA, busca, autenticação, storage, integrações, páginas, fluxos e decisões, preparado para análise posterior do backend.

### 26.3 Referências técnicas de apoio

Consultadas em 18 de setembro de 2026. Apoiam capacidades gerais; não aprovam tecnologias, versões ou compatibilidade do Códice. Revalidar versões e contratos quando implementar.

- **T1 — PostgreSQL Full Text Search.** Busca linguística e relevância: https://www.postgresql.org/docs/17/textsearch-intro.html
- **T2 — pgvector.** Extensão de similaridade vetorial para PostgreSQL: https://github.com/pgvector/pgvector
- **T3 — OCRmyPDF.** Camada textual pesquisável para PDF escaneado: https://ocrmypdf.readthedocs.io/en/latest/
- **T4 — OPDS Specifications.** Família de especificações de distribuição de publicações: https://specs.opds.io/
- **T5 — OPDS Progression 1.0 draft.** Proposta separada de estado de leitura: https://drafts.opds.io/opds-progression-1.0.html

### 26.4 Registro para a próxima revisão

Data: a preencher. Responsável: a preencher. Versão do backend ou commit: a preencher. Itens aprovados: a preencher. Itens substituídos: a preencher. Evidências e testes: a preencher. Próxima versão: a definir após análise.
