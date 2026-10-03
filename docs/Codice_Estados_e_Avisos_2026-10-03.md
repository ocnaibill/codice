# Códice: estados, avisos e mensagens (issue #77, 03/10/2026)

O que ficou decidido e feito para os **estados de erro** e os **avisos**, para as próximas telas seguirem o mesmo. A linguagem visual é a do protótipo do Stitch (os tokens de `frontend/src/index.css`); aqui está a **regra de uso**.

## Papéis de cor

Só os papéis do tema, nunca uma cor da paleta do framework (`red-700`, `amber-100`, `zinc-900`, `blue-600`): **`brand`** (a ação principal), **`ink`**, **`danger`** (erro, o que não se desfaz), **`warning`** (atenção), **`success`** (deu certo) e **`info-soft`**, cada um com o seu `-soft` para o fundo. Um teste (`src/designSystem.test.js`) lê o código e **recusa** uma cor solta.

## Componentes-base

| Componente | Para quê | Quando |
|---|---|---|
| `Notice` (`components/ui/Notice.jsx`) | Um aviso **que fica na página**, em tom (`danger`, `warning`, `success`, `info`), com título e uma ação opcional. Erro é `role="alert"`; os outros, `status`. | O que precisa ser visto até a pessoa agir: erro de formulário, falha de carregar, atenção. |
| `LoadError` (`components/ui/LoadError.jsx`) | **O que se carregava não veio**: a mensagem e **"Tentar de novo"** (o `refetch` da consulta; "Tentando…" enquanto pergunta). | **Toda** tela que carrega dados do servidor. Um teste exige que cada `LoadError` receba `onRetry`. |
| Toast (`components/ui/toast.js`) | O que **passa**: "Destaque salvo", "Trecho copiado". | Resultado de uma ação, sem exigir resposta. |
| `ErrorBoundary` | **A tela quebrou ao ser desenhada**: "Algo deu errado", Recarregar e, quando há, Voltar; o erro técnico fica numa lista dobrada. | Em volta do **aplicativo** (`main.jsx`) e do **leitor** (`Reader.jsx`, que recomeça a cada arquivo). |
| `EmptyState` | **Não há nada ainda** (não é erro). | Lista vazia depois de carregar. |

Uma lista **nunca** diz "vazio" por cima de um erro: antes, "Armazenamento" mostrava "Nenhuma pasta autorizada" e "Nenhum backup registrado nesta instância" quando a consulta falhava.

## Mensagens do servidor

O servidor responde em texto puro e, na maior parte, **em inglês** (a API serve também a programas). As telas mostravam isso como vinha ("Invalid username or password"). Agora **`lib/serverMessage.js`**:

- diz **em português** o que uma pessoa pode provocar (cerca de 120 mensagens e padrões com número: "a tag has at most 40 characters");
- **nunca mostra inglês que ninguém traduziu**: cai na frase da própria tela;
- **não lê** uma falha do próprio servidor (5xx), que não diz nada que a pessoa possa fazer.

Um teste lê o **código do servidor** (`http.Error(...)` e os `ErrXxx = errors.New(...)`) e **falha** quando aparece uma mensagem nova que não está traduzida nem listada como "para programas". É a decisão que mensagem nova obriga a tomar.

## Diálogos

Modal de decisão: fundo `bg-black/50` com `animate-fade-in`, painel branco `rounded-2xl` com `animate-pop-in`, título em `font-display`, botões de **44 px** (`min-h-11`), o principal em `brand` e o outro em `surface-alt`; **Escape** é a resposta cuidadosa (não marca nada, não muda de posição). Foi feito em "Obra finalizada?" e "Continuar de onde parou?"; o `ConfirmDialog` da administração segue o mesmo desenho.

## Sem permissão

"Você não pode" não é "falhou": nada deu errado, a conta é assim. Três formas, todas em `components/ui/PermissionNote.jsx`:

- **`PermissionNote`**, uma nota **onde a coisa estaria**: uma seção que a pessoa pode ver e não mudar diz **quem pode** ("Só o dono do acervo liga ou desliga os provedores."), com um cadeado, no mesmo texto em toda parte e **nunca "owner"** (é "o dono do acervo" ou "quem administra"). Está nos Provedores, Dicionários, OCR, Pastas autorizadas, Lixeira e Sugestões.
- **`NoPermission`**, a **tela inteira** que a pessoa não pode ver: o que é, o que fazer, e um caminho de volta. A administração para uma conta de leitura.
- **`LoadError` com o `error`**: quando o servidor diz **403**, ele diz que a pessoa não tem permissão e **não oferece "Tentar de novo"**, que só repetiria o mesmo não. Um teste exige que todo `LoadError` receba o `error`.

## Processamento parcial (RN-018)

Uma obra pode estar **legível e ainda não pesquisável**: o arquivo abre, o texto ainda está na fila, uma digitalização não tem texto até o OCR. `lib/processing.js` diz isso do mesmo jeito na ficha e na busca:

- **Na ficha**, cada arquivo mostra o estado do **texto para a busca** (`fileTextState`): *texto indexado* (verde), *texto na fila* (o arquivo já pode ser aberto), *sem texto pesquisável* e *texto não lido* (aviso); uma digitalização deixa a frase para o OCR, e quadrinho e audiolivro **não** dizem "texto na fila".
- **Na busca**, abaixo de "Passagens", **uma frase diz o que a busca ainda não vê** (`coverageNote`): quantos arquivos têm o texto ainda sendo lido, quantos não puderam ser lidos e quantos não têm texto. Vem de `coverage` na resposta de `GET /search` (`reading`: trabalhos `extract_text` na fila ou rodando; `failed` e `noText`: arquivos disponíveis de obras não retiradas), para que "nenhuma passagem" signifique "nada" ou "ainda não olhei em tudo".
