# Proposta: sincronização com o KOReader (e, por ele, com o Kindle com jailbreak)

**Proposta, não decisão.** Nada aqui está implementado nem aprovado; as decisões que dependem do mantenedor estão na seção 6. Escrita em 16/10/2026 a partir de uma conversa sobre "ter o Códice dentro do Kindle". O que o protocolo exige foi lido no código do KOReader (seção 2); o que é desenho do Códice é marcado como tal.

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

**Quando ele fala com o servidor** (só com `auto_sync` ligado, que vem desligado): puxa ao abrir o livro e ao acordar o aparelho, empurra 10 s depois de uma virada de página (a cada N páginas), ao fechar o livro e ao suspender. Há espera mínima de 25 s entre chamadas iguais e uma fila para quando está sem rede. **Conflito:** o cliente decide pela hora (`timestamp` do servidor contra a última virada de página local); "avançar" e "voltar" têm cada um a opção silenciosa, perguntar ou nunca (voltar vem desligado).

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

## 4 Etapas propostas

1. **Servidor KOSync:** token com escopo, coluna e preenchimento do hash, os quatro endpoints, o mapeamento do EPUB, do PDF e do CBZ, testes de integração, e **um roteiro de teste no Kindle do mantenedor** (KOReader com `auto_sync` ligado, ler, fechar, conferir no leitor web e o caminho de volta).
2. **Tela de Dispositivos:** o token, as instruções e a lista de aparelhos.
3. **Anotações:** importar `My Clippings.txt` (serve também ao Kindle sem jailbreak) e os destaques do KOReader, como notas do Códice (DEC-109 e a regra de notas).
4. **Envio por e-mail** para o Kindle sem jailbreak.
5. **Plugin em Lua** (estantes, favoritos e notas dentro do KOReader): só depois de 1 a 3 mostrarem uso.

Isto entra no que a especificação já chama de "KOReader e OPDS Progression: adaptadores candidatos" (E3, QA-018); a matriz de clientes homologados passa a ter o KOReader no Kindle 10ª geração.

## 5 O que não sei e é preciso medir

- Que valor o KOReader manda em `x-auth-key` (suposição: `md5(senha)`).
- O MD5 parcial exato (o deslocamento da primeira amostra) contra um arquivo real do acervo e o hash que o aparelho calcula do mesmo arquivo baixado por OPDS.
- Quanto o `DocFragment` de um EPUB do acervo corresponde ao item do spine que o Códice usa (EPUBs com itens não lineares ou com a capa fora da lista).
- Se o KOReader do Kindle acorda o Wi-Fi a tempo de puxar o progresso (é uma opção do plugin que depende do aparelho).

## 6 O que depende do mantenedor

Confirmadas pelo mantenedor em 16/10/2026:

1. **Última gravação vale.** Uma gravação do KOReader nunca é recusada por ser mais antiga que a do leitor web.
2. **O progresso do KOReader move o ponto da wiki**, sempre para o começo do capítulo (seção 3, item 5).

Ainda em aberto:

3. **Precisão de capítulo na v1** (recomendado) ou esperar até haver precisão de parágrafo?
4. **Token separado com escopo `koreader`** (recomendado) ou aceitar qualquer token de aplicativo?
5. Vale registrar como DEC e abrir a issue de acompanhamento antes de começar?

## 7 Levar o livro ao aparelho

O KOSync só leva o **progresso**; o livro vai por outro caminho, e cada aparelho tem o seu:

| Aparelho | Como o livro chega | Quem puxa e quem empurra |
| --- | --- | --- |
| Kindle com KOReader | **OPDS** (o Códice já tem): o KOReader lista o acervo e baixa; também a **sincronização de uma pasta** pela nuvem do KOReader (WebDAV, FTP ou Dropbox), que traria os livros novos sozinha, mas pede um servidor WebDAV ao lado do Códice | o aparelho puxa |
| Kindle sem jailbreak | **Enviar por e-mail** para o endereço do Kindle (a Amazon aceita EPUB e converte); precisa de SMTP configurado no Códice e do endereço do servidor na lista de remetentes aprovados da conta Amazon | o Códice empurra |
| qualquer um | USB | a pessoa |

A v1 desta proposta usa o OPDS que já existe; a pasta WebDAV e o e-mail são extensões separadas (etapa 4).
