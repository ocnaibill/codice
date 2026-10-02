# Avaliação do OCR (QA-007)

Este benchmark opcional mede o OCR que lê os PDFs escaneados (issue #24, DEC-102 e DEC-104). Ele **não roda no
`make test`**, que só testa as métricas e o desenhador de páginas, sem o motor. A especificação (15.3) pede precisão da
transcrição, ordem de leitura e utilidade para a busca, e **não aprova nenhum limiar**: o relatório descreve, não aprova.

## De onde vêm as páginas

Os textos são livros de domínio público do Project Gutenberg, em português ([Dom Casmurro, #55752](https://www.gutenberg.org/ebooks/55752)),
inglês ([The Adventures of Sherlock Holmes, #1661](https://www.gutenberg.org/ebooks/1661)), espanhol
([Don Quijote, #2000](https://www.gutenberg.org/ebooks/2000)), francês ([Candide, #4650](https://www.gutenberg.org/ebooks/4650)) e italiano
([Le avventure di Pinocchio, #52484](https://www.gutenberg.org/ebooks/52484)). Não ficam no repositório: o comando baixa os
arquivos declarados em `benchmark.py` para `tmp/ocr-benchmark/src/` e confere o SHA-256 antes de usá-los. Se o Gutenberg
atualizar um arquivo, o benchmark para em vez de medir outra coisa em silêncio.

As páginas são **desenhadas aqui** (`pages.py`) a partir desse texto e entregues como um PDF só de imagem, sem camada de
texto, na resolução em que o worker as encontra. Por isso a verdade não é o que um motor leu, e sim o que foi escrito na
página. O que muda de uma condição para outra: uma ou duas colunas, fonte (serifa, sem serifa, itálico) e tamanho,
resolução (100, 150, 200, 300 dpi), página girada em 90° e 180°, inclinada, manchas, papel e tinta fracos, JPEG ruim,
cabeçalho com número de página, e uma página "suja" que junta várias dessas coisas. O mesmo trecho do livro é desenhado
em todas as condições, para compará-las no mesmo texto.

## O que se mede

| Medida | O que diz |
|---|---|
| erro de letra e de palavra | quanto do texto está errado (distância de edição, sem contar onde as linhas quebram) |
| busca sem acento | das palavras distintas da página (4 letras ou mais), quantas estão no texto lido, sem diferenciar caixa nem acento, como a busca do Códice |
| busca com acento | o mesmo, exigindo a palavra escrita igual; a diferença para a anterior é o que os acentos custam |
| ordem (e quantos trechos foram achados) | quantos pares de trechos saem na ordem certa; uma página de duas colunas lida de lado a lado cai para perto de metade |
| s/página | tempo da leitura de uma página |

E, para as escolhas que o dono faz:

- **O idioma dito ao motor** (`por`, `por+eng`, `eng`) em livros em português, inglês e **misto**, com a página limpa e a suja.
- **Outros idiomas** (espanhol, francês, italiano) lidos no idioma certo e no padrão `por+eng`, que é o que acontece quando o
  idioma não é descoberto.
- **Descobrir o idioma** (DEC-104): com texto limpo de 40, 100, 300 e 1000 caracteres, e com o texto que o OCR leu
  com o padrão, que é o que o worker faz.

## Execução

Precisa do Docker. O motor e os idiomas são os da imagem `ocr`, que são os de produção:

```sh
make benchmark-ocr
```

O relatório sai em `tmp/ocr-benchmark/report.md`, e os números de cada página em `tmp/ocr-benchmark/results.json`. Opções
(`ARGS='...'`):

- `--pages N`: páginas por condição (padrão 3).
- `--only limpa,fr`: só as condições de um nome de aparência ou de um idioma de texto.
- `--seed N`: outra amostra de trechos.
- `--no-detect`: não mede a descoberta de idioma.

Sem o Docker, `worker/venv/bin/python benchmarks/ocr/benchmark.py` roda com o Tesseract da máquina, mas os números só valem
para a versão e os idiomas que ele tiver.

## Páginas reais

Uma página desenhada é mais limpa que um escaneamento de verdade: não tem lombada curva, sombra, tinta desbotada, fonte
antiga. Para medir páginas reais, **fora do repositório**, ponha numa pasta dentro do repositório (por exemplo
`tmp/ocr-real/`, que o Git ignora) pares `nome.png` (ou `.jpg`, ou um `.pdf` de uma página) e `nome.txt` com o texto certo
da página:

```sh
make benchmark-ocr ARGS='--real ../tmp/ocr-real --real-dpi 300'
```

Cada página entra no relatório como `real`, lida em cada idioma de `--real-languages` (padrão `por,por+eng,eng`).

## O que não está aqui

- **Mangá com texto vertical e imagens longas**, que a especificação lista: dependem do OCR de quadrinhos (#27), e o motor
  da imagem não tem o idioma vertical.
- **Qualidade de OCR real**: só as páginas reais, acima, medem isso.
- **Ortografia**: o texto em português é de 1899 e usa a ortografia de antes dos acordos ("cousa", "phrases", "physiologia").
  Serve para medir o motor, mas o vocabulário não é o de um livro de hoje.
