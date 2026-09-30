# Leitor — moldura Stitch

Referências: projeto Stitch **Códice Digital Library Interface** (`8308131525132414636`), telas desktop **Códice Desktop - Leitor Imersivo: Duna** (`a2c434d5d4b445468d5721236e9d76f6`) e mobile **Códice Mobile - Leitor Imersivo: Duna** (`f50ed55999624b759f60caa29b8c53a6`).

## Escopo

- A moldura compartilhada do leitor ganhou superfície clara, altura da tela e cabeçalho responsivo com título, autor, formato, idioma, retorno ao acervo, favorito e notas. Os dados e as ações continuam ligados à API e ao estado existente.
- EPUB, PDF, quadrinhos, texto, Markdown e áudio continuam com os seus próprios mecanismos, controles e progresso. Os estados de carregamento, erro e formato indisponível foram alinhados visualmente e traduzidos para português.
- O EPUB agora começa no tema claro, coerente com a moldura. Tanto o tema claro quanto o escuro impõem uma cor de texto legível dentro do conteúdo do livro, usando regras associadas à classe do tema ativo. Isso corrige o EPUB de Duna, que mantinha letras pretas ao escurecer o fundo, e permite alternar de volta para o claro. A cor autoral do texto pode ser substituída nesses modos para preservar contraste; imagens não são recoloridas.
- A página EPUB usa uma altura explícita baseada na tela para evitar uma faixa vazia no celular e dar ao `epubjs` uma medida estável ao paginar.

As telas Stitch mostram sincronização com e-reader, tempo restante, telemetria e controles globais de tipografia. A moldura não apresenta esses recursos sem contratos e dados reais. As ferramentas de tipografia e navegação que já existem permanecem dentro de cada formato. O painel de notas e os controles específicos de cada leitor não foram redesenhados nesta fatia.

## Validação

- `npm test`: 263 testes passaram, incluindo os controles da moldura e as regras de contraste dos temas EPUB.
- `npm run build`: aprovado; permanece o aviso do chunk principal acima de 500 kB.
- `npm run lint`: sem erros; sete avisos preexistentes.
- No navegador, Duna EPUB do acervo real abriu no desktop e em 390 × 844 px. Texto legível no tema claro e no escuro; a troca escuro → claro também foi confirmada. O retorno, título, autor, formato e favorito apareceram corretamente. A altura mobile foi conferida após nova abertura do livro, quando o `epubjs` refaz sua paginação.

Não foram revalidados visualmente todos os formatos nem o contraste de todo EPUB possível. O tema escuro força a cor do texto, mas um EPUB pode conter fundos ou elementos gráficos autorais que requeiram tratamento específico.
