# Passada de segurança, 4 de outubro de 2026

Revisão antes de abrir o Códice a mais gente (alpha no homelab, depois o beta). Três partes: **dependências**, **varredura de vulnerabilidades conhecidas** nas três pilhas e **leitura do código** nos pontos onde um arquivo ou um cliente de fora chega ao servidor. O que era defeito foi corrigido neste PR, com teste; o que é decisão do mantenedor está em "Decisões para o mantenedor".

## 1. Dependências do frontend

`npm audit` começou com 1 moderada e 5 altas. Ficou **2** (uma só causa).

| Pacote | Situação | O que foi feito |
| --- | --- | --- |
| `react-reader`, `react-router-dom` | **Não eram usados** (o app navega por query string; o leitor usa o `epubjs` direto). Traziam `cookie`, `set-cookie-parser` e outros. | Removidos. `epubjs` passou a ser dependência **direta**, que é o que o código já importava. |
| `vitest` | Estava em `dependencies` (vai para a imagem sem necessidade) e com falha moderada (`@vitest/mocker`, leitura de arquivo por *mock*). | Movido para `devDependencies`, `4.1.11`. |
| `axios` | Direto, 12 avisos (poluição de protótipo, injeção de cabeçalho, SSRF por redirecionamento, a maioria só no adaptador de Node). | `1.20.0`. |
| `nanoid` (via `postcss`, só na compilação), `undici` (via `jsdom`, só nos testes) | Indiretas, fora do que roda para o usuário. | Atualizadas por `npm audit fix`. |
| `@xmldom/xmldom` (via `epubjs`) | **Restam 2 avisos.** O `epubjs` usa o `XMLSerializer` nativo do navegador quando existe (`section.js`), então o `xmldom` **não é alcançado no navegador**. A correção do `npm audit` é a troca para `epubjs@0.4.2`, que muda o motor do leitor e **invalida os CFI das notas** (estudo do motor, 3 de outubro). | **Aceito, documentado.** Reavaliar junto de uma eventual troca de motor. |

Nota de ferramenta: o `npm` 10 quebra (`Cannot read properties of null (reading 'edgesOut')`) ao resolver os pares opcionais do `vitest` 4.1.11. O lockfile foi gerado com `--legacy-peer-deps`; `npm ci` sem a opção instala normalmente (conferido).

## 2. Varredura de vulnerabilidades conhecidas

- **Go (`govulncheck`)**: **5 falhas na biblioteca padrão** (`net/http`, `crypto/tls`, `net/url`, `encoding/asn1`, `golang.org/x/net/idna` embutido), todas corrigidas na **1.26.6**. `go.mod` passou a pedir `go 1.26.6` (a imagem usa `golang:1.26-alpine`, que acompanha o último patch). Depois: **0 afetando o código**. Em módulos, restavam 3 que o código **não chama** (`x/crypto/ssh`, duas, e `x/crypto/openpgp`, sem correção): `x/crypto` foi para `0.56.0`; a do `openpgp` não tem versão corrigida e o pacote não é importado.
- **Python (`pip-audit` no ambiente do worker)**: `pip` e `urllib3` desatualizados **no ambiente local**. A imagem instala `requirements.txt` com `>=`, então uma imagem refeita já pega o `urllib3` corrigido; nada no repositório a mudar. Nenhuma das dependências **do worker** (`redis`, `psycopg2-binary`, `PyMuPDF`, `requests`, `lxml`, `rarfile`, `mutagen`) tem aviso.

## 3. Leitura do código

Pontos conferidos **sem achado**: arquivos servidos só se o catálogo os conhece e sem listar diretório (`FilesHandler`, `insideStorage`); senhas com `bcrypt`, com comparação feita também para usuário inexistente (sem diferença de tempo); JWT recusa segredo vazio e o *placeholder* do `.env.example`, e confere o método de assinatura; limite de 10 pedidos por minuto por cliente nas chamadas de entrada, com o endereço real só quando o proxy é declarado confiável; CORS de **uma** origem, sem credenciais; limites de leitura de arquivo no worker (RNF-007) e `lxml` do índice de texto já com `resolve_entities=False`; extensões aceitas (`filecheck.Supported`) não incluem nada que o navegador execute (HTML, XHTML, SVG como livro).

### Achados corrigidos

