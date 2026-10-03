# Códice: levantamento do dicionário (issue #109, DEC-115)

Relatório do primeiro passo da DEC-115: **de onde vêm os verbetes, com que licença, de que tamanho, e o que de fato há neles**. Escrito em 03/10/2026. O que está medido num arquivo real está marcado **(medido)**; o que veio só de ler uma página está marcado **(lido)**; o que não foi verificado está marcado **(não verificado)**.

## Resumo

- **Fonte recomendada para a primeira versão:** a extração do **Wikcionário em português** feita pelo Wiktextract (`pt-extract.jsonl.gz`, em kaikki.org). Um só arquivo de **35,4 MB** baixados (357 MB descompactado) serve **os dois cartões** que o mantenedor pediu: **definição no mesmo idioma** para palavra em português e **tradução** (glosa em português) para palavra em inglês e outras línguas.
- **O que o arquivo resolve bem (medido):** português **completo** nos 36 vocábulos de teste, **inclusive formas flexionadas** ("correram", "falaram", "meninos"), que apontam para o lema ("correram → correr") sem lematizador. Os verbetes de português têm definições **em português**.
- **O que ele não resolve:** **espanhol, francês e italiano** têm pouca cobertura nele (de 50% a 80% das palavras de teste, quase nenhuma forma flexionada). Se a biblioteca tiver muito disso, é preciso o **Wikcionário daquele idioma** (arquivos próprios, listados abaixo), numa segunda etapa. **Exemplos de uso são raros** (17 mil verbetes de 401 mil).
- **Licença:** CC BY-SA e GFDL (**lido**). O Códice **não redistribui** os dados: o dono baixa e importa. O cartão mostra a **atribuição**.
- **Tamanho no servidor (estimado):** cerca de **120 MB** de JSON enxuto para pt, en, es, fr e it; no PostgreSQL, na ordem de **250 MB** com índices (**não medido**).

## O que foi examinado

| Fonte | Como foi examinada | Estado |
|---|---|---|
| **Extração do Wikcionário em português** (`pt-extract.jsonl.gz`, kaikki.org) | **Baixado e analisado inteiro** (35,4 MB, com permissão do mantenedor): 627.516 linhas | **medido** |
| Extração do Wikcionário em inglês (kaikki.org) | Páginas de verbetes ("correr", "correram") e a página de arquivos; **o arquivo (23,9 GB) não foi baixado** | **lido** |
| Documentação do Wiktextract (GitHub) | Campos do verbete e licença da ferramenta | **lido** |
| Extrações do Wikcionário em espanhol, italiano e francês | Só tamanhos na página de arquivos; **não baixadas** | **lido** |
| Dicionário Aberto (português) | Só resumos de artigos e do site | **lido**, **não verificado** |
| StarDict, FreeDict, dictd | Só descrições gerais | **lido** |

## A extração do Wikcionário em português (medido)

**Arquivo:** `https://kaikki.org/dictionary/downloads/pt/pt-extract.jsonl.gz`, 37.158.613 bytes (35,4 MB); descompactado, 357.567.002 bytes (341 MB). Uma linha por verbete e classe gramatical, em JSON. Extraído do dump `ptwiktionary` de 01/09/2026 (página do Kaikki: extração de 02/10/2026; o `.gz` traz 28/09/2026).

**Quantos:** 627.516 linhas; **401.234 de português**, 31.880 de galego, 18.127 de inglês, 11.406 de espanhol, 8.729 de japonês, 6.117 de francês, 5.016 de italiano, 3.948 de latim, entre mais de cem línguas. Em português: 565.624 sentidos; **verbo 292.953** (a maioria são **formas conjugadas**, cada uma com seu verbete), substantivo 64.173, adjetivo 27.875, locução 8.497, advérbio 4.557.

**Campos de um verbete (medido):** `word`, `lang_code`, `pos`, `senses` (cada um com `glosses`, `tags`, `form_of`, `examples`, `topics`), e, no verbete: `forms` (formas flexionadas do lema), `translations`, `synonyms`, `etymology_texts`, `sounds`, `derived`, `expressions`. A linha mais longa tem 104 mil caracteres: a importação deve ler **linha a linha**.

### Português para português: definições no mesmo idioma

Verbetes reais (medido):

