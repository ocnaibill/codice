# Anotações no leitor — fatia Stitch

Referências: projeto Stitch **Códice Digital Library Interface** (`8308131525132414636`), telas **Marcadores & Anotações: Duna** desktop (`d8af4ccd602b48ce9e2737dc17b95950`) e mobile (`e90c8340a8d448e08a6d5da9da88d7f4`), além de **Adicionar Anotação: Duna** desktop (`80ef97bb6f4147a0a9e2546885379de9`) e mobile (`f7d09cf226654149be342f745a25970a`).

## Escopo

- A gaveta de anotações recebeu a superfície clara e a tipografia editorial do leitor. Ocupa toda a largura no celular e fica à direita no desktop. O cabeçalho e a exportação permanecem visíveis enquanto a lista rola.
- A criação mostra claramente o ponto atual do arquivo quando há locator. Notas, destaques e marcadores são exibidos em cartões com tipo, posição, trecho, texto em Markdown e tags reais.
- Filtros locais por tipo usam apenas os registros carregados. Se a API indicar mais registros do que o limite carregado, o cabeçalho informa a contagem parcial em vez de fingir que a lista está completa. A exportação continua sendo feita pelo servidor para a obra inteira.
- O painel foca o botão de fechar ao abrir, fecha com Escape e devolve o foco anterior. Criação, edição, exclusão com confirmação, abertura no ponto e exportação mantêm os contratos anteriores.

As telas Stitch contêm capítulos e citações de exemplo, busca global, cópia de trecho e sincronização. Esta fatia não inventa esses dados nem implementa essas ações. O painel continua dentro do leitor; uma tela global de anotações é trabalho separado.

## Validação

- `npm test`: 265 testes passaram, inclusive os filtros por tipo, foco e Escape, além dos fluxos existentes de escrita, abertura no ponto e exportação.
- `npm run build`: aprovado; permanece o aviso do chunk principal acima de 500 kB.
- `npm run lint`: sem erros; sete avisos preexistentes.
- Navegador com Duna EPUB do acervo local: gaveta desktop e celular em 390 × 844 px, dados da obra e posição real, sem transbordamento horizontal. Escape fechou o painel e devolveu o foco. Nenhuma anotação foi criada no acervo local durante o ensaio visual.
