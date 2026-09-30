# Ficha da obra — fatia Stitch

Referências: projeto Stitch **Códice Digital Library Interface** (`8308131525132414636`), telas desktop **Códice - Ficha de Detalhes & Leitor: Duna** (`83226c1ed3014fb4bbfc3832eec5083e`) e mobile **Códice Mobile - Ficha & Leitor: Duna** (`371cf76f39d7483282a847eb0322be11`).

## Escopo

- A ficha existente usa a superfície clara e a tipografia editorial do hub. No celular, ocupa a tela; no desktop, é um painel largo com cabeçalho e conteúdo rolável.
- Capa, título, autor, série, descrição e dados da edição em foco vêm da API. Quando a descrição não existe, a ficha oferece orientação simples, sem texto fictício.
- A ação principal abre a versão que está em andamento quando ela está disponível; caso contrário, a primeira versão legível da edição principal, ou a primeira versão legível da obra. Edições e arquivos continuam listados separadamente, cada qual com sua posição e ações próprias.
- O fechamento por Escape devolve o foco ao controle anterior. Arquivos ausentes, marcação de conclusão, releitura, abertura do começo, OCR pendente e texto indexado mantêm o comportamento anterior.

As telas Stitch também mostram avaliação comunitária, citações, telemetria, envio para e-reader e prévia interativa do leitor. Esses elementos são ilustrativos ou pertencem a outras fatias; **não** são apresentados como funcionalidades ou dados reais nesta implementação.

## Validação

- `npm test`: 259 testes passaram, incluindo a escolha da versão em andamento pela ação principal, o fallback para um arquivo disponível e o fechamento por Escape.
- `npm run build`: aprovado; permanece o aviso de chunk principal acima de 500 kB.
- `npm run lint`: sem erros; sete avisos preexistentes fora desta mudança.
- Navegador com a API e o acervo reais via proxy local: ficha de Duna abriu com capa, autor, idioma, editora, data, formato, estado de leitura e arquivo corretos. Em 320, 390, 768 e 1440 px, o painel e o documento não tiveram transbordamento horizontal. O proxy de desenvolvimento não recebeu eventos WebSocket porque sua origem não foi adicionada ao `CORS_ALLOWED_ORIGINS`; isso não bloqueou a ficha.