- **livro** (substantivo): "objeto feito de várias folhas de papel, organizadas em ordem e contendo um texto", com exemplo e fonte; mais dois sentidos. Há **também** um verbete "livro" que é **forma do verbo livrar** ("primeira pessoa do singular do presente do indicativo do verbo livrar"): **homógrafos** são normais e o cartão deve mostrar os dois, o lema primeiro.
- **saudade**: "memória ou recordação grata de pessoas, familiares ou objetos"; "sentimento experimentado entre duas ou mais pessoas que estão distantes…"; tradução para dezenas de línguas.
- **correr**: "mover-se com rapidez", "apressar-se", "percorrer"; `forms` com "correndo", "corrido", "corro", "corres"…

**Formas flexionadas:** 305.525 verbetes de português têm `form_of` apontando para o lema. **"correram"** vem como "terceira pessoa do plural do pretérito perfeito do indicativo do verbo correr", com `form_of: correr`. Isso resolve o risco que a #109 apontava ("sem lematização a busca falha em português") **sem lematizador**, para o que o Wikcionário cobre.

**Cobertura em 36 palavras comuns de português (medido):** **36 de 36**, incluindo "correram", "falaram", "disse", "fez", "viu", "meninos", "livros", "felizmente", "apesar", "saudade".

### Outras línguas para português: a tradução

No arquivo, uma palavra inglesa vem com **glosa em português**, que é a tradução: `book` → "livro" (com exemplo traduzido: "My life is an open book." → "Minha vida é um livro aberto."), `book` (verbo) → "reservar", `run` → "correr", "dirigir". Cobertura em 31 palavras comuns de inglês: **30 de 31** (faltou "books", o plural).

**Limites (medido):**

- Só há **18.127 verbetes de inglês** (o Wikcionário em inglês tem muito mais, **não medido**). Palavras comuns vêm; palavras raras, não.
- Formas flexionadas de inglês vêm como glosa em texto ("passado simples e particípio do verbo (to) walk"), com `form_of` em apenas 2.651 dos 18.127: o **lema precisa ser lido da glosa** ou o cartão mostra a glosa como está.
- **Espanhol, francês e italiano:** 11.406, 6.117 e 5.016 verbetes; cobertura nas palavras de teste de **67%, 81% e 50%**, e quase nenhuma forma flexionada ("corrieron", "coururent", "corsero" faltam). Aqui o arquivo **não basta**.

### Exemplos e qualidade

- **Exemplos de uso:** só **17.155 verbetes de português** (de 401 mil) e 22.391 sentidos têm exemplo. O cartão não pode contar com eles.
- O **Kaikki marca as extrações de edições que não são a inglesa como "em andamento"** (**lido**): pode haver erros e lacunas. A amostra examinada não mostrou erros visíveis, mas **40 verbetes não são uma auditoria**.
- Há ruído pontual: na edição inglesa, o verbete "correram" aparece em uma categoria de "entradas com cabeçalho de idioma incorreto" (**lido**); nada que impeça o uso.

## Tamanho (estimado a partir do arquivo, medido)

Mantendo só o que o cartão usa (palavra, classe, sentidos com glosa, etiquetas, `form_of`, até 2 exemplos, formas e traduções para pt, en, es, fr, it), a importação tem **441.900 linhas e 120 MB de JSON**, com **559.255 palavras ou formas distintas** a indexar. O maior verbete enxuto tem 9,8 KB. No PostgreSQL, com índices por idioma e palavra, estimo **na ordem de 250 MB** (**não medido**; vale medir na primeira importação). Importar é uma passada linear: **minutos**, em segundo plano, com andamento visível (**não medido**).

## Licença e redistribuição (lido)

- Os dados do Wikcionário são **CC BY-SA** (e GFDL), como diz a página do Kaikki. A ferramenta Wiktextract é **MIT**.
- **O projeto não redistribui o dicionário**: o dono do acervo baixa o arquivo e o importa para o **seu** servidor. Não há dado de terceiros no repositório (a AGPLv3 do código não se mistura com a CC BY-SA do dado).
- **O cartão mostra a atribuição** ("Fonte: Wikcionário, CC BY-SA; extraído com Wiktextract, kaikki.org"), e o pacote guarda a **fonte, a edição e a data** de cada importação. Quem reutilizar o banco importado precisa manter a licença (compartilhamento pela mesma licença): é uso privado do dono.
- Para a citação acadêmica, o Kaikki pede o artigo do Wiktextract (Ylonen, LREC 2022); não se aplica ao uso no aplicativo, mas cabe na página "sobre".

