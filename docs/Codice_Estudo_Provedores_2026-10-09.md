# Estudo dos provedores de metadados (09/10/2026)

Itens 5 e 8 das notas do beta: *"os provedores parecem fracos; não sei se é o provedor ou o nosso código"*, *"o ComicVine aponta sempre para o mesmo capítulo"*, *"para mangá nem existe"*. Este documento é um **estudo**: mede, explica e recomenda. **Nada foi alterado no código**; a refatoração vem depois, pelas decisões da seção 7.

O material do experimento (casos, scripts, resultados) está em [`estudos/provedores-2026-10-09/`](estudos/provedores-2026-10-09/LEIAME.md) e pode ser repetido.

## 1. Resposta curta

**É principalmente o nosso código.** Os provedores têm limites reais, mas o que o mantenedor viu se explica, em boa parte, por cinco defeitos nossos, quase todos baratos de corrigir:

1. **A Open Library parece "pobre" porque não pedimos os campos.** A busca dela só devolve ISBN, editora e assuntos se o pedido disser `fields=`; o nosso não diz. Resultado medido: **ISBN, editora e assuntos vieram em 0 de 13 respostas** (com o campo pedido, a mesma busca traz, para *Dune*, 240 ISBNs, 92 editoras e 20 assuntos). A **descrição nunca é pedida** (ela vive em outra chamada, `/works/{id}`): 0 de 13.
2. **O Google Books provavelmente nunca funcionou sem chave, e nunca saberíamos.** Sem chave, o pedido cai na **cota diária compartilhada de um projeto anônimo do Google**, que hoje está esgotada (HTTP 429, "Queries per day… project_number:…"); a documentação diz que o pedido sem OAuth "deve enviar uma chave". O nosso código aceita só o HTTP 200 e **descarta qualquer outro status sem registrar nada**.
3. **Pedimos um resultado só e o aceitamos sem conferir.** `limit=1` / `maxResults=1`, e o ranking final só escolhe entre esses poucos. Não há limiar: se o provedor devolve outra coisa, vira sugestão. Medido: **1 livro errado em 16** ("Duna" → um livro do Brian Herbert) e, nos quadrinhos e mangás, **2 errados e 1 duvidoso em 9** ("Saga 001" → um livro de Christopher Paolini, "One Piece 001" → "100 deadly skills").
4. **A consulta é só o título do arquivo.** O pipeline manda `metadata.title`; o autor e o ISBN que o próprio arquivo traz **não vão**, e o título sujo quebra a busca: "A Nuvem 2 - Neal Shusterman" e "Scythe - Neal Shusterman" (nome de arquivo com o autor) devolvem **nada**.
5. **O ComicVine é consultado do jeito errado para o que o mantenedor viu** (seção 4). Não consegui confirmar sem chave; a seção 8 traz um roteiro curto para o mantenedor confirmar com a chave dele.

**Do lado dos provedores**, três limites reais: títulos em português de obras cuja ficha está em inglês (a Open Library não os acha); mangá e quadrinhos têm dado **por título (a série)** e dado **por volume**, em bases diferentes; e há regras de uso (chave, taxa, identificação, uso não comercial) que hoje o código ignora.

## 2. Como o código pede hoje

| Fato | Onde | Consequência |
|---|---|---|
| Só `metadata.title` é enviado | `worker/pipeline.py` (`search_best(metadata.title, …)`) | autor e ISBN do arquivo não ajudam |
| Um resultado por provedor (`limit: 1`, `maxResults: 1`) | os três provedores | o primeiro resultado é "a resposta" |
| Sem limiar de aceitação | `registry._rank_results` | o "melhor" é sempre devolvido, certo ou não; o ranking soma bônus de completude |
| Open Library sem `fields`, sem `/works/{id}` | `openlibrary.py` | sem ISBN, editora, assuntos, descrição |
| Google Books: chave opcional; status ≠ 200 vira `None` calado | `google_books.py` | cota anônima esgotada passa despercebida |
| ComicVine: `resources=issue`, `limit=1` | `comicvine.py` | ver seção 4 |
| `User-Agent` padrão do `requests` (OL, GB) | os provedores | a Wikimedia e o MangaDex recusam; a Open Library dá 1/3 da taxa |
| Sem registro de saúde por provedor | — | a aba Provedores não diz "sem chave", "cota esgotada" ou "não achou" |

## 3. O experimento

