# Códice: estudo do motor do leitor de EPUB (issue #106, 03/10/2026)

O mantenedor pediu para **estudar trocar o que desenha o EPUB**. Este é o relatório curto, com recomendação, **antes de qualquer troca**. Medido ou lido no código está marcado **(código)**; o que veio de páginas externas, **(lido)**; o que não foi verificado, **(não verificado)**.

## Recomendação

**Manter o epub.js por enquanto.** O que o #106 pedia (mais temas, fonte, tamanho, margens, justificar, fonte para dislexia) **coube todo dentro do epub.js**, por regras de CSS que vencem as do livro. Trocar de motor custaria a migração dos locators das notas (CFI) e dos destaques, sem resolver nada que hoje nos trave. **Revisitar** se acontecer uma destas: (a) um defeito do epub.js que bloqueie o uso e não tenha contorno; (b) a necessidade de rolagem contínua ou de EPUB de layout fixo bom; (c) uma falha de segurança do epub.js sem correção. Se for preciso trocar, o **primeiro candidato é o foliate-js** (usa CFI), depois o Readium.

## O que o epub.js controla, e o que não (código)

- **Tema e fonte:** `rendition.themes.register(nome, regras)` e `themes.select`. As regras do leitor usam `!important` e **vencem a cor, a fonte e o espaçamento que o livro força**; código (`pre`, `code`) fica de fora. Com "Do livro" nada é imposto.
- **Fontes embutidas:** o livro é desenhado num **iframe que não enxerga as fontes do app**, então cada fonte é **declarada de novo dentro dele** (`@font-face` por `contents.addStylesheetCss`). Foi assim que entraram a Newsreader, a Plus Jakarta e agora a **OpenDyslexic**. Não usamos `themes.font`.
- **Tamanho:** `themes.fontSize` em porcentagem.
- **Margens:** não vêm do epub.js. São o **espaço de cada lado da caixa que o contém** (`padding-inline`), e o `ResizeObserver` do leitor refaz a paginação.
- **Justificar:** regra de CSS só nos parágrafos (`p, li, blockquote, dd`), com hifenização, que depende do idioma que o livro declara.
- **CFI e anotações:** o epub.js gera e resolve **CFI** (`rendition.currentLocation`, `annotations.highlight` com a cor de cada destaque). **Os locators das notas são CFI com o trecho**, então isso é o que mais custa trocar.
- **Seleção e gestos:** feitos por nós dentro do iframe (o observador de seleção, o toque).

## Alternativas

| | **epub.js 0.3.93** (hoje) | **foliate-js** | **Readium ts-toolkit** |
|---|---|---|---|
| Manutenção | **Quase parada**: a última versão tem **cerca de 4 anos** (lido); BSD-2 | Ativo, mas **um mantenedor** e o próprio README diz **"longe de completo ou estável"** (lido); MIT | **Ativo** (lançamentos em 2025 e 2026, API de decorações, ESM) (lido); BSD-3 |
| CFI | **Sim** (é o que usamos) | **Sim** (`epubcfi.js`), com a mesma ideia: **migração dos locators mais simples** | **Não**: usa o `Locator` do Readium (progressão e posição), **exigiria converter** as notas |
| Layout fixo | Razoável | **Inacabado** (lido) | **Sim**, reflowable e fixed layout (lido) |
| Rolagem contínua | Tem (não usamos) | **Não tem** (lido) | Sim (não verificado em detalhe) |
| Anotações | `annotations.highlight` (usamos) | `overlayer.js` | API de **decorações** |
| Esforço de troca | Nenhum | **Médio**: CFI igual, API e eventos diferentes, sem pacote npm estável (o README sugere copiar o código) | **Grande**: locators, API, peso |

## Um achado de segurança, fora do #106

O leitor abre o livro com **`allowScriptedContent: true`** (código, `EpubViewer.jsx`), e o epub.js monta o iframe com **`sandbox="allow-same-origin allow-scripts"`**. Essa combinação **anula o sandbox**: um EPUB com `<script>` roda **na mesma origem do app** e pode ler o token da sessão (`localStorage`) ou chamar a API como a pessoa. A flag entrou na troca do `react-reader` pelo `epubjs` direto (commit `6a25bff`), **sem motivo documentado**. Abri uma tarefa à parte para reproduzir, testar com `false` num navegador real e, se algo quebrar, achar outro jeito (uma política de conteúdo, por exemplo). **Não foi mexido neste PR.** Isso pesa na decisão de motor: o epub.js não nos obriga a isso, foi uma escolha nossa.

## Como decidir uma troca (quando chegar a hora)

Um **ensaio** (não agora), contra os mesmos livros do corpus de teste: (1) **ida e volta do CFI** de todas as notas existentes; (2) seleção e destaque sublinhado; (3) tamanho do pacote (hoje `epub.min.js` tem 224 KB); (4) paginação e retomada de posição; (5) acessibilidade (leitor de tela, foco); (6) livros com layout fixo e com scripts. A troca só se justifica se resolver algo que o epub.js não resolve.
