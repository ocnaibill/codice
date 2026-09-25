# Benchmark público de posição equivalente

Este benchmark opcional mede o motor de produção (`internal/equivalence`) com textos que podem ser
redistribuídos, sem usar sua própria saída como verdade. Ele não roda no `make test`.

O conjunto positivo usa três edições de *Da Terra à Lua*, de Jules Verne: [Gutenberg #28341
(português)](https://www.gutenberg.org/ebooks/28341), [#44278 (inglês)](https://www.gutenberg.org/ebooks/44278)
e [#83 (inglês)](https://www.gutenberg.org/ebooks/83). Um número que aparece uma única vez no mesmo
capítulo das duas edições liga os trechos; antes da avaliação, **todos** os números são removidos do
texto entregue ao motor. Portanto, a pista que constrói a verdade não pode denunciar a resposta.

O conjunto negativo usa [*The Adventures of Sherlock Holmes*, #1661](https://www.gutenberg.org/ebooks/1661)
e [*A Study in Scarlet*, #244](https://www.gutenberg.org/ebooks/244): livros diferentes do mesmo autor
que compartilham personagens. Neles, qualquer oferta é indevida.

Os EPUBs não ficam no repositório. O comando baixa as edições declaradas em `benchmark.py` para
`tmp/equivalence-benchmark/` e confere o SHA-256 antes de usá-las. Se o Gutenberg atualizar um arquivo,
o benchmark interrompe em vez de trocar silenciosamente o corpus.

## Execução

O perfil determinístico só precisa das dependências normais do worker:

```sh
worker/venv/bin/python benchmarks/equivalence/benchmark.py
```

Para comparar os perfis locais opcionais, instale `worker/requirements-embeddings.txt` e execute:

```sh
worker/venv/bin/python benchmarks/equivalence/benchmark.py \
  --profile deterministic --profile e5-small --profile labse
```

`--json caminho.json` conserva cada observação para análises posteriores. A tabela mostra cobertura,
precisão entre as ofertas, acerto efetivo sobre todo o conjunto, ofertas indevidas no conjunto negativo,
latência mediana do motor Go e o pico de memória do processo que carrega o modelo. O tempo do perfil
inclui embeddings e avaliação; o download inicial fica de fora.

## Limites da régua

- A verdade é independente, mas números raros favorecem passagens técnicas; ela não substitui uma
  amostra humana estratificada de prosa.
- `≤1` aceita o trecho correto ou um vizinho, pois uma mesma passagem pode cair de lados diferentes
  da fronteira de segmentação em duas edições.
- O pico de memória é do processo Python inteiro, não apenas dos pesos.
- Estes livros cobrem português e inglês em EPUB. OCR, PDFs, alfabetos distantes, edições abreviadas e
  capítulos reorganizados exigem conjuntos adicionais.
