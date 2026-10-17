# Wiki automática: laboratório e decisões (13 a 15/10/2026)

Relatório do laboratório da wiki gerada a partir do texto dos livros (issue #286), feito na worktree do lab (`lab/wiki/`, branch local `lab/ocr-quadrinhos`). Registra o formato aprovado, o que foi medido, os defeitos achados e as decisões do mantenedor. As decisões estão na especificação como **DEC-177 a DEC-183** e os requisitos como **RF-050 a RF-052**. Nada disto está implementado no Códice; a ordem de trabalho está na seção 7.

## 1 O que é

Para cada obra ou série, uma página por entidade (pessoa, lugar, organização, conceito, objeto, evento) no estilo de uma wiki de fãs, escrita **só com o que o texto do livro sustenta**, cada frase com referência ao capítulo e citação literal, e **sem revelar nada além do ponto em que a pessoa está lendo**. Junto, um grafo das relações entre as entidades.

## 2 Como é gerada (o pipeline do lab)

1. **Texto:** o EPUB vira capítulos com título (do sumário), partes e parágrafos numerados. Epígrafes vão para o capítulo seguinte; hifens suaves (U+00AD) e caracteres de largura zero saem; o que vem depois da última parte (propaganda de outros livros, agradecimentos) é material editorial.
2. **Nomes (sem LLM):** reconhecimento de entidades do spaCy (`pt_core_news_md`), que dá um glossário de nomes ao passo seguinte.
3. **Mapa por capítulo (LLM):** para cada capítulo, as entidades presentes com papel, estado ao fim do capítulo, fatos novos, relações e **uma citação literal conferida no parágrafo citado**. Entidade sem citação que confere é descartada. Capítulos fracos são refeitos ("remendos").
4. **Registro (identidade):** junta, entre capítulos e livros, o que é a mesma entidade: mesmo nome sem títulos ("Honorável Ceifadora Anastássia"); nome curto que é parte de um único nome longo ("Possuelo"); a nota que diz "adotou o nome"; e pares decididos pelo modelo **só com prova copiada das notas que cite o nome do outro lado**. Um "não são a mesma" do modelo nunca é desfeito por transitividade.
5. **Linha do tempo (sem LLM):** o estado de cada entidade capítulo a capítulo, tirado do mapa.
6. **Relações (sem LLM):** refeitas do mapa nos dois sentidos (o que a ficha da entidade diz do outro e o que a ficha do outro diz dela), sem corte.
7. **Página (LLM):** busca no texto todos os parágrafos que citam a entidade e levanta notas com citação conferida; escreve cada seção **uma vez**, só a partir de fatos numerados, e cada frase diz em que fatos se apoia; uma **segunda passada confere frase a frase** (mesmo sujeito, nada acrescentado) e descarta o que não se sustenta.

## 3 O formato da página (aprovado)

- **Cabeçalho:** nome, tipo, livros em que aparece.
- **Ficha:** apelidos e relações agrupadas (Família, Mentores e aprendizes, Romance, Aliados e amigos, Inimigos e rivais, Organizações e lugares, **Objetos, conceitos e eventos**, Outros), cada nome com link para a página dele. Objeto, conceito e evento só entram com 3 registros ou mais ("o anel de madeira" entra, "vinho" não). Romance só com palavra de vínculo amoroso nos registros; Família só com parentesco.
- **Situação neste ponto:** uma frase, a do último capítulo lido em que a entidade aparece.
- **Quem é:** 3 a 6 frases que começam pela identidade (o que ou quem é, nomes e títulos, papel na história) e seguem a trajetória até ali, com a nota "atualizado ao fim de…". Reescrito do zero a cada parte, **a partir da biografia já conferida**, nunca do "Quem é" anterior (reescrever o anterior fazia o texto encolher e perder fatos).
- **Biografia:** uma subseção por livro e por parte, em texto corrido, escrita uma vez e nunca reescrita; o livro seguinte só acrescenta subseções. Na parte em andamento, a situação capítulo a capítulo em vez do texto.
- **Personalidade.** Aparência e Habilidades **não** são geradas: o mapa guarda eventos e estado, e as seções saíam pobres (DEC-178).
- **Linha do tempo**, capítulo a capítulo, recolhida.
- **Referências** numeradas: capítulo, os fatos que sustentam a frase e a citação literal do livro.

## 4 O que foi medido

Três obras: a série Scythe (*O Ceifador*, *A Nuvem*, *O Timbre* e o livro de contos *As Foices*, pt-BR) e o livro avulso *A vida invisível de Addie LaRue* (pt-BR, 106 capítulos, linha do tempo não linear). Três modelos, todos remotos: DeepSeek V4-Flash (API oficial, paga), Nemotron 3 Super (NVIDIA NIM, gratuito) e `deepseek-v4.1-flash:free` (roteador Tokenharbor, gratuito).

| | DeepSeek oficial | Nemotron (NIM) | v4.1-flash gratuito |
| --- | --- | --- | --- |
| Mapa: entidades verificadas | 98% | 83% (Scythe), 86% (Addie) | não medido |
| Páginas de Citra, Nimbo-Cúmulo e Goddard: notas da busca / frases mantidas / descartadas na conferência | 220/147/8, 213/171/6, 242/131/5 | 219/124/18, 233/151/18, 196/80/18 | 188/127/7, 214/135/7, 244/124/6 |
| Tempo das 3 páginas | ~4 min | 25 min | 5,8 min |
| Série Scythe inteira (mapas a páginas) | ~30 min, ~US$ 1,30 | 90 a 270 min, grátis | não medido |
| Respostas 429 / conexões mortas | — | 119 / 0 | 0 / 1 em ~260 pedidos |

Modelos locais de 4B a 8B (qwen, gemma, llama) num Mac de 16 GB travaram ou erraram o básico; o recurso, por isso, exige provedor configurado (DEC-177). O custo do DeepSeek oficial por série é baixo; o gratuito do Tokenharbor tem qualidade parecida, mas os termos dele permitem reter e usar o texto enviado para "melhoria de produto" (DEC-177 trata da política à vista).

## 5 Defeitos achados e corrigidos no lab

- **O "Quem é" encolhia:** reescrito a partir do anterior com teto de palavras, copiava o texto antigo e cortava o resto (a Nimbo-Cúmulo terminava a série com a custódia de um corpo do primeiro livro). Virou seção escrita do zero, a partir da biografia.
- **Erro de sujeito:** o modelo atribuía à Nimbo-Cúmulo o que era do Cirro. A conferência frase a frase passou a descartar ("o fato fala do Cirro, não dela").
- **Partes:** agrupar capítulos pelo nome da parte esticava "Final" sobre o livro inteiro quando havia capítulos sem parte no começo e no fim. Partes viraram faixas contíguas.
- **Capítulos só com numeral** ("I", "II") repetem em cada parte; levam o nome da parte.
- **Nomes de 3 letras** (Luc, Bea) nunca eram procurados no texto.
- **Relações de um lado só e cortadas nas 40 mais antigas:** a Addie tinha 1 registro sobre o Henry ("cliente e potencial ladrão", do primeiro encontro); o Henry, 6 sobre ela ("interesse romântico"). Relações refeitas do mapa nos dois sentidos, sem corte.
- **Busca que falhou virava "sem notas" para sempre** no cache; o cache só guarda busca completa e refaz o que falhou.
- **Servidor gratuito:** dois processos na mesma chave davam 429; um limite de silêncio de 60 s derrubava pedidos só lentos; sem limite nenhum, um fluxo morto travava até 1 hora. Ficou um processo por chave e 300 s de silêncio.

## 6 Limites que continuam

- **Identidade ambígua:** um antagonista chamado de "a Escuridão" até ganhar nome (Luc) não é ligado ao nome; "Bea" e "Beatrice", "Jeri" e "Jerico" ficam separados; um "deus" que é o Luc entra em "Deus da mãe" por ter o mesmo nome. Nenhuma regra sem prova resolve isso sem errar outros casos: vira **sugestão confirmada por admin** (DEC-181).
- **Livro que vai e volta no tempo:** a ordem de leitura é a certa para não dar spoiler, mas a biografia segue a ordem de leitura, não a cronológica da história.
- **Grafo denso** num livro com núcleo pequeno de personagens: os lugares do dia a dia viram os maiores nós. Pede filtro e rótulos só nos nós importantes (DEC-182).
- **Escrita** às vezes telegráfica em partes com poucos fatos; melhorias de texto ficaram para depois, por decisão do mantenedor.

## 6b Ensaio dos outros nomes (16/10/2026, DEC-189 e DEC-193)

10 capítulos da Addie LaRue (5, 10, 15, 19, 28, 33, 38, 42, 50, 64) com o Nemotron 3.5 Super: 21 pedidos (capítulos longos se dividem), sem falhas, 11 respostas 429 absorvidas pelas novas tentativas; 97.707 tokens de entrada e 342.706 de saída. 23 outros nomes: 13 conferidos, 10 recusados.

| Conferidos | Leitura |
| --- | --- |
| Nomes certos (5) | Addie LaRue, Henry Samuel, Beatrice (2×), Adeline |
| Epítetos certos (3) | "a velha senhora", "o velho gato do sebo", "o sebo" |
| Perigosos (5) | "o diabo", "um monstro", "um deus" (predicados de uma pergunta do texto), "a garota" (o capítulo 19 tem outra "a garota"), "o anel" (o capítulo 38 tem o "anel de madeira") |

Recusados por acerto: "o novo Deus", "os flocos doces e escuros" (o conteúdo, não a lata), "rei dos duendes", "Emily", "Estele" (já junta por regra). Recusados por rigidez: "Beatrice Helen" (a cópia não traz "Bea") e "a sombra" (a cópia não traz "Luc"; é o Luc, confirmado pelo mantenedor). A conferência antiga só exigia que *algum* token de cada nome estivesse na cópia: provava que os dois nomes estão escritos juntos, não que são o mesmo. A regra nova está na DEC-193.

## 7 Integração no Códice (ordem combinada em 15/10/2026)

| PR | Conteúdo |
| --- | --- |
| A | Esta especificação: DEC-177 a DEC-183, RF-050 a RF-052 |
| B | Backend: tabelas, API de leitura com o **filtro de spoiler no servidor** e um importador das saídas do lab |
| C | A tela (DEC-183): página da entidade, índice da obra, entrada pelo leitor, grafo |
| D1 | A aba da wiki na administração (DEC-185): provedor com a chave cifrada, fila e andamento, obras e séries com o lote e as exceções |
| D2 | O gerador no worker: o pipeline do lab como job da fila, com progresso por etapa, retomada e a atualização de livro novo |
| E | Revisão e edição manual: o que mudou com um livro novo, sugestões de junção, frases marcadas como erradas, **escrever página à mão, criar entidades no grafo e ligar vínculos e momentos a elas** (DEC-186) |

### Quantas entidades ganham página, por corte de capítulos (medido nos pacotes do laboratório, 15/10/2026)

Capítulos em que a entidade aparece, somando os livros. Tudo o que fica abaixo do corte continua no índice, no grafo e na ficha (DEC-186).

| Corte | Addie LaRue (104 entidades) | Scythe (166 entidades) |
| --- | --- | --- |
| 2 ou mais | 103 | 163 |
| 3 ou mais | 67 | 102 |
| **5 ou mais (padrão)** | **37** (14 pessoas, 11 lugares, 6 conceitos, 5 objetos, 1 organização) | **63** (34 pessoas, 9 lugares, 8 organizações, 6 conceitos, 4 objetos, 2 eventos) |
| 8 ou mais | 16 | 40 |
| 12 ou mais | 9 | 27 |

Entidades sem página que aparecem em alguma aresta do grafo: 98 de 99 na Addie e 161 de 163 no Scythe. Sem teto, o custo cresce com o número de páginas; o preço por página ainda **não foi medido** neste formato e vai ser medido no D2.