## Alternativas (lido, não verificado)

| Fonte | Para que serviria | Por que não é a primeira |
|---|---|---|
| **Wikcionário em inglês** (kaikki.org, 23,9 GB descompactado, 2,8 GB `.gz`) | Definições **em inglês** de palavras de todas as línguas, a maior cobertura | Definição de palavra portuguesa viria em inglês; o arquivo é 80 vezes maior |
| **Wikcionários em espanhol, italiano e francês** (kaikki.org: es 97,3 MB, it 41,6 MB e fr 699,9 MB compactados) | Definições no **próprio idioma** para livros nessas línguas | Só vale se a biblioteca tiver essas línguas; **não foram baixados** |
| **Dicionário Aberto** | Dicionário de português em domínio público (TEI/XML) | Parte de um dicionário de **1913** (vocabulário e grafia antigos); sem formas flexionadas; **não examinado** |
| **StarDict** | Qualquer dicionário em formato pronto | Cada verbete é um texto solto, **sem campos e sem índice de formas**; licença varia por dicionário. Serve como **importador opcional** depois |
| **FreeDict (TEI/dictd)** | Dicionários bilíngues de tradução | Traduz palavras, **não define**; não atende a leitura de literatura em português |

## Desenho proposto (para a #109, a confirmar)

1. **Servidor:** `dictionary_packages` (fonte, edição, idioma, data, licença, contagem) e `dictionary_entries` (pacote, idioma, lema, classe, sentidos, etiquetas, formas) e uma tabela **`dictionary_forms`** (idioma, forma normalizada → lema), para "correram → correr". Normalização da busca: minúsculas, sem pontuação e **sem acento** (índice por forma normalizada, mostrando a forma original).
2. **A palavra sai da seleção:** o menu de seleção (#123) ganha **"Dicionário"** na seleção de **uma palavra**; o **idioma** é o do arquivo (DEC-092, 096), com troca manual no cartão.
3. **O cartão:** **lema e classe** primeiro; **definições em português** (ou, para outra língua, a **tradução**); a forma ("forma de correr") quando a palavra selecionada for flexionada; os **homógrafos** juntos (lema antes de forma verbal); a **atribuição** embaixo.
4. **Importar de um arquivo, não de um endereço:** na primeira versão a administração **recebe o arquivo** que o dono baixou (**nenhum acesso do servidor à internet**). Um botão "baixar" do próprio servidor, com tamanho à vista e permissão do dono, fica para depois. Importação por um **job do worker**, com limite de memória (linha a linha) e andamento visível.
5. **Fatias, um PR cada:** (a) migração, importador e testes, com um **arquivo de amostra** pequeno de verbetes reais; (b) a consulta (`GET /dictionary?lang=&word=`) e o cartão no leitor; (c) a tela de administração (instalar, ver, remover); (d) outras línguas (es, it, fr) se a biblioteca precisar.

## Decisões a tomar (do mantenedor)

1. **Fonte da primeira versão:** a extração do Wikcionário em português (`pt-extract`, 35,4 MB). De acordo?
2. **Quais línguas além do português importam de verdade?** Se a biblioteca tiver espanhol, francês ou italiano, a segunda etapa traz o Wikcionário daquela língua. Posso olhar os idiomas dos arquivos do acervo para responder com dados.
3. **Importar de um arquivo enviado pelo dono** na primeira versão, deixando o "baixar pelo servidor" para depois?

## O que não foi verificado

- **Qualidade em geral** da extração em português (uma amostra pequena, sem auditoria).
- **As extrações em espanhol, italiano e francês** (só tamanhos).
- **O Dicionário Aberto e o StarDict** (só descrições).
- **O tamanho final no PostgreSQL e o tempo de importação** (estimados).
- **Atualização**: o Kaikki diz que atualiza a cada poucos dias; o Códice trataria cada importação como uma **versão** do pacote (a política de atualizar fica para a fatia (c)).
