# Códice: passada de acessibilidade (4 de outubro de 2026)

Item 4 do caminho até o beta. A pergunta: **uma pessoa que usa só o teclado, um leitor de tela, o zoom ou pouca visão consegue usar o app?** Esta passada mediu, corrigiu o que podia ser corrigido sem decisão de design e deixa registrado o que **não** foi coberto. O critério é o WCAG 2.2 nível AA.

Foi feita em três PRs: as fundações (página, contraste, foco do teclado), o foco dos diálogos, e este (auditoria com o `axe-core` no navegador real, painéis do leitor, nomes e papéis, zoom).

## Como foi medido

- **`axe-core` 4.13 no navegador real**, com layout e contraste de verdade, sobre o app rodando contra um backend descartável: tela inicial, as 13 abas da administração, os 6 diálogos (os 5 da conta e o de adicionar arquivos), a busca, a ficha da obra e a edição de metadados, os leitores (EPUB, EPUB de layout fixo, PDF, CBZ, texto) com os painéis abertos, o login, a recuperação de senha e o cadastro. Regras: `wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa`, `wcag22aa` e as boas práticas.
- **Teclado de verdade** (Tab, Shift+Tab, Enter, Escape) no navegador, nos diálogos, na ficha com a edição por cima, e nos painéis do leitor.
- **Reflow e zoom:** janelas de 640 px (equivalente a 200% de zoom sobre 1280) e de 320 px (400%, o critério de reflow do WCAG 1.4.10), medindo rolagem horizontal e elementos que passam da borda, na tela inicial, na administração, nos diálogos e nos leitores.
- **Contraste dos tokens**, calculado a partir do próprio CSS (`src/accessibility.test.js`), para as **duas paletas** do app (a do tema e a mais quente que a biblioteca coloca por cima).
- **No CI:** `src/accessibility.test.js` (contraste, foco, movimento reduzido, diálogos), `src/dialogs.focus.test.jsx`, `src/lib/useDialog.test.jsx` e `src/axe.test.jsx` (o `axe` em jsdom sobre as peças que consertamos e sobre os diálogos).

## O que a auditoria achou, e o estado de cada achado

| Achado | Onde | Estado |
|---|---|---|
| `ink-faint` abaixo de 4,5:1 (4,0 a 3,8:1) | tema (`index.css`) | **corrigido** (parte 1): #887369 → #725f55 |
| `ink-faint` abaixo de 4,5:1 (4,27:1 sobre a superfície alternativa, 4,13:1 sobre o aviso) | **paleta da biblioteca** (`library-shell.css`), que sobrescreve os tokens | **corrigido**: #78685c → #6f5f54. A parte 1 não chegou aqui porque só olhou o tema: **o teste agora lê as duas paletas** |
| Contador "(5)" com `opacity-80` sobre o botão | aba Trabalhos | **corrigido** |
| `<div aria-label="Avisos">` sem papel (atributo proibido) e fora de qualquer marco | região dos avisos | **corrigido**: `role="region"` |
| `<dl>` com um `<p>` solto entre `dt` e `dd` | estatísticas da tela inicial | **corrigido**: o detalhe virou `<dd>` |
| Sem `<main>`, conteúdo fora de marcos | login, recuperação, cadastro, primeiro acesso, convite | **corrigido**: `AuthCard` é um `<main>` |
| Sem `<main>` e sem `<h1>` | os leitores | **corrigido**: o título do livro é o `<h1>` e a leitura, um `<main>` |
| `<iframe>` do EPUB sem `title` (o leitor de tela diz só "frame") | EPUB | **corrigido**: "Texto de “Título”", ou "Texto do livro" |
| Os painéis do leitor (aparência, sumários, menu de modo do mangá) não recebiam o foco, não fechavam o foco ao Escape e não devolviam o foco | EPUB, PDF, texto, mangá | **corrigido**: o foco entra, o Escape fecha só se não há diálogo modal por cima, o foco volta ao botão; o Tab fica livre (são popovers ao lado da página, não diálogos sobre ela) |

