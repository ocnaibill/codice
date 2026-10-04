# Proposta: telas de conta e administração que faltam (4 de outubro de 2026)

**Status: proposta **confirmada pelo mantenedor em 04/10/2026** (seção "Respostas"); nada daqui foi implementado.** Cada tela vira um PR pequeno, e o que mudar de regra vira DEC só depois de confirmado.

## Respostas do mantenedor (04/10/2026)

| Decisão | Resposta |
| --- | --- |
| Contato do dono na tela **Sobre** | **Não mostrar** (DEC-120) |
| **IP na sessão** | **Guardar** (DEC-121) |
| **Registro de entradas**: tabela própria e retenção | **Tabela própria, 90 dias, ajustável pelo dono** (DEC-121) |
| **Painel de backup**: nome ou caminho do pacote | **Mostrar o caminho** (a quem: ver a pergunta abaixo) |
| **Ingestão e revisão** | **Deixar para depois**, como issue (#141) (DEC-122) |
| **Orçamento dos provedores** | **Fora do beta; contador de chamadas antes do dinheiro** (DEC-122) |
| Dono com **leitura e escrita** na página de backup | **Gera e verifica; restaurar e baixar ficam fora do HTTP; caminho completo só ao dono** (DEC-123) |
| **Ordem das telas** | **1 → 4 → 2 → 3 → 5** (Sobre, Backup e saúde, Sessões, Entradas, Exportar), 6 e 7 depois do beta (DEC-123) |

## 1. Onde estamos

A especificação já nomeia as páginas complementares (UI-14 a UI-23, §21.2). O que existe hoje no app:

| Lugar | O que tem |
| --- | --- |
| **Menu "Minha conta"** | Administração, Preferências, Aplicativos (tokens do OPDS), sair |
| **Administração** (abas) | Trabalhos, Armazenamento, Lixeira, Sugestões, Provedores, Duplicatas, OCR, Dicionários, Contas e, só para o dono, IA local e Login externo |
| **Entrada** | Primeira execução, login, convite, redefinição de senha, vínculo de conta (UI-14 existe) |
| **Já com API e sem tela** | `GET /admin/backup` (data do último backup), `GET /notes/export` (só notas, com tela própria) |

O que **não existe** nem na API nem na tela: lista de sessões, registro de entradas, exportar todos os dados da conta, uso/orçamento dos provedores, página "Sobre" (versão, licença, código-fonte).

## 2. Resumo e ordem sugerida

| # | Tela | Para o beta? | Tamanho | Depende de |
| --- | --- | --- | --- | --- |
| 1 | **Sobre** (versão, licença AGPL, código-fonte, licenças de terceiros) | **Sim**: a AGPL pede oferecer o código a quem usa pela rede | pequeno | nada |
| 2 | **Sessões e dispositivos** (minha conta) | **Sim**: é o que limita o dano de um token vazado | médio | migração (IP e último uso) |
| 3 | **Registro de entradas** (administração; issue #137) | Recomendado | médio | 2 (mesma coleta de IP) |
| 4 | **Backup e saúde** (administração, leitura) | **Sim**: quem opera precisa ver | pequeno | `GET /admin/backup` já existe |
| 5 | **Exportar meus dados** (minha conta) | Recomendado (LGPD) | médio | nada |
| 6 | **Ingestão e revisão** (rejeitados e divergências) | Depois de ver dados reais | médio | levantamento (§3.6) |
| 7 | **Uso e orçamento dos provedores** (só dono; RF-046) | **Não**: provedores vêm desligados | grande | decisão de modelo |

As telas 1, 4 e 7 não mexem em autenticação. As telas 2 e 3 mexem na tabela de sessões e na gravação das entradas, então vão juntas na ordem 2 → 3.

## 3. Uma a uma

### 3.1 Sobre (UI-22)

- **Onde:** item "Sobre" no menu "Minha conta", abrindo um painel (modal, como "Aplicativos").
- **Conteúdo:** nome e **versão** (o commit da imagem, injetado na construção; o `/healthz` público **não** informa versões, de propósito), **licença AGPLv3** com link para o texto, **link do código-fonte** (a AGPL exige oferecê-lo a quem usa o serviço pela rede; vale o repositório público), lista de **licenças de terceiros** (OpenDyslexic, OFL 1.1; Newsreader e Plus Jakarta Sans, OFL; epub.js, BSD; Wiktionary via kaikki.org, CC BY-SA, que **exige atribuição** nos verbetes do dicionário), e o endereço de quem administra, se o dono quiser mostrar.
- **API:** `GET /about` (versão e data da imagem, injetadas na construção). Sem dados da instância.
- **Estados:** carregamento e erro; não tem vazio nem permissão (qualquer pessoa autenticada vê).
- **Decidido:** **sem** endereço de contato do dono na tela (DEC-120).

### 3.2 Sessões e dispositivos (UI-15, UI-21)

- **Onde:** "Minha conta → Sessões e dispositivos". O dono e os administradores veem as sessões **de uma conta** a partir da aba Contas.
- **Conteúdo:** uma linha por sessão **ativa**: aparelho e navegador (a partir do `User-Agent`, escrito em português: "Firefox em Windows"), **IP**, **entrada em** e **último uso**, e "Esta sessão" marcada. Ações: **Encerrar** (uma), **Encerrar todas as outras**. Os **tokens de aplicativos (OPDS)** ficam na mesma tela, em uma segunda seção, no lugar do modal "Aplicativos" (que continua abrindo aqui).
- **Regras:** encerrar vale na hora (DEC-070); encerrar a sessão atual equivale a sair; bloquear a conta já encerra tudo (DEC-060); **o dono e os administradores podem encerrar as sessões de uma conta, mas não ver o IP de outra pessoa além do que o registro de entradas (3.3) já mostra**.
- **API nova:** `GET /auth/sessions`, `DELETE /auth/sessions/{id}`, `POST /auth/sessions/revoke-others`; para a equipe, `GET /users/{id}/sessions` e `DELETE /users/{id}/sessions/{sid}`. **Migração:** `sessions.ip` e `sessions.last_seen_at` (hoje só há `user_agent`). O último uso é gravado **no máximo uma vez por minuto** por sessão, para não escrever no banco a cada requisição.
- **Estados:** vazio não ocorre (há sempre a atual); erro de carga; permissão insuficiente quando um leitor tenta ver outra conta; "sessão já encerrada" ao tentar de novo.
- **Decidido:** (a) **guardar o IP** na sessão (DEC-121), com a retenção do 3.3. (b) O tempo de vida da sessão é hoje de 7 dias e se ajusta no servidor (`JWT_EXPIRATION_HOURS`); quer que o dono o ajuste pela interface? Proposta: depois do beta.

### 3.3 Registro de entradas (issue #137)

- **Onde:** nova aba **Entradas** em Administração (equipe vê; cada pessoa vê só as suas na tela 3.2).
- **Conteúdo, filtros e cuidados:** como descrito na issue: horário, conta, resultado (entrou, senha errada, usuário desconhecido, bloqueada, diretório indisponível, limite de tentativas), meio (local ou LDAP), IP e plataforma; filtro por resultado, conta e período. **O nome digitado em "usuário desconhecido" é truncado e descartado se parecer uma senha**; falhas repetidas do mesmo IP e conta se **agrupam** numa janela; há **teto de linhas** e **retenção** (proposta: 90 dias, ajustável pelo dono).
- **Aviso de configuração:** se **todas** as entradas recentes têm o mesmo IP, a tela diz que `CODICE_TRUSTED_PROXIES` provavelmente falta (o limite de login fica coletivo).
- **Decidido:** **tabela própria** `login_events`, retenção de **90 dias** ajustável pelo dono (DEC-121); o leitor comum vê as próprias entradas na tela 3.2.

### 3.4 Backup e saúde (UI-19)

- **O que já está decidido:** **criar e restaurar backup ficam no `codice-admin`, no servidor**, de propósito: exige acesso à máquina e um restore substitui o banco, então nenhuma requisição HTTP o inicia (comentário do `BackupAdminHandler`). A proposta **respeita isso**.
- **Onde:** aba **Armazenamento** ganha um bloco "Backup e saúde" (ou uma aba própria, **Sistema**, se ficar grande).
- **Conteúdo (somente leitura):**
  - **Último backup:** data, idade ("há 9 horas"), tamanho e se foi **verificado**; aviso em cor de atenção se passou de 36 horas ou se nunca houve. **Aviso fixo quando o destino é o mesmo disco dos dados** (o alpha está assim): "esta cópia não protege contra a falha do disco".
  - **Espaço:** livre e usado nos discos do acervo e do banco, com aviso abaixo de 15% livre.
  - **Saúde:** o `/healthz` por componente (banco, Redis, worker, OCR, embeddings), com a hora da última resposta do worker.
  - **Como restaurar:** o comando exato do `codice-admin`, com botão de copiar, e um link para o guia.
- **API:** `GET /admin/backup` já existe (só `lastBackup`); acrescentar tamanho, verificação e o destino por mesmo-disco. Espaço e saúde saem do que o servidor já mede.
- **Decidido:** o painel **mostra o caminho** do pacote. Fica a pergunta de **para quem**: recomendo o **caminho completo só para o dono** e o **nome do pacote para os administradores** (que podem não ter acesso ao servidor).

- **Escrita pelo dono (pergunta do mantenedor, 04/10/2026; **confirmada como DEC-123**).** Muda a decisão anterior de que nada HTTP inicia backup ou restauração. Por ação:
  - **Gerar backup agora: sim, só o dono.** Roda como **job** (a infraestrutura já existe), um de cada vez, **sempre criptografado** com a frase que já está no servidor (a frase **nunca** passa pelo navegador), grava só na **pasta de backups configurada** (nunca num caminho vindo do pedido), pede a **senha do dono de novo** e entra no log de auditoria. Custo: a imagem da API precisa do `pg_dump` da versão certa (o `codice-admin` já roda nela) e a pasta precisa ser gravável.
  - **Verificar um pacote: sim, só o dono.** É o `verify-backup --deep`, que restaura num banco temporário e **não toca nos dados**; também como job.
  - **Baixar o pacote pelo navegador: não por padrão.** É o banco inteiro; um token do dono roubado levaria tudo, e são centenas de MB. Se um dia for útil, só com senha de novo e link de vida curta.
  - **Restaurar pela interface: não.** Troca o banco debaixo da API que está respondendo, invalida as sessões (a do dono inclusive) e, se falhar no meio, **não há tela para consertar**. Fica no `codice-admin`, com o comando copiável.
  - **Agendar e definir retenção pela interface: depois.** Hoje o agendamento é do sistema (um timer, no alpha); trazê-lo para dentro do app exige um agendador próprio.
- Resumo: **o dono lê e gera/verifica; restaurar e baixar continuam fora do HTTP.** Não considero problemático desde que seja só do dono, com senha de novo e auditoria.

### 3.5 Exportar meus dados (UI-22)

- **Onde:** "Minha conta → Preferências → Meus dados".
- **Conteúdo:** botão **Exportar** que gera **um arquivo ZIP** com tudo que é da pessoa: **notas e destaques** (Markdown e JSON, o formato que já existe em `/notes/export`), **progresso e histórico de leitura**, **favoritos**, **preferências** (inclusive as do leitor), **tags** e a lista de **aplicativos e sessões** (sem segredos, só nome e data). **Sem os livros** (são do acervo, não da pessoa) e **sem a senha**.
- **Como roda:** o pedido cria um **job**; a pessoa vê "preparando" e baixa quando pronto; o arquivo **expira em 24 horas** e só ela o baixa. Registrado no log de auditoria (quem exportou e quando, sem conteúdo).
- **API:** `POST /auth/export` e `GET /auth/export/{id}` (estado e download).
- **Estados:** vazio ("você ainda não tem anotações": o arquivo sai mesmo assim, com as preferências), em preparo, pronto, expirado, erro com tentativa de novo.
- **Excluir a conta** (DEC-060: ação separada) **não entra aqui**; fica como proposta própria, porque mexe em notas compartilhadas e transferência da posse.

### 3.6 Ingestão e revisão (UI-16)

- **Estado atual:** a entrada de arquivos já tem **Duplicatas**, **Sugestões** (metadados e idioma) e **Trabalhos**; o que **falta** é um lugar onde ver **o que foi recusado ou ficou pela metade** e por quê (arquivo corrompido, formato aceito mas ilegível, extração "não suportada").
- **Dado real do alpha:** 6 de 28 arquivos ficaram como "extração não suportada" (a causa não foi verificada; o acervo tem 5 CBR e 1 CBZ, formatos de imagem, o que pode explicar) e nenhum job falhou. Por isso **recomendo adiar esta tela até haver dados do teste de fogo (#89)** e **não inventar uma "quarentena"** que a especificação não define. Alternativa barata, já agora: um **filtro "com problema"** na aba Trabalhos (falhos e recusados, com o motivo em português).
- **Decidido:** **adiar**, como a issue #141 (DEC-122).

### 3.7 Uso e orçamento dos provedores (UI-23, RF-046)

- **Regra já confirmada (RF-046):** só o dono; leitor e administrador **não** acessam valores financeiros nem pela interface, nem pela API; há limite global mensal e limites por área; **estimativas aparecem rotuladas**; ao atingir o limite, **novas chamadas externas são bloqueadas**.
- **Realidade hoje:** os provedores externos vêm **desligados**, e o único com chave no alpha é o ComicVine (gratuito, com limite de chamadas, não de dinheiro). O modelo financeiro ainda **não está desenhado** (preço por chamada, moeda, o que é "apurado" e o que é "estimado").
- **Recomendação:** **fora do beta.** Antes de qualquer tela, um PR de modelo: tabela de **chamadas** por provedor e mês (o que se pode medir **sem** inventar preço), contador visível ao dono e **bloqueio por número de chamadas**. A parte monetária só entra quando houver um provedor pago de verdade.
- **Decidido:** **fora do beta**, contador de chamadas antes do dinheiro, na issue #142 (DEC-122).

## 4. Todas as telas: estados

Seguem o padrão da #77 (`Notice`, `LoadError`, `PermissionNote`): carregamento, vazio, erro com tentativa de novo, **sem permissão** e **parcial**, todos em português. As mensagens do servidor passam por `serverMessage.js` e o teste de conferência com o código do servidor continua valendo.

## 5. Próximo passo

Cada tela vira um PR com testes, mutação e e2e no navegador, na ordem da DEC-123: **Sobre** primeiro. Se faltar alguma tela na lista (por exemplo, **excluir minha conta** ou **papéis e permissões**), entra como proposta nova.
