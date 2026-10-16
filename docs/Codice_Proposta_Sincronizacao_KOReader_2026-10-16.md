# Proposta: sincronização com o KOReader (e, por ele, com o Kindle com jailbreak)

**Etapa 1 decidida (DEC-190, 16/10/2026); o resto continua proposta.** Nada aqui está implementado. As decisões estão na seção 6. Escrita em 16/10/2026 a partir de uma conversa sobre "ter o Códice dentro do Kindle". O que o protocolo exige foi lido no código do KOReader (seção 2); o que é desenho do Códice é marcado como tal.

## 1 O que se quer, e o que cada aparelho permite

| Aparelho | Entregar livros | Progresso de leitura | Anotações |
| --- | --- | --- | --- |
| Kindle sem jailbreak | por e-mail (a Amazon converte); só ida | **não há**: o Whispersync é fechado e não tem API | só o arquivo `My Clippings.txt`, copiado por USB |
| Kindle com jailbreak + KOReader | **OPDS**, que o Códice já tem (DEC-071) | **KOSync**, que o Códice não tem | arquivos de anotação do KOReader |
| Kobo, Boox, Android com KOReader | OPDS | KOSync | o mesmo |

O que decide é o **KOSync**, não o Kindle: um servidor compatível com ele dá sincronização de progresso a **qualquer aparelho com KOReader**, e o Kindle com jailbreak entra junto. O aparelho de teste do mantenedor é um Kindle de 10ª geração, firmware 5.17.1.0.3, já com jailbreak e KUAL.

Limites: não existe firmware customizado de verdade para Kindle (o jailbreak dá execução de scripts e apps); a tela e o processador não servem para a wiki nem para o grafo; o jailbreak depende da versão do firmware (as brechas de versões recentes foram fechadas pela Amazon), então a promessa para outras pessoas é "se o seu aparelho já aceita KOReader".

## 2 O protocolo, lido no código do KOReader

Fontes: `plugins/kosync.koplugin/` (`KOSyncClient.lua`, `main.lua`, `api.json`) e `frontend/util.lua`, em `koreader/koreader`.

**Endpoints** (cabeçalho `Accept: application/vnd.koreader.v1+json`; JSON):

| Chamada | Método e caminho | Corpo | Respostas |
| --- | --- | --- | --- |
| registrar | `POST /users/create` | `username`, `password` | 201 criado, 402 já existe |
| autorizar | `GET /users/auth` | nenhum | 200, 401 |
| gravar progresso | `PUT /syncs/progress` | `document`, `progress` (texto), `percentage`, `device`, `device_id` e, se ligado, `metadata` (nome do arquivo, título, autores) | 200 ou 202, 401 |
| ler progresso | `GET /syncs/progress/:document` | nenhum | 200 com os mesmos campos mais `timestamp`, 401 |

Todas, menos o registro, mandam `x-auth-user` (o nome) e `x-auth-key`. O cliente (`KOSyncClient.lua`) repassa o valor sem tratar; **quem transforma a senha no valor enviado é a tela de login**, e relatos de outros servidores dizem que é o **MD5 da senha**. Isso **precisa ser conferido no aparelho** antes de implementar.

**O documento** é identificado por um hash do arquivo, não pelo nome nem por um id do servidor: por padrão, o **MD5 parcial** (`util.partialMD5`): até 12 amostras de 1 KiB, nos deslocamentos 1024 × 4ⁱ (1 KiB, 4 KiB, 16 KiB, ... 1 GiB), e a primeira amostra pelo que o LuaJIT fizer com o deslocamento negativo (provavelmente o início do arquivo; **conferir contra um arquivo real**). Há também um modo por nome de arquivo (MD5 do nome), que o Códice não vai suportar: o nome muda.

**O que vai em `progress`:** número da página nos documentos paginados (PDF, CBZ) e um **xpointer** nos reflowable (EPUB), por exemplo `/body/DocFragment[7]/body/div/p[12]/text().0`; `percentage` é um número de 0 a 1.

**Quando ele fala com o servidor** (só com `auto_sync` ligado, que vem desligado): puxa ao abrir o livro e ao acordar o aparelho, empurra 10 s depois de uma virada de página (a cada N páginas), ao fechar o livro e ao suspender. Há espera mínima de 25 s entre chamadas iguais e uma fila para quando está sem rede. **Conflito:** o cliente decide **pela hora** (`timestamp` do servidor contra a última virada de página local), **não pela posição**: "avançar" quer dizer "o do servidor é mais novo" e "voltar" quer dizer "o do servidor é mais velho", e cada um tem a opção de pular em silêncio, perguntar ou nunca (padrões: perguntar e nunca). Medido na seção 5.