Fora desta passada, já corrigidos nas partes 1 e 2: `lang="pt-BR"` e título "Códice"; 25 `outline-none` sem substituto (anel de foco global); texto de exemplo (*placeholder*) meio transparente; os 17 diálogos sem foco, sem Tab preso, sem foco de volta, com Escape sem ver qual estava por cima.

## O que o `axe` não achou, e o que isso **não** garante

O `axe` acha o que uma máquina consegue ver (nomes que faltam, papéis proibidos, contraste, estrutura). Costuma alcançar **apenas uma parte** dos problemas reais de acessibilidade (as estimativas comuns ficam abaixo da metade). **Ficou sem medir**, e é o que importa dizer com franqueza:

- **Leitor de tela de verdade** (VoiceOver no Mac, NVDA no Windows, TalkBack no Android). Quem usa um lê o app de um jeito que nenhuma regra automática prevê: a ordem de leitura, o que é anunciado quando um aviso aparece, se "Ler: título" faz sentido fora do contexto. **Recomendo cobrir com a segunda pessoa do beta (#89)**, em uma máquina Windows com NVDA e um Android com TalkBack, que é a matriz que o mantenedor disse ter.
- **O conteúdo do livro.** O texto do EPUB, do PDF e da imagem do quadrinho é do arquivo: PDF sem estrutura marcada, página de quadrinho sem texto alternativo e EPUB com imagens sem `alt` não são problema do app, mas a pessoa vai esbarrar neles. O app não tenta consertá-los.
- **O iframe do EPUB por dentro:** o `axe` roda no documento do app; o documento do livro tem estilo e estrutura do arquivo.

## O que fica para o mantenedor decidir (design)

1. **O texto de apoio é pequeno.** Há rótulos de 8 a 11 px em fonte monoespaçada (as legendas das estatísticas, os títulos de seção da barra lateral, os cartões). O WCAG AA **não** fixa um tamanho mínimo, e o contraste agora passa, mas 8 a 9 px é difícil de ler para quem tem pouca visão e some no zoom que não é de página inteira. **Recomendação:** subir o piso para 11 a 12 px nos rótulos que levam informação (não nos decorativos). Muda o desenho das estatísticas.
2. **A borda dos campos tem 1,3:1** (`border-hairline` sobre a superfície). O WCAG 1.4.11 pede 3:1 para o que identifica um controle; muitos campos são `bg-surface` sobre branco, com um preenchimento quase igual. O anel de foco global resolve o **foco**, mas não o "onde está o campo" antes de focar. **Recomendação:** uma borda de 3:1 (um tom do `ink-faint` a 1 px) nos campos de texto, ou um fundo com contraste. Muda a cara dos formulários.
3. **`ink-faint` e `ink-soft` ficaram mais próximos** (1,4:1 a 1,5:1 entre eles; antes eram 1,5:1 no tema e 1,5:1 na biblioteca contra 2,1:1 do desenho original). A hierarquia depende também de tamanho e peso. Se preferir outro tom, `src/accessibility.test.js` diz o limite (4,5:1 sobre todo fundo em que o texto aparece).

## Estado

- **Corrigido e testado** nesta passada: tudo o que está marcado acima.
- **Zoom e reflow:** sem rolagem horizontal da página em 640 px nem em 320 px na tela inicial, em 13 abas da administração, em 6 diálogos e nos leitores. Dentro de uma faixa de filtros da biblioteca há rolagem horizontal **local** (a barra de abas "Todos, Livros digitais, Mangás…" passa da largura): é uma barra de ferramentas, exceção prevista no WCAG 1.4.10.
- **Falta:** o leitor de tela real (acima), a decisão de design sobre o tamanho do texto de apoio e a borda dos campos, e os PDFs/EPUBs de teste com estrutura acessível.
