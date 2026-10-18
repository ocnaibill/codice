# Proposta: destaques e notas do leitor, a par do Códice web

**Proposta, aguardando confirmação do mantenedor.** A regra de fundo foi decidida em 18/10/2026 (DEC-199): o leitor passa a escrever anotações no Códice, além do progresso, e a Anotações do leitor e a do web mostram as mesmas coisas. Este documento diz *como*; nada dele foi implementado, e o que está aqui não é decisão até ser confirmado. Parte da issue [#327](https://github.com/ocnaibill/codice/issues/327).

## 1. O que existe hoje

- **No Códice:** uma anotação (`notes`) é um trecho marcado (`quote`) e/ou um texto da pessoa (`body`), com cor, etiquetas e um **locator** tipado por formato: no EPUB, `cfi` + `href` + `progression`; no PDF e no quadrinho, página e regiões (`internal/locator`). O web mostra, cria, edita e apaga.
- **No KOReader:** um destaque é uma entrada da lista `annotations` do documento (arquivo `.sdr` ao lado do livro): `pos0` e `pos1` (xpointers no EPUB; no PDF, páginas e retângulos), `text` (o trecho), `note` (o texto da pessoa, quando há), `color`, `drawer` (`lighten`, `underscore`, `strikeout`, `invert`), `chapter`, `datetime`, `page`. O evento do plugin traz esses campos.
- **Já temos** o caminho entre o xpointer do aparelho e o CFI do web, **por ponto** (DEC-190, item 6): o parágrafo do EPUB vai e volta; o deslocamento dentro do parágrafo fica de fora. O progresso de leitura já usa isso.
- O aparelho hoje só **lê** o Códice e **escreve o progresso** (DEC-197); a rota de notas do leitor é só leitura (`GET /device/notes`).

## 2. O que a pessoa quer (DEC-199)

Grifar ou anotar no Kindle e ver no Códice web; grifar no web e ver no Kindle, no livro, ao abrir. Apagar ou editar num lado vale no outro. Sem perder nada, com ou sem rede.

## 3. Proposta

### 3.1 Identidade

Cada destaque do aparelho ganha um **id de origem estável**: `sha1(fileId + pos0 + pos1)`, mais a data de criação (`datetime`) como desempate. O servidor guarda `device_ref` na anotação (coluna nova, única por pessoa e arquivo). É o que torna o envio **idempotente** (mandar de novo não duplica) e o que permite editar e apagar o mesmo destaque.

### 3.2 Do aparelho para o Códice

- O plugin ouve as mudanças da lista de anotações do KOReader (criar, editar a nota ou a cor, apagar) e as põe numa **fila no aparelho** (um arquivo), que se esvazia quando há rede, na ordem em que aconteceram, como o KOSync faz com o progresso.
- Rotas novas, sob o mesmo acesso do aparelho e o mesmo escopo `koreader`:
  - `PUT /device/files/{fileId}/notes/{ref}`: cria ou atualiza (corpo: `pos0`, `pos1`, `text`, `note`, `color`, `drawer`, `chapter`, `datetime`, `page`).
  - `DELETE /device/files/{fileId}/notes/{ref}`: apaga **só** a anotação que veio daquele aparelho com aquele `ref` (um destaque do web nunca é apagado por engano).
  - `GET /device/files/{fileId}/notes`: as anotações do arquivo, no formato do aparelho (3.3).
- O servidor converte `pos0`/`pos1` em **locator** do Códice. Para EPUB: os dois pontos viram CFIs (o do começo vale como lugar da anotação) e o **trecho (`quote`) vem do texto que o aparelho mandou**, que é o que o web mostra e o que o usa para reencontrar o lugar quando o texto muda. Para PDF e quadrinho, página e retângulos. **O que não se converte não se inventa:** sem locator, a anotação entra com o trecho e sem lugar, e o web diz isso (já faz para anotação importada).
- Cor: o aparelho tem poucas (`yellow`, `red`, `green`, `blue`, `orange`, `purple`, `gray`...); o web tem as suas. Uma tabela fixa leva de uma à outra e volta (a que não tem par vira a mais próxima, e a original fica guardada para voltar igual ao aparelho).

### 3.3 Do Códice para o aparelho

- Ao abrir um livro com rede, o plugin pede `GET /device/files/{fileId}/notes?since=<carimbo>` e **junta** à lista do documento: o que ainda não está lá entra (o servidor devolve cada anotação com `pos0`/`pos1` em xpointer, pelo caminho inverso CFI → xpointer, e o trecho); o que mudou no web muda aqui; o que foi apagado no web sai daqui, **só** se tinha vindo de lá ou já tinha sido enviado.
- No aparelho, essas anotações aparecem como qualquer destaque do KOReader (no livro, na lista de marcadores, nas exportações dele).
- Uma anotação do web sem lugar que o aparelho entenda (sem CFI convertível) **não vai** para o livro; aparece na aba Anotações do plugin, que já lista as do Códice.

### 3.4 Conflitos

**Última edição vale**, por anotação (o carimbo de edição do servidor contra o do aparelho). Apagar vence editar de quem não viu a exclusão **apenas** quando as duas coisas vêm depois da última sincronização do aparelho; no mais, quem editou por último decide. Nada é perdido em silêncio: o servidor guarda a versão anterior (histórico de edição que o Códice já tem para anotações, se houver; senão, a decisão de guardar vira parte deste trabalho).

### 3.5 O que fica de fora da primeira versão

PDF e quadrinho (só EPUB e formatos de texto reflowable primeiro); marcadores de página sem trecho; etiquetas e vínculos `[[link]]` (o aparelho não os tem; o web os mantém e o aparelho não os apaga); anotações de áudio.

## 4. Servidor e plugin

**Servidor:** migração (`device_ref`, origem, carimbos), as três rotas, o conversor de ponto a intervalo (reaproveita `kosync_place.go`), testes de ida e volta (xpointer → CFI → xpointer), idempotência, escopo do acesso, apagar só o que é do aparelho, cor.
**Plugin:** a fila, o ouvinte do KOReader, o envio, a junção ao abrir, a tela de conflito quando uma edição colide, testes com um KOReader de mentira e no emulador.

## 5. Decisões que peço ao mantenedor

1. **Origem do apagar:** apagar no aparelho apaga no web (só o que o aparelho criou, ou também o que veio do web)? Eu proponho: o que o aparelho **criou ou já recebeu** apaga nos dois lados; o que só existe no web e nunca foi ao aparelho não é tocado.
2. **Cores:** aceita a tabela fixa com a cor original guardada?
3. **Primeira versão só EPUB (e textos reflowable)**, PDF e quadrinhos depois?
4. **Notas do web sem lugar no livro** ficam só na aba Anotações do plugin?
5. **Ordem:** primeiro aparelho → Códice (grifar no Kindle e aparecer no web), depois Códice → aparelho; ou os dois juntos?

## 6. Fases (um PR por fase)

1. Servidor: migração, `PUT`/`DELETE`, conversor, testes. Plugin: fila e envio. **Entrega:** o que se grifa no Kindle aparece no web.
2. Servidor: `GET` com `since` e o caminho inverso. Plugin: junção ao abrir. **Entrega:** o que se grifa no web aparece no Kindle.
3. Edição e exclusão nos dois sentidos, conflitos e o histórico.
4. PDF e quadrinhos, se o mantenedor quiser.