## 3 Desenho (do Códice)

1. **Autenticação.** O KOReader manda o MD5 da senha digitada. Hoje o Códice guarda só um hash do token de aplicativo, então não tem como comparar. Proposta: um token de aplicativo com **escopo `koreader`**, gerado pelo Códice e mostrado uma vez; quem o digita como "senha" no KOReader envia `md5(token)`, e o servidor guarda `sha256(md5(token))` e compara com isso. O valor guardado não permite refazer o que o aparelho manda, e o token tem entropia de sobra para o MD5 não ser o elo fraco. `x-auth-user` é o nome da conta; o registro (`/users/create`) responde recusando, porque contas não nascem por ali.
2. **Identidade do arquivo.** Uma coluna nova em `files` com o MD5 parcial, calculada pelo worker (lê até 12 KiB por arquivo; custa quase nada) para os arquivos que existem e para os que entram. Mais de um arquivo com o mesmo hash só acontece com cópias idênticas, que o Códice já trata como duplicata por `sha256`.
3. **Progresso.** `PUT /syncs/progress` vira uma gravação em `reading_progress` do arquivo, com o aparelho escrito no campo `device` (por exemplo "KOReader · Kindle"). `GET` devolve o que o Códice sabe do arquivo, com o `timestamp` que o cliente espera.
   - **EPUB:** o xpointer traz o `DocFragment[N]`, que é o N-ésimo item da lista de leitura (spine); o servidor o liga ao `href` do item e grava o localizador do Códice no **começo desse item**; o `percentage` vai para `percent_complete`. **A precisão da v1 é a do capítulo.** No sentido contrário (Códice → KOReader), o servidor devolve o xpointer do começo do item.
   - **PDF e CBZ:** a página vai direto.
   - **Livros que o KOReader abre e o Códice não reconhece:** 200 sem corpo (o cliente entende como "sem progresso").
4. **Conflito.** O KOSync não tem revisão; o Códice tem (`revision`, RF-031). A v1 é **última gravação vale**, com a hora do servidor, e cada gravação sobe a `revision` normalmente; o leitor web, que manda `base_revision`, continua protegido de apagar o que o KOReader gravou.
5. **A wiki.** O ponto que a wiki respeita (DEC-179) vem do progresso de leitura. Gravar o progresso do KOReader no **começo do item** erra sempre para trás: nunca adianta o corte, só pode mostrar menos. Isso é o comportamento seguro e fica como está.
6. **Interface.** Em Conta > Dispositivos (UI-21): criar o token `koreader`, e as instruções de três passos (servidor, usuário, "senha"), com o endereço do servidor já preenchido; e uma linha por aparelho que já sincronizou (nome, última vez).

## 4 Etapas

0. **OPDS primeiro** (confirmado em 16/10/2026): o catálogo atual tem defeitos (o feed de busca usa o prefixo `opensearch` sem declará-lo, não há paginação nem miniaturas, só um arquivo por obra) e quase nenhuma navegação (só "Recentes" e a busca). Dois PRs: (a) correção e base, (b) navegar por autor, série, categoria, coleção, favoritos e "continuar lendo". **Antes deles, o token curto e o `opds.lua` pronto**: o Kindle do mantenedor recusou o catálogo porque digitar o token de 47 caracteres numa tela de e-ink falhou (a aba Entradas mostrou a tentativa como "aplicativo com senha errada").
1. **Servidor KOSync:** token com escopo, coluna e preenchimento do hash, os quatro endpoints, o mapeamento do EPUB, do PDF e do CBZ, testes de integração, e **um roteiro de teste no Kindle do mantenedor** (KOReader com `auto_sync` ligado, ler, fechar, conferir no leitor web e o caminho de volta).
2. **Tela de Dispositivos:** o token, a lista de aparelhos e o **arquivo `kosync.lua` pronto para baixar** (ver seção 5), no lugar de ensinar a digitar o endereço e a senha no aparelho.
3. **Anotações:** importar `My Clippings.txt` (serve também ao Kindle sem jailbreak) e os destaques do KOReader, como notas do Códice (DEC-109 e a regra de notas).
4. **Envio por e-mail** para o Kindle sem jailbreak.
5. **Plugin em Lua** (estantes, favoritos e notas dentro do KOReader; a aparência do KOReader parecida com a do Códice): só depois de 0 a 3 mostrarem uso.
   - **"Parou bem aqui" no sentido contrário** (ideia do mantenedor, 16/10/2026): o leitor web já destaca o parágrafo e avisa quando o livro abre onde um aparelho parou (seção 8); no Kindle isso **não dá hoje**, porque o KOReader não recebe um destaque de fora, e o Moon+ Reader nem usa o KOSync. Um plugin poderia marcar, ao abrir o livro, o parágrafo onde a pessoa parou no web, com o nome do aparelho do outro lado ("Seu Códice parou bem aqui").