**Método.** 25 casos com resposta conhecida (16 livros, 4 quadrinhos ocidentais, 5 mangás), cada um com o que o pipeline mandaria. Medi o **código real** do worker (`current.py`, só Open Library, porque o Google Books está sem cota e o ComicVine exige chave) contra um **protótipo** (`improved.py`, descartável) que: lê o título do arquivo (número, autor), pede até 8 candidatos com título e autor separados e o texto livre, pontua pelo título e pelo autor, **só aceita o que é próximo**, e então busca a descrição da obra escolhida. Resultado em três classes: **certo**, **nada** (nenhuma sugestão) e **errado** (sugestão de outra obra). Para o acervo, *nada* é muito melhor do que *errado*.

### Livros (16)

| | certo | nada | errado | ISBN | editora | assuntos | descrição | capa |
|---|---|---|---|---|---|---|---|---|
| **Código atual** | 12 | 3 | 1 | 0 de 13 | 0 de 13 | 0 de 13 | 0 de 13 | 13 de 13 |
| **Protótipo** | 13 | 3 | 0 | 13 de 13 | 13 de 13 | 13 de 13 | 10 de 13 | 12 de 13 |

- A Open Library é um serviço vivo: uma segunda rodada do código atual deu 13 certos e 2 "nada" (a ordem dos resultados varia). A conclusão não muda.
- O ganho em **riqueza** (os campos que o mantenedor sentiu falta) vem de pedir os campos e a obra, não de uma busca mais esperta. O ganho em **segurança** vem do limiar: o "errado" some.
- Os 3 "nada" do protótipo são todos **títulos em português de obras cuja ficha na Open Library está em inglês** ("Duna", "Harry Potter e a Pedra Filosofal", "A Nuvem"). Isso é limite da fonte, e a Wikidata resolve (seção 5).

### Quadrinhos ocidentais (4) e mangás (5)

| | código atual (Open Library) | alternativa medida |
|---|---|---|
| Quadrinhos ocidentais | 0 certos, 2 nada, 1 errado, 1 duvidoso (*Absolute Batman 006*, *Sandman 001*: nada; *Saga 001*: errado; *Watchmen 01*: edição sem autor, não dá para conferir) | não medida: ComicVine e Metron exigem chave ou conta |
| Mangás | 2 certos, 2 nada, 1 errado (*Berserk* e *Naruto*: nada; *One Piece 001*: "100 deadly skills") | **AniList e MangaDex acharam os 5 certos**, com autores, gêneros, ano, sinopse e capa; o MangaDex ainda diz o **público** (*seinen*, *shounen*) |

O mangá mostra o desenho certo: o **dado do título** (série, autores, gêneros, público, ano, sinopse) vem de AniList/MangaDex, ricos e certos; o **dado do volume** (ISBN, capa do volume, data) vem de base de livros: a Open Library achou 3 volumes certos de 5 (*Vagabond 1*, *Naruto 01*, *One Piece 1*) e 2 discutíveis (uma edição *Deluxe* de *Berserk*; uma caixa de 13 volumes de *Chainsaw Man*).

## 4. O ComicVine ("sempre o mesmo capítulo")

**Hipótese forte, não confirmada.** O código busca `resources=issue` com `limit=1` e o nome do arquivo ("Absolute Batman 006"). A maioria das edições de quadrinhos **não tem nome próprio**; o que casa é o nome da **série**, então todas as edições empatam e o primeiro resultado é sempre o mesmo: **qualquer número que se envie devolve a mesma edição**, como o mantenedor viu. O número do arquivo é ignorado na busca (só entra depois, no ranking).

**O jeito certo** (a confirmar com a chave): buscar o **volume** (`resources=volume`, escolhendo por nome, ano de início e editora) e então pedir a **edição desse volume** por filtro (`filter=volume:{id},issue_number:{n}`), e só então a ficha da edição (`/issue/4000-{id}`) para os créditos. A documentação pública do ComicVine lista `volume` e `issue_number` como campos da edição, o formato de filtro `filter=campo:valor`, e `person_credits` como campo da edição, não do resultado de busca; **que os dois campos de fato filtram é o que o roteiro da seção 8 confirma**.

**Confirmação.** `probe_comicvine_googlebooks.py` (só leitura) mostra, com a chave do mantenedor, o que a busca de hoje responde para três edições da mesma série e o que o caminho volume→edição responde. Ver a seção 8.

## 5. Autores (item 4 das notas: "consumir de algum local sobre o autor")

Medi Open Library, Wikidata e Wikipédia para 6 autores (*Frank Herbert, Machado de Assis, Paulo Coelho, Kentaro Miura, Andrew Tanenbaum, Neal Shusterman*).

