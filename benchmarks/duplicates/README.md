# Benchmark do mesmo texto em outro arquivo

Este benchmark opcional mede a **impressão digital do texto** (`backend/internal/fingerprint`, #38) que o Códice usa
para propor que dois arquivos são o mesmo livro (outro formato, outra edição), sem olhar título nem autor. Ele **não
roda no `make test`**.

## Como a impressão digital funciona

De cada arquivo com texto publicado, só o **corpo do livro** (a licença, o prefácio e os anexos que muitos arquivos de
uma mesma fonte trazem iguais ficam de fora) vira uma lista de palavras, sem caixa nem acento. Cada sequência de 8
palavras seguidas recebe um número, e fica **1 em cada 32**. Dois arquivos do mesmo texto guardam quase os mesmos
números, qualquer que seja a página, o capítulo ou a hifenização; livros que só se parecem (uma continuação, outro
livro do mesmo autor) quase não guardam nenhum, porque oito palavras seguidas não se repetem por acaso. Um livro de cem
mil palavras guarda cerca de 3 mil números.

Dois arquivos são **o mesmo texto** quando cada um tem pelo menos 70% dos números do outro. Se um tem quase tudo do
outro e o outro não (um volume dentro de uma coletânea, um trecho), **não** é o mesmo texto e não é proposto.

## O conjunto

Os mesmos EPUBs do Project Gutenberg do [benchmark da posição equivalente](../equivalence/README.md), baixados com
SHA-256 fixado para `tmp/equivalence-benchmark/`:

| par | o que é | esperado |
|---|---|---|
| duas edições em inglês de *Da Terra à Lua* | duas edições do mesmo texto | mesmo texto |
| o EPUB e um **PDF feito dele** (páginas desenhadas aqui e lidas pelo leitor de PDF do worker) | mesmo texto, outro formato | mesmo texto |
| a outra edição e o PDF | | mesmo texto |
| um arquivo **sem sumário**, com a licença e tudo, e a outra edição | | mesmo texto |
| um terço do livro e o livro | um trecho | contém |
| o livro e uma coletânea que o traz | um volume de coletânea | contém |
| a tradução em português e o original | nenhuma sequência de palavras em comum | nenhum |
| *As Aventuras* e *Um Estudo em Vermelho*, de Sherlock Holmes | mesmo autor, mesmos personagens | nenhum |
| livros sem relação | | nenhum |

## Execução

```sh
make benchmark-duplicates
```

Precisa do Go e do `worker/venv`. Mostra uma linha por par, com o tamanho da amostra, quantos números têm em comum, a
parte de cada um que está no outro, o que a impressão digital diz e o que se esperava. Termina dizendo quantos pares
saíram fora do esperado.

## O que não está aqui

- **Tradução.** Dois arquivos em idiomas diferentes não dividem sequências de palavras; encontrá-los é outro problema,
  da segunda PR da #38 (alinhamento de ponta a ponta).
- **Escaneados com OCR ruim**: o texto errado derruba as sequências. Um PDF escaneado só entra depois de lido pelo OCR, e
  seus erros diminuem a parte em comum; não foi medido.
- **Acervo real**: a régua são cinco livros e dois formatos. Quantos pares reais um acervo grande tem, e o que há de
  quase igual e diferente nele, só um acervo real mostra (#89).