**O token curto e os arquivos prontos** (confirmados): o escopo `koreader` usa 16 caracteres (minúsculas e números sem os que se confundem), em grupos de 4, e o servidor ignora espaços e maiúsculas; serve de senha tanto do OPDS quanto do KOSync. O KOReader guarda os catálogos em `settings/opds.lua` (lista `servers`, cada um com `title`, `url`, `username`, `password`) e o KOSync em `settings/kosync.lua` (`custom_server`, `username`, `userkey`, `auto_sync`); a tela gera esses arquivos com o token na hora em que ele é criado (é a única vez que ele existe em claro). Cuidado a dizer na tela: o `opds.lua` gerado **substitui** o do aparelho; quem já tem catálogos seus deve copiar só a entrada.

Isto entra no que a especificação já chama de "KOReader e OPDS Progression: adaptadores candidatos" (E3, QA-018); a matriz de clientes homologados passa a ter o KOReader no Kindle 10ª geração.

## 5 O que foi medido e o que falta

**Medido em 16/10/2026** num Kindle de 10ª geração (o KOReader o chama de `KindleBasic3`), firmware 5.17.1.0.3, com jailbreak, contra um servidor de mentira que só registrava as chamadas:

- **A chave é o MD5 da senha.** Com a senha `teste123`, o registro mandou `password` = `aa1bf4646de67fd9086cf6c79007026c`, que é exatamente o MD5 dela, e as chamadas seguintes mandaram o mesmo valor em `x-auth-key`, com o nome em `x-auth-user`. O código do KOReader confirma (`local userkey = md5(password)`; guarda só esse valor). O desenho da seção 3, item 1, está certo.
- **O hash do arquivo é o MD5 parcial com a primeira amostra no início do arquivo** (deslocamento 0, e não 256). O hash que o aparelho mandou (`836c0286...`) é o do EPUB `As foices` do acervo do mantenedor, e os dois exemplares idênticos da pasta dão o mesmo hash.
- **O progresso de um EPUB** vem como `/body/DocFragment[6]/body/div/p[166]/text().0`, com `percentage` = `0.0625`, `device` = `KindleBasic3` e um `device_id` de 32 caracteres. `DocFragment[6]` é o **sexto item da lista de leitura (spine)** do EPUB: no arquivo, o sexto item é `Text/cap02.xhtml`, que tem 181 parágrafos (o `p[166]` cabe), e o Códice conhece esse capítulo pelo mesmo `href`. O `DocFragment` é, portanto, 1-based no spine.
- **Quando fala:** ao abrir o livro, o KOReader fez o `GET` do progresso; 33 segundos depois, ao virar páginas, fez o `PUT`. O servidor respondeu `{}` ao `GET` (sem progresso) e o KOReader aceitou.
- **Padrões do KOReader (do código):** `auto_sync` **desligado**; "sincronizar para frente" em **perguntar**; "para trás" em **nunca**. A pessoa precisa ligar o `auto_sync` uma vez.

**Segundo teste (o servidor devolveu um progresso de outro aparelho, "Codice-web"):**

- **Servidor à frente do Kindle** (`DocFragment[9]`, com o Kindle no 6): o KOReader **perguntou se queria sincronizar com "Codice-web"**; ao aceitar, abriu **no começo do capítulo 5, "Travessa dos infratores"**, que é o item 9 da lista de leitura do EPUB. O salto para um capítulo de outro aparelho funciona.
- **Servidor atrás do Kindle** (`DocFragment[3]`, 5%): o KOReader **também perguntou** ("Sincronizar para a localização mais recente (5%) do dispositivo Codice-web?"). Não era o esperado (a opção "para trás" vem em "nunca"), e a explicação é a regra da seção 2: o servidor devolveu o `timestamp` de agora, mais novo que a última virada de página local, e o KOReader o tratou como "mais novo" e aplicou a opção **de avançar**, que vem em "perguntar". **Quem decide é a hora, não a posição.**
- **O que isso pede do Códice:** o `timestamp` do `GET` tem de ser **a hora real da última gravação de progresso** (`updated_at` de `reading_progress`), não a hora da consulta. Com isso, quem leu por último vale (decisão 1), e o KOReader ignora sozinho o progresso que ele mesmo gravou (mesmo `device` e `device_id`), sem perguntar.
- **A pergunta aparece sempre que o outro aparelho é o mais recente.** O mantenedor decidiu manter a pergunta (seção 6, decisão 3).

