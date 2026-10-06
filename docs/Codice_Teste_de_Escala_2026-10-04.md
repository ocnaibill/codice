# Códice: teste de escala (1.000 e 10.000 obras)

Item 3 do caminho até o beta. A pergunta: **o Códice aguenta o acervo de uma pessoa que importa a biblioteca inteira de uma vez?** Mediu-se a leitura (lista, busca, contagens, ficha, OPDS, capas, administração, login), o uso simultâneo e o trabalho do worker para ler tudo, com 1.000 e depois 10.000 obras sintéticas, numa pilha descartável de `docker-compose.full.yml`.

Como repetir: [`benchmarks/scale/README.md`](../benchmarks/scale/README.md). O gerador é `testdata/generate_scale.py`; o medidor, `benchmarks/scale/measure.py`.

## O que foi (e o que não foi) medido

- **Máquina:** Mac com 10 CPUs e 8 GB para o Docker, disco SSD, PostgreSQL 15, **um** worker, **um** processo da API. Uma máquina de homelab modesta vai ser mais lenta; **compare o antes e o depois, não o número absoluto.**
- **Acervo:** 65% EPUB (com capa), 20% CBZ (4 páginas), 10% TXT, 5% PDF; ~9 KiB por arquivo; 2.500 autores e 833 séries; cinco idiomas. **Só a quantidade de obras escala aqui, não o tamanho dos arquivos.** Um acervo real tem arquivos de dezenas de MiB: o que isso muda (tempo de leitura de cada arquivo, espaço) **não foi medido**.
- **Cobertura:** a API e o banco, e a troca de páginas na tela do navegador. **Não** se mediu o desenho da interface com milhares de itens nem aparelhos fracos.
- **Uso simultâneo:** 20 requisições ao mesmo tempo, 300 de cada mistura, sem pausa entre elas. É **mais** do que uma família faz; serve para achar o ponto que quebra.

## Resultados

### Uma requisição de cada vez (p50, em milissegundos)

| Requisição | 1.000 obras | 10.000, **sem** o índice (como estava) | 10.000, **com** o índice (corrigido) |
|---|---:|---:|---:|
| lista: primeira página (50) | 11 | 71 | 59 |
| lista: página do meio | 39 | **2.303** | 471 |
| lista: última página | 99 | **4.152** | 526 |
| lista: só quadrinhos | 10 | 70 | 37 |
| lista: filtro por palavra do título | 7 | 63 | 33 |
| contagens da barra lateral (`/stats`) | 20 | 75 | 83 |
| ficha da obra | 5 | 4 | 3 |
| busca: palavra do título (muitos resultados) | 53 | 501 | 476 |
| busca: palavra comum no texto | 47 | 485 | 439 |
| busca: palavra que está numa obra só | 4 | 24 | 25 |
| busca: sem resultado | 3 | 28 | 22 |
| OPDS: recentes (50) | 2 | 3 | 2 |
| OPDS: busca | 2 | 4 | 3 |
| administração: trabalhos | 1 | 7 | 5 |
| capa (uma imagem) | 1 | 1 | 1 |
| login (6 entradas) | 208 | | 217 |

(A coluna de 1.000 obras foi medida **sem** o índice; é a comparação justa com a do meio.)

### Vinte de uma vez (requisições por segundo e p95 em ms)

| Mistura | 1.000 obras | 10.000 sem o índice | 10.000 com o índice |
|---|---:|---:|---:|
| a lista (primeira, meio, última) | 83 req/s, p95 628 | **1 req/s, p95 37.348** | 9 req/s, p95 4.320 |
| a busca (título, palavra comum, uma obra) | 60 req/s, p95 658 | 5 req/s, p95 6.838 | 6 req/s, p95 5.921 |
| o que abrir o aplicativo pede | 495 req/s, p95 83 | 63 req/s, p95 808 | 65 req/s, p95 886 |
| capas | 1.716 req/s, p95 25 | 969 req/s, p95 44 | 942 req/s, p95 48 |

Erros: **0** em todas as medições.

### Trazer o acervo (worker)

| | 1.000 obras | mais 9.000 obras (até 10.000) |
|---|---:|---:|
| tempo até a fila zerar | **172 s** (5,8 obras/s) | **3.293 s** (55 min, 2,7 obras/s) |
| jobs rodados | 4.807 | 40.521 |
| falhas | 0 | 0 |
| a chamada de importação em massa | 2,4 s | até 46 s (varre 10.000 arquivos, 1.000 já conhecidos) |