1. **Corpo de requisição sem limite nas rotas públicas.** Login, cadastro, configuração inicial e redefinição de senha liam o JSON inteiro que o cliente mandasse (36 pontos usam `json.NewDecoder(r.Body)` sem limite). Qualquer pessoa na rede enche a memória do servidor sem entrar. **Correção:** `middleware.LimitBody` limita **todo** corpo que não seja o envio de arquivo (`multipart/form-data`, que grava em disco e tem limite próprio) a **1 MiB**; os limites mais apertados que cada rota já tinha continuam valendo.
2. **Servidor HTTP sem tempo limite de cabeçalho** (`http.ListenAndServe` direto). Um cliente que abre conexões e manda o cabeçalho um byte por vez as segura abertas (Slowloris). **Correção:** `ReadHeaderTimeout` de 10 s e `IdleTimeout` de 2 min; **sem** `ReadTimeout`/`WriteTimeout`, porque envio, download e o websocket são longos de propósito. O erro de `ListenAndServe` deixou de ser ignorado.
3. **Extrator de metadados do worker lia membros de arquivo sem olhar o tamanho.** Um EPUB, CBZ ou CBR que **declara** um membro de gigabytes (e comprime a poucos KB) era carregado inteiro na memória do worker. O índice de texto já tinha esse limite (RNF-007); **o extrator de metadados, que roda primeiro, não**. **Correção:** `extractors.base.read_member` confere o tamanho declarado **antes** de ler (capítulo/capa/página: 40 MB; `ComicInfo.xml` e `container.xml`: 2 MB) e é usado em todas as leituras de membro dos três extratores.
4. **XML do EPUB com entidades resolvidas.** O `lxml` do extrator de metadados abria o OPF com `recover=True` e **expandia entidades internas** (a "bomba de bilhão de risos" aparecia no texto da descrição; o teste reproduzia). Entidades externas (`file://`) já não eram lidas pela versão atual da biblioteca, mas dependia da versão. **Correção:** `resolve_entities=False, no_network=True` nos dois pontos, como o índice de texto já fazia.
5. **Arquivos servidos sem `X-Content-Type-Options`.** O navegador podia "adivinhar" o tipo do que foi enviado por outra pessoa; e um SVG aberto sozinho (capa) é um documento que pode ter script, **na origem do app**. **Correção:** `nosniff` em tudo o que `/files` e `/covers` servem, e, para SVG, `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox` (como já tinha o *placeholder*). Como `<img>` o SVG continua sendo desenhado.

Cada correção tem teste que **falha sem ela** (mutação dos casos listados no PR).

## Decisões para o mantenedor

- **Política de conteúdo (CSP) geral.** Não existe. A correção do EPUB (scripts do livro desligados, 3 de outubro) fechou o caminho que se conhecia, e as de cima fecham os outros dois (SVG, tipo adivinhado), mas uma CSP na resposta do `nginx` seria a rede de segurança caso apareça outro. **Recomendação:** `default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; frame-ancestors 'self'; base-uri 'self'; object-src 'none'` e **medir no navegador** (o leitor de PDF usa *worker* `blob:`/`.mjs`, o EPUB usa `blob:` e `srcdoc`; é o tipo de ajuste que quebra o leitor se feito às cegas). Proposta: um PR próprio, com o teste no navegador, **depois do CI**.
- **Token de sessão no `localStorage`.** Qualquer script na origem lê o token. Trocar por *cookie* `HttpOnly` mexe em autenticação, OPDS (que usa Basic) e no websocket; é uma mudança grande e **não é recomendada antes do beta**: a defesa aqui é não deixar nenhum script de fora rodar (a CSP acima). A **lista e a revogação de sessões** (já na proposta de telas) é o que limita o dano se um token vazar.
- **Quem envia é quem administra** (dono e administradores): um administrador mal-intencionado é o modelo de ameaça dos itens 3 a 5. Mantido assim; os limites acima fazem o arquivo dele não derrubar o servidor nem o worker.
- **`xmldom` via `epubjs`:** ver a tabela; reavaliar junto de uma troca de motor.

## Não coberto

Teste de intrusão externo, revisão do Authentik/LDAP do mantenedor (fora do repositório), segredos em histórico do Git e as camadas das imagens Docker. Conferido apenas que `backend` e `worker` rodam com `USER codice` (sem root) e que o `nginx` escuta na 8080.