**Ainda falta:**

- PDF e CBZ: o formato de `progress` nesses (a página, pelo código).
- Se o KOReader do Kindle acorda o Wi-Fi a tempo de puxar ao acordar o aparelho.

**A configuração fica num arquivo.** O plugin guarda tudo em `settings/kosync.lua` (chaves `custom_server`, `username`, `userkey`, `auto_sync`, `sync_forward`...). O Códice pode gerar esse arquivo já preenchido (endereço da instância, nome, o `userkey` que o aparelho calcularia e `auto_sync` ligado) e a pessoa só o copia para a pasta `settings` do KOReader: **sem digitar nada no Kindle**. É bem menos do que um plugin e resolve o incômodo de configurar na mão. Isso fica como alternativa de interface na etapa 2.

## 6 O que depende do mantenedor

Confirmadas pelo mantenedor em 16/10/2026:

1. **Última gravação vale.** Uma gravação do KOReader nunca é recusada por ser mais antiga que a do leitor web.
2. **O progresso do KOReader move o ponto da wiki**, sempre para o começo do capítulo (seção 3, item 5).
3. **O KOReader pergunta** quando o outro aparelho é o mais recente, como já faz (o `kosync.lua` gerado não muda a opção). Razão do mantenedor: quem volta páginas de propósito precisa que o aparelho saiba disso, e a pergunta deixa a pessoa escolher.

Também confirmadas em 16/10/2026 ("ok, concordo"): **precisão de capítulo na v1** e **token separado com escopo `koreader`**. Tudo isso é a **DEC-190**, que cobre só a etapa 1 (o servidor); as etapas seguintes ainda precisam de decisão.

## 7 Levar o livro ao aparelho

O KOSync só leva o **progresso**; o livro vai por outro caminho, e cada aparelho tem o seu:

| Aparelho | Como o livro chega | Quem puxa e quem empurra |
| --- | --- | --- |
| Kindle com KOReader | **OPDS** (o Códice já tem): o KOReader lista o acervo e baixa; também a **sincronização de uma pasta** pela nuvem do KOReader (WebDAV, FTP ou Dropbox), que traria os livros novos sozinha, mas pede um servidor WebDAV ao lado do Códice | o aparelho puxa |
| Kindle sem jailbreak | **Enviar por e-mail** para o endereço do Kindle (a Amazon aceita EPUB e converte); precisa de SMTP configurado no Códice e do endereço do servidor na lista de remetentes aprovados da conta Amazon | o Códice empurra |
| qualquer um | USB | a pessoa |

A v1 desta proposta usa o OPDS que já existe; a pasta WebDAV e o e-mail são extensões separadas (etapa 4).

## 8 O leitor web diz onde o aparelho parou

Depois que o KOSync passou a guardar o **parágrafo** (e não só o capítulo), o leitor web pode mostrar o que o aparelho sabia. Ao abrir um livro **no ponto em que outro aparelho o deixou**, o leitor avisa ("Seu Kindle parou bem aqui") e **sublinha o parágrafo por alguns segundos**. Regras (decididas pelo mantenedor em 16/10/2026):

- O nome é o que a pessoa escreveu em **"Qual aplicativo vai usar?"** ao criar o acesso (por exemplo "Kindle da sala"), não o nome interno do KOReader ("KindleBasic3"). O servidor guarda os dois: o nome da pessoa para o web, e o modelo do KOReader para devolver a ele o que ele espera.
- Só avisa quando a última gravação veio de **outro aparelho** (um KOReader), não de outro navegador do web; e **uma vez por gravação**: abrir o mesmo livro de novo, sem nada novo do aparelho, não repete. Não avisa quando a pessoa pediu um ponto (uma nota, uma referência da wiki) ou escolheu começar do início.
- No EPUB, o aviso e o sublinhado; em PDF e quadrinho, só o aviso, com a página ("Seu Kindle parou na página 12").
- Acessível: o aviso vai para a região de avisos (leitor de tela), o sublinhado tem contorno além de cor, e some sem movimento para quem pediu menos movimento.
