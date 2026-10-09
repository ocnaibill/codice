# Estudo dos provedores de metadados: material do experimento

O relatório é [`../../Codice_Estudo_Provedores_2026-10-09.md`](../../Codice_Estudo_Provedores_2026-10-09.md). Aqui está o que foi rodado, para
repetir ou ampliar. Tudo só lê: não grava nada no acervo nem no banco.

| Arquivo | O que faz |
|---|---|
| `cases.py` | o conjunto de casos: 16 livros (inglês, português, título sujo com o autor), 4 quadrinhos e 5 mangás, cada um com a resposta certa |
| `current.py` | roda o **provedor da Open Library do worker, como está**, com a consulta que o pipeline manda (o título só) |
| `improved.py` | protótipo (não é código de produção) da busca melhor: lê o título do arquivo, pede vários candidatos com título e autor separados, pontua, só aceita o que é próximo e busca a descrição |
| `series.py` | AniList e MangaDex para a série, e a Open Library para o volume |
| `authors.py` | Open Library, Wikidata e Wikipédia para os autores |
| `probe_comicvine_googlebooks.py` | **para o mantenedor rodar com as chaves dele**: o que não deu para medir sem chave |
| `resultado_*.json` | o que cada um devolveu (sem os textos de sinopse e biografia, que são de terceiros) |

Para repetir (da raiz do repositório; o worker precisa do `venv`):

```bash
cd docs/estudos/provedores-2026-10-09
../../../worker/venv/bin/python current.py && ../../../worker/venv/bin/python improved.py
../../../worker/venv/bin/python series.py && ../../../worker/venv/bin/python authors.py
```

As APIs mudam e têm limite de uso: os scripts esperam entre as chamadas e se identificam com `CodiceStudy/1.0 (https://github.com/ocnaibill/codice)`.
A Wikimedia recusa (429) quem não se identifica, e a Open Library dá o triplo de taxa a quem se identifica.