- **Buscar autor por nome é perigoso.** A busca de autores da Open Library devolveu **o homônimo errado** para *Frank Herbert* (um Frank Herbert Hayward, nascido em 1872) e *Paulo Coelho* (o Coelho Netto, 1902). Nome não identifica pessoa (já é a lógica das DEC-093 e DEC-095).
- **A chave do autor, vinda da obra casada, identifica.** A busca de obras da Open Library devolve `author_key` (*Dune* → `OL79034A`); a ficha desse autor (`/authors/OL79034A`) é a certa e aponta a Wikidata (`Q7934`); e a Wikidata, buscada por `haswbstatement:P648=OL79034A`, devolve a mesma pessoa. **Verificado.**
- **A Wikidata e a Wikipédia dão o que a página de autor quer**: descrição e biografia curta **em português** (137 a 669 caracteres nos 6 autores), data de nascimento, foto (5 de 6), e identificadores de autoridade (VIAF, ISNI, Open Library) em todos os 6. Os identificadores casam com o que a DEC-095 já espera (a chave só vale depois que um humano aceita o autor).
- **Licença:** a Wikidata é CC0; o texto da Wikipédia é CC BY-SA 4.0 (pede atribuição e link). A Wikimedia **exige `User-Agent` identificável** (sem ele, 429).

**Títulos em português (o limite da Open Library).** A Wikidata busca por título em português e acha a obra: *"Duna"* → `Q190192` (*"romance de ficção científica de 1965 do Frank Herbert"*), *"Harry Potter e a Pedra Filosofal"* → `Q43361`, *"Dom Casmurro"* → `Q1236531`, e cada obra traz **o id da obra na Open Library** (`P648`), o autor e o ano. É a ponte de "título em português" para "obra na Open Library", e funciona para os 3 casos em que o protótipo disse *nada*. (Testado a busca e a ponte; a escolha entre candidatos homônimos, como o filme *Duna*, pede o autor e o tipo "obra literária".)

## 6. Os provedores, lado a lado

"Ligado" quer dizer o que a DEC-097 já manda: todos começam **desligados**, e o dono liga um a um.

| Fonte | Traz | Chave / conta | Limite | Uso e licença | Veredito |
|---|---|---|---|---|---|
| **Open Library** | obra, autores (com chave), ISBNs, editoras, assuntos, séries (`series_name`), descrição (por obra) | não | 1 req/s; **3 req/s com `User-Agent` com contato** (doc oficial) | não usar a API para baixar em massa (há dumps mensais); dados abertos | **manter e consertar** (campos, obra, vários candidatos) |
| **Google Books** | título localizado, descrição, ISBN, categorias, capa | **sim**: a doc exige chave sem OAuth; sem ela, cota anônima compartilhada (esgotada hoje) | cota por projeto (não conferi o número) | termos do Google | **manter, exigindo chave e dizendo quando falta**; é a melhor fonte para título em português |
| **ComicVine** | quadrinhos ocidentais: volume, edição, créditos, capa | **sim** | não conferi | termos da Fandom; uso não comercial (não confirmei no texto) | **manter, consertando a consulta** (volume→edição), depois do roteiro |
| **AniList** | mangá/anime: título (romaji, inglês, nativo), autores, gêneros, tags, ano, sinopse, capa | não | 90 req/min (pode estar em 30) | grátis para uso não comercial, e comercial abaixo de US$ 150/mês; **proíbe usar a API como cópia de dados e a coleta em massa** | **adicionar para mangá**; guardar só o que o admin aceita |
| **MangaDex** | mangá: títulos e sinopse em vários idiomas, autores, **público** (seinen, shounen…), tags, capas | não | ~5 req/s por IP; `User-Agent` obrigatório e sem disfarce; **sem hotlink de imagem** | política de uso aceitável (leitura de terceiros diz uso não comercial; **confirmar o texto**) | **adicionar como complemento** (público e títulos em português) |
| **Metron** | quadrinhos ocidentais, abertos | **conta** | 20 req/min e 5.000/dia | licença **não confirmada** | **candidato para depois**, se o ComicVine não bastar |
| **Wikidata** | obras e autores em português, ids de autoridade, ponte para a Open Library | não | pedir com `User-Agent` identificável | CC0 | **adicionar** (títulos e autores) |
| **Wikipédia** | biografia curta, foto | não | idem | CC BY-SA 4.0 (atribuição) | **adicionar** para a página do autor |
| Hardcover | livros, GraphQL | chave | 60 req/min | a doc fala em "uso offline por enquanto": **incerto** | **fora por ora** |

Não medi: Google Books, ComicVine e Metron (chave ou conta), Grand Comics Database (só há dump), Jikan e Kitsu (o AniList e o MangaDex já cobrem o mangá).

## 7. O que recomendo, e o que depende do mantenedor

### Desenho

