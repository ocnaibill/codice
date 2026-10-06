# Relatório da rodada de testes reais: alpha, 4 a 6 de outubro de 2026

Primeira rodada do roteiro da [issue #89](https://github.com/ocnaibill/codice/issues/89), no servidor do alpha do mantenedor, com o acervo dele. Os números vêm de `scripts/relatorio-rodada.sh` (leituras antes, depois e durante um incidente) e das observações ao vivo do agente que opera o servidor. **Não traz o nome de nenhuma obra nem de arquivo.** Modelo: [`Codice_Rodada_de_Testes_MODELO.md`](Codice_Rodada_de_Testes_MODELO.md).

## 1. Quem e onde

| | |
|---|---|
| Servidor | LXC 104 num Proxmox, i5 de 12ª geração, x86_64; 10 GiB de memória visível aos contêineres; pilha completa (`docker-compose.full.yml`) com o worker, o OCR, o serviço de embeddings (LaBSE) e o outpost LDAP do Authentik |
| Versão da rodada | `75c259f` (a `main` de 4 de outubro); depois da rodada, atualizado para `9220b04` |
| Acesso | `https://codice.ocnaibill.dev`, por túnel da Cloudflare; contas: 1 dono e 2 leitores (entradas por senha local e por LDAP) |
| Aparelhos com sessão aberta | Linux (3), macOS (3), Android (1), mais 6 de aplicativo/OPDS/script |

## 2. O acervo

**2.958 obras, um arquivo cada.** Importadas por varredura de pastas (**2.930 referenciadas**, ~41 GiB que ficam fora da pilha; **28 gerenciadas**).

| Formato | Arquivos | Tamanho |
|---|---:|---:|
| PDF | 2.645 | 40 GB |
| MP3 | 295 | 1.464 MB |
| EPUB | 12 | 25 MB |
| CBR | 5 | 198 MB |
| CBZ | 1 | 15 MB |

Por tamanho: 658 abaixo de 1 MiB; 1.203 de 1 a 10 MiB; 1.044 de 10 a 100 MiB; 53 de 100 MiB a 1 GiB. 2.940 edições sem idioma declarado.

## 3. O que o servidor fez

**Linha do tempo (UTC).** A varredura (`scan`) levou 30,6 min (04/10 21:30 a 22:01) e enfileirou 2.930 ingestões. A ingestão terminou 170 min depois (05/10 00:20). **A fila toda (texto, OCR, embeddings, deduplicação) só esvaziou em 06/10 10:11**: cerca de **36,7 h** depois do início da varredura. Resultado final: **18.726 trabalhos concluídos, 0 falhas** (depois de repetir os 56 de OCR, ver abaixo).

| Tipo de trabalho | Trabalhos | Média | p95 | Máximo |
|---|---:|---:|---:|---:|
| ingest (ler o arquivo, capa, metadados) | 2.958 | 3,4 s | 5,8 s | 31 s |
| extract_text | 2.958 | 1,1 s | 4,7 s | 60 s |
| organize | 2.958 | 0,2 s | 0,01 s | 271 s |
| ocr | 2.018 | 42 s | 197 s | 37 min |
| embed_text | 3.378 | 32 s | 139 s | 10 min |
| dedupe | 4.451 | 14 s | 53 s | 17 min |

**Uma obra fica lida e abrível cedo** (ingestão e texto em ~4,5 s por arquivo); **o que demora é o enriquecimento em segundo plano**, que é feito um trabalho por vez em cada serviço: o OCR (≈ 24 h de relógio), os embeddings (≈ 30 h) e a deduplicação (≈ 17 h do worker principal) correm em paralelo entre si.

**Texto e OCR.** Texto pronto em 2.679 arquivos (1.092 de PDF com texto, 965 mistos, 622 só por OCR; 573 MB de texto no total); 265 sem texto possível (áudio, quadrinhos) e 14 vazios. OCR: **28.736 páginas lidas** em 1.587 arquivos e 9.212 em branco; nenhuma falha ao final. Idioma detectado no texto: português em 2.274 arquivos, inglês em 62.

**Metadados e duplicatas.** Sugestões pendentes: 2.326 detectadas, 1.669 da Open Library e 28 da ComicVine (19 aceitas e 7 rejeitadas à mão). Possíveis duplicatas pendentes: 568 de tradução, 300 de título e autor, 142 de conteúdo; 5 já ligadas. 29 junções de pessoas propostas.

## 4. Espaço e peso

**O banco ficou com 10 GB** para uma biblioteca de 41 GB (a pasta gerenciada, 2,4 GB). O que pesa:

| Tabela | Tamanho | O que é |
|---|---:|---|
| `document_segments` | 4.022 MB | o texto para a busca, **7,0 vezes** os 573 MB de texto |
| `document_segment_embeddings` | 2.856 MB | os vetores do modelo local (3.378 trabalhos) |
| `dictionary_entries` + `dictionary_forms` + `dictionary_links` | 3.491 MB | os dicionários instalados (pt, en, fr, ja); com só 28 obras o banco já tinha 3,7 GB, quase tudo isto |
| `text_fingerprints` | 157 MB | a deduplicação por conteúdo |
| `ocr_pages` | 66 MB | o texto lido por OCR (entra no backup) |

**Medido com texto real**, a razão do índice de busca (7,0×) confirma a do teste de escala com texto sintético (6,5×): **reserve de 7 a 8 vezes o texto só para a busca**, mais os vetores e os dicionários se estiverem ligados. Memória em repouso: backend 21 a 26 MiB, banco 215 MiB, worker 61 MiB; durante a leitura, embeddings chegou a 886 MiB e o OCR a 336 MiB.

## 5. O que passou, o que falhou, o que não foi verificado

| Item | Resultado | Evidência |
|---|---|---|
| Importar 2.930 arquivos reais por varredura de pasta referenciada | **passou** | 0 falhas na ingestão e no texto |
| Ler 41 GiB de PDFs (digitais, mistos e escaneados) com texto | **passou** | 2.679 com texto; 0 falhas |
| OCR de PDFs escaneados reais | **passou depois de corrigir a pasta** | 28.736 páginas; ver o achado 2 |
| Modelo local de embeddings (LaBSE) com o acervo real | **passou** | 3.378 trabalhos, 0 falhas; pesa 2,9 GB no banco |
| Dicionários (pt, en, fr, ja) instalados | **passou** | 4 trabalhos, ~3,5 GB |
| Backup criptografado, verificação profunda e ensaio de restauração | **passou** | pacote verificado; 2.958 obras restauradas num banco temporário; 2.958 arquivos listados e 28 incluídos (**os referenciados precisam de backup à parte**, já dito no guia) |
| Entrada por LDAP (Authentik) e local | **passou** | entradas bem-sucedidas pelos dois caminhos; um nome desconhecido recusado e registrado |
| Queda de energia e reinício do host | **passou** | os sete serviços voltaram sozinhos e saudáveis (saída 0, sem falta de memória, sem pânico nos logs); o PVE caiu sem desligar |
| Uso do app durante a importação | **falhou, corrigido** | achado 1 |
| Leituras, notas, progresso e o resto do roteiro 4 e 6 | **não informado neste relatório** | o mantenedor os está exercitando |
| Leitor de tela (VoiceOver, NVDA, TalkBack) | **não verificado** | depende da segunda pessoa do beta |

## 6. O que quebrou

1. **A importação travou o app (corrigido, #160).** Durante a varredura e de novo na fila, o Postgres foi a 258–324% de CPU com 25 a 28 consultas ativas, o pool de 25 conexões encheu, o login ficou 35 s sem resposta, `/healthz` deu 503 e o dicionário não respondeu; o backend ficou `unhealthy`, sem reiniciar e sem falta de memória. **Causa:** cada obra processada gera dois eventos e cada evento fazia **cada aba aberta** refazer a tela inicial inteira (5 consultas), sem limite. **Correção:** o cliente agrupa os eventos (uma atualização e, depois, no máximo uma a cada 4 s por aba, nenhuma em aba escondida) e o servidor limita as leituras pesadas do catálogo a 8 por vez (`CODICE_CATALOG_CONCURRENCY`), antes da autenticação. Reproduzido localmente com 30 mil obras, 7 abas e 4 eventos por segundo: `/auth/me` de 6,7 s para 7 ms; 40 eventos, de 162 para 15 requisições por aba.
2. **O OCR não achava os arquivos (documentação, #161).** Foram 56 trabalhos de OCR com `FileNotFoundError`: a pasta externa estava montada no backend e no worker, como o README mandava, mas **o serviço de OCR também a lê**. Montada no OCR, os trabalhos repetidos passaram. O README agora monta nos três.
3. **O script de relatório não media tudo (corrigido, #161).** O "Biblioteca em disco" só conta a pasta gerenciada (os 41 GiB referenciados não apareciam) e o "do primeiro ao último" misturava trabalhos de antes da rodada: ganhou o tamanho por modo e o `--desde`.

## 7. A deduplicação: o que o perfil mostrou, e por que não vou mexer nela agora

Os trabalhos de `dedupe` mais lentos (de 5 a 17 min) são todos da hora do incidente (04/10, 22h a 23h, banco saturado): os 4 trabalhos da hora 23h tiveram **média de 799 s**. **Sem esses 4, a média do conjunto é de ~13 s**, e por hora ela varia de **7 s a 48 s** (as horas de 03h a 07h do dia 05, com 340 a 520 trabalhos, ficaram em 7 a 14 s; as de 08h a 12h, com poucos trabalhos e já com muito texto de OCR por perto, em 23 a 48 s). A duração **não cresce com o tamanho do texto** (a faixa de 50 mil a 500 mil caracteres é a mais lenta, de 24 a 29 s de média, mas é a que tem os trabalhos do incidente; a de 1 a 3 milhões de caracteres, só 204 trabalhos, ficou em 13 s).

**Decisão: não otimizar o `dedupe` antes do beta.** O custo é de segundo plano: uma obra fica abrível em ~4,5 s, e o `dedupe` do worker principal (≈ 17 h) corre em paralelo com o OCR (≈ 24 h) e os embeddings (≈ 30 h), de modo que **mesmo que ele fosse instantâneo, a fila toda ainda levaria ~30 h** (os embeddings dominam). A hipótese do teste de escala de que ele seria o próximo gargalo valia para o custo de comparar com todas as obras, mas neste acervo o que apareceu foi **contenção do incidente**, não o algoritmo. Fica registrado, para olhar de novo com um acervo maior ou se o tempo de fila incomodar: 1.432 das 2.958 obras tiveram o `dedupe` rodado de 2 a 3 vezes (depois da ingestão e de novo quando o texto chega ou o OCR termina), e as leituras de pares de possíveis traduções que não deram em nada se repetem a cada rodada.

## 8. Para a próxima rodada

- Rodar de novo o relatório com `--desde` e uma re-importação pequena, **já com o #160**, para confirmar que o app fica responsivo durante a fila.
- **Experimento de paralelismo:** a máquina estava quase ociosa depois da fila (carga 0,3), e cada serviço trabalha em um trabalho por vez. Em **Administração → Sistema → Desempenho** (DEC-125) o dono sobe, um de cada vez, as **páginas lidas juntas pelo OCR** e as **comparações de duplicatas ao mesmo tempo**, e mede o efeito no tempo da fila, na CPU e na memória com o relatório. (Uma correção a este relatório: eu tinha sugerido subir `JOBS_MAX_CONCURRENT` e `EMBEDDINGS_MAX_CONCURRENT`; **isso não acelera nada num contêiner só**, porque cada processo executa um trabalho por vez. Paralelizar a ingestão e os embeddings exigiria threads com conexões próprias e fica para depois.)
- Registrar a leitura de verdade (EPUB, PDF, CBR, áudio) e o que a segunda pessoa encontrar em Windows e Android.