Na segunda etapa as 9.000 obras novas **já existiam como linhas do banco desde o início da importação**; os jobs é que as liam aos poucos. A correção do índice entrou **no meio do caminho** (aos ~6 min, com ~2.600 das 10.000 obras lidas): até ali a fila andava a **5,6 jobs/s**, com o `dedupe` levando **~5 s por arquivo** (a carga das 10.000 linhas era varrida a cada vez); depois subiu para 13 a 25 jobs/s. **Sem o índice o mesmo trabalho levaria muitas horas** (não esperei para medir).

### Espaço e memória, com 10.000 obras

- **Biblioteca em disco:** 147 MiB (arquivos de ~9 KiB: não vale como estimativa de acervo real).
- **Banco:** 467 MiB, dos quais **419 MiB são o texto para a busca** (`document_segments`: 62.744 trechos, 64 MiB de texto, e o resto é o índice de busca guardado em duas configurações). **O banco ficou ~6,5 vezes maior que o texto.** Isto foi medido com texto **sintético** (palavras quase todas distintas): eu esperava que num livro real o índice pesasse menos, mas **a primeira rodada real (`Codice_Rodada_2026-10-04.md`) mediu 7,0 vezes** (4 GB de `document_segments` para 573 MB de texto): a razão sintética estava certa, e ela decide o espaço de disco de quem importa uma biblioteca grande. O backup **não** leva esse índice (é refeito depois de restaurar).
- **Memória em repouso:** com 1.000 obras, API 14 MiB, worker 52 MiB, banco 80 MiB; com **10.000**, API 26 MiB, worker 51 MiB, **banco 156 MiB**, Redis 5 MiB, site 8 MiB (durante a leitura o banco passou de 180% de CPU). Nada perto de pressionar uma máquina de 4 GB.
- **Tela (navegador real, com 10.000 obras):** a biblioteca abre com a barra lateral mostrando `[10.000]`; a troca de página leva ~53 ms; a página paginada (12 obras por vez, 834 páginas) **não** tem rolagem infinita, então o navegador nunca segura milhares de cartões.

## Achados

### 1. Faltava um índice em `editions(work_id)`: **corrigido neste PR** (migração 00050)

`editions` só tinha a chave primária e um índice "a edição primária de cada obra". Toda consulta "as edições desta obra" lia **a tabela inteira**: o cartão da lista (para cada obra), a ficha, e a carga do detector de duplicatas. O custo crescia com o **quadrado** do acervo: a última página da lista foi de 99 ms (1.000 obras) para **4.152 ms** (10.000), 42 vezes mais para 10 vezes mais obras. Carregar as obras para a checagem de duplicatas levava **5,8 s a cada arquivo importado** com 10.000 obras (0,14 s com o índice). Com vinte pessoas na lista ao mesmo tempo, **1 requisição por segundo e 37 s de espera**; com o índice, 9 por segundo.

### 2. A lista por deslocamento (`OFFSET`) ainda custava por página: **corrigido no PR seguinte**

Mesmo com o índice, a página do meio e a última levavam ~500 ms com 10.000 obras (e a primeira, 59 ms). O plano de execução mostrou o motivo: para a página 100 (`OFFSET 4950`) o banco montava o cartão completo das **5.000** obras anteriores (ligações, contagens, *tags*) e só então descartava 4.950. **0,09 ms por obra pulada**, linear no deslocamento. Na tela real isso quase não aparecia, mas aparecia sob uso simultâneo (9 req/s com p95 de 4 s para 20 pessoas).

**Correção:** a página agora é escolhida antes dos cartões, no `FROM` mais barato que os filtros permitem (só `works` na visão padrão, na busca por título e na ordem por título), e só as obras da página recebem o cartão. Medido no mesmo acervo de 10.000 obras (semeado direto no banco, `main` contra o ramo, p50 em ms):

| Consulta (12 por página) | antes | depois |
|---|---:|---:|
| primeira, meio e última página (padrão) | 41 / 117 / 469 | **4 / 4 / 4** |
| página 800 | 472 | 4 |
| página 800, ordem por título | 567 | 12 |
| página 800, ordem por autor | 591 | 113 |
| página 500, só livros digitais | 170 | 75 |
| página 200, busca por "caminho" | 102 | 17 |
| página 300, busca e ordem por título | 513 | 27 |
| primeira página, só livros digitais | 39 | 41 |
| 20 de uma vez na lista | 20 req/s, p95 2.754 | **683 req/s, p95 53** |