1. **Ler o arquivo melhor**: título limpo, **número** (volume/edição) e **autor** separados (o protótipo já separa "A Nuvem 2 - Neal Shusterman"), e **ISBN** quando o arquivo tem (a busca por ISBN é a mais certa que existe).
2. **Vários candidatos e pontuação**: título, autor, número, ano e idioma; **limiar**; **"nada" é resposta válida**. A tela de sugestões passa a mostrar **por que** (qual candidato e o quanto casou).
3. **Dois níveis para quadrinhos e mangá**: a série (AniList/MangaDex; ComicVine pelo volume) e depois o volume ou edição (Open Library/Google Books; ComicVine pela edição).
4. **Detalhes só do escolhido**: descrição, assuntos, séries.
5. **Título em português**: Google Books (com `langRestrict`) e a ponte da Wikidata, antes de desistir.
6. **Autor pela chave, não pelo nome**: Open Library → Wikidata → Wikipédia, com atribuição, e o identificador guardado como chave de autoridade (DEC-093 e DEC-095).
7. **Saúde por provedor**: registrar o status de cada chamada (sem chave, 401, 429, vazio) e mostrá-lo na aba Provedores ("Google Books: sem chave; ligue só depois de configurar"), com um botão de teste.
8. **Identificação**: `User-Agent` com o nome do projeto e a URL, e um contato **opcional configurado pelo dono** (a Open Library dá o triplo da taxa com contato; a Wikimedia e o MangaDex exigem identificação).
9. **Assuntos viram categorias**: os assuntos da Open Library (com `fields`), as categorias do Google Books, os gêneros do AniList e o **público do MangaDex** alimentam as tags, e daí as regras de categoria (DEC-140): "seinen" e "shounen" passam a vir prontos.

### Decisões que são do mantenedor

1. **Google Books exige chave?** Recomendo que **sim**: o provedor só liga com chave, e a aba diz isso. A chave é gratuita (projeto no Google Cloud) e é sua para criar.
2. **Adicionar AniList e MangaDex para mangá?** Recomendo **sim** (AniList primeiro, MangaDex de complemento), ambos desligados por padrão. Condição: uso pessoal/não comercial, e guardar só o que o admin aceitar (a regra do AniList contra "coleta em massa" pede isso).
3. **Adicionar Wikidata e Wikipédia para autores e títulos?** Recomendo **sim**, com atribuição do texto da Wikipédia.
4. **ComicVine**: manter e consertar, **depois** de rodar o roteiro da seção 8. **Metron**: só se o ComicVine não bastar.
5. **Contato no `User-Agent`**: o e-mail do dono iria a terceiros, então é decisão sua. Sem ele, tudo funciona, só mais devagar na Open Library.
6. **"Nada" em vez de "errado"**: recomendo o limiar (o protótipo usa 70 de pontuação **e** 0,85 de proximidade do título), e a tela mostrar quando nada foi aceito, com o melhor candidato e a nota.
7. **Mostrar os candidatos ao admin** (e não só a melhor sugestão): mais trabalho de tela; recomendo.
8. **Uso comercial**: o Códice é AGPL e auto-hospedado, e os provedores acima servem a esse uso. Quem **oferecer** o Códice como serviço pago esbarra em AniList, MangaDex e ComicVine; vale uma frase na documentação.

### Ordem sugerida de PRs (cada um pequeno, com teste e conferência)

- **A.** *Consertos baratos* (maior ganho, menor risco): `fields` e a obra na Open Library, `User-Agent`, status e chave do Google Books com aviso, vários candidatos e limiar.
- **B.** Ler o título do arquivo (número, autor) e usar o ISBN.
- **C.** Mangá em dois níveis (AniList, MangaDex).
- **D.** ComicVine volume→edição (depois do roteiro).
- **E.** Autores (Open Library → Wikidata → Wikipédia), junto com o desenho da página do autor (item 4 das notas).
- **F.** Aba Provedores com saúde e botão de teste.

## 8. Limites do estudo, e o que falta medir

- **Amostra pequena** (25 casos, 5 mangás, 4 quadrinhos) e **julgamento meu** dos resultados; serve para achar os defeitos e dimensionar, não para dar uma taxa de acerto.
- **Google Books** não foi medido (cota anônima esgotada). **ComicVine** e **Metron**, idem (chave e conta). O **fluxo volume→edição** do ComicVine é hipótese até alguém rodá-lo.
- A Open Library varia entre rodadas; os números acima são os dos arquivos `resultado_*.json`.
- Não testei AniList e MangaDex com títulos em português nem com séries que a comunidade nomeia de outro jeito.

**Para o mantenedor fechar o que falta** (só lê, não muda nada, e não mostra a chave):

```bash
COMICVINE_API_KEY=... GOOGLE_BOOKS_API_KEY=... worker/venv/bin/python docs/estudos/provedores-2026-10-09/probe_comicvine_googlebooks.py
```

Com o resultado dele eu confirmo ou corrijo a seção 4 e completo a tabela de livros com o Google Books.
