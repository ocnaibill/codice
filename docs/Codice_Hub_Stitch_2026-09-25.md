# Hub responsivo — primeira fatia do Stitch

Implementação da UI-01 em React, usando o projeto Stitch **Códice Digital Library Interface** (`8308131525132414636`). Referências: desktop “Papel Natural Aquecido” (`4ca516090a6e4882bed56ec9ebb31cb7`) e mobile (`cf1a5664682444fd8bb2df97e68920d2`).

## O que mudou

- Paleta de papel aquecido no shell, hierarquia editorial e cartões fluidos. Newsreader, Plus Jakarta Sans e JetBrains Mono são distribuídas pelo próprio frontend via Fontsource, sob OFL-1.1, eliminando a chamada anterior ao Google Fonts. As fontes da interface são compartilhadas; as cores novas ficam no shell. O leitor mantém seus controles de aparência.
- Navegação lateral no desktop, menu modal nos tamanhos menores e navegação inferior no celular. As entradas disponíveis abrem todas as obras, formatos, leituras em andamento e favoritos, usando os filtros existentes da API.
- Grade/lista efetivas e paginação de 12 obras. Trocar de coleção ou formato volta à primeira página. A busca global continua acessível no cabeçalho, com atalho Ctrl/Cmd+K.
- Estatísticas, retomada da última versão lida, favoritos, notas e download usam os dados e regras existentes. Falhas de consulta têm mensagem e nova tentativa; capas ausentes ou quebradas têm substituto com título e autor.
- Upload e administração continuam restritos a owner/admin. Administração fica na navegação e no menu de conta. Menus têm fechamento por Escape; o menu mobile usa dialog nativo, com foco contido e retorno ao botão.

## Adaptação do design

Os números, capas e indicadores de infraestrutura do Stitch são ilustrativos. O hub mostra contagens reais de obras, andamento, conclusões no mês e tempo de leitura já disponíveis. Telemetria ZFS, OPDS, sincronização e notificações decorativas não são apresentadas como operantes. Favoritos e anotações existentes continuam no final da página.

Esta fatia não entrega tema escuro, grafo, trilhas de leitura nem redesenho dos leitores, da ficha ou da administração.

## Validação

- `cd frontend && npm test`: 253 testes passaram, incluindo paginação, filtros de formato e coleção, alternância grade/lista, falha e nova tentativa, ausência de controles administrativos para leitor, retomada de versão e fallback de capa.
- `npm run build`: aprovado. Persiste o aviso de chunk principal acima de 500 kB.
- `npm run lint`: sem erros; sete avisos em arquivos fora desta mudança.
- Navegador: prévia isolada com componentes reais e respostas fictícias da API; larguras 320, 390, 768, 1024 e 1440 px sem transbordamento horizontal do documento. Exercitados menu mobile, filtro, grade/lista, paginação, busca e retorno ao acervo; Escape fecha o menu e devolve o foco.

A prévia não usa o banco nem o acervo do mantenedor. Esses ensaios validam a interface e seus contratos simulados; não representam nova homologação de upload, leitura ou autenticação de ponta a ponta com o backend.