O que ainda precisa dos joins (filtro por formato, favoritos, em leitura, ordem por autor) continua pagando por eles, mas só até a página pedida. O total (`COUNT`) também deixou de montar os joins quando nenhum filtro os usa.

### 3. A busca por palavra comum custa ~450 ms com 10.000 obras: **proposta, não feita**

A busca ordena **todos** os trechos que casam (`ts_rank_cd` em cada um, depois `DISTINCT ON` e ordenação) para só então pegar os 20 primeiros. Uma palavra comum ("caminho", "silêncio") casa com dezenas de milhares de trechos: **886 ms** na medição do banco, 439 ms ponta a ponta. Cresce com o número de trechos que casam. Uma palavra rara ou ausente leva 22 a 25 ms. **Aceitável para uma pessoa**; sob 20 ao mesmo tempo, 6 req/s e p95 de 6 s. Opções (a decidir): limitar os trechos classificados por palavra muito comum, ou classificar por obra antes de por trecho. Não é urgente.

### 4. O detector de duplicatas relê **todas** as obras a cada arquivo: **proposta, não feita**

Mesmo com o índice, `dupes.Detect` carrega todas as obras do acervo e compara uma a uma **para cada arquivo importado** (ISBN, título e autor em memória). O `dedupe` foi o tipo de job mais caro (15.328 jobs, média de 0,21 s, máximo de 7,9 s) e **o custo por arquivo cresceu durante a leitura**: 0,048 s nos primeiros 300 `dedupe` e 0,171 s nos últimos 300 (com o índice). **Não separei** quanto desse crescimento é a comparação das obras (linear no acervo) e quanto é a checagem de conteúdo parecido (`detectContent`/`detectTranslation`, que cresce com o texto já lido). Em ordem de grandeza, o total de uma importação cresce **mais que linear** com o acervo: para a faixa do beta (centenas a poucos milhares de obras) é **desprezível**; para quem importa dezenas de milhares de uma vez, é o gargalo previsível (extrapolação linear, **não medida**: algo como 0,8 s por arquivo com 50.000 obras). **Proposta:** primeiro **medir cada parte do `Detect`** com acervo grande e, se a comparação em memória for a parte maior, buscar candidatos no banco (ISBN e título normalizado guardados e indexados) em vez de carregar tudo; exigiria guardar o título normalizado (migração e PR próprios).

### 5. O OPDS só mostra 50 obras (recentes) ou os resultados da busca

`/opds/v1.2/recent` e `/opds/v1.2/search` têm `LIMIT 50` e não paginam; **não há navegação pelo acervo inteiro** (autores, séries, alfabética). Com 10.000 obras, um leitor externo (KOReader, Moon+, Panels) só alcança as 50 mais novas ou o que achar buscando. Os dois feeds respondem em 2 a 4 ms; **o limite é de funcionalidade, não de desempenho**. Fica como lacuna do beta: ver se é decisão do mantenedor (issue própria).

### 6. O resto está bem

A ficha (3 a 5 ms), as capas (1 ms; ~950 req/s), as telas de administração (≤ 7 ms), o OPDS (2 a 4 ms), abrir o aplicativo (sidebar, favoritos, perfil: 65 req/s com 20 de uma vez), o login (~210 ms, o custo do *hash* da senha, estável) e o uso de memória não mudaram de ordem de grandeza de 1.000 para 10.000 obras. As contagens da barra lateral (`/stats`) são 20 ms → 83 ms: crescem com o acervo e vale vigiar.

## O que fica para o mantenedor decidir

1. **Fazer os PRs 3 e 4 agora ou depois do beta?** O 2 (lista) já foi feito, a pedido do mantenedor. Recomendação: **depois do beta**, só se os testes de fogo mostrarem acervo grande de verdade. Para a faixa de poucos milhares de obras, nada disso pesa.
2. **A paginação do OPDS** (achado 5): é lacuna de funcionalidade, não de escala.
3. **O espaço do índice de busca** (achado de espaço): medir com a biblioteca real nos testes de fogo (#89) antes de dizer ao dono quanto disco reservar.

## Limites desta medição

- Texto sintético (vocabulário de 3.000 palavras): a busca e o tamanho do índice **num texto real podem ser diferentes**.
- Arquivos pequenos: **não** mede a leitura de arquivos grandes, nem OCR, nem o modelo local de similaridade (desligados).
- Um worker e uma API: a concorrência de 20 é sobre o banco e a API, não sobre vários workers.
- A máquina de medição não é a de homelab: **repita no seu servidor** (o README do conjunto diz como) antes de tirar conclusões de tempo absoluto.
